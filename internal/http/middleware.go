// Package httphandler provides HTTP handlers and middleware for the presenca-facial API.
package httphandler

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// key type for context values
type contextKey int

const (
	keyDeviceIP contextKey = iota
)

// DeviceIPFromContext retrieves the device IP stored by IPAllowlistMiddleware.
func DeviceIPFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(keyDeviceIP).(string); ok {
		return v
	}
	return ""
}

// IPAllowlistMiddleware restricts the webhook path to requests from registered device IPs.
// It injects the client IP into the context for downstream use.
// allowedIPs is a function that returns the current set of allowed IPs (dynamic — reads from DB).
func IPAllowlistMiddleware(allowedIPs func() []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := extractClientIP(r)

		// Check against the allowed list
		allowed := allowedIPs()
		for _, ip := range allowed {
			if ip == clientIP {
				// Inject the IP into context and proceed
				ctx := context.WithValue(r.Context(), keyDeviceIP, clientIP)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		http.Error(w, "forbidden: IP not in allowlist", http.StatusForbidden)
	})
}

// AdminAuthMiddleware validates the Authorization: Bearer {ADMIN_TOKEN} header.
// Returns 401 if header is absent, 403 if the token is incorrect.
func AdminAuthMiddleware(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "unauthorized: missing Authorization header", http.StatusUnauthorized)
			return
		}
		const bearerPrefix = "Bearer "
		if !strings.HasPrefix(auth, bearerPrefix) || auth[len(bearerPrefix):] != adminToken {
			http.Error(w, "forbidden: invalid admin token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenBucket holds rate limiting state per IP.
type tokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	maxTokens float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newTokenBucket(maxPerMinute int) *tokenBucket {
	return &tokenBucket{
		maxTokens:  float64(maxPerMinute),
		tokens:     float64(maxPerMinute),
		refillRate: float64(maxPerMinute) / 60.0,
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware enforces a per-IP token bucket rate limit.
type RateLimitMiddleware struct {
	mu           sync.Mutex
	buckets      map[string]*tokenBucket
	maxPerMinute int
}

// NewRateLimitMiddleware creates a rate limiter allowing maxPerMinute requests per IP per minute.
func NewRateLimitMiddleware(maxPerMinute int) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		buckets:      make(map[string]*tokenBucket),
		maxPerMinute: maxPerMinute,
	}
}

// Handler returns the middleware http.Handler.
func (m *RateLimitMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)

		m.mu.Lock()
		tb, ok := m.buckets[ip]
		if !ok {
			tb = newTokenBucket(m.maxPerMinute)
			m.buckets[ip] = tb
		}
		m.mu.Unlock()

		if !tb.allow() {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SyncSerializer serializes /admin/sync to prevent concurrent runs.
type SyncSerializer struct {
	mu          sync.Mutex
	running     bool
	lastRunAt   time.Time
	minInterval time.Duration
}

// NewSyncSerializer creates a serializer with a minimum interval between runs.
func NewSyncSerializer(minIntervalSeconds int) *SyncSerializer {
	return &SyncSerializer{
		minInterval: time.Duration(minIntervalSeconds) * time.Second,
	}
}

// TryAcquire returns true and a release function if the lock was acquired.
// Returns false if already running or if the minimum interval hasn't elapsed.
func (s *SyncSerializer) TryAcquire() (acquired bool, release func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return false, nil
	}
	if time.Since(s.lastRunAt) < s.minInterval {
		return false, nil
	}

	s.running = true
	return true, func() {
		s.mu.Lock()
		s.running = false
		s.lastRunAt = time.Now()
		s.mu.Unlock()
	}
}

// extractClientIP extracts the client IP from the request.
// Prefers X-Forwarded-For (first entry) or X-Real-IP; falls back to RemoteAddr.
func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First IP in the chain
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		return r.RemoteAddr
	}
	return ip
}
