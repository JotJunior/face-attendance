//go:build integration
// +build integration

// Testes de integração para repositórios de fluxo.
// Exigem PostgreSQL ativo (execute make docker-up antes).
// Execute com: go test -tags integration ./internal/repository/...
// Ref: docs/specs/face-flow/tasks.md §6.3
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// cleanupFlows trunca tabelas de fluxo em ordem de dependência (FK).
func cleanupFlows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"TRUNCATE flow_execution_logs, background_images, flows RESTART IDENTITY CASCADE")
	if err != nil {
		t.Logf("cleanupFlows warning: %v", err)
	}
}

// insertTestDevice insere um device auxiliar e retorna seu ID (necessário para FKs).
func insertTestDevice(t *testing.T, pool *pgxpool.Pool, identifier string) int64 {
	t.Helper()
	devRepo := repository.NewDeviceRepository(pool)
	ip := "192.168.1.200"
	mac := "CC:DD:EE:FF:AA:BB"
	if err := devRepo.Upsert(context.Background(), domain.Device{
		DeviceIdentifier: identifier,
		IPAddress:        &ip,
		MACAddress:       &mac,
	}); err != nil {
		t.Fatalf("insertTestDevice: %v", err)
	}
	dev, err := devRepo.FindByIdentifier(context.Background(), identifier)
	if err != nil || dev == nil {
		t.Fatalf("insertTestDevice FindByIdentifier: err=%v dev=%v", err, dev)
	}
	return dev.ID
}

// ---------- FlowRepository ----------

// TestFlowRepository_CreateFind verifica criação e lookup de fluxo por ID.
// Ref: tasks.md §6.3.1
func TestFlowRepository_CreateFind(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	repo := repository.NewPgxFlowRepository(pool)
	ctx := context.Background()

	f := &flow.Flow{
		Name:   "Fluxo de Teste",
		Status: "inactive",
		Nodes: []flow.FlowNode{
			{ID: "node-start", Type: flow.NodeTypeStart},
		},
		Edges: []flow.FlowEdge{},
	}

	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("ID deve ser preenchido após Create")
	}
	if created.Name != "Fluxo de Teste" {
		t.Errorf("Name = %q, want 'Fluxo de Teste'", created.Name)
	}
	if created.Status != "inactive" {
		t.Errorf("Status = %q, want 'inactive'", created.Status)
	}
	if len(created.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(created.Nodes))
	}

	found, err := repo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Name != created.Name {
		t.Errorf("FindByID Name = %q, want %q", found.Name, created.Name)
	}
	if len(found.Nodes) != 1 || found.Nodes[0].ID != "node-start" {
		t.Errorf("Nodes round-trip falhou: %+v", found.Nodes)
	}
}

// TestFlowRepository_FindByID_NotFound verifica que pgx.ErrNoRows é retornado para ID inexistente.
func TestFlowRepository_FindByID_NotFound(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	repo := repository.NewPgxFlowRepository(pool)

	_, err := repo.FindByID(context.Background(), 999999)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("FindByID ID inexistente: got err=%v, want pgx.ErrNoRows", err)
	}
}

// TestFlowRepository_UniqueDeviceID verifica que dois fluxos não podem ter o mesmo device_id.
// Ref: tasks.md §6.3.1
func TestFlowRepository_UniqueDeviceID(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	repo := repository.NewPgxFlowRepository(pool)
	ctx := context.Background()

	devID := insertTestDevice(t, pool, "EE:FF:00:11:22:33")

	// Criar primeiro fluxo e vincular device
	f1, err := repo.Create(ctx, &flow.Flow{Name: "Fluxo A", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}})
	if err != nil {
		t.Fatalf("Create f1: %v", err)
	}
	if err := repo.SetDeviceID(ctx, f1.ID, &devID); err != nil {
		t.Fatalf("SetDeviceID f1: %v", err)
	}

	// Tentar vincular o mesmo device a um segundo fluxo deve retornar ErrFlowDeviceConflict
	f2, err := repo.Create(ctx, &flow.Flow{Name: "Fluxo B", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}})
	if err != nil {
		t.Fatalf("Create f2: %v", err)
	}
	if err := repo.SetDeviceID(ctx, f2.ID, &devID); !errors.Is(err, repository.ErrFlowDeviceConflict) {
		t.Errorf("SetDeviceID conflito: got err=%v, want ErrFlowDeviceConflict", err)
	}
}

// TestFlowRepository_SetDeviceID_Nil verifica que nil desvincula o device corretamente.
func TestFlowRepository_SetDeviceID_Nil(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	repo := repository.NewPgxFlowRepository(pool)
	ctx := context.Background()

	devID := insertTestDevice(t, pool, "AA:11:22:33:44:55")

	f, err := repo.Create(ctx, &flow.Flow{Name: "Fluxo C", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Vincular
	if err := repo.SetDeviceID(ctx, f.ID, &devID); err != nil {
		t.Fatalf("SetDeviceID (vincular): %v", err)
	}

	// Desvincular (nil)
	if err := repo.SetDeviceID(ctx, f.ID, nil); err != nil {
		t.Fatalf("SetDeviceID (desvincular): %v", err)
	}

	found, err := repo.FindByID(ctx, f.ID)
	if err != nil {
		t.Fatalf("FindByID após desvincular: %v", err)
	}
	if found.DeviceID != nil {
		t.Errorf("DeviceID deveria ser nil após desvincular; got %v", found.DeviceID)
	}
}

// TestFlowRepository_FindActiveByDeviceID verifica que apenas fluxos ativos são retornados.
// Ref: tasks.md §6.3.2
func TestFlowRepository_FindActiveByDeviceID(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	repo := repository.NewPgxFlowRepository(pool)
	ctx := context.Background()

	devID := insertTestDevice(t, pool, "BB:22:33:44:55:66")

	// Fluxo inativo vinculado ao device
	fInativo, err := repo.Create(ctx, &flow.Flow{Name: "Inativo", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}})
	if err != nil {
		t.Fatalf("Create inativo: %v", err)
	}
	if err := repo.SetDeviceID(ctx, fInativo.ID, &devID); err != nil {
		t.Fatalf("SetDeviceID inativo: %v", err)
	}
	// Status padrão é 'inactive' — não precisa de SetStatus

	// Fluxo ativo não vinculado: FindActiveByDeviceID não deve retorná-lo
	result, err := repo.FindActiveByDeviceID(ctx, devID)
	if err != nil {
		t.Fatalf("FindActiveByDeviceID (inativo): %v", err)
	}
	if result != nil {
		t.Errorf("esperava nil para fluxo inativo, got %+v", result)
	}

	// Ativar o fluxo
	if err := repo.SetStatus(ctx, fInativo.ID, "active"); err != nil {
		t.Fatalf("SetStatus active: %v", err)
	}

	result, err = repo.FindActiveByDeviceID(ctx, devID)
	if err != nil {
		t.Fatalf("FindActiveByDeviceID (ativo): %v", err)
	}
	if result == nil {
		t.Fatal("esperava fluxo ativo, got nil")
	}
	if result.ID != fInativo.ID {
		t.Errorf("ID do fluxo ativo = %d, want %d", result.ID, fInativo.ID)
	}
	if result.Status != "active" {
		t.Errorf("Status = %q, want 'active'", result.Status)
	}
}

// ---------- FlowExecutionLogRepository ----------

// TestFlowExecutionLog_IdempotentEventKey verifica que segundo insert com mesmo
// event_key retorna nil (idempotência — Constituição §II, spec FR-023).
// Ref: tasks.md §6.3.3
func TestFlowExecutionLog_IdempotentEventKey(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	ctx := context.Background()

	// Criar device e fluxo para satisfazer FKs
	devID := insertTestDevice(t, pool, "CC:33:44:55:66:77")
	flowRepo := repository.NewPgxFlowRepository(pool)
	f, err := flowRepo.Create(ctx, &flow.Flow{
		Name:  "Fluxo Log",
		Nodes: []flow.FlowNode{},
		Edges: []flow.FlowEdge{},
	})
	if err != nil {
		t.Fatalf("Create flow para log: %v", err)
	}

	logRepo := repository.NewPgxFlowExecutionLogRepository(pool)
	now := time.Now().UTC()
	errMsg := "circuito aberto"
	failedNode := "node-wait"

	log1 := &repository.FlowExecutionLog{
		FlowID:       f.ID,
		DeviceID:     devID,
		EventKey:     "test-event-idempotency-001",
		Status:       "circuit_break",
		FailedNodeID: &failedNode,
		Error:        &errMsg,
		StartedAt:    now,
		FinishedAt:   now.Add(time.Second),
	}

	// Primeiro insert deve ter sucesso
	if err := logRepo.Create(ctx, log1); err != nil {
		t.Fatalf("Create (primeiro): %v", err)
	}

	// Segundo insert com mesmo event_key deve retornar nil (idempotência)
	if err := logRepo.Create(ctx, log1); err != nil {
		t.Errorf("Create (segundo, mesmo event_key): esperava nil, got %v", err)
	}

	// Verificar que existe apenas 1 linha para o event_key
	rows, err := pool.Query(ctx,
		"SELECT COUNT(*) FROM flow_execution_logs WHERE event_key = $1",
		"test-event-idempotency-001")
	if err != nil {
		t.Fatalf("COUNT query: %v", err)
	}
	defer rows.Close()
	var count int
	rows.Next()
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("Scan COUNT: %v", err)
	}
	if count != 1 {
		t.Errorf("COUNT(event_key) = %d, want 1 (idempotência violada)", count)
	}
}

// TestFlowExecutionLog_FindByFlowID verifica paginação e ordenação dos logs.
func TestFlowExecutionLog_FindByFlowID(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	ctx := context.Background()

	devID := insertTestDevice(t, pool, "DD:44:55:66:77:88")
	flowRepo := repository.NewPgxFlowRepository(pool)
	f, err := flowRepo.Create(ctx, &flow.Flow{
		Name:  "Fluxo Logs Multi",
		Nodes: []flow.FlowNode{},
		Edges: []flow.FlowEdge{},
	})
	if err != nil {
		t.Fatalf("Create flow: %v", err)
	}

	logRepo := repository.NewPgxFlowExecutionLogRepository(pool)
	base := time.Now().UTC()

	// Inserir 3 logs com started_at crescentes e event_keys distintos
	for i := 0; i < 3; i++ {
		key := "find-key-" + string(rune('A'+i))
		status := "completed"
		if i == 1 {
			status = "circuit_break"
		}
		if err := logRepo.Create(ctx, &repository.FlowExecutionLog{
			FlowID:     f.ID,
			DeviceID:   devID,
			EventKey:   key,
			Status:     status,
			StartedAt:  base.Add(time.Duration(i) * time.Minute),
			FinishedAt: base.Add(time.Duration(i)*time.Minute + 5*time.Second),
		}); err != nil {
			t.Fatalf("Create log %d: %v", i, err)
		}
	}

	// FindByFlowID com limit=2 deve retornar os 2 mais recentes (DESC)
	logs, err := logRepo.FindByFlowID(ctx, f.ID, 2, 0)
	if err != nil {
		t.Fatalf("FindByFlowID: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("len(logs) = %d, want 2", len(logs))
	}
	// Resultado ordenado DESC: o mais recente (i=2, event_key=C) vem primeiro
	if logs[0].EventKey != "find-key-C" {
		t.Errorf("logs[0].EventKey = %q, want 'find-key-C'", logs[0].EventKey)
	}
	if logs[1].EventKey != "find-key-B" {
		t.Errorf("logs[1].EventKey = %q, want 'find-key-B'", logs[1].EventKey)
	}

	// Offset=2 deve retornar o mais antigo (i=0, event_key=A)
	logs2, err := logRepo.FindByFlowID(ctx, f.ID, 10, 2)
	if err != nil {
		t.Fatalf("FindByFlowID offset=2: %v", err)
	}
	if len(logs2) != 1 {
		t.Fatalf("len(logs2) = %d, want 1", len(logs2))
	}
	if logs2[0].EventKey != "find-key-A" {
		t.Errorf("logs2[0].EventKey = %q, want 'find-key-A'", logs2[0].EventKey)
	}
}
