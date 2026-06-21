package httphandler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	httphandler "github.com/jotjunior/face-attendance/internal/http"
	"github.com/jotjunior/face-attendance/internal/logging"
)

// --- Stubs ---

type stubDeviceRepo struct{ upsertCalled bool }

func (s *stubDeviceRepo) Upsert(_ context.Context, _ domain.Device) error {
	s.upsertCalled = true
	return nil
}

func (s *stubDeviceRepo) FindByMAC(_ context.Context, _ string) (*domain.Device, error) {
	return nil, nil
}

type stubMemberRepo struct{ member *domain.Member }

func (s *stubMemberRepo) FindByCPF(_ context.Context, _ string) (*domain.Member, error) {
	return s.member, nil
}

type stubGOBClient struct{ markCalled bool }

func (s *stubGOBClient) MarkAttendance(_ context.Context, _ string) error {
	s.markCalled = true
	return nil
}

type stubEventRepo struct {
	insertCalled bool
	markCalled   bool
	insertResult bool // whether to simulate "new event"
}

func (s *stubEventRepo) InsertIfNotExists(_ context.Context, _ domain.AttendanceEvent) (bool, error) {
	s.insertCalled = true
	return s.insertResult, nil
}

func (s *stubEventRepo) MarkAsMarked(_ context.Context, _ string) error {
	s.markCalled = true
	return nil
}

type stubHealthChecker struct{ dbOK, rabbitOK bool }

func (s *stubHealthChecker) PingDB(_ context.Context) error {
	if !s.dbOK {
		return context.DeadlineExceeded
	}
	return nil
}

func (s *stubHealthChecker) PingRabbitMQ() error {
	if !s.rabbitOK {
		return context.DeadlineExceeded
	}
	return nil
}

type stubScheduler struct{ cycleCalled bool }

func (s *stubScheduler) RunMemberLoadCycle(_ context.Context) error {
	s.cycleCalled = true
	return nil
}

// --- Helpers ---

func newEventHandler(t *testing.T,
	deviceRepo *stubDeviceRepo,
	memberRepo *stubMemberRepo,
	gobClient *stubGOBClient,
	eventRepo *stubEventRepo,
) *httphandler.EventHandler {
	t.Helper()
	logger := logging.NewWithHandler(noopHandler{})
	return httphandler.NewEventHandler(deviceRepo, memberRepo, gobClient, eventRepo, logger)
}

type noopHandler struct{}

func (noopHandler) Enabled(_ context.Context, _ slog.Level) bool       { return false }
func (noopHandler) Handle(_ context.Context, _ slog.Record) error       { return nil }
func (noopHandler) WithAttrs(_ []slog.Attr) slog.Handler                { return noopHandler{} }
func (noopHandler) WithGroup(_ string) slog.Handler                     { return noopHandler{} }

// --- Tests ---

// TestEventHandler_Heartbeat_DeviceUpserted verifies that any inbound event causes device upsert.
func TestEventHandler_Heartbeat_DeviceUpserted(t *testing.T) {
	deviceRepo := &stubDeviceRepo{}
	memberRepo := &stubMemberRepo{}
	gobClient := &stubGOBClient{}
	eventRepo := &stubEventRepo{}

	payload := map[string]interface{}{
		"eventType":  "videoloss",
		"ipAddress":  "192.168.1.50",
		"macAddress": "AA:BB:CC:DD:EE:FF",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := newEventHandler(t, deviceRepo, memberRepo, gobClient, eventRepo)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !deviceRepo.upsertCalled {
		t.Error("expected device upsert to be called")
	}
	if gobClient.markCalled {
		t.Error("GOB mark should NOT be called for non-AccessControllerEvent")
	}
}

// TestEventHandler_AttendanceMarked verifies full happy path: AccessControllerEvent → mark attendance.
func TestEventHandler_AttendanceMarked(t *testing.T) {
	deviceRepo := &stubDeviceRepo{}
	cpf := "12345678901"
	memberRepo := &stubMemberRepo{member: &domain.Member{
		ID:              1,
		FederalDocument: cpf,
		Name:            "Test User",
	}}
	gobClient := &stubGOBClient{}
	eventRepo := &stubEventRepo{insertResult: true}

	// Datetime RFC3339 required for deterministic event_key
	dt := time.Now().UTC().Format(time.RFC3339)

	payload := map[string]interface{}{
		"eventType":  "AccessControllerEvent",
		"ipAddress":  "192.168.1.50",
		"macAddress": "AA:BB:CC:DD:EE:FF",
		"dateTime":   dt,
		"AccessControllerEvent": map[string]interface{}{
			"employeeNoString": cpf,
			"attendanceStatus": "authorized",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := newEventHandler(t, deviceRepo, memberRepo, gobClient, eventRepo)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !eventRepo.insertCalled {
		t.Error("expected event insert to be called")
	}
	if !gobClient.markCalled {
		t.Error("expected GOB mark to be called for authorized event")
	}
	if !eventRepo.markCalled {
		t.Error("expected MarkAsMarked to be called")
	}
}

// TestEventHandler_DuplicateEvent verifies that duplicate events are silently ignored (idempotency FR-016).
func TestEventHandler_DuplicateEvent(t *testing.T) {
	deviceRepo := &stubDeviceRepo{}
	cpf := "12345678901"
	memberRepo := &stubMemberRepo{member: &domain.Member{
		ID:              1,
		FederalDocument: cpf,
	}}
	gobClient := &stubGOBClient{}
	eventRepo := &stubEventRepo{insertResult: false} // simulate duplicate

	dt := time.Now().UTC().Format(time.RFC3339)
	payload := map[string]interface{}{
		"eventType":  "AccessControllerEvent",
		"ipAddress":  "192.168.1.50",
		"macAddress": "AA:BB:CC:DD:EE:FF",
		"dateTime":   dt,
		"AccessControllerEvent": map[string]interface{}{
			"employeeNoString": cpf,
			"attendanceStatus": "authorized",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler := newEventHandler(t, deviceRepo, memberRepo, gobClient, eventRepo)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gobClient.markCalled {
		t.Error("GOB mark should NOT be called for duplicate event")
	}
}

// TestEventHandler_InvalidCPF verifies that events with invalid employeeNoString are rejected cleanly.
func TestEventHandler_InvalidCPF(t *testing.T) {
	deviceRepo := &stubDeviceRepo{}
	memberRepo := &stubMemberRepo{}
	gobClient := &stubGOBClient{}
	eventRepo := &stubEventRepo{}

	payload := map[string]interface{}{
		"eventType":  "AccessControllerEvent",
		"macAddress": "AA:BB:CC:DD:EE:FF",
		"AccessControllerEvent": map[string]interface{}{
			"employeeNoString": "not-a-cpf",
			"attendanceStatus": "authorized",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler := newEventHandler(t, deviceRepo, memberRepo, gobClient, eventRepo)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if eventRepo.insertCalled {
		t.Error("event insert should NOT be called for invalid CPF")
	}
}

// TestHealthHandler_AllOK verifies 200 when both DB and RabbitMQ are healthy.
func TestHealthHandler_AllOK(t *testing.T) {
	handler := httphandler.NewHealthHandler(&stubHealthChecker{dbOK: true, rabbitOK: true})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
}

// TestHealthHandler_DBDown verifies 503 when DB is unreachable.
func TestHealthHandler_DBDown(t *testing.T) {
	handler := httphandler.NewHealthHandler(&stubHealthChecker{dbOK: false, rabbitOK: true})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// TestAdminSyncHandler_Accepted verifies 202 on valid admin sync request.
func TestAdminSyncHandler_Accepted(t *testing.T) {
	sched := &stubScheduler{}
	serializer := httphandler.NewSyncSerializer(0)
	logger := logging.NewWithHandler(noopHandler{})

	handler := httphandler.NewAdminSyncHandler(sched, serializer, logger)

	req := httptest.NewRequest(http.MethodPost, "/admin/sync", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}
