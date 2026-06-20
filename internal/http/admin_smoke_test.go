package httphandler

// Teste smoke de integração do wiring do servidor (task 2.6.7 + 4.4.3).
// Valida: POST /admin/api/login → cookie → GET /admin/api/stats → JSON com 5 campos.
// Usa httptest.NewServer (servidor real em porta efêmera) + fakes de repositório.
// Ref: tasks.md §2.6.7, spec.md §FR-001/FR-003, contracts §login / §stats.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// smokeRepos implementa todas as interfaces de repositório admin para o smoke test.
type smokeRepos struct{}

func (s *smokeRepos) CountMembersWithSelfie(ctx context.Context) (int, error) { return 7, nil }
func (s *smokeRepos) ListMembersPaged(ctx context.Context, q string, cursor, limit int) ([]domain.MemberView, int, bool, error) {
	return []domain.MemberView{}, 0, false, nil
}
func (s *smokeRepos) CountDevicesByActivity(ctx context.Context, t int) (int, int, error) {
	return 2, 0, nil
}
func (s *smokeRepos) ListDevicesAll(ctx context.Context) ([]domain.Device, error) { return nil, nil }
func (s *smokeRepos) GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error) {
	return nil, fmt.Errorf("not found")
}
func (s *smokeRepos) CountAttendanceSince(ctx context.Context, since time.Time) (int, error) {
	return 3, nil
}
func (s *smokeRepos) ListEventsPaged(ctx context.Context, from, to time.Time, cursor domain.CursorEvt, limit int) ([]domain.EventView, domain.CursorEvt, bool, error) {
	return []domain.EventView{}, domain.CursorEvt{}, false, nil
}

// TestSmoke_LoginThenStats: POST /admin/api/login → cookie → GET /admin/api/stats → 5 campos.
func TestSmoke_LoginThenStats(t *testing.T) {
	repos := &smokeRepos{}
	loginCfg := AdminLoginConfig{
		Username:      "operador",
		Password:      "smoke_pass",
		SessionSecret: "smoke-secret-32-bytes-for-hmac--",
		SessionTTL:    time.Hour,
	}
	apiCfg := AdminAPIConfig{
		MemberRepo:             repos,
		DeviceRepo:             repos,
		AttendanceRepo:         repos,
		DeviceOfflineThreshold: 24,
	}

	srv := NewServer(ServerConfig{
		Addr:                    ":0",
		WebhookPathSecret:       "smoke-wh",
		AdminToken:              "smoke-bearer",
		WebhookRateLimitPerMin:  100,
		AdminSyncMinIntervalSec: 0,
		EventHandler:            http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		HealthHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		AdminHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		AllowedWebhookIPs: func() []string { return []string{"127.0.0.1"} },
		AdminUIEnabled:    true,
		AdminLoginCfg:     loginCfg,
		AdminAPICfg:       apiCfg,
		AdminAssets:       nil,
	})

	ts := httptest.NewServer(srv.inner.Handler)
	defer ts.Close()

	client := &http.Client{
		// Não seguir redirects automaticamente
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 1. POST /admin/api/login
	loginBody := `{"username":"operador","password":"smoke_pass"}`
	loginResp, err := client.Post(ts.URL+"/admin/api/login", "application/json", strings.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login request falhou: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusNoContent {
		t.Fatalf("login: status = %d, want 204", loginResp.StatusCode)
	}

	// Capturar cookie de sessão
	var sessionCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "admin_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("cookie admin_session não encontrado após login")
	}

	// 2. GET /admin/api/stats com cookie
	statsReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin/api/stats", nil)
	statsReq.AddCookie(sessionCookie)
	statsResp, err := client.Do(statsReq)
	if err != nil {
		t.Fatalf("stats request falhou: %v", err)
	}
	defer statsResp.Body.Close()

	if statsResp.StatusCode != http.StatusOK {
		t.Fatalf("stats: status = %d, want 200", statsResp.StatusCode)
	}

	// 3. Validar JSON com 5 campos
	var body map[string]interface{}
	if err := json.NewDecoder(statsResp.Body).Decode(&body); err != nil {
		t.Fatalf("falha ao decodificar stats: %v", err)
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

	// Verificar valores dos fakes
	if got := int(body["members_with_selfie"].(float64)); got != 7 {
		t.Errorf("members_with_selfie = %d, want 7", got)
	}
	if got := int(body["devices_active"].(float64)); got != 2 {
		t.Errorf("devices_active = %d, want 2", got)
	}
}

// TestSmoke_StatsWithoutCookieReturns401: GET /admin/api/stats sem sessão → 401.
func TestSmoke_StatsWithoutCookieReturns401(t *testing.T) {
	repos := &smokeRepos{}
	loginCfg := AdminLoginConfig{
		Username:      "operador",
		Password:      "smoke_pass",
		SessionSecret: "smoke-secret-32-bytes-for-hmac--",
		SessionTTL:    time.Hour,
	}
	apiCfg := AdminAPIConfig{
		MemberRepo:             repos,
		DeviceRepo:             repos,
		AttendanceRepo:         repos,
		DeviceOfflineThreshold: 24,
	}

	srv := NewServer(ServerConfig{
		Addr:                   ":0",
		WebhookPathSecret:      "smoke-wh",
		AdminToken:             "smoke-bearer",
		WebhookRateLimitPerMin: 100,
		EventHandler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		HealthHandler:          http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
		AdminHandler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }),
		AllowedWebhookIPs:      func() []string { return []string{"127.0.0.1"} },
		AdminUIEnabled:         true,
		AdminLoginCfg:          loginCfg,
		AdminAPICfg:            apiCfg,
	})

	ts := httptest.NewServer(srv.inner.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/admin/api/stats")
	if err != nil {
		t.Fatalf("request falhou: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 sem cookie", resp.StatusCode)
	}
}
