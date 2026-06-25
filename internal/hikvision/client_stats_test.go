package hikvision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestGetDeviceStats_AggregatesAllFields verifies that GetDeviceStats aggregates the 4 payloads
// into a DeviceStats with all fields correct (tasks 1.10.3).
func TestGetDeviceStats_AggregatesAllFields(t *testing.T) {
	var callCount int32

	// Payloads — SOURCED field names from Stats.php:globalStats()
	userCountPayload := `{"UserInfoCount":{"userNumber":150,"bindFaceUserNumber":120,"bindCardUserNumber":30}}`
	userCapPayload := `{"UserInfo":{"maxRecordNum":500}}`
	eventCountPayload := `{"AcsEventTotalNum":{"totalNum":4200}}`
	// @max comes from PHP XML attribute-to-array conversion
	eventCapPayload := `{"AcsEvent":{"totalNum":{"@max":100000}}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		switch n {
		case 1: // GET UserInfo/Count
			w.Write([]byte(userCountPayload)) //nolint:errcheck
		case 2: // GET UserInfo/capabilities
			w.Write([]byte(userCapPayload)) //nolint:errcheck
		case 3: // POST AcsEventTotalNum
			w.Write([]byte(eventCountPayload)) //nolint:errcheck
		case 4: // GET AcsEventTotalNum/capabilities
			w.Write([]byte(eventCapPayload)) //nolint:errcheck
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	stats, err := c.GetDeviceStats(context.Background())
	if err != nil {
		t.Fatalf("GetDeviceStats: %v", err)
	}

	// Users
	if stats.Users.Total != 150 {
		t.Errorf("Users.Total = %d, want 150", stats.Users.Total)
	}
	if stats.Users.Faces != 120 {
		t.Errorf("Users.Faces = %d, want 120", stats.Users.Faces)
	}
	if stats.Users.Cards != 30 {
		t.Errorf("Users.Cards = %d, want 30", stats.Users.Cards)
	}
	if stats.Users.Max != 500 {
		t.Errorf("Users.Max = %d, want 500", stats.Users.Max)
	}

	// Events
	if stats.Events.Total != 4200 {
		t.Errorf("Events.Total = %d, want 4200", stats.Events.Total)
	}
	if stats.Events.Max != 100000 {
		t.Errorf("Events.Max = %d, want 100000", stats.Events.Max)
	}

	if int(atomic.LoadInt32(&callCount)) != 4 {
		t.Errorf("expected 4 ISAPI calls, got %d", callCount)
	}
}

// TestGetDeviceStats_FailsWithStepContext verifies that an error on one of the 4 calls
// returns an error that names the failing step (tasks 1.10.3).
func TestGetDeviceStats_FailsWithStepContext(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.WriteHeader(200)
			w.Write([]byte(`{"UserInfoCount":{"userNumber":1}}`)) //nolint:errcheck
			return
		}
		// Fail at step (2) — capabilities
		w.WriteHeader(503)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetDeviceStats(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	// Error should mention which call failed
	if !strings.Contains(err.Error(), "capabilities") && !strings.Contains(err.Error(), "UserInfo") {
		t.Errorf("error should name the failing ISAPI call, got: %v", err)
	}
}

// TestGetDeviceStats_EventCountPOST verifies that the event count call uses POST with condition body.
// SOURCED: Stats.php:eventsCount() — {AcsEventTotalNumCond:{major:0, minor:0}}
func TestGetDeviceStats_EventCountPOST(t *testing.T) {
	var (
		eventMethod string
		eventBody   string
	)
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		switch n {
		case 1:
			w.Write([]byte(`{"UserInfoCount":{"userNumber":0}}`)) //nolint:errcheck
		case 2:
			w.Write([]byte(`{"UserInfo":{"maxRecordNum":0}}`)) //nolint:errcheck
		case 3:
			// Capture event count request
			eventMethod = r.Method
			buf := make([]byte, 256)
			nn, _ := r.Body.Read(buf)
			eventBody = string(buf[:nn])
			w.Write([]byte(`{"AcsEventTotalNum":{"totalNum":0}}`)) //nolint:errcheck
		case 4:
			w.Write([]byte(`{"AcsEvent":{"totalNum":{"@max":0}}}`)) //nolint:errcheck
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, err := c.GetDeviceStats(context.Background()); err != nil {
		t.Fatalf("GetDeviceStats: %v", err)
	}

	if eventMethod != http.MethodPost {
		t.Errorf("event count should use POST, got %s", eventMethod)
	}
	if !strings.Contains(eventBody, "AcsEventTotalNumCond") {
		t.Errorf("event count body should contain AcsEventTotalNumCond, got: %s", eventBody)
	}
}
