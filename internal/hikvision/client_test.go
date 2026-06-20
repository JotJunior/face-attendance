package hikvision_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

func makeISAPIServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, hikvision.DeviceConfig) {
	t.Helper()
	srv := httptest.NewServer(handler)
	cfg := hikvision.DeviceConfig{
		Host:     srv.Listener.Addr().String(),
		Username: "admin",
		Password: "test_pass",
	}
	return srv, cfg
}

// TestUpsertUser_Create tests creating a user (POST returns 201).
func TestUpsertUser_Create(t *testing.T) {
	var receivedXML string
	var receivedMethod string

	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		receivedXML = string(buf[:n])
		w.WriteHeader(http.StatusCreated) // 201
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.UpsertUser(context.Background(), "12345678901", "Test User")
	if err != nil {
		t.Fatalf("UpsertUser() error: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", receivedMethod)
	}
	if !strings.Contains(receivedXML, "<employeeNo>12345678901</employeeNo>") {
		t.Errorf("XML missing employeeNo, got: %s", receivedXML)
	}
	if !strings.Contains(receivedXML, "<name>Test User</name>") {
		t.Errorf("XML missing name, got: %s", receivedXML)
	}
}

// TestUpsertUser_Update tests that 409 on POST triggers a PUT.
func TestUpsertUser_Update(t *testing.T) {
	callCount := 0
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict) // 409 — user exists
		} else if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusOK) // 200
		}
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.UpsertUser(context.Background(), "12345678901", "Test User")
	if err != nil {
		t.Fatalf("UpsertUser() error on update path: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (POST+PUT), got %d", callCount)
	}
}

// TestUpsertUser_500Retriable tests that 500 returns a retriable error.
func TestUpsertUser_500Retriable(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.UpsertUser(context.Background(), "12345678901", "Test")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if hikvision.IsNonRetriable(err) {
		t.Error("500 should be retriable, not non-retriable")
	}
}

// TestUpsertUser_400NonRetriable tests that 400 returns a non-retriable error.
func TestUpsertUser_400NonRetriable(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.UpsertUser(context.Background(), "12345678901", "Test")
	if err == nil {
		t.Fatal("expected error for 400")
	}
	if !hikvision.IsNonRetriable(err) {
		t.Error("400 should be non-retriable")
	}
}

// TestConfigureWebhook_XML tests that the webhook XML contains all required fields.
func TestConfigureWebhook_XML(t *testing.T) {
	var receivedXML string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedXML = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.ConfigureWebhook(context.Background(), "http://192.168.1.50:8080/webhook/abc123")
	if err != nil {
		t.Fatalf("ConfigureWebhook() error: %v", err)
	}

	requiredTags := []string{
		"<HttpHostNotification>",
		"<id>",
		"<protocolType>HTTP</protocolType>",
		"<parameterFormatType>XML</parameterFormatType>",
		"<addressingFormatType>ipaddress</addressingFormatType>",
		"<ipAddress>192.168.1.50</ipAddress>",
		"<portNo>8080</portNo>",
		"<path>/webhook/abc123</path>",
		"<httpAuthenticationMethod>none</httpAuthenticationMethod>",
	}
	for _, tag := range requiredTags {
		if !strings.Contains(receivedXML, tag) {
			t.Errorf("ConfigureWebhook XML missing %s; got:\n%s", tag, receivedXML)
		}
	}
}

// TestConfigureWebhook_500 tests that 500 returns an error.
func TestConfigureWebhook_500(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	err := client.ConfigureWebhook(context.Background(), "http://192.168.1.1:8080/webhook/secret")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}
