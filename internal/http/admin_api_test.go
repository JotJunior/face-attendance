package httphandler

// Testes de API admin (FASE 4 — tarefa 4.3).
// Cobre: stats 200 com 5 campos (4.3.1), stats 503 com DB inacessível (4.3.2),
// members paginação (4.3.3), members busca sem resultados = 200 vazio (4.3.4),
// limit clampeado a 200 (4.3.5), events com filtro from/to (4.3.6),
// ausência de CPF cru nas respostas (4.3.7), devices com status derivado (4.3.8).
// Todos os testes usam httptest.ResponseRecorder — headless, sem DB real.
// Ref: tasks.md §4.3, spec.md §FR-003/004/005/006/007, contracts/admin-api.md, CHK-A08/A15/A16.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// --- Fakes de repositório ---

// fakeMemberRepo implementa memberAdminRepo com dados configuráveis.
type fakeMemberRepo struct {
	countErr    error
	countResult int
	listResult  []domain.MemberView
	listErr     error
	nextCursor  int
	hasMore     bool
}

func (f *fakeMemberRepo) CountMembersWithSelfie(ctx context.Context) (int, error) {
	return f.countResult, f.countErr
}

func (f *fakeMemberRepo) ListMembersPaged(ctx context.Context, q string, cursor, limit int) ([]domain.MemberView, int, bool, error) {
	if f.listErr != nil {
		return nil, 0, false, f.listErr
	}
	return f.listResult, f.nextCursor, f.hasMore, nil
}

// fakeDeviceRepo implementa deviceAdminRepo com dados configuráveis.
type fakeDeviceRepo struct {
	countActiveErr   error
	active, inactive int
	listResult       []domain.Device
	listErr          error
	getResult        *domain.Device
	getErr           error
}

func (f *fakeDeviceRepo) CountDevicesByActivity(ctx context.Context, thresholdHours int) (int, int, error) {
	return f.active, f.inactive, f.countActiveErr
}

func (f *fakeDeviceRepo) ListDevicesAll(ctx context.Context) ([]domain.Device, error) {
	return f.listResult, f.listErr
}

func (f *fakeDeviceRepo) GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error) {
	return f.getResult, f.getErr
}

// fakeAttendanceRepo implementa attendanceAdminRepo com dados configuráveis.
type fakeAttendanceRepo struct {
	countResult int
	countErr    error
	listResult  []domain.EventView
	listErr     error
	nextCursor  domain.CursorEvt
	hasMore     bool
}

func (f *fakeAttendanceRepo) CountAttendanceSince(ctx context.Context, since time.Time) (int, error) {
	return f.countResult, f.countErr
}

func (f *fakeAttendanceRepo) ListEventsPaged(ctx context.Context, from, to time.Time, cursor domain.CursorEvt, limit int) ([]domain.EventView, domain.CursorEvt, bool, error) {
	if f.listErr != nil {
		return nil, domain.CursorEvt{}, false, f.listErr
	}
	return f.listResult, f.nextCursor, f.hasMore, nil
}

// --- Helpers ---

// newTestAdminAPICfg cria um AdminAPIConfig com fakes configuráveis.
func newTestAdminAPICfg(mRepo memberAdminRepo, dRepo deviceAdminRepo, aRepo attendanceAdminRepo) AdminAPIConfig {
	return AdminAPIConfig{
		MemberRepo:             mRepo,
		DeviceRepo:             dRepo,
		AttendanceRepo:         aRepo,
		DeviceOfflineThreshold: 24,
	}
}

// getJSON faz GET num handler e desserializa o corpo JSON no target.
func getJSON(t *testing.T, handler http.Handler, path string, target interface{}) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if target != nil && rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(target); err != nil {
			t.Fatalf("falha ao decodificar resposta JSON: %v (body: %s)", err, rr.Body.String())
		}
	}
	return rr
}

// --- Testes 4.3 ---

// 4.3.1: GET /admin/api/stats com DB ativo → 200 JSON com 5 campos corretos.
func TestAdminStats_200WithFiveFields(t *testing.T) {
	mRepo := &fakeMemberRepo{countResult: 42}
	dRepo := &fakeDeviceRepo{active: 3, inactive: 1}
	aRepo := &fakeAttendanceRepo{countResult: 17}

	handler := AdminStatsHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))

	var body map[string]interface{}
	rr := getJSON(t, handler, "/admin/api/stats", &body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	expectedFields := []string{
		"members_with_selfie",
		"devices_active",
		"devices_inactive",
		"attendance_last_24h",
		"device_offline_threshold_hours",
	}
	for _, f := range expectedFields {
		if _, ok := body[f]; !ok {
			t.Errorf("campo %q ausente na resposta de stats", f)
		}
	}

	if got := int(body["members_with_selfie"].(float64)); got != 42 {
		t.Errorf("members_with_selfie = %d, want 42", got)
	}
	if got := int(body["devices_active"].(float64)); got != 3 {
		t.Errorf("devices_active = %d, want 3", got)
	}
	if got := int(body["devices_inactive"].(float64)); got != 1 {
		t.Errorf("devices_inactive = %d, want 1", got)
	}
	if got := int(body["attendance_last_24h"].(float64)); got != 17 {
		t.Errorf("attendance_last_24h = %d, want 17", got)
	}
	if got := int(body["device_offline_threshold_hours"].(float64)); got != 24 {
		t.Errorf("device_offline_threshold_hours = %d, want 24", got)
	}
}

// 4.3.2: GET /admin/api/stats com DB inacessível → 503 JSON (CHK-A08).
func TestAdminStats_503WhenDBDown(t *testing.T) {
	mRepo := &fakeMemberRepo{countErr: errors.New("connection refused")}
	dRepo := &fakeDeviceRepo{}
	aRepo := &fakeAttendanceRepo{}

	handler := AdminStatsHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))
	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 quando DB inacessível", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Errorf("resposta 503 não é JSON válido: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Error("resposta 503 não contém campo 'error'")
	}
}

// 4.3.3: GET /admin/api/members?limit=50 → paginação com has_more e cursor.
func TestAdminMembers_Pagination(t *testing.T) {
	// Seed: 50 membros na página atual, mais páginas disponíveis
	members := make([]domain.MemberView, 50)
	for i := range members {
		cpf := fmt.Sprintf("%011d", i)
		members[i] = domain.MemberView{
			ID:                    int64(i + 1),
			Name:                  fmt.Sprintf("Membro %d", i+1),
			FederalDocumentMasked: domain.MaskCPF(cpf),
			Status:                "active",
			SyncStatus:            "synced",
		}
	}

	mRepo := &fakeMemberRepo{
		listResult: members,
		nextCursor: 50,
		hasMore:    true,
	}
	dRepo := &fakeDeviceRepo{}
	aRepo := &fakeAttendanceRepo{}

	handler := AdminMembersHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))

	var body struct {
		Members    []domain.MemberView `json:"members"`
		NextCursor *int                `json:"next_cursor"`
		HasMore    bool                `json:"has_more"`
	}
	rr := getJSON(t, handler, "/admin/api/members?limit=50", &body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if len(body.Members) != 50 {
		t.Errorf("members count = %d, want 50", len(body.Members))
	}
	if !body.HasMore {
		t.Error("has_more deveria ser true")
	}
	if body.NextCursor == nil {
		t.Error("next_cursor deveria ser não-nulo com has_more=true")
	}
	if body.NextCursor != nil && *body.NextCursor != 50 {
		t.Errorf("next_cursor = %d, want 50", *body.NextCursor)
	}

	// Segunda página: usar next_cursor
	rr2 := getJSON(t, handler, "/admin/api/members?cursor=50&limit=50", &body)
	if rr2.Code != http.StatusOK {
		t.Errorf("segunda página: status = %d, want 200", rr2.Code)
	}
}

// 4.3.4: GET /admin/api/members?q=naoexiste → 200 com array vazio (não 404).
func TestAdminMembers_EmptySearchReturns200(t *testing.T) {
	mRepo := &fakeMemberRepo{listResult: nil, hasMore: false}
	dRepo := &fakeDeviceRepo{}
	aRepo := &fakeAttendanceRepo{}

	handler := AdminMembersHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))

	var body struct {
		Members    []domain.MemberView `json:"members"`
		NextCursor *int                `json:"next_cursor"`
		HasMore    bool                `json:"has_more"`
	}
	rr := getJSON(t, handler, "/admin/api/members?q=teste_sem_resultado", &body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 para busca sem resultado", rr.Code)
	}
	if body.Members == nil {
		t.Error("members deveria ser array vazio [], não null")
	}
	if len(body.Members) != 0 {
		t.Errorf("members count = %d, want 0", len(body.Members))
	}
	if body.HasMore {
		t.Error("has_more deveria ser false")
	}
	if body.NextCursor != nil {
		t.Error("next_cursor deveria ser null")
	}
}

// 4.3.5: GET /admin/api/members?limit=300 → clampeado a 200 (teto CHK-A16).
// Validado via inspeção do handler: clampInt garante max=200.
// O fake retorna o que recebe — aqui validamos que o handler não passa > 200 para o repo.
func TestAdminMembers_LimitClampedTo200(t *testing.T) {
	var capturedLimit int
	mRepo := &captureLimitRepo{}
	dRepo := &fakeDeviceRepo{}
	aRepo := &fakeAttendanceRepo{}

	handler := AdminMembersHandler(AdminAPIConfig{
		MemberRepo:             mRepo,
		DeviceRepo:             dRepo,
		AttendanceRepo:         aRepo,
		DeviceOfflineThreshold: 24,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/api/members?limit=300", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	capturedLimit = mRepo.capturedLimit
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if capturedLimit > 200 {
		t.Errorf("limit passado para o repo = %d, want <= 200 (teto CHK-A16)", capturedLimit)
	}
	if capturedLimit != 200 {
		t.Errorf("limit = %d, want 200 (clampeado do 300 solicitado)", capturedLimit)
	}
}

// captureLimitRepo captura o limit passado pelo handler para verificar clamping.
type captureLimitRepo struct {
	capturedLimit int
}

func (r *captureLimitRepo) CountMembersWithSelfie(ctx context.Context) (int, error) { return 0, nil }
func (r *captureLimitRepo) ListMembersPaged(ctx context.Context, q string, cursor, limit int) ([]domain.MemberView, int, bool, error) {
	r.capturedLimit = limit
	return []domain.MemberView{}, 0, false, nil
}

// 4.3.6: GET /admin/api/events?from=&to= → apenas eventos no intervalo (filtro server-side).
// Valida que os parâmetros from/to são parseados e passados ao repositório.
func TestAdminEvents_DateFilterParsed(t *testing.T) {
	aRepo := &captureFilterRepo{}
	mRepo := &fakeMemberRepo{}
	dRepo := &fakeDeviceRepo{}

	handler := AdminEventsHandler(AdminAPIConfig{
		MemberRepo:             mRepo,
		DeviceRepo:             dRepo,
		AttendanceRepo:         aRepo,
		DeviceOfflineThreshold: 24,
	})

	from := "2026-01-01"
	to := "2026-12-31"
	req := httptest.NewRequest(http.MethodGet, "/admin/api/events?from="+from+"&to="+to, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	// Verificar que from foi parseado corretamente (ano 2026)
	if aRepo.capturedFrom.IsZero() {
		t.Error("from não foi parseado (IsZero)")
	}
	if aRepo.capturedFrom.Year() != 2026 {
		t.Errorf("from.Year = %d, want 2026", aRepo.capturedFrom.Year())
	}
	if aRepo.capturedTo.IsZero() {
		t.Error("to não foi parseado (IsZero)")
	}
	if aRepo.capturedTo.Year() != 2026 {
		t.Errorf("to.Year = %d, want 2026", aRepo.capturedTo.Year())
	}
}

// captureFilterRepo captura os parâmetros de filtro passados pelo handler.
type captureFilterRepo struct {
	capturedFrom time.Time
	capturedTo   time.Time
}

func (r *captureFilterRepo) CountAttendanceSince(ctx context.Context, since time.Time) (int, error) {
	return 0, nil
}
func (r *captureFilterRepo) ListEventsPaged(ctx context.Context, from, to time.Time, cursor domain.CursorEvt, limit int) ([]domain.EventView, domain.CursorEvt, bool, error) {
	r.capturedFrom = from
	r.capturedTo = to
	return []domain.EventView{}, domain.CursorEvt{}, false, nil
}

// 4.3.7: nenhum campo federal_document (CPF cru) aparece em respostas de /admin/api/members
// ou /admin/api/events. Validado via: (a) grep no código-fonte dos handlers; (b) inspeção do JSON.
func TestAdminAPI_NoCPFRawInResponses(t *testing.T) {
	// Sonda empírica — lê os arquivos fonte (CHK-S13/SC-006)
	for _, file := range []string{"admin_api_handlers.go", "admin_ui_handlers.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Skipf("arquivo %s não encontrado: %v", file, err)
		}
		// Verificar que o campo cru "federal_document" (sem sufixo _masked) não aparece
		// em json tags ou respostas (deve aparecer apenas como federal_document_masked)
		content := string(src)
		// Permite "federal_document_masked" mas não "federal_document" isolado
		clean := strings.ReplaceAll(content, "federal_document_masked", "MASKED")
		if strings.Contains(clean, "federal_document") {
			t.Errorf("%s contém referência a federal_document (CPF cru) fora de federal_document_masked — SC-006 violado", file)
		}
	}

	// Validação via resposta JSON: member com CPF mascarado correto
	maskedCPF := domain.MaskCPF("00514904712")
	mRepo := &fakeMemberRepo{
		listResult: []domain.MemberView{
			{
				ID:                    1,
				Name:                  "Teste Silva",
				FederalDocumentMasked: maskedCPF,
				Status:                "active",
				SyncStatus:            "synced",
			},
		},
	}
	dRepo := &fakeDeviceRepo{}
	aRepo := &fakeAttendanceRepo{}
	handler := AdminMembersHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))

	req := httptest.NewRequest(http.MethodGet, "/admin/api/members", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	// O JSON não deve conter o CPF completo
	if strings.Contains(body, "00514904712") {
		t.Errorf("resposta JSON contém CPF cru '00514904712' — SC-006 violado")
	}
	// Deve conter a versão mascarada
	if !strings.Contains(body, "***") {
		t.Errorf("resposta JSON não contém CPF mascarado (esperado '***'): %s", body)
	}
}

// 4.3.8: GET /admin/api/devices → resposta inclui device_offline_threshold_hours e status derivado.
func TestAdminDevices_StatusAndThresholdInResponse(t *testing.T) {
	now := time.Now().UTC()
	recentHeartbeat := now.Add(-1 * time.Hour)    // ativo (< 24h)
	oldHeartbeat := now.Add(-48 * time.Hour)      // offline (> 24h)
	ipActive := "192.168.1.10"
	ipOffline := "192.168.1.20"

	dRepo := &fakeDeviceRepo{
		listResult: []domain.Device{
			{
				ID:               1,
				DeviceIdentifier: "AA:BB:CC:DD:EE:01",
				IPAddress:        &ipActive,
				LastHeartbeatAt:  &recentHeartbeat,
				WebhookConfigured: true,
				CreatedAt:        now,
			},
			{
				ID:               2,
				DeviceIdentifier: "AA:BB:CC:DD:EE:02",
				IPAddress:        &ipOffline,
				LastHeartbeatAt:  &oldHeartbeat,
				WebhookConfigured: false,
				CreatedAt:        now,
			},
		},
	}
	mRepo := &fakeMemberRepo{}
	aRepo := &fakeAttendanceRepo{}

	handler := AdminDevicesHandler(newTestAdminAPICfg(mRepo, dRepo, aRepo))

	var body struct {
		Devices                     []map[string]interface{} `json:"devices"`
		DeviceOfflineThresholdHours int                      `json:"device_offline_threshold_hours"`
	}
	rr := getJSON(t, handler, "/admin/api/devices", &body)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	// Verificar device_offline_threshold_hours
	if body.DeviceOfflineThresholdHours != 24 {
		t.Errorf("device_offline_threshold_hours = %d, want 24", body.DeviceOfflineThresholdHours)
	}

	if len(body.Devices) != 2 {
		t.Fatalf("devices count = %d, want 2", len(body.Devices))
	}

	// Dispositivo 1 — heartbeat recente → active
	status1, _ := body.Devices[0]["status"].(string)
	if status1 != "active" {
		t.Errorf("device 1 status = %q, want active (heartbeat < 24h)", status1)
	}

	// Dispositivo 2 — heartbeat antigo → offline
	status2, _ := body.Devices[1]["status"].(string)
	if status2 != "offline" {
		t.Errorf("device 2 status = %q, want offline (heartbeat > 24h)", status2)
	}

	// Verificar que webhook_configured está presente e correto
	wh1, _ := body.Devices[0]["webhook_configured"].(bool)
	if !wh1 {
		t.Error("device 1 webhook_configured deveria ser true")
	}
	wh2, _ := body.Devices[1]["webhook_configured"].(bool)
	if wh2 {
		t.Error("device 2 webhook_configured deveria ser false")
	}
}

// TestToDeviceResponse_IsapiCredentialsSet verifies tasks.md §2.2.5:
// isapi_credentials_set is true when username + password_enc are present, false otherwise.
func TestToDeviceResponse_IsapiCredentialsSet(t *testing.T) {
	user := "admin"
	enc := []byte("nonce||ciphertext")

	// With credentials
	d := domain.Device{
		ID:               1,
		DeviceIdentifier: "mac1",
		ISAPIUsername:    &user,
		ISAPIPasswordEnc: enc,
	}
	resp := toDeviceResponse(d, 24)
	if !resp.IsapiCredentialsSet {
		t.Error("expected IsapiCredentialsSet=true when username+password_enc are set")
	}

	// Without credentials (nil username)
	d2 := domain.Device{
		ID:               2,
		DeviceIdentifier: "mac2",
		ISAPIUsername:    nil,
		ISAPIPasswordEnc: nil,
	}
	resp2 := toDeviceResponse(d2, 24)
	if resp2.IsapiCredentialsSet {
		t.Error("expected IsapiCredentialsSet=false when no credentials")
	}

	// Empty username string — still false
	emptyUser := ""
	d3 := domain.Device{
		ID:               3,
		DeviceIdentifier: "mac3",
		ISAPIUsername:    &emptyUser,
		ISAPIPasswordEnc: enc,
	}
	resp3 := toDeviceResponse(d3, 24)
	if resp3.IsapiCredentialsSet {
		t.Error("expected IsapiCredentialsSet=false when username is empty string")
	}
}

// TestToDeviceResponse_CapacityFields verifies tasks.md §2.2.4:
// max_users and max_faces are included in the response when set.
func TestToDeviceResponse_CapacityFields(t *testing.T) {
	maxU := 5000
	maxF := 5000

	d := domain.Device{
		ID:               1,
		DeviceIdentifier: "mac1",
		MaxUsers:         &maxU,
		MaxFaces:         &maxF,
	}
	resp := toDeviceResponse(d, 24)
	if resp.MaxUsers == nil || *resp.MaxUsers != 5000 {
		t.Errorf("MaxUsers: got %v, want 5000", resp.MaxUsers)
	}
	if resp.MaxFaces == nil || *resp.MaxFaces != 5000 {
		t.Errorf("MaxFaces: got %v, want 5000", resp.MaxFaces)
	}

	// Nil when not set
	d2 := domain.Device{ID: 2, DeviceIdentifier: "mac2"}
	resp2 := toDeviceResponse(d2, 24)
	if resp2.MaxUsers != nil {
		t.Errorf("MaxUsers: expected nil, got %v", resp2.MaxUsers)
	}
	if resp2.MaxFaces != nil {
		t.Errorf("MaxFaces: expected nil, got %v", resp2.MaxFaces)
	}
}
