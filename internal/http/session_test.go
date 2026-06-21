package httphandler

// Testes unitários para session.go: signToken, verifyToken e SessionMiddleware.
// Cobre: token válido, token expirado, HMAC adulterado, token malformado,
// comparação timing-safe (verifica uso de ConstantTimeCompare via grep — vide 2.1.5).
// Ref: tasks.md §2.1.5, spec.md §FR-001, FR-012, dec-006, CHK-S03/S04/S06.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testSecret = "segredo-hmac-de-teste-32-bytes-x"

func TestSignAndVerifyToken_Valid(t *testing.T) {
	token := signToken(testSecret, "operador", time.Hour)
	if token == "" {
		t.Fatal("signToken retornou string vazia")
	}

	sub, ok := verifyToken(testSecret, token)
	if !ok {
		t.Fatal("verifyToken falhou para token válido")
	}
	if sub != "operador" {
		t.Errorf("sub = %q, want %q", sub, "operador")
	}
}

func TestVerifyToken_Expired(t *testing.T) {
	// Token com TTL negativo — já expirado
	token := signToken(testSecret, "operador", -time.Second)
	if token == "" {
		t.Fatal("signToken retornou string vazia")
	}

	_, ok := verifyToken(testSecret, token)
	if ok {
		t.Error("verifyToken aceitou token expirado, esperava false")
	}
}

func TestVerifyToken_TamperedHMAC(t *testing.T) {
	token := signToken(testSecret, "operador", time.Hour)

	// Adulterar o último caractere da assinatura
	tampered := token[:len(token)-1] + "X"
	if tampered == token {
		tampered = token[:len(token)-1] + "Y"
	}

	_, ok := verifyToken(testSecret, tampered)
	if ok {
		t.Error("verifyToken aceitou HMAC adulterado, esperava false")
	}
}

func TestVerifyToken_Malformed(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"vazio", ""},
		{"sem ponto", "sempontonontoken"},
		{"apenas ponto", "."},
		{"base64 inválido na sig", "abc.!!!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := verifyToken(testSecret, tc.token)
			if ok {
				t.Errorf("verifyToken aceitou token malformado %q", tc.token)
			}
		})
	}
}

func TestVerifyToken_WrongSecret(t *testing.T) {
	token := signToken(testSecret, "operador", time.Hour)

	_, ok := verifyToken("outro-segredo-completamente-diferente", token)
	if ok {
		t.Error("verifyToken aceitou token com secret errado, esperava false")
	}
}

func TestSessionMiddleware_ValidCookie(t *testing.T) {
	token := signToken(testSecret, "operador", time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})

	rr := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	SessionMiddleware(testSecret)(next).ServeHTTP(rr, req)

	if !called {
		t.Error("next handler não foi chamado com cookie válido")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestSessionMiddleware_NoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	rr := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	SessionMiddleware(testSecret)(next).ServeHTTP(rr, req)

	if called {
		t.Error("next handler foi chamado sem cookie — deveria ter sido bloqueado")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestSessionMiddleware_InvalidCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/api/members", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: "token.invalido"})
	rr := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	SessionMiddleware(testSecret)(next).ServeHTTP(rr, req)

	if called {
		t.Error("next handler foi chamado com cookie inválido — deveria ter sido bloqueado")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestSessionMiddleware_401PreservesRedirectHeader(t *testing.T) {
	// FR-012: 401 deve incluir X-Redirect-To com a URL atual
	req := httptest.NewRequest(http.MethodGet, "/admin/api/members?q=teste", nil)
	rr := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	SessionMiddleware(testSecret)(next).ServeHTTP(rr, req)

	redirectTo := rr.Header().Get("X-Redirect-To")
	if redirectTo != "/admin/api/members?q=teste" {
		t.Errorf("X-Redirect-To = %q, want %q", redirectTo, "/admin/api/members?q=teste")
	}
}

func TestSessionMiddleware_ExpiredToken(t *testing.T) {
	token := signToken(testSecret, "operador", -time.Second)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/stats", nil)
	req.AddCookie(&http.Cookie{Name: "admin_session", Value: token})
	rr := httptest.NewRecorder()
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	SessionMiddleware(testSecret)(next).ServeHTTP(rr, req)

	if called {
		t.Error("next handler foi chamado com token expirado")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
