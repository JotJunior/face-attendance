package gob_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/gob"
)

// makeGobServer creates a test HTTP server with customizable handler.
func makeGobServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

// TestListMembers_Valid tests successful member listing.
func TestListMembers_Valid(t *testing.T) {
	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		selfie := "https://example.com/avatar.jpg"
		resp := map[string]any{
			"success": true,
			"data": []map[string]any{
				{
					"id": 1, "status": "REGULAR", "created_at": "2026-01-01T00:00:00.000000Z",
					"updated_at": "2026-01-01T00:00:00.000000Z",
					"federal_document": "12345678901", "name": "Test User",
					"url_selfie": selfie,
				},
				{
					"id": 2, "status": "REGULAR", "created_at": "2026-01-01T00:00:00.000000Z",
					"updated_at": "2026-01-01T00:00:00.000000Z",
					"federal_document": "98765432100", "name": "No Selfie User",
					// url_selfie intentionally absent
				},
			},
		}
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "tok123", srv.Client())
	members, err := client.ListMembers(context.Background())
	if err != nil {
		t.Fatalf("ListMembers() error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
	if !members[0].HasSelfie() {
		t.Error("member[0] should have selfie")
	}
	if members[1].HasSelfie() {
		t.Error("member[1] should NOT have selfie")
	}
}

// TestListMembers_SuccessFalse tests that success=false returns an error.
func TestListMembers_SuccessFalse(t *testing.T) {
	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": false, "data": []any{}}) //nolint:errcheck
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "tok", srv.Client())
	_, err := client.ListMembers(context.Background())
	if err == nil {
		t.Fatal("expected error for success=false, got nil")
	}
}

// TestListMembers_HTTP500 tests that HTTP 500 returns an error.
func TestListMembers_HTTP500(t *testing.T) {
	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "tok", srv.Client())
	_, err := client.ListMembers(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500: %v", err)
	}
}

// TestMarkAttendance_Success tests successful attendance marking.
func TestMarkAttendance_Success(t *testing.T) {
	var receivedBody string
	var receivedAuth string

	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		b, _ := json.Marshal(map[string]string{})
		_ = b
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "secret_token", srv.Client())
	err := client.MarkAttendance(context.Background(), "12345678901")
	if err != nil {
		t.Fatalf("MarkAttendance() error: %v", err)
	}

	// Verify: CPF in masked format
	if !strings.Contains(receivedBody, `"cpf":"123.456.789-01"`) {
		t.Errorf("body should contain masked CPF, got: %s", receivedBody)
	}

	// Verify: Authorization without Bearer prefix
	if receivedAuth != "secret_token" {
		t.Errorf("Authorization = %q, want %q (no Bearer prefix)", receivedAuth, "secret_token")
	}
}

// TestMarkAttendance_4xx tests that 4xx returns an error.
func TestMarkAttendance_4xx(t *testing.T) {
	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "tok", srv.Client())
	err := client.MarkAttendance(context.Background(), "12345678901")
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

// TestMarkAttendance_500Retriable tests that 500 returns an error (caller handles retry).
func TestMarkAttendance_500Retriable(t *testing.T) {
	srv := makeGobServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	})
	defer srv.Close()

	client := gob.NewWithHTTPClient(srv.URL, "tok", srv.Client())
	err := client.MarkAttendance(context.Background(), "12345678901")
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
