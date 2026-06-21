package httphandler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httphandler "github.com/jotjunior/face-attendance/internal/http"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestIPAllowlistMiddleware_Allowed tests that an allowed IP passes through.
func TestIPAllowlistMiddleware_Allowed(t *testing.T) {
	allowed := func() []string { return []string{"192.168.1.10"} }
	handler := httphandler.IPAllowlistMiddleware(allowed, okHandler())

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", nil)
	req.RemoteAddr = "192.168.1.10:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestIPAllowlistMiddleware_ForbiddenPublic tests that an unknown PUBLIC IP gets 403.
// (Allowlist auto-curável: só IPs públicos desconhecidos são barrados; ver abaixo.)
func TestIPAllowlistMiddleware_ForbiddenPublic(t *testing.T) {
	allowed := func() []string { return []string{"192.168.1.10"} }
	handler := httphandler.IPAllowlistMiddleware(allowed, okHandler())

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", nil)
	req.RemoteAddr = "203.0.113.50:12345" // TEST-NET-3: público, fora da allowlist
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unknown public IP, got %d", w.Code)
	}
}

// TestIPAllowlistMiddleware_UnknownLANAllowed verifica a allowlist auto-curável:
// um device novo / com IP trocado na LAN (RFC1918) é aceito para se registrar pelo
// heartbeat — mesmo sem estar na allowlist do banco.
func TestIPAllowlistMiddleware_UnknownLANAllowed(t *testing.T) {
	allowed := func() []string { return []string{"192.168.1.10"} }
	handler := httphandler.IPAllowlistMiddleware(allowed, okHandler())

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", nil)
	req.RemoteAddr = "10.0.0.99:12345" // privado, fora da allowlist → permitido p/ registro
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for unknown LAN IP (auto-register), got %d", w.Code)
	}
}

// TestAdminAuthMiddleware_Absent tests that missing token returns 401.
func TestAdminAuthMiddleware_Absent(t *testing.T) {
	handler := httphandler.AdminAuthMiddleware("secrettoken", okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/sync", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// TestAdminAuthMiddleware_Wrong tests that wrong token returns 403.
func TestAdminAuthMiddleware_Wrong(t *testing.T) {
	handler := httphandler.AdminAuthMiddleware("secrettoken", okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/sync", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// TestAdminAuthMiddleware_Correct tests that correct token returns 200.
func TestAdminAuthMiddleware_Correct(t *testing.T) {
	handler := httphandler.AdminAuthMiddleware("secrettoken", okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/sync", nil)
	req.Header.Set("Authorization", "Bearer secrettoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// TestRateLimitMiddleware_BurstExceeded tests that burst above limit returns 429.
func TestRateLimitMiddleware_BurstExceeded(t *testing.T) {
	// Set very low limit to trigger easily in tests
	rl := httphandler.NewRateLimitMiddleware(2) // 2 per minute
	handler := rl.Handler(okHandler())

	// First 2 requests should pass
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.RemoteAddr = "10.0.0.1:9000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 3rd request should be rate-limited
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.RemoteAddr = "10.0.0.1:9000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 on 3rd request, got %d", w.Code)
	}
}

// TestSyncSerializer_Concurrent tests that concurrent syncs return 409.
func TestSyncSerializer_Concurrent(t *testing.T) {
	s := httphandler.NewSyncSerializer(0) // no interval for tests

	acquired1, release1 := s.TryAcquire()
	if !acquired1 {
		t.Fatal("first acquire should succeed")
	}
	defer release1()

	acquired2, _ := s.TryAcquire()
	if acquired2 {
		t.Error("second acquire while first is running should fail")
	}
}
