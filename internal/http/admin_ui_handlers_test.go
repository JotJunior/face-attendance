package httphandler

// Testes para admin_ui_handlers.go: login correto, login errado,
// payload inválido, logout limpa cookie.
// Rate limit (429) é testado separadamente no wiring (tasks.md §2.6 + §4.1.5).
// Ref: tasks.md §2.2.8, spec.md §FR-001, contracts §login/logout, CHK-S03/S04/S19.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testLoginConfig = AdminLoginConfig{
	Username:      "operador",
	Password:      "senha_segura_teste",
	SessionSecret: "segredo-hmac-32-bytes-para-testes-",
	SessionTTL:    time.Hour,
}

func TestAdminLoginHandler_Success(t *testing.T) {
	body := `{"username":"operador","password":"senha_segura_teste"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204; body: %s", rr.Code, rr.Body.String())
	}

	// Verificar que cookie admin_session foi emitido com atributos corretos
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "admin_session" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("cookie admin_session não foi emitido")
	}
	if !sessionCookie.HttpOnly {
		t.Error("cookie deve ser HttpOnly")
	}
	if !sessionCookie.Secure {
		t.Error("cookie deve ser Secure")
	}
	if sessionCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", sessionCookie.SameSite)
	}
	if sessionCookie.Path != "/admin" {
		t.Errorf("Path = %q, want /admin", sessionCookie.Path)
	}
	if sessionCookie.MaxAge <= 0 {
		t.Errorf("MaxAge = %d, deve ser positivo (TTL em segundos)", sessionCookie.MaxAge)
	}
	if sessionCookie.Value == "" {
		t.Error("cookie value não pode ser vazio")
	}
}

func TestAdminLoginHandler_WrongPassword(t *testing.T) {
	body := `{"username":"operador","password":"senha_errada"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "credenciais inválidas") {
		t.Errorf("corpo não contém mensagem genérica; got: %s", rr.Body.String())
	}
	// Não deve emitir cookie em caso de falha
	for _, c := range rr.Result().Cookies() {
		if c.Name == "admin_session" {
			t.Error("cookie admin_session não deve ser emitido em login inválido")
		}
	}
}

func TestAdminLoginHandler_WrongUsername(t *testing.T) {
	body := `{"username":"outro","password":"senha_segura_teste"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAdminLoginHandler_MalformedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader("nao-e-json{"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAdminLoginHandler_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	// Campos vazios → credenciais inválidas (sem revelar qual campo está ausente)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAdminLoginHandler_PayloadTooLarge(t *testing.T) {
	// Body > 1KB — deve retornar 400 (CHK-S19 via MaxBytesReader)
	bigPayload := make([]byte, 1025)
	for i := range bigPayload {
		bigPayload[i] = 'x'
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", bytes.NewReader(bigPayload))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	// MaxBytesReader retorna erro de leitura → Decode falha → 400
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 para payload > 1KB", rr.Code)
	}
}

func TestAdminLogoutHandler_ClearsCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/admin/api/logout", nil)
	// Simular sessão ativa
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "token_existente"})
	rr := httptest.NewRecorder()

	AdminLogoutHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}

	// Verificar que cookie foi limpo (MaxAge=0)
	var cleared bool
	for _, c := range rr.Result().Cookies() {
		if c.Name == "admin_session" && c.MaxAge == 0 {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Error("cookie admin_session não foi limpo (MaxAge=0 esperado)")
	}
}

func TestAdminLoginHandler_SubsequentRequestAuthorized(t *testing.T) {
	// Login → capturar cookie → usar cookie em requisição protegida
	body := `{"username":"operador","password":"senha_segura_teste"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	AdminLoginHandler(testLoginConfig).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("login falhou: status %d", rr.Code)
	}

	var sessionToken string
	for _, c := range rr.Result().Cookies() {
		if c.Name == "admin_session" {
			sessionToken = c.Value
			break
		}
	}
	if sessionToken == "" {
		t.Fatal("token não capturado")
	}

	// Usar o token no middleware de sessão
	protectedReq := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	protectedReq.AddCookie(&http.Cookie{Name: "admin_session", Value: sessionToken})
	protectedRR := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	SessionMiddleware(testLoginConfig.SessionSecret)(next).ServeHTTP(protectedRR, protectedReq)

	if !called {
		t.Error("próxima requisição com cookie válido foi bloqueada — esperava acesso autorizado")
	}
}
