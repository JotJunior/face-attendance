package httphandler

// Testes dos handlers de fluxo admin — uso de mocks de repositório.
// Ref: tasks.md §4.1.4.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// --- Mocks ---

type mockFlowRepo struct {
	flows    map[int64]*flow.Flow
	nextID   int64
	setDevID func(ctx context.Context, id int64, deviceID *int64) error
}

func newMockFlowRepo() *mockFlowRepo {
	return &mockFlowRepo{flows: make(map[int64]*flow.Flow), nextID: 1}
}

func (m *mockFlowRepo) Create(_ context.Context, f *flow.Flow) (*flow.Flow, error) {
	f.ID = m.nextID
	m.nextID++
	f.CreatedAt = time.Now()
	f.UpdatedAt = time.Now()
	if f.Status == "" {
		f.Status = "inactive"
	}
	cp := *f
	m.flows[cp.ID] = &cp
	return &cp, nil
}

func (m *mockFlowRepo) FindByID(_ context.Context, id int64) (*flow.Flow, error) {
	f, ok := m.flows[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	cp := *f
	return &cp, nil
}

func (m *mockFlowRepo) FindAll(_ context.Context) ([]*flow.Flow, error) {
	out := make([]*flow.Flow, 0, len(m.flows))
	for _, f := range m.flows {
		cp := *f
		out = append(out, &cp)
	}
	return out, nil
}

func (m *mockFlowRepo) Update(_ context.Context, f *flow.Flow) (*flow.Flow, error) {
	existing, ok := m.flows[f.ID]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	existing.Name = f.Name
	existing.Nodes = f.Nodes
	existing.Edges = f.Edges
	existing.SealedConfig = f.SealedConfig
	existing.UpdatedAt = time.Now()
	cp := *existing
	return &cp, nil
}

func (m *mockFlowRepo) SetStatus(_ context.Context, id int64, status string) error {
	f, ok := m.flows[id]
	if !ok {
		return pgx.ErrNoRows
	}
	f.Status = status
	return nil
}

func (m *mockFlowRepo) SetDeviceID(ctx context.Context, id int64, deviceID *int64) error {
	if m.setDevID != nil {
		return m.setDevID(ctx, id, deviceID)
	}
	f, ok := m.flows[id]
	if !ok {
		return pgx.ErrNoRows
	}
	f.DeviceID = deviceID
	return nil
}

func (m *mockFlowRepo) Delete(_ context.Context, id int64) error {
	delete(m.flows, id)
	return nil
}

type mockFlowLogRepo struct{}

func (m *mockFlowLogRepo) FindByFlowID(_ context.Context, _ int64, _, _ int) ([]*repository.FlowExecutionLog, error) {
	return []*repository.FlowExecutionLog{}, nil
}

type mockDeviceAdminRepo struct {
	device *domain.Device
}

func (m *mockDeviceAdminRepo) CountDevicesByActivity(_ context.Context, _ int) (int, int, error) {
	return 0, 0, nil
}

func (m *mockDeviceAdminRepo) ListDevicesAll(_ context.Context) ([]domain.Device, error) {
	return nil, nil
}

func (m *mockDeviceAdminRepo) GetDeviceByID(_ context.Context, id int64) (*domain.Device, error) {
	if m.device != nil && m.device.ID == id {
		return m.device, nil
	}
	return nil, pgx.ErrNoRows
}

func (m *mockDeviceAdminRepo) DeleteDevice(_ context.Context, _ int64) error {
	return nil
}

// --- Helpers ---

func newTestFlowConfig() AdminFlowsConfig {
	return AdminFlowsConfig{
		FlowRepo:   newMockFlowRepo(),
		DeviceRepo: &mockDeviceAdminRepo{},
		LogRepo:    &mockFlowLogRepo{},
		Logger:     logging.New(),
	}
}

func withSession(r *http.Request) *http.Request {
	// Não há middleware de sessão nos testes unitários; testamos os handlers diretamente.
	return r
}

// --- Testes ---

// TestAdminFlowsRootHandler_ListEmpty verifica que GET /admin/api/flows retorna lista vazia.
func TestAdminFlowsRootHandler_ListEmpty(t *testing.T) {
	cfg := newTestFlowConfig()
	h := AdminFlowsRootHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/flows", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	flows, ok := resp["flows"]
	if !ok {
		t.Fatal("campo 'flows' ausente na resposta")
	}
	list, ok := flows.([]interface{})
	if !ok || len(list) != 0 {
		t.Fatalf("esperado lista vazia, obtido: %v", flows)
	}
}

// TestAdminFlowsRootHandler_Create verifica que POST /admin/api/flows cria um fluxo.
func TestAdminFlowsRootHandler_Create(t *testing.T) {
	cfg := newTestFlowConfig()
	h := AdminFlowsRootHandler(cfg)

	body := `{"name":"Fluxo Teste"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/flows", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperado 201, obtido %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if resp["name"] != "Fluxo Teste" {
		t.Errorf("nome inesperado: %v", resp["name"])
	}
	if resp["status"] != "inactive" {
		t.Errorf("status inesperado: %v", resp["status"])
	}
}

// TestAdminFlowsRootHandler_Create_EmptyName verifica que nome vazio retorna 400.
func TestAdminFlowsRootHandler_Create_EmptyName(t *testing.T) {
	cfg := newTestFlowConfig()
	h := AdminFlowsRootHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/flows", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, obtido %d", rec.Code)
	}
}

// TestAdminFlowsSubRouter_GetNotFound verifica 404 para fluxo inexistente.
func TestAdminFlowsSubRouter_GetNotFound(t *testing.T) {
	cfg := newTestFlowConfig()
	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/flows/999", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d", rec.Code)
	}
}

// TestAdminFlowsSubRouter_UpdateValidatesFlow verifica que PUT retorna 422 se o fluxo é inválido.
func TestAdminFlowsSubRouter_UpdateValidatesFlow(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	// Criar fluxo de teste.
	f := &flow.Flow{Name: "Teste", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	// Fluxo sem nó start — inválido.
	updateBody := map[string]interface{}{
		"name":  "Teste",
		"nodes": []map[string]interface{}{},
		"edges": []map[string]interface{}{},
	}
	bodyJSON, _ := json.Marshal(updateBody)
	req := httptest.NewRequest(http.MethodPut,
		"/admin/api/flows/"+intToStr(created.ID), bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtido %d: %s", rec.Code, rec.Body.String())
	}
	var resp validationErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta 422 não é JSON: %v", err)
	}
	if len(resp.Errors) == 0 {
		t.Fatal("esperado pelo menos um erro de validação")
	}
}

// TestAdminFlowsSubRouter_Activate_InvalidFlow verifica 422 ao ativar fluxo inválido.
func TestAdminFlowsSubRouter_Activate_InvalidFlow(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	// Criar fluxo sem nós.
	f := &flow.Flow{Name: "Vazio", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodPut,
		"/admin/api/flows/"+intToStr(created.ID)+"/activate", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("esperado 422, obtido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminFlowsSubRouter_SetDevice_DeviceNotFound verifica 404 ao vincular device inexistente.
func TestAdminFlowsSubRouter_SetDevice_DeviceNotFound(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	f := &flow.Flow{Name: "F", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	body := `{"device_id":42}` // device 42 não existe no mock
	req := httptest.NewRequest(http.MethodPut,
		"/admin/api/flows/"+intToStr(created.ID)+"/device", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, obtido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminFlowsSubRouter_SetDevice_Conflict verifica 409 ao vincular device já vinculado.
func TestAdminFlowsSubRouter_SetDevice_Conflict(t *testing.T) {
	devRepo := &mockDeviceAdminRepo{device: &domain.Device{ID: 1}}
	repo := newMockFlowRepo()
	repo.setDevID = func(_ context.Context, _ int64, _ *int64) error {
		return repository.ErrFlowDeviceConflict
	}
	cfg := AdminFlowsConfig{
		FlowRepo:   repo,
		DeviceRepo: devRepo,
		LogRepo:    &mockFlowLogRepo{},
		Logger:     logging.New(),
	}

	f := &flow.Flow{Name: "F2", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	body := `{"device_id":1}`
	req := httptest.NewRequest(http.MethodPut,
		"/admin/api/flows/"+intToStr(created.ID)+"/device", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperado 409, obtido %d: %s", rec.Code, rec.Body.String())
	}
}

// TestAdminFlowsSubRouter_Delete verifica exclusão de fluxo.
func TestAdminFlowsSubRouter_Delete(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	f := &flow.Flow{Name: "Del", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodDelete,
		"/admin/api/flows/"+intToStr(created.ID), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("esperado 204, obtido %d", rec.Code)
	}
	// Confirmar que foi removido.
	if _, ok := repo.flows[created.ID]; ok {
		t.Error("fluxo ainda presente após DELETE")
	}
}

// TestAdminFlowsSubRouter_SealedConfig verifica mascaramento de headers selados no GET.
func TestAdminFlowsSubRouter_SealedConfig_Masked(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	// Criar fluxo com https_call contendo header selado.
	nodeConfig, _ := json.Marshal(map[string]interface{}{
		"url":    "https://example.com",
		"method": "POST",
		"headers": map[string]string{
			"Authorization": flowSealedSentinel,
		},
	})
	f := &flow.Flow{
		Name:   "Sealed",
		Status: "inactive",
		Nodes: []flow.FlowNode{
			{ID: "n1", Type: flow.NodeTypeHTTPSCall, Config: nodeConfig},
		},
		Edges:        []flow.FlowEdge{},
		SealedConfig: map[string]string{"n1.Authorization": "encryptedbase64"},
	}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/api/flows/"+intToStr(created.ID), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d: %s", rec.Code, rec.Body.String())
	}

	// Verificar que a resposta não contém o sentinela "__sealed__" nem o "encryptedbase64".
	body := rec.Body.String()
	if strings.Contains(body, flowSealedSentinel) {
		t.Errorf("resposta não deve conter sentinela %q: %s", flowSealedSentinel, body)
	}
	if strings.Contains(body, "encryptedbase64") {
		t.Errorf("resposta não deve conter valor cifrado: %s", body)
	}
	// Verificar que o mascaramento está presente.
	if !strings.Contains(body, secretMasked) {
		t.Errorf("resposta deve conter valor mascarado %q: %s", secretMasked, body)
	}
	// Verificar que sealed_config não está exposto na resposta.
	if strings.Contains(body, "sealed_config") {
		t.Errorf("resposta não deve expor sealed_config: %s", body)
	}
}

// TestAdminFlowsSubRouter_Logs verifica GET de logs de execução.
func TestAdminFlowsSubRouter_Logs(t *testing.T) {
	cfg := newTestFlowConfig()
	repo := cfg.FlowRepo.(*mockFlowRepo)

	f := &flow.Flow{Name: "Logs", Status: "inactive", Nodes: []flow.FlowNode{}, Edges: []flow.FlowEdge{}}
	created, _ := repo.Create(context.Background(), f)

	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodGet,
		"/admin/api/flows/"+intToStr(created.ID)+"/logs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusOK {
		t.Fatalf("esperado 200, obtido %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta não é JSON: %v", err)
	}
	if _, ok := resp["logs"]; !ok {
		t.Error("campo 'logs' ausente na resposta")
	}
}

// TestAdminFlowsSubRouter_MethodNotAllowed verifica 405 para método inválido.
func TestAdminFlowsSubRouter_MethodNotAllowed(t *testing.T) {
	cfg := newTestFlowConfig()
	h := adminFlowsSubRouter(cfg)

	req := httptest.NewRequest(http.MethodPost, "/admin/api/flows/1/activate", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withSession(req))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("esperado 405, obtido %d", rec.Code)
	}
}

// --- helpers de teste ---

func intToStr(id int64) string {
	return strings.TrimSpace(string([]byte(itoa(id))))
}

func itoa(n int64) string {
	b := make([]byte, 0, 20)
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if negative {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
