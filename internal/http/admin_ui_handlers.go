package httphandler

// Handlers de autenticação da interface de administração web.
// Login com validação timing-safe + emissão de cookie httpOnly HMAC.
// Logout limpando o cookie.
// Ref: spec.md §FR-001, contracts §POST /admin/api/login, §POST /admin/api/logout,
//      plan.md §S3/S4/S5, CHK-S03/S04/S19, dec-006, tasks.md §2.2.

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// adminLoginRequest é o corpo JSON esperado pelo endpoint de login.
type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AdminLoginConfig agrupa os parâmetros necessários para o handler de login.
// Separado de ServerConfig para permitir testes independentes do servidor.
type AdminLoginConfig struct {
	Username      string
	Password      string        // sensível — nunca logar
	SessionSecret string        // sensível — nunca logar
	SessionTTL    time.Duration // derivado de AdminSessionTTLHours
}

// AdminLoginHandler cria um http.Handler que processa POST /admin/api/login.
// Protege contra brute force via RateLimitMiddleware (CHK-A13 — envolto no wiring).
// O handler em si não faz rate limit; isso é responsabilidade do wiring (2.6).
func AdminLoginHandler(cfg AdminLoginConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"método não permitido"}`, http.StatusMethodNotAllowed)
			return
		}

		// Limitar tamanho do payload a 1KB (CHK-S19 — previne DoS de leitura)
		r.Body = http.MaxBytesReader(w, r.Body, 1024)

		var req adminLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"corpo da requisição inválido"}`)) //nolint:errcheck
			return
		}

		// Validar presença dos campos (mensagem genérica — não revela qual campo falhou)
		if req.Username == "" || req.Password == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"credenciais inválidas"}`)) //nolint:errcheck
			return
		}

		// Comparação timing-safe para username e password (CHK-S03 — anti-timing attack)
		usernameMatch := subtle.ConstantTimeCompare([]byte(req.Username), []byte(cfg.Username))
		passwordMatch := subtle.ConstantTimeCompare([]byte(req.Password), []byte(cfg.Password))

		if usernameMatch != 1 || passwordMatch != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"credenciais inválidas"}`)) //nolint:errcheck
			return
		}

		// Credenciais válidas — emitir token HMAC e cookie httpOnly (dec-006)
		token := signToken(cfg.SessionSecret, cfg.Username, cfg.SessionTTL)
		if token == "" {
			http.Error(w, `{"error":"erro interno ao criar sessão"}`, http.StatusInternalServerError)
			return
		}

		ttlSeconds := int(cfg.SessionTTL.Seconds())
		http.SetCookie(w, &http.Cookie{
			Name:     "admin_session",
			Value:    token,
			Path:     "/admin",
			MaxAge:   ttlSeconds,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	})
}

// AdminLogoutHandler cria um http.Handler que processa POST /admin/api/logout.
// Limpa o cookie de sessão emitindo MaxAge=0.
func AdminLogoutHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"método não permitido"}`, http.StatusMethodNotAllowed)
			return
		}

		// Limpar o cookie de sessão (MaxAge=0 remove imediatamente)
		http.SetCookie(w, &http.Cookie{
			Name:     "admin_session",
			Value:    "",
			Path:     "/admin",
			MaxAge:   0,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	})
}

// adminJSONError escreve uma resposta de erro JSON padronizada.
// Uso interno dos handlers admin.
func adminJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
