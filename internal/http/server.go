package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Server is the HTTP server for the presenca-facial API.
type Server struct {
	inner *http.Server
}

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Addr                    string
	WebhookPathSecret       string
	AdminToken              string
	WebhookRateLimitPerMin  int
	AdminSyncMinIntervalSec int

	EventHandler  http.Handler
	HealthHandler http.Handler
	AdminHandler  http.Handler

	// AllowedWebhookIPs returns the current list of allowed IPs (queried from DB).
	AllowedWebhookIPs func() []string
}

// NewServer constructs an HTTP server with the routes and middleware wired up.
//
// Routes:
//
//	POST /webhook/{secret}   — HikVision events (IP allowlist + rate limit)
//	GET  /health             — health check (no auth)
//	POST /admin/sync         — trigger member load (admin auth)
func NewServer(cfg ServerConfig) *Server {
	mux := http.NewServeMux()

	// Webhook route: IP allowlist + rate limiting (spec.md §FR-013, §FR-014)
	webhookPath := "/webhook/" + cfg.WebhookPathSecret
	rl := NewRateLimitMiddleware(cfg.WebhookRateLimitPerMin)
	webhookHandler := IPAllowlistMiddleware(
		cfg.AllowedWebhookIPs,
		rl.Handler(cfg.EventHandler),
	)
	mux.Handle(webhookPath, webhookHandler)

	// Health check — no auth
	mux.Handle("/health", cfg.HealthHandler)

	// Admin sync — Bearer token auth
	adminHandler := AdminAuthMiddleware(cfg.AdminToken, cfg.AdminHandler)
	mux.Handle("/admin/sync", adminHandler)

	inner := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{inner: inner}
}

// ListenAndServe starts the HTTP server. Blocks until the server exits.
func (s *Server) ListenAndServe() error {
	if err := s.inner.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server with the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}
