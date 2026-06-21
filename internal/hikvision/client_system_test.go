package hikvision_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestReboot_200_ReturnsNil verifies Reboot returns nil on HTTP 200.
// SOURCED: DeviceService.php:222.
func TestReboot_200_ReturnsNil(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/System/reboot") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).Reboot(context.Background())
	if err != nil {
		t.Errorf("Reboot: expected nil, got %v", err)
	}
}

// TestReboot_401_ReturnsNonRetriableError verifies Reboot returns NonRetriableError on 401.
func TestReboot_401_ReturnsNonRetriableError(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).Reboot(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !hikvision.IsNonRetriable(err) {
		t.Errorf("expected NonRetriableError for 401, got %T: %v", err, err)
	}
}

// TestGetTime_ParsesJSON verifies GetTime correctly parses the ISAPI JSON response.
// SOURCED: DeviceService.php:parseTimeData (L395-406).
func TestGetTime_ParsesJSON(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/System/time") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"Time":{"localTime":"2026-06-21T14:30:00","timeZone":"CST-8:00:00","timeMode":"manual"}}`)) //nolint:errcheck
	})
	defer srv.Close()

	td, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).GetTime(context.Background())
	if err != nil {
		t.Fatalf("GetTime: %v", err)
	}
	if td.LocalTime != "2026-06-21T14:30:00" {
		t.Errorf("LocalTime: got %q", td.LocalTime)
	}
	if td.TimeZone != "CST-8:00:00" {
		t.Errorf("TimeZone: got %q", td.TimeZone)
	}
	if td.TimeMode != "manual" {
		t.Errorf("TimeMode: got %q", td.TimeMode)
	}
}

// TestSetTime_ManualMode_SendsCorrectBody verifies SetTime sends JSON with timeMode=manual.
// SOURCED: DeviceService.php:278-320.
func TestSetTime_ManualMode_SendsCorrectBody(t *testing.T) {
	var capturedBody string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.Contains(r.URL.Path, "/System/time") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		b := make([]byte, 1024)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).SetTime(context.Background(), hikvision.TimeSetRequest{
		LocalTime: "2026-06-21T14:30:00",
		TimeZone:  "CST-8:00:00",
		TimeMode:  "manual",
	})
	if err != nil {
		t.Fatalf("SetTime: %v", err)
	}
	if !strings.Contains(capturedBody, `"timeMode"`) || !strings.Contains(capturedBody, "manual") {
		t.Errorf("body missing timeMode=manual: %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "2026-06-21T14:30:00") {
		t.Errorf("body missing localTime: %q", capturedBody)
	}
}
