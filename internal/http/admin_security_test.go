package httphandler

// Testes de segurança para a interface de administração web (FASE 4 — tarefa 4.1).
// Cobre: proteção de rotas /admin/api/* por sessão (4.1.1), endpoint de login público (4.1.2),
// cookie expirado (4.1.3), HMAC adulterado (4.1.4), rate limit de login (4.1.5),
// payload > 1KB (4.1.6), uso de ConstantTimeCompare via grep estático (4.1.7),
// logout + requisição subsequente sem cookie retorna 401 (4.1.8).
// Ref: tasks.md §4.1, spec.md §SC-003/FR-001, plan.md §S1/S3/S4/S5, CHK-S03/S04/A13/S19.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// 4.1.1: todas as rotas /admin/api/* (exceto login) requerem sessão — retornam 401 sem cookie.
// Validado via SessionMiddleware diretamente (sem servidor real — foco no middleware).
func TestSecurity_ProtectedRoutesRequireSession(t *testing.T) {
	secret := "test-secret-32-bytes-for-hmac-ok"

	routes := []string{
		"/admin/api/stats",
		"/admin/api/devices",
		"/admin/api/members",
		"/admin/api/events",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			protected := SessionMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rr := httptest.NewRecorder()
			protected.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s sem cookie: status %d, want 401", route, rr.Code)
			}
		})
	}
}

// 4.1.2: endpoint de login é público — processa sem cookie de sessão (não retorna 401 por sessão).
func TestSecurity_LoginEndpointIsPublic(t *testing.T) {
	loginCfg := AdminLoginConfig{
		Username:      "admin",
		Password:      "pass",
		SessionSecret: "test-secret-32-bytes-for-hmac-ok",
		SessionTTL:    time.Hour,
	}
	body := `{"username":"admin","password":"pass"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	AdminLoginHandler(loginCfg).ServeHTTP(rr, req)
	// Sem cookie de sessão: deve processar normalmente (204), nunca 401 por sessão ausente
	if rr.Code == http.StatusUnauthorized {
		t.Error("login endpoint não deve retornar 401 por ausência de cookie de sessão")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("login: status = %d, want 204", rr.Code)
	}
}

// 4.1.3: cookie de sessão expirado retorna 401.
// Nota de implementação: signToken usa Unix (granularidade em segundos);
// -1ms trunca para o mesmo segundo → não expira. Usar -time.Second garante
// exp = now-1s → time.Now().Unix() > p.Exp → bloqueado.
func TestSecurity_ExpiredCookieReturns401(t *testing.T) {
	secret := "test-secret-32-bytes-for-hmac-ok"
	// Token com TTL = -1s (expirado há 1 segundo)
	token := signToken(secret, "admin", -time.Second)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
	rr := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	SessionMiddleware(secret)(next).ServeHTTP(rr, req)

	if called {
		t.Error("handler chamado com token expirado — esperava bloqueio")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 para token expirado", rr.Code)
	}
}

// 4.1.4: HMAC adulterado no cookie retorna 401.
func TestSecurity_TamperedHMACReturns401(t *testing.T) {
	secret := "test-secret-32-bytes-for-hmac-ok"
	token := signToken(secret, "admin", time.Hour)

	// Adulterar: substituir último caractere
	tampered := token[:len(token)-1] + "X"
	if tampered == token {
		tampered = token[:len(token)-1] + "Y"
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: tampered})
	rr := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	SessionMiddleware(secret)(next).ServeHTTP(rr, req)

	if called {
		t.Error("handler chamado com HMAC adulterado — esperava bloqueio")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 para HMAC adulterado", rr.Code)
	}
}

// 4.1.5: rate limit de login — após 10 requests, a 11ª retorna 429.
func TestSecurity_RateLimitLogin(t *testing.T) {
	loginCfg := AdminLoginConfig{
		Username:      "admin",
		Password:      "wrongpass", // credenciais erradas para evitar emissão de cookie
		SessionSecret: "test-secret-32-bytes-for-hmac-ok",
		SessionTTL:    time.Hour,
	}

	// Rate limiter com max=10 (mesmo config do wiring — CHK-A13)
	rl := NewRateLimitMiddleware(10)
	handler := rl.Handler(AdminLoginHandler(loginCfg))

	var lastCode int
	body := `{"username":"admin","password":"wrongpass"}`

	// 11 requests com mesmo IP — a 11ª deve retornar 429
	for i := 1; i <= 11; i++ {
		req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.1:12345" // IP fixo para acionar o bucket
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		lastCode = rr.Code
	}

	if lastCode != http.StatusTooManyRequests {
		t.Errorf("11ª request: status = %d, want 429 (rate limit CHK-A13)", lastCode)
	}
}

// 4.1.6: payload de login > 1KB retorna 400 (CHK-S19 via MaxBytesReader).
func TestSecurity_LoginPayloadOver1KBReturns400(t *testing.T) {
	loginCfg := AdminLoginConfig{
		Username:      "admin",
		Password:      "pass",
		SessionSecret: "test-secret-32-bytes-for-hmac-ok",
		SessionTTL:    time.Hour,
	}
	bigPayload := strings.Repeat("x", 1025)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(bigPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	AdminLoginHandler(loginCfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 para payload > 1KB (CHK-S19)", rr.Code)
	}
}

// 4.1.7: verificação estática de que subtle.ConstantTimeCompare é usado no handler de login.
// Sonda empírica: lê o arquivo-fonte e verifica presença do símbolo (CHK-S03).
func TestSecurity_ConstantTimeCompareUsedInLogin(t *testing.T) {
	src, err := os.ReadFile("admin_ui_handlers.go")
	if err != nil {
		t.Skipf("arquivo admin_ui_handlers.go não encontrado: %v", err)
	}
	if !strings.Contains(string(src), "ConstantTimeCompare") {
		t.Error("ConstantTimeCompare não encontrado em admin_ui_handlers.go — CHK-S03 violado")
	}
}

// 4.1.8: logout limpa o cookie; requisição subsequente sem cookie retorna 401.
// Nota: o HMAC stateless não tem revogação server-side. O teste valida que:
// (a) logout emite MaxAge=0 no cookie; (b) ausência de cookie → 401.
func TestSecurity_LogoutThenRequestReturns401(t *testing.T) {
	secret := "test-secret-32-bytes-for-hmac-ok"
	loginCfg := AdminLoginConfig{
		Username:      "admin",
		Password:      "pass",
		SessionSecret: secret,
		SessionTTL:    time.Hour,
	}

	// 1. Login — obter token
	loginBody := `{"username":"admin","password":"pass"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	AdminLoginHandler(loginCfg).ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusNoContent {
		t.Fatalf("login falhou: status %d", loginRR.Code)
	}
	var sessionToken string
	for _, c := range loginRR.Result().Cookies() {
		if c.Name == "admin_session" {
			sessionToken = c.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("token não obtido após login")
	}

	// 2. Logout — deve limpar cookie (MaxAge=0)
	logoutReq := httptest.NewRequest(http.MethodPost, "/admin/api/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "admin_session", Value: sessionToken})
	logoutRR := httptest.NewRecorder()
	AdminLogoutHandler().ServeHTTP(logoutRR, logoutReq)
	if logoutRR.Code != http.StatusNoContent {
		t.Errorf("logout: status %d, want 204", logoutRR.Code)
	}
	var cookieCleared bool
	for _, c := range logoutRR.Result().Cookies() {
		if c.Name == "admin_session" && c.MaxAge == 0 {
			cookieCleared = true
		}
	}
	if !cookieCleared {
		t.Error("cookie não foi limpo pelo logout (MaxAge=0 esperado)")
	}

	// 3. Requisição sem cookie → 401 (cliente descartou cookie após logout)
	protectedReq := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	protectedRR := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	SessionMiddleware(secret)(next).ServeHTTP(protectedRR, protectedReq)

	if called {
		t.Error("handler foi chamado sem cookie pós-logout — esperava 401")
	}
	if protectedRR.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 pós-logout sem cookie", protectedRR.Code)
	}
}
