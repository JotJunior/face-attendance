package httphandler_test

// Testes de integração do webhook com o motor de fluxo.
// Ref: docs/specs/face-flow/tasks.md §6.4
// Utiliza stubs — não requer DB ou RabbitMQ.
// Verifica que AccessControllerEvent dispara o motor; heartbeat não dispara.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	httphandler "github.com/jotjunior/face-attendance/internal/http"
	"github.com/jotjunior/face-attendance/internal/logging"
)

// ─── Stubs locais de suporte ──────────────────────────────────────────────────

type stubDeviceRepoWF struct{ dev *domain.Device }

func (s *stubDeviceRepoWF) Upsert(_ context.Context, _ domain.Device) error { return nil }
func (s *stubDeviceRepoWF) FindByMAC(_ context.Context, _ string) (*domain.Device, error) {
	return s.dev, nil
}

type stubMemberRepoWF struct{ member *domain.Member }

func (s *stubMemberRepoWF) FindByCPF(_ context.Context, _ string) (*domain.Member, error) {
	return s.member, nil
}

type stubGOBClientWF struct{}

func (s *stubGOBClientWF) MarkAttendance(_ context.Context, _ string) error { return nil }

type stubEventRepoWF struct{ insertResult bool }

func (s *stubEventRepoWF) InsertIfNotExists(_ context.Context, _ domain.AttendanceEvent) (bool, error) {
	return s.insertResult, nil
}

func (s *stubEventRepoWF) MarkAsMarked(_ context.Context, _ string) error { return nil }

// stubFlowEngine registra chamadas a TriggerForDevice de forma goroutine-safe.
type stubFlowEngine struct {
	mu      sync.Mutex
	calls   int
	lastMAC string
}

func (e *stubFlowEngine) TriggerForDevice(mac string, _ *domain.AttendanceEvent, _ *domain.Member, _ *domain.Device) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	e.lastMAC = mac
}

func (e *stubFlowEngine) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newHandlerWF(t *testing.T, eng *stubFlowEngine, insertResult bool, cpf string) *httphandler.EventHandler {
	t.Helper()
	ip := "192.168.1.55"
	mac := "AA:BB:CC:DD:EE:FF"
	dev := &domain.Device{ID: 7, DeviceIdentifier: mac, IPAddress: &ip, MACAddress: &mac}
	deviceRepo := &stubDeviceRepoWF{dev: dev}
	memberRepo := &stubMemberRepoWF{member: &domain.Member{ID: 1, FederalDocument: cpf, Name: "Test"}}
	gobClient := &stubGOBClientWF{}
	eventRepo := &stubEventRepoWF{insertResult: insertResult}
	logger := logging.NewWithHandler(noopHandler{})

	h := httphandler.NewEventHandler(deviceRepo, memberRepo, gobClient, eventRepo, logger)
	if eng != nil {
		h.SetFlowEngine(eng)
	}
	return h
}

func accessControllerPayload(cpf string) []byte {
	dt := time.Now().UTC().Format(time.RFC3339)
	p := map[string]interface{}{
		"eventType":  "AccessControllerEvent",
		"ipAddress":  "192.168.1.55",
		"macAddress": "AA:BB:CC:DD:EE:FF",
		"dateTime":   dt,
		"AccessControllerEvent": map[string]interface{}{
			"employeeNoString": cpf,
			"attendanceStatus": "authorized",
		},
	}
	b, _ := json.Marshal(p)
	return b
}

func heartbeatPayload() []byte {
	p := map[string]interface{}{
		"eventType":  "StatusControllerEvent",
		"ipAddress":  "192.168.1.55",
		"macAddress": "AA:BB:CC:DD:EE:FF",
	}
	b, _ := json.Marshal(p)
	return b
}

// ─── Testes 6.4 ──────────────────────────────────────────────────────────────

// TestWebhook_AccessControllerEvent_TriggersEngine verifica que um evento
// AccessControllerEvent com CPF válido e novo aciona o motor de fluxo.
// Ref: tasks.md §6.4.1
func TestWebhook_AccessControllerEvent_TriggersEngine(t *testing.T) {
	const cpf = "12345678901"
	eng := &stubFlowEngine{}
	h := newHandlerWF(t, eng, true /* new event */, cpf)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(accessControllerPayload(cpf)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("esperado 200, obteve %d", w.Code)
	}

	// TriggerForDevice é chamado em goroutine — aguardar até 200ms.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) && eng.callCount() == 0 {
		time.Sleep(5 * time.Millisecond)
	}

	if eng.callCount() != 1 {
		t.Errorf("TriggerForDevice deveria ter sido chamado 1 vez, obteve %d", eng.callCount())
	}
}

// TestWebhook_HeartbeatIgnoredByEngine verifica que eventos de heartbeat
// (não-AccessControllerEvent) NÃO acionam o motor de fluxo.
// Ref: tasks.md §6.4.2
func TestWebhook_HeartbeatIgnoredByEngine(t *testing.T) {
	eng := &stubFlowEngine{}
	h := newHandlerWF(t, eng, false, "12345678901")

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(heartbeatPayload()))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("esperado 200, obteve %d", w.Code)
	}

	// Garantir que não há disparo tardio.
	time.Sleep(20 * time.Millisecond)
	if eng.callCount() != 0 {
		t.Errorf("TriggerForDevice NÃO deve ser chamado para heartbeat, obteve %d chamada(s)", eng.callCount())
	}
}

// TestWebhook_NoFlowForDevice verifica que o webhook retorna 200 sem erro
// quando não há motor de fluxo configurado (device sem fluxo).
// Ref: tasks.md §6.4.3
func TestWebhook_NoFlowForDevice(t *testing.T) {
	const cpf = "12345678901"
	// eng = nil → flowEngine não configurado no handler (simula device sem fluxo)
	h := newHandlerWF(t, nil, true, cpf)

	req := httptest.NewRequest(http.MethodPost, "/webhook/secret", bytes.NewReader(accessControllerPayload(cpf)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("esperado 200 sem motor de fluxo, obteve %d", w.Code)
	}
}
