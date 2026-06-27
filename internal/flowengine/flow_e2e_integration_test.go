//go:build integration
// +build integration

// Teste E2E de integração do motor de fluxo contra Postgres REAL.
// Exige PostgreSQL ativo com as migrations aplicadas (make docker-up + make migrate-up).
// Execute com: go test -tags integration ./internal/flowengine/...
//
// Valida o caminho completo da feature face-flow SEM o device físico:
//   fluxo persistido (repos pgx reais) → engine lê snapshot → executa HTTPS →
//   nó de decisão ramifica pelo resultado (200-204?) → grava FlowExecutionLog →
//   idempotência por event_key.
//
// As partes que tocam o device (camera/background ISAPI) e o gatilho real
// (reconhecimento facial) NÃO são exercitadas aqui — exigem hardware/rosto real.
package flowengine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/flowengine"
	"github.com/jotjunior/face-attendance/internal/repository"
)

func e2ePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://presenca:presenca_dev@localhost:5432/presenca_facial?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func e2eTruncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"TRUNCATE flow_execution_logs, background_images, flows, attendance_events, members, devices RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// e2eGateDecisionFlow: start → gate(https) → decision → valid_node(https) | invalid_node(https).
func e2eGateDecisionFlow(base string) *flow.Flow {
	mk := func(id, path string) flow.FlowNode {
		cfg := flow.HTTPSCallConfig{URL: base + path, Method: "GET"}
		b, _ := json.Marshal(cfg)
		return flow.FlowNode{ID: id, Type: flow.NodeTypeHTTPSCall, Config: b}
	}
	return &flow.Flow{
		Name:   "E2E gate→decisão",
		Status: "inactive",
		Nodes: []flow.FlowNode{
			{ID: "s", Type: flow.NodeTypeStart},
			mk("gate", "/gate"),
			{ID: "d", Type: flow.NodeTypeDecision},
			mk("valid_node", "/valid"),
			mk("invalid_node", "/invalid"),
		},
		Edges: []flow.FlowEdge{
			{From: "s", To: "gate"},
			{From: "gate", To: "d"},
			{From: "d", To: "valid_node", Label: "valid"},
			{From: "d", To: "invalid_node", Label: "invalid"},
		},
	}
}

// setupActiveFlow cria device + fluxo gate→decisão vinculado e ativo; retorna (flowID, device).
func setupActiveFlow(t *testing.T, pool *pgxpool.Pool, base, devIdent string) (int64, *domain.Device) {
	t.Helper()
	ctx := context.Background()

	devRepo := repository.NewDeviceRepository(pool)
	ip, mac := "192.168.1.210", "AA:BB:CC:DD:EE:FF"
	if err := devRepo.Upsert(ctx, domain.Device{DeviceIdentifier: devIdent, IPAddress: &ip, MACAddress: &mac}); err != nil {
		t.Fatalf("device upsert: %v", err)
	}
	dev, err := devRepo.FindByIdentifier(ctx, devIdent)
	if err != nil || dev == nil {
		t.Fatalf("find device: %v", err)
	}

	flowRepo := repository.NewPgxFlowRepository(pool)
	created, err := flowRepo.Create(ctx, e2eGateDecisionFlow(base))
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	if err := flowRepo.SetDeviceID(ctx, created.ID, &dev.ID); err != nil {
		t.Fatalf("bind device: %v", err)
	}
	if err := flowRepo.SetStatus(ctx, created.ID, "active"); err != nil {
		t.Fatalf("activate flow: %v", err)
	}
	return created.ID, dev
}

func e2eEngine(t *testing.T, pool *pgxpool.Pool) *flowengine.Engine {
	return flowengine.New(flowengine.Config{
		FlowRepo:    repository.NewPgxFlowRepository(pool),
		LogRepo:     repository.NewPgxFlowExecutionLogRepository(pool),
		BgImageRepo: repository.NewPgxBackgroundImageRepository(pool),
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
		SSRFChecker: func(string) error { return nil }, // httptest é loopback
	})
}

func e2eEvent(key string) *domain.AttendanceEvent {
	s := "authorized"
	return &domain.AttendanceEvent{EventKey: key, AttendanceStatus: &s}
}

// TestE2E_FlowGateDecision_Success: gate 200-204 → ramo "valid"; log "completed" no DB.
func TestE2E_FlowGateDecision_Success(t *testing.T) {
	pool := e2ePool(t)
	e2eTruncate(t, pool)

	var branch atomic.Value
	branch.Store("")
	var validHits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gate":
			w.WriteHeader(204)
		case "/valid":
			atomic.AddInt32(&validHits, 1)
			branch.Store("/valid")
			w.WriteHeader(200)
		case "/invalid":
			branch.Store("/invalid")
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	flowID, dev := setupActiveFlow(t, pool, srv.URL, "E2E-DEV-1")
	eng := e2eEngine(t, pool)

	eng.TriggerForDevice("", e2eEvent("e2e-success-1"), nil, dev)
	time.Sleep(300 * time.Millisecond)

	if branch.Load() != "/valid" {
		t.Fatalf("gate 204 → esperado ramo /valid; got %q", branch.Load())
	}

	// Log persistido no DB com status completed.
	logRepo := repository.NewPgxFlowExecutionLogRepository(pool)
	logs, err := logRepo.FindByFlowID(context.Background(), flowID, 10, 0)
	if err != nil {
		t.Fatalf("FindByFlowID: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("esperado 1 log de execução; got %d", len(logs))
	}
	if logs[0].Status != "completed" {
		t.Fatalf("status do log = %q; want completed", logs[0].Status)
	}

	// Idempotência: re-disparar o MESMO event_key não re-executa (side-effect once).
	eng.TriggerForDevice("", e2eEvent("e2e-success-1"), nil, dev)
	time.Sleep(300 * time.Millisecond)
	if h := atomic.LoadInt32(&validHits); h != 1 {
		t.Fatalf("idempotência: ramo /valid atingido %d vezes; want 1", h)
	}
	logs2, _ := logRepo.FindByFlowID(context.Background(), flowID, 10, 0)
	if len(logs2) != 1 {
		t.Fatalf("idempotência: esperado 1 log após re-disparo; got %d", len(logs2))
	}
}

// TestE2E_FlowGateDecision_Failure: gate NÃO-2xx → ramo "invalid", mesmo com face autorizada.
func TestE2E_FlowGateDecision_Failure(t *testing.T) {
	pool := e2ePool(t)
	e2eTruncate(t, pool)

	var branch atomic.Value
	branch.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gate":
			w.WriteHeader(500)
		case "/valid", "/invalid":
			branch.Store(r.URL.Path)
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	flowID, dev := setupActiveFlow(t, pool, srv.URL, "E2E-DEV-2")
	eng := e2eEngine(t, pool)

	eng.TriggerForDevice("", e2eEvent("e2e-fail-1"), nil, dev) // face autorizada
	time.Sleep(300 * time.Millisecond)

	if branch.Load() != "/invalid" {
		t.Fatalf("gate 500 + face autorizada → esperado /invalid (override do https); got %q", branch.Load())
	}
	logRepo := repository.NewPgxFlowExecutionLogRepository(pool)
	logs, _ := logRepo.FindByFlowID(context.Background(), flowID, 10, 0)
	if len(logs) != 1 || logs[0].Status != "completed" {
		t.Fatalf("esperado 1 log completed; got %+v", logs)
	}
}
