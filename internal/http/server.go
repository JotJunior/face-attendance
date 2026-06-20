package httphandler

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Server is the HTTP server for the presenca-facial API.
type Server struct {
	inner *http.Server
}

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	Addr                    string
	WebhookPathSecret       string
	AdminToken              string
	WebhookRateLimitPerMin  int
	AdminSyncMinIntervalSec int

	EventHandler  http.Handler
	HealthHandler http.Handler
	AdminHandler  http.Handler

	// AllowedWebhookIPs returns the current list of allowed IPs (queried from DB).
	AllowedWebhookIPs func() []string

	// Admin UI (painel de administração web — FASE 2/3)
	AdminUIEnabled    bool
	AdminLoginCfg     AdminLoginConfig
	AdminAPICfg       AdminAPIConfig
	AdminAssets       http.FileSystem // embed.FS servindo /admin/*
}

// NewServer constructs an HTTP server with the routes and middleware wired up.
//
// Routes:
//
//	POST /webhook/{secret}         — HikVision events (IP allowlist + rate limit)
//	GET  /health                   — health check (no auth)
//	POST /admin/sync               — trigger member load (Bearer token auth)
//	POST /admin/api/login          — autenticação do painel (sem sessão, rate-limited)
//	POST /admin/api/logout         — encerrar sessão do painel
//	GET  /admin/api/stats          — métricas do dashboard (sessão obrigatória)
//	GET  /admin/api/devices        — lista dispositivos (sessão obrigatória)
//	GET  /admin/api/devices/{id}   — detalhe de dispositivo (sessão obrigatória)
//	GET  /admin/api/members        — lista membros paginada (sessão obrigatória)
//	GET  /admin/api/events         — lista eventos paginada (sessão obrigatória)
//	POST /admin/api/sync           — sync manual via cookie (sessão obrigatória)
//	GET  /admin/                   — SPA assets (embed.FS, sem sessão)
func NewServer(cfg ServerConfig) *Server {
	mux := http.NewServeMux()

	// Webhook route: IP allowlist + rate limiting (spec.md §FR-013, §FR-014)
	webhookPath := "/webhook/" + cfg.WebhookPathSecret
	rl := NewRateLimitMiddleware(cfg.WebhookRateLimitPerMin)
	webhookHandler := IPAllowlistMiddleware(
		cfg.AllowedWebhookIPs,
		rl.Handler(cfg.EventHandler),
	)
	mux.Handle(webhookPath, webhookHandler)

	// Health check — no auth
	mux.Handle("/health", cfg.HealthHandler)

	// Admin sync — Bearer token auth (rota legada, mantida para compatibilidade)
	adminHandler := AdminAuthMiddleware(cfg.AdminToken, cfg.AdminHandler)
	mux.Handle("/admin/sync", adminHandler)

	// Admin UI — rotas da interface de administração web (FASE 2/3)
	if cfg.AdminUIEnabled {
		sessionMW := SessionMiddleware(cfg.AdminLoginCfg.SessionSecret)

		// Login: rate-limit (10/min/IP) + sem sessão obrigatória
		loginRL := NewRateLimitMiddleware(10)
		mux.Handle("/admin/api/login", loginRL.Handler(AdminLoginHandler(cfg.AdminLoginCfg)))

		// Logout: sem sessão obrigatória (permite limpar cookie expirado)
		mux.Handle("/admin/api/logout", AdminLogoutHandler())

		// Rotas protegidas por sessão HMAC
		mux.Handle("/admin/api/stats", sessionMW(AdminStatsHandler(cfg.AdminAPICfg)))
		mux.Handle("/admin/api/devices/", sessionMW(adminDevicesRouter(cfg.AdminAPICfg)))
		mux.Handle("/admin/api/devices", sessionMW(AdminDevicesHandler(cfg.AdminAPICfg)))
		mux.Handle("/admin/api/members", sessionMW(AdminMembersHandler(cfg.AdminAPICfg)))
		mux.Handle("/admin/api/events", sessionMW(AdminEventsHandler(cfg.AdminAPICfg)))

		// Sync manual via cookie (wraps o AdminSyncHandler existente com sessão)
		mux.Handle("/admin/api/sync", sessionMW(cfg.AdminHandler))

		// Assets da SPA (index.html + CSS + JS) — sem autenticação (login page pública)
		if cfg.AdminAssets != nil {
			assetServer := http.FileServer(cfg.AdminAssets)
			mux.Handle("/admin/", http.StripPrefix("/admin", assetServer))
		}
	}

	inner := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	return &Server{inner: inner}
}

// adminDevicesRouter roteia /admin/api/devices/* para o handler correto.
// Padrão: /admin/api/devices → lista; /admin/api/devices/42 → detalhe.
func adminDevicesRouter(cfg AdminAPIConfig) http.Handler {
	listHandler := AdminDevicesHandler(cfg)
	detailHandler := AdminDeviceDetailHandler(cfg)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seg := extractLastPathSegment(r.URL.Path)
		if seg == "devices" || seg == "" {
			listHandler.ServeHTTP(w, r)
		} else {
			detailHandler.ServeHTTP(w, r)
		}
	})
}

// ListenAndServe starts the HTTP server. Blocks until the server exits.
func (s *Server) ListenAndServe() error {
	if err := s.inner.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server with the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}
