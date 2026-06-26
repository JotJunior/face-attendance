package hikvision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sampleVerifyWeekPlan builds a sample 7-day verify week plan JSON.
func sampleVerifyWeekPlan(mode string) []byte {
	plans := make([]WeekPlanCfg, 7)
	for i := range plans {
		plans[i] = WeekPlanCfg{WeekNo: i + 1, VerifyMode: mode}
	}
	env := verifyWeekPlanEnvelope{
		VerifyWeekPlanCfg: VerifyWeekPlan{WeekPlanCfgs: plans},
	}
	b, _ := json.Marshal(env)
	return b
}

// TestGetVerifyMode_ParsesSevenPlans checks that GetVerifyMode correctly parses a
// 7-entry week plan response (spec §FR-001 / tasks 1.1.3).
func TestGetVerifyMode_ParsesSevenPlans(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/AccessControl/VerifyWeekPlanCfg/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write(sampleVerifyWeekPlan("face")) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	plan, err := c.GetVerifyMode(context.Background())
	if err != nil {
		t.Fatalf("GetVerifyMode: %v", err)
	}
	if len(plan.WeekPlanCfgs) != 7 {
		t.Errorf("expected 7 week plans, got %d", len(plan.WeekPlanCfgs))
	}
	for i, p := range plan.WeekPlanCfgs {
		if p.WeekNo != i+1 {
			t.Errorf("plan[%d].WeekNo = %d, want %d", i, p.WeekNo, i+1)
		}
		if p.VerifyMode != "face" {
			t.Errorf("plan[%d].VerifyMode = %q, want %q", i, p.VerifyMode, "face")
		}
	}
}

// TestGetVerifyMode_401_NonRetriable checks that 401 → NonRetriableError (tasks 1.1.3).
func TestGetVerifyMode_401_NonRetriable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetVerifyMode(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNonRetriable(err) {
		t.Errorf("expected NonRetriableError, got %T: %v", err, err)
	}
}

// TestSetVerifyMode_PayloadAllPlans checks that SetVerifyMode sends verifyMode
// in all 7 entries of the plan (tasks 1.2.3).
func TestSetVerifyMode_PayloadAllPlans(t *testing.T) {
	var captured []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write(sampleVerifyWeekPlan("card")) //nolint:errcheck
		case http.MethodPut:
			var buf [1 << 20]byte
			n, _ := r.Body.Read(buf[:])
			captured = make([]byte, n)
			copy(captured, buf[:n])
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected method: %s", r.Method)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.SetVerifyMode(context.Background(), "face"); err != nil {
		t.Fatalf("SetVerifyMode: %v", err)
	}

	var env verifyWeekPlanEnvelope
	if err := json.Unmarshal(captured, &env); err != nil {
		t.Fatalf("parse PUT body: %v", err)
	}
	if len(env.VerifyWeekPlanCfg.WeekPlanCfgs) != 7 {
		t.Errorf("PUT body has %d plans, want 7", len(env.VerifyWeekPlanCfg.WeekPlanCfgs))
	}
	for i, p := range env.VerifyWeekPlanCfg.WeekPlanCfgs {
		if p.VerifyMode != "face" {
			t.Errorf("plan[%d].VerifyMode = %q, want %q", i, p.VerifyMode, "face")
		}
	}
}

// TestSetVerifyMode_Idempotent checks that SetVerifyMode twice with the same mode
// produces identical payload (Constitution II / tasks 1.2.2).
func TestSetVerifyMode_Idempotent(t *testing.T) {
	var bodies [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write(sampleVerifyWeekPlan("face")) //nolint:errcheck
		case http.MethodPut:
			var buf [1 << 20]byte
			n, _ := r.Body.Read(buf[:])
			b := make([]byte, n)
			copy(b, buf[:n])
			bodies = append(bodies, b)
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		if err := c.SetVerifyMode(context.Background(), "face"); err != nil {
			t.Fatalf("SetVerifyMode call %d: %v", i+1, err)
		}
	}

	if len(bodies) != 2 {
		t.Fatalf("expected 2 PUT bodies, got %d", len(bodies))
	}
	if string(bodies[0]) != string(bodies[1]) {
		t.Errorf("SetVerifyMode is NOT idempotent: first=%q second=%q",
			string(bodies[0]), string(bodies[1]))
	}
}

// newTestClient creates a hikvision Client pointed at the test server.
// Reuses the pattern from client_test.go (NewWithHTTPClient).
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	// Extract host without scheme (DeviceConfig.Host format is host:port)
	host := srv.URL[len("http://"):]
	cfg := DeviceConfig{Host: host, Username: "admin", Password: "test"}
	return NewWithHTTPClient(cfg, srv.Client())
}
