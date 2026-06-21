package hikvision_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestListWebhooks_ParsesXML verifies ListWebhooks parses the SOURCED XML response format.
// SOURCED: NotificationService.php:397-429 (parseWebhookConfig).
func TestListWebhooks_ParsesXML(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<HttpHostNotificationList>` +
		`<HttpHostNotification>` +
		`<id>abc123</id>` +
		`<url>http://192.168.1.5:9090/webhook/secret</url>` +
		`<protocolType>HTTP</protocolType>` +
		`</HttpHostNotification>` +
		`</HttpHostNotificationList>`

	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "httpHosts") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xmlResp)) //nolint:errcheck
	})
	defer srv.Close()

	hosts, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].ID != "abc123" {
		t.Errorf("ID: got %q, want %q", hosts[0].ID, "abc123")
	}
	if !strings.Contains(hosts[0].URL, "/webhook/secret") {
		t.Errorf("URL: got %q", hosts[0].URL)
	}
	if hosts[0].Protocol != "HTTP" {
		t.Errorf("Protocol: got %q, want HTTP", hosts[0].Protocol)
	}
}

// TestDeleteWebhook_SendsDELETE verifies DeleteWebhook sends DELETE to the correct path.
// SOURCED: NotificationService.php:92-123.
func TestDeleteWebhook_SendsDELETE(t *testing.T) {
	var capturedPath string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).DeleteWebhook(context.Background(), "abc123")
	if err != nil {
		t.Errorf("DeleteWebhook: %v", err)
	}
	if !strings.HasSuffix(capturedPath, "/abc123") {
		t.Errorf("path: got %q, want suffix /abc123", capturedPath)
	}
}

// TestDeleteWebhook_401_NonRetriable verifies 401 maps to NonRetriableError.
func TestDeleteWebhook_401_NonRetriable(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).DeleteWebhook(context.Background(), "xyz")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !hikvision.IsNonRetriable(err) {
		t.Errorf("expected NonRetriableError for 401, got %T: %v", err, err)
	}
}

// TestListWebhooks_EmptyOnNotFound verifies 404 returns empty list (some firmware behavior).
func TestListWebhooks_EmptyOnNotFound(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	hosts, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ListWebhooks(context.Background())
	if err != nil {
		t.Fatalf("expected no error for 404, got %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected empty list for 404, got %d hosts", len(hosts))
	}
}
