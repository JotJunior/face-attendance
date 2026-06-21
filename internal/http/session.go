package httphandler

// Gerenciamento de sessão HMAC-SHA256 para o painel de administração.
// Implementa signToken / verifyToken e SessionMiddleware.
// Sem armazenamento em banco — token stateless validado por HMAC.
// Ref: spec.md §FR-001, FR-012, dec-006, plan.md §S1/S3/S5, CHK-S03/S04/S06.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// tokenPayload é o payload JSON embutido no token de sessão.
type tokenPayload struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"` // Unix timestamp de expiração
}

// signToken gera um token de sessão no formato <base64url(payload)>.<base64url(HMAC)>.
// sub é o identificador do usuário; ttl é o tempo de vida do token.
// Retorna token assinado ou string vazia em caso de erro de marshal (improvável).
func signToken(secret, sub string, ttl time.Duration) string {
	payload := tokenPayload{
		Sub: sub,
		Exp: time.Now().Add(ttl).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	b64Payload := base64.RawURLEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b64Payload))
	sig := mac.Sum(nil)
	b64Sig := base64.RawURLEncoding.EncodeToString(sig)

	return b64Payload + "." + b64Sig
}

// verifyToken valida o token e retorna o sub (identificador do usuário) se válido.
// Verifica: formato, HMAC (timing-safe via ConstantTimeCompare) e expiração.
// Retorna ("", false) para qualquer caso de invalidade.
func verifyToken(secret, token string) (sub string, ok bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	b64Payload, b64SigRecv := parts[0], parts[1]

	// Recomputar HMAC esperado
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(b64Payload))
	expectedSig := mac.Sum(nil)

	// Decodificar HMAC recebido
	sigRecv, err := base64.RawURLEncoding.DecodeString(b64SigRecv)
	if err != nil {
		return "", false
	}

	// Comparação timing-safe — evita timing attack (CHK-S03)
	if subtle.ConstantTimeCompare(expectedSig, sigRecv) != 1 {
		return "", false
	}

	// Decodificar payload (só após validação do HMAC)
	rawPayload, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return "", false
	}
	var p tokenPayload
	if err := json.Unmarshal(rawPayload, &p); err != nil {
		return "", false
	}

	// Verificar expiração
	if time.Now().Unix() > p.Exp {
		return "", false
	}

	return p.Sub, true
}

// SessionMiddleware cria um middleware que protege rotas admin.
// Lê o cookie "admin_session", verifica HMAC e expiração.
// Em caso de falha: responde 401 JSON {"error":"sessão inválida ou expirada"}.
// O path atual é incluído no header Location para redirect pós-login (FR-012).
func SessionMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("admin_session")
			if err != nil {
				// Cookie ausente — redirecionar para login com URL atual (FR-012)
				rejectSession(w, r)
				return
			}

			sub, ok := verifyToken(secret, cookie.Value)
			if !ok || sub == "" {
				rejectSession(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// rejectSession responde 401 JSON preservando a URL atual no header para redirect.
// O frontend usa X-Redirect-To para navegar para ?redirect=<path> pós-login (FR-012).
func rejectSession(w http.ResponseWriter, r *http.Request) {
	currentPath := r.URL.RequestURI()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Redirect-To", currentPath)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":"sessão inválida ou expirada"}`)) //nolint:errcheck
}
