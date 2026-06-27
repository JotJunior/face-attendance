package flowengine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/flowengine"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// ───────────────────────────────────────────────────────────────
// Fakes (mocks mínimos)
// ───────────────────────────────────────────────────────────────

type fakeFlowRepo struct {
	activeFlow *flow.Flow
	err        error
}

func (r *fakeFlowRepo) FindActiveByDeviceID(_ context.Context, _ int64) (*flow.Flow, error) {
	return r.activeFlow, r.err
}

type fakeLogRepo struct {
	mu       sync.Mutex
	entries  []*repository.FlowExecutionLog
	byKey    map[string]*repository.FlowExecutionLog
	createFn func(*repository.FlowExecutionLog) error
}

func newFakeLogRepo() *fakeLogRepo {
	return &fakeLogRepo{byKey: make(map[string]*repository.FlowExecutionLog)}
}

func (r *fakeLogRepo) Create(_ context.Context, log *repository.FlowExecutionLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createFn != nil {
		if err := r.createFn(log); err != nil {
			return err
		}
	}
	r.entries = append(r.entries, log)
	r.byKey[log.EventKey] = log
	return nil
}

func (r *fakeLogRepo) FindByEventKey(_ context.Context, eventKey string) (*repository.FlowExecutionLog, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byKey[eventKey], nil
}

func (r *fakeLogRepo) lastEntry() *repository.FlowExecutionLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return nil
	}
	return r.entries[len(r.entries)-1]
}

func (r *fakeLogRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

type fakeBgImageRepo struct {
	img *repository.BackgroundImage
	err error
}

func (r *fakeBgImageRepo) FindByID(_ context.Context, _ int64) (*repository.BackgroundImage, error) {
	return r.img, r.err
}

// ───────────────────────────────────────────────────────────────
// Helpers
// ───────────────────────────────────────────────────────────────

func rawJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func node(id string, t flow.NodeType, cfg any) flow.FlowNode {
	return flow.FlowNode{ID: id, Type: t, Config: rawJSON(cfg)}
}

func edge(from, to string) flow.FlowEdge {
	return flow.FlowEdge{From: from, To: to}
}

func edgeLabeled(from, to, label string) flow.FlowEdge {
	return flow.FlowEdge{From: from, To: to, Label: label}
}

func waitFlow(durationSec int) *flow.Flow {
	return &flow.Flow{
		ID: 1,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("w", flow.NodeTypeWait, flow.WaitConfig{DurationSeconds: durationSec}),
		},
		Edges: []flow.FlowEdge{
			edge("s", "w"),
		},
	}
}

func decisionFlow(authorized bool) (*flow.Flow, bool) {
	return &flow.Flow{
		ID: 2,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("d", flow.NodeTypeDecision, nil),
			node("ok", flow.NodeTypeStart, nil),   // reutilizar start como dummy terminal
			node("nok", flow.NodeTypeStart, nil),  // idem
		},
		Edges: []flow.FlowEdge{
			edge("s", "d"),
			edgeLabeled("d", "ok", "valid"),
			edgeLabeled("d", "nok", "invalid"),
		},
	}, authorized
}

// noSSRF é um SSRFChecker que não bloqueia nenhuma URL.
// APENAS para uso em testes unitários onde o servidor roda em loopback (httptest).
// Em produção o Engine sempre usa checkSSRF padrão.
func noSSRF(_ string) error { return nil }

// testEngine cria um Engine para testes de lógica de fluxo (não testa SSRF).
// Desabilita o SSRF guard pois httptest.Server usa loopback (127.0.0.1).
func testEngine(activeFlow *flow.Flow, logRepo *fakeLogRepo) *flowengine.Engine {
	return flowengine.New(flowengine.Config{
		FlowRepo:    &fakeFlowRepo{activeFlow: activeFlow},
		LogRepo:     logRepo,
		BgImageRepo: &fakeBgImageRepo{},
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
		SSRFChecker: noSSRF, // httptest usa loopback; SSRF desabilitado intencionalmente
	})
}

// testEngineWithSSRF cria um Engine com o SSRF guard padrão habilitado.
// Usar nos testes que validam o comportamento do guard (TestEngine_SSRFBlocked, etc.).
func testEngineWithSSRF(activeFlow *flow.Flow, logRepo *fakeLogRepo) *flowengine.Engine {
	return flowengine.New(flowengine.Config{
		FlowRepo:    &fakeFlowRepo{activeFlow: activeFlow},
		LogRepo:     logRepo,
		BgImageRepo: &fakeBgImageRepo{},
		HTTPClient:  &http.Client{Timeout: 2 * time.Second},
		// SSRFChecker não definido → usa checkSSRF padrão (bloqueia IPs internos)
	})
}

func testEvent(key, status string) *domain.AttendanceEvent {
	s := status
	return &domain.AttendanceEvent{
		EventKey:         key,
		AttendanceStatus: &s,
	}
}

func testDevice() *domain.Device {
	return &domain.Device{ID: 42}
}

func triggerAndWait(e *flowengine.Engine, evt *domain.AttendanceEvent, d *domain.Device) {
	e.TriggerForDevice("", evt, nil, d)
	// Aguardar goroutine concluir — simples sleep (testes unitários).
	time.Sleep(50 * time.Millisecond)
}

// ───────────────────────────────────────────────────────────────
// Testes
// ───────────────────────────────────────────────────────────────

// TestEngine_NoFlow: device sem fluxo ativo → TriggerForDevice retorna silenciosamente.
// Ref: tasks.md §6.2.1.
func TestEngine_NoFlow(t *testing.T) {
	logRepo := newFakeLogRepo()
	e := flowengine.New(flowengine.Config{
		FlowRepo:    &fakeFlowRepo{activeFlow: nil},
		LogRepo:     logRepo,
		BgImageRepo: &fakeBgImageRepo{},
	})
	e.TriggerForDevice("", testEvent("k1", "authorized"), nil, testDevice())
	time.Sleep(50 * time.Millisecond)
	if logRepo.count() != 0 {
		t.Fatalf("esperado 0 log entries; got %d", logRepo.count())
	}
}

// TestEngine_InvalidFlow: fluxo inválido (ciclo) → circuit-break imediato no validate.
// Ref: tasks.md §6.2.2.
func TestEngine_InvalidFlow(t *testing.T) {
	// Fluxo com ciclo: s → a → b → a
	cycleFlow := &flow.Flow{
		ID: 10,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("a", flow.NodeTypeWait, flow.WaitConfig{DurationSeconds: 1}),
			node("b", flow.NodeTypeWait, flow.WaitConfig{DurationSeconds: 1}),
		},
		Edges: []flow.FlowEdge{
			edge("s", "a"),
			edge("a", "b"),
			edge("b", "a"), // ciclo
		},
	}
	logRepo := newFakeLogRepo()
	e := testEngine(cycleFlow, logRepo)
	triggerAndWait(e, testEvent("k2", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil {
		t.Fatal("esperado log entry de circuit_break; nenhum encontrado")
	}
	if entry.Status != "circuit_break" {
		t.Fatalf("status esperado circuit_break; got %s", entry.Status)
	}
}

// TestEngine_WaitNode: start→wait(1s) → deve completar com status "completed".
// Ref: tasks.md §6.2.3.
func TestEngine_WaitNode(t *testing.T) {
	if testing.Short() {
		t.Skip("pula TestEngine_WaitNode em modo -short (demora ~1s)")
	}
	logRepo := newFakeLogRepo()
	e := testEngine(waitFlow(1), logRepo)

	start := time.Now()
	// TriggerForDevice retorna imediatamente; a goroutine executa em background.
	e.TriggerForDevice("", testEvent("k3", "authorized"), nil, testDevice())

	// Aguardar que a goroutine de execução termine (1s de wait + margem).
	time.Sleep(1500 * time.Millisecond)

	elapsed := time.Since(start)

	entry := logRepo.lastEntry()
	if entry == nil {
		t.Fatal("esperado log entry; nenhum encontrado")
	}
	if entry.Status != "completed" {
		t.Fatalf("status esperado completed; got %s", entry.Status)
	}
	// A execução deve ter durado pelo menos 1s (o nó wait).
	if elapsed < 900*time.Millisecond {
		t.Fatalf("execução muito rápida; esperado >= 1s; got %v", elapsed)
	}
}

// TestEngine_HTTPSCallTimeout: servidor não-responsivo + timeout 1s → circuit-break.
// Ref: tasks.md §6.2.4.
func TestEngine_HTTPSCallTimeout(t *testing.T) {
	// Servidor que aguarda o cancelamento do contexto para não deixar goroutine pendurada.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(30 * time.Second):
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { srv.CloseClientConnections(); srv.Close() })

	httpsFlow := &flow.Flow{
		ID: 20,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL:            srv.URL + "/ping",
				Method:         "POST",
				TimeoutSeconds: 1, // timeout de 1s
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}

	logRepo := newFakeLogRepo()
	e := flowengine.New(flowengine.Config{
		FlowRepo:    &fakeFlowRepo{activeFlow: httpsFlow},
		LogRepo:     logRepo,
		BgImageRepo: &fakeBgImageRepo{},
		HTTPClient:  &http.Client{},
		SSRFChecker: noSSRF, // httptest usa loopback
	})

	triggerAndWait(e, testEvent("k4", "authorized"), testDevice())
	time.Sleep(1500 * time.Millisecond) // aguardar timeout

	entry := logRepo.lastEntry()
	if entry == nil {
		t.Fatal("esperado log entry de circuit_break; nenhum encontrado")
	}
	if entry.Status != "circuit_break" {
		t.Fatalf("status esperado circuit_break; got %s", entry.Status)
	}
}

// TestEngine_DecisionValid: event.authorized=true → ramo "valid" executado.
// Ref: tasks.md §6.2.5.
func TestEngine_DecisionValid(t *testing.T) {
	// Fluxo: start → decision → [valid: wait(0)] | [invalid: wait(0)]
	// Usamos nó wait com duração mínima em ambos os ramos para poder distinguir.
	var visited atomic.Value
	visited.Store("")

	// Servidor que registra qual ramo foi chamado.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited.Store(r.URL.Path)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := &flow.Flow{
		ID: 30,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("d", flow.NodeTypeDecision, nil),
			node("valid_node", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/valid", Method: "GET",
			}),
			node("invalid_node", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/invalid", Method: "GET",
			}),
		},
		Edges: []flow.FlowEdge{
			edge("s", "d"),
			edgeLabeled("d", "valid_node", "valid"),
			edgeLabeled("d", "invalid_node", "invalid"),
		},
	}

	logRepo := newFakeLogRepo()
	e := testEngine(f, logRepo)
	evt := testEvent("k5", "authorized")
	e.TriggerForDevice("", evt, nil, testDevice())
	time.Sleep(200 * time.Millisecond)

	if visited.Load() != "/valid" {
		t.Fatalf("esperado ramo /valid; got %q", visited.Load())
	}
	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "completed" {
		t.Fatalf("esperado completed; got %v", entry)
	}
}

// TestEngine_DecisionInvalid: event.authorized=false → ramo "invalid" executado.
// Ref: tasks.md §6.2.5.
func TestEngine_DecisionInvalid(t *testing.T) {
	var visited atomic.Value
	visited.Store("")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visited.Store(r.URL.Path)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := &flow.Flow{
		ID: 31,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("d", flow.NodeTypeDecision, nil),
			node("valid_node", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/valid", Method: "GET",
			}),
			node("invalid_node", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/invalid", Method: "GET",
			}),
		},
		Edges: []flow.FlowEdge{
			edge("s", "d"),
			edgeLabeled("d", "valid_node", "valid"),
			edgeLabeled("d", "invalid_node", "invalid"),
		},
	}

	logRepo := newFakeLogRepo()
	e := testEngine(f, logRepo)
	// AttendanceStatus != "authorized" → IsAuthorized() == false
	notAuth := "denied"
	evt := &domain.AttendanceEvent{EventKey: "k6", AttendanceStatus: &notAuth}
	e.TriggerForDevice("", evt, nil, testDevice())
	time.Sleep(200 * time.Millisecond)

	if visited.Load() != "/invalid" {
		t.Fatalf("esperado ramo /invalid; got %q", visited.Load())
	}
}

// sampleIdentityTerminalXML é um corpo mínimo válido de IdentityTerminal para o
// GET do device simulado (campos suficientes para o round-trip de showMode).
const sampleIdentityTerminalXML = `<?xml version="1.0" encoding="UTF-8"?>
<IdentityTerminal version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<camera>1</camera>
<fingerPrintModule>0</fingerPrintModule>
<faceAlgorithm>hisign</faceAlgorithm>
<saveCertifiedImage>true</saveCertifiedImage>
<readInfoOfCard>false</readInfoOfCard>
<workMode>normal</workMode>
<enableScreenOff>true</enableScreenOff>
<popUpPreviewWindow>false</popUpPreviewWindow>
<screenOffTimeout>600</screenOffTimeout>
<showMode>normal</showMode>
<previewShowTime>10</previewShowTime>
<standbyTimeout>30</standbyTimeout>
<advertisingDisplayType>full</advertisingDisplayType>
</IdentityTerminal>`

// sampleWeekPlanJSON monta um plano semanal de verificação com o modo dado em todos os slots.
func sampleWeekPlanJSON(mode string) []byte {
	slots := make([]map[string]any, 7)
	for i := range slots {
		slots[i] = map[string]any{"weekNo": i + 1, "verifyMode": mode, "enable": true}
	}
	doc := map[string]any{
		"VerifyWeekPlanCfg": map[string]any{"WeekPlanCfg": slots},
	}
	b, _ := json.Marshal(doc)
	return b
}

// xmlTagValue extrai o conteúdo de <tag>...</tag> de um corpo XML simples (teste).
func xmlTagValue(body, tag string) string {
	open, closeT := "<"+tag+">", "</"+tag+">"
	i := strings.Index(body, open)
	if i < 0 {
		return ""
	}
	i += len(open)
	j := strings.Index(body[i:], closeT)
	if j < 0 {
		return ""
	}
	return body[i : i+j]
}

// faceReaderTestServer simula os endpoints ISAPI tocados pelos nós de leitor facial
// e captura o verifyMode (PUT VerifyWeekPlanCfg) e o showMode (PUT IdentityTerminal).
func faceReaderTestServer(t *testing.T, gotVerifyMode, gotShowMode *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ISAPI/AccessControl/VerifyWeekPlanCfg/1" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			w.Write(sampleWeekPlanJSON("card")) //nolint:errcheck
		case r.URL.Path == "/ISAPI/AccessControl/VerifyWeekPlanCfg/1" && r.Method == http.MethodPut:
			var doc map[string]any
			_ = json.NewDecoder(r.Body).Decode(&doc)
			cfg, _ := doc["VerifyWeekPlanCfg"].(map[string]any)
			plans, _ := cfg["WeekPlanCfg"].([]any)
			if len(plans) > 0 {
				if m, ok := plans[0].(map[string]any); ok {
					*gotVerifyMode, _ = m["verifyMode"].(string)
				}
			}
			w.WriteHeader(200)
		case r.URL.Path == "/ISAPI/AccessControl/IdentityTerminal" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(sampleIdentityTerminalXML)) //nolint:errcheck
		case r.URL.Path == "/ISAPI/AccessControl/IdentityTerminal" && r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			*gotShowMode = xmlTagValue(string(body), "showMode")
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
}

// testEngineWithHik cria um Engine com HikClientFor apontando para o client dado.
func testEngineWithHik(activeFlow *flow.Flow, logRepo *fakeLogRepo, hikFor func(*domain.Device) (*hikvision.Client, error)) *flowengine.Engine {
	return flowengine.New(flowengine.Config{
		FlowRepo:     &fakeFlowRepo{activeFlow: activeFlow},
		LogRepo:      logRepo,
		BgImageRepo:  &fakeBgImageRepo{},
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		SSRFChecker:  noSSRF,
		HikClientFor: hikFor,
	})
}

// hikClientForServer constrói um *hikvision.Client apontado para o httptest server.
func hikClientForServer(srv *httptest.Server) func(*domain.Device) (*hikvision.Client, error) {
	host := srv.URL[len("http://"):]
	c := hikvision.NewWithHTTPClient(
		hikvision.DeviceConfig{Host: host, Username: "admin", Password: "test"},
		srv.Client(),
	)
	return func(*domain.Device) (*hikvision.Client, error) { return c, nil }
}

// TestEngine_CameraOn_EnablesFaceReader: nó camera_on (sem config) habilita o leitor
// facial — verifyMode default "cardOrFace" + showMode "normal" — e o fluxo completa.
// Contrato SOURCED: legacy/hik2go/examples/1-device/face-enable.php.
func TestEngine_CameraOn_EnablesFaceReader(t *testing.T) {
	var gotVerifyMode, gotShowMode string
	srv := faceReaderTestServer(t, &gotVerifyMode, &gotShowMode)
	defer srv.Close()

	f := &flow.Flow{
		ID: 40,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("c", flow.NodeTypeCameraOn, nil),
		},
		Edges: []flow.FlowEdge{edge("s", "c")},
	}
	logRepo := newFakeLogRepo()
	e := testEngineWithHik(f, logRepo, hikClientForServer(srv))
	triggerAndWait(e, testEvent("k7", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "completed" {
		t.Fatalf("esperado completed; got %v", entry)
	}
	if gotVerifyMode != "cardOrFace" {
		t.Fatalf("verifyMode aplicado = %q; want cardOrFace", gotVerifyMode)
	}
	if gotShowMode != "normal" {
		t.Fatalf("showMode aplicado = %q; want normal", gotShowMode)
	}
}

// TestEngine_CameraOff_ConfigurableVerifyMode: nó camera_off com verify_mode
// configurado no nó usa o valor do operador (não o default "card").
// Contrato SOURCED: legacy/hik2go/examples/1-device/face-disable.php.
func TestEngine_CameraOff_ConfigurableVerifyMode(t *testing.T) {
	var gotVerifyMode, gotShowMode string
	srv := faceReaderTestServer(t, &gotVerifyMode, &gotShowMode)
	defer srv.Close()

	f := &flow.Flow{
		ID: 41,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("c", flow.NodeTypeCameraOff, flow.CameraConfig{VerifyMode: "cardOrFpOrPw"}),
		},
		Edges: []flow.FlowEdge{edge("s", "c")},
	}
	logRepo := newFakeLogRepo()
	e := testEngineWithHik(f, logRepo, hikClientForServer(srv))
	triggerAndWait(e, testEvent("k7b", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "completed" {
		t.Fatalf("esperado completed; got %v", entry)
	}
	if gotVerifyMode != "cardOrFpOrPw" {
		t.Fatalf("verifyMode aplicado = %q; want cardOrFpOrPw (override do nó)", gotVerifyMode)
	}
}

// TestEngine_BlockedNode_Message: nó send_message → circuit-break com BLOCKED_API.
// Ref: tasks.md §6.2.6.
func TestEngine_BlockedNode_Message(t *testing.T) {
	f := &flow.Flow{
		ID: 41,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("m", flow.NodeTypeSendMessage, nil),
		},
		Edges: []flow.FlowEdge{edge("s", "m")},
	}
	logRepo := newFakeLogRepo()
	e := testEngine(f, logRepo)
	triggerAndWait(e, testEvent("k8", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "circuit_break" {
		t.Fatalf("esperado circuit_break; got %v", entry)
	}
	if !strings.Contains(*entry.Error, "BLOCKED_API") {
		t.Fatalf("esperado BLOCKED_API no erro; got %q", *entry.Error)
	}
}

// TestEngine_ConcurrentExecutions: dois disparos simultâneos → execuções independentes.
// Ref: tasks.md §6.2.7.
func TestEngine_ConcurrentExecutions(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := &flow.Flow{
		ID: 50,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/ping", Method: "POST",
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}

	logRepo := newFakeLogRepo()
	e := flowengine.New(flowengine.Config{
		FlowRepo:      &fakeFlowRepo{activeFlow: f},
		LogRepo:       logRepo,
		BgImageRepo:   &fakeBgImageRepo{},
		HTTPClient:    &http.Client{},
		SemaphoreSize: 5,
		SSRFChecker:   noSSRF, // httptest usa loopback
	})

	// Disparar 2 execuções com event_keys distintos (idempotência não deve suprimir).
	e.TriggerForDevice("", testEvent("ka", "authorized"), nil, testDevice())
	e.TriggerForDevice("", testEvent("kb", "authorized"), nil, testDevice())
	time.Sleep(500 * time.Millisecond)

	if callCount.Load() != 2 {
		t.Fatalf("esperado 2 chamadas HTTP; got %d", callCount.Load())
	}
	if logRepo.count() != 2 {
		t.Fatalf("esperado 2 log entries; got %d", logRepo.count())
	}
}

// TestEngine_SSRFBlocked: URL apontando para 192.168.x.x → circuit-break.
// Ref: tasks.md §6.2.8, §3.3.3 (SEC-SSRF).
func TestEngine_SSRFBlocked(t *testing.T) {
	f := &flow.Flow{
		ID: 60,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL:    "http://192.168.1.100/api",
				Method: "POST",
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}
	logRepo := newFakeLogRepo()
	// Usar Engine com SSRF guard real (não noSSRF).
	e := testEngineWithSSRF(f, logRepo)
	triggerAndWait(e, testEvent("k9", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "circuit_break" {
		t.Fatalf("esperado circuit_break; got %v", entry)
	}
	if !strings.Contains(*entry.Error, "SSRF bloqueado") {
		t.Fatalf("esperado 'SSRF bloqueado' no erro; got %q", *entry.Error)
	}
}

// TestEngine_IdempotencyGuard: segundo evento com mesmo event_key após completed → skip silencioso.
// Ref: tasks.md §6.2.9, §3.7 (Constituição §II).
func TestEngine_IdempotencyGuard(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := &flow.Flow{
		ID: 70,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/ping", Method: "GET",
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}

	logRepo := newFakeLogRepo()
	e := flowengine.New(flowengine.Config{
		FlowRepo:    &fakeFlowRepo{activeFlow: f},
		LogRepo:     logRepo,
		BgImageRepo: &fakeBgImageRepo{},
		HTTPClient:  &http.Client{},
		SSRFChecker: noSSRF, // httptest usa loopback
	})

	evt := testEvent("k10", "authorized")

	// Primeira execução: deve processar.
	e.TriggerForDevice("", evt, nil, testDevice())
	time.Sleep(200 * time.Millisecond)

	if callCount.Load() != 1 {
		t.Fatalf("primeira execução: esperado 1 chamada HTTP; got %d", callCount.Load())
	}

	// Segunda execução com o MESMO event_key: deve ser ignorada (skip idempotente).
	e.TriggerForDevice("", evt, nil, testDevice())
	time.Sleep(200 * time.Millisecond)

	if callCount.Load() != 1 {
		t.Fatalf("segunda execução deveria ser ignorada; got %d chamadas HTTP", callCount.Load())
	}
	// Apenas 1 log entry (completed); a segunda deve ter sido descartada silenciosamente.
	if logRepo.count() != 1 {
		t.Fatalf("esperado 1 log entry; got %d", logRepo.count())
	}
}

// TestSSRFLoopback: loopback (127.x) deve ser bloqueado.
func TestSSRFLoopback(t *testing.T) {
	f := &flow.Flow{
		ID: 80,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL:    "http://127.0.0.1:9999/secret",
				Method: "GET",
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}
	logRepo := newFakeLogRepo()
	// Usar Engine com SSRF guard real.
	e := testEngineWithSSRF(f, logRepo)
	triggerAndWait(e, testEvent("k11", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "circuit_break" {
		t.Fatalf("esperado circuit_break para loopback; got %v", entry)
	}
	if !strings.Contains(*entry.Error, "SSRF bloqueado") {
		t.Fatalf("esperado 'SSRF bloqueado'; got %q", *entry.Error)
	}
}

// TestSSRFLinkLocal: link-local (169.254.x) deve ser bloqueado.
func TestSSRFLinkLocal(t *testing.T) {
	f := &flow.Flow{
		ID: 81,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL:    "http://169.254.169.254/metadata",
				Method: "GET",
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}
	logRepo := newFakeLogRepo()
	// Usar Engine com SSRF guard real.
	e := testEngineWithSSRF(f, logRepo)
	triggerAndWait(e, testEvent("k12", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "circuit_break" {
		t.Fatalf("esperado circuit_break para link-local; got %v", entry)
	}
	if !strings.Contains(*entry.Error, "SSRF bloqueado") {
		t.Fatalf("esperado 'SSRF bloqueado'; got %q", *entry.Error)
	}
}

// TestWait_OutOfBounds: duration_seconds=0 → circuit-break.
func TestWait_OutOfBounds(t *testing.T) {
	f := &flow.Flow{
		ID: 90,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("w", flow.NodeTypeWait, flow.WaitConfig{DurationSeconds: 0}),
		},
		Edges: []flow.FlowEdge{edge("s", "w")},
	}
	logRepo := newFakeLogRepo()
	e := testEngine(f, logRepo)
	triggerAndWait(e, testEvent("k13", "authorized"), testDevice())

	entry := logRepo.lastEntry()
	if entry == nil || entry.Status != "circuit_break" {
		t.Fatalf("esperado circuit_break para wait=0; got %v", entry)
	}
}

// TestConcurrencyBound: semáforo de 1 deve descartar segundo evento simultâneo.
func TestConcurrencyBound(t *testing.T) {
	started := make(chan struct{})
	hold := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{} // sinaliza que a goroutine começou
		<-hold                // aguarda liberação
		w.WriteHeader(200)
	}))
	defer srv.Close()

	f := &flow.Flow{
		ID: 100,
		Nodes: []flow.FlowNode{
			node("s", flow.NodeTypeStart, nil),
			node("h", flow.NodeTypeHTTPSCall, flow.HTTPSCallConfig{
				URL: srv.URL + "/block", Method: "GET", TimeoutSeconds: 30,
			}),
		},
		Edges: []flow.FlowEdge{edge("s", "h")},
	}

	logRepo := newFakeLogRepo()
	e := flowengine.New(flowengine.Config{
		FlowRepo:      &fakeFlowRepo{activeFlow: f},
		LogRepo:       logRepo,
		BgImageRepo:   &fakeBgImageRepo{},
		HTTPClient:    &http.Client{},
		SemaphoreSize: 1, // apenas 1 goroutine simultânea
		SSRFChecker:   noSSRF, // httptest usa loopback
	})

	// Disparar primeiro evento — vai bloquear no servidor.
	e.TriggerForDevice("", testEvent("concA", "authorized"), nil, testDevice())

	// Aguardar a goroutine chegar no handler.
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine não iniciou a tempo")
	}

	// Segundo evento: semáforo cheio → deve ser descartado silenciosamente.
	e.TriggerForDevice("", testEvent("concB", "authorized"), nil, testDevice())
	time.Sleep(50 * time.Millisecond)

	// Liberar o primeiro.
	close(hold)
	time.Sleep(200 * time.Millisecond)

	// Apenas 1 log entry (do primeiro evento).
	if logRepo.count() != 1 {
		t.Fatalf("esperado 1 log entry (segundo descartado); got %d", logRepo.count())
	}
	_ = fmt.Sprintf("") // evitar import não-usado
}
