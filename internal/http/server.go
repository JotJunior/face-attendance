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
	AdminResyncCfg    AdminResyncConfig         // reenvio individual de membro
	DeviceConfigCfg   DeviceConfigConfig        // configuração ISAPI dos dispositivos (FASE 4 — device-config)
	FlowsAPICfg       AdminFlowsConfig          // fluxos de reconhecimento facial (face-flow)
	BackgroundImgCfg  AdminBackgroundImagesConfig // imagens de fundo dos fluxos (face-flow)
	AdminAssets       http.FileSystem           // embed.FS servindo /admin/*
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
//	GET  /admin/api/devices/{id}                           — detalhe de dispositivo (sessão obrigatória)
//	PUT  /admin/api/devices/{id}/credentials               — credenciais ISAPI (sessão obrigatória)
//	POST /admin/api/devices/{id}/actions/reboot            — reboot (sessão obrigatória)
//	POST /admin/api/devices/{id}/actions/factory-reset     — factory reset (sessão obrigatória)
//	GET  /admin/api/devices/{id}/time                      — ler hora (sessão obrigatória)
//	PUT  /admin/api/devices/{id}/time                      — configurar hora (sessão obrigatória)
//	GET  /admin/api/devices/{id}/doors                     — portas (sessão obrigatória)
//	GET  /admin/api/devices/{id}/doors/{door_id}/status    — status de porta (sessão obrigatória)
//	POST /admin/api/devices/{id}/doors/{door_id}/control   — controlar porta (sessão obrigatória)
//	GET  /admin/api/devices/{id}/users                     — usuários (sessão obrigatória)
//	DELETE /admin/api/devices/{id}/users                   — limpar usuários (sessão obrigatória)
//	DELETE /admin/api/devices/{id}/faces                   — limpar faces (sessão obrigatória)
//	GET  /admin/api/devices/{id}/webhooks                  — webhooks (sessão obrigatória)
//	POST /admin/api/devices/{id}/webhooks                  — provisionar/reparar webhook (sessão obrigatória)
//	DELETE /admin/api/devices/{id}/webhooks/{webhook_id}   — remover webhook (sessão obrigatória)
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
		mux.Handle("/admin/api/logout", AdminLogoutHandler(cfg.AdminLoginCfg.CookieSecure))

		// Rotas protegidas por sessão HMAC
		mux.Handle("/admin/api/stats", sessionMW(AdminStatsHandler(cfg.AdminAPICfg)))
		// /admin/api/devices/ subtree: detail + todos os 13 endpoints ISAPI de device-config
		mux.Handle("/admin/api/devices/", sessionMW(adminDevicesRouter(cfg.AdminAPICfg, cfg.DeviceConfigCfg)))
		mux.Handle("/admin/api/devices", sessionMW(AdminDevicesHandler(cfg.AdminAPICfg)))
		mux.Handle("/admin/api/members", sessionMW(AdminMembersHandler(cfg.AdminAPICfg)))
		// Subtree p/ ações por membro: POST /admin/api/members/{id}/resync
		mux.Handle("/admin/api/members/", sessionMW(AdminMemberResyncHandler(cfg.AdminResyncCfg)))
		mux.Handle("/admin/api/events", sessionMW(AdminEventsHandler(cfg.AdminAPICfg)))

		// Sync manual via cookie (wraps o AdminSyncHandler existente com sessão)
		mux.Handle("/admin/api/sync", sessionMW(cfg.AdminHandler))

		// Fluxos de reconhecimento facial (face-flow — FASE 4).
		// Subtree /admin/api/flows/{id}/... primeiro; rota raiz depois.
		if cfg.FlowsAPICfg.FlowRepo != nil {
			mux.Handle("/admin/api/flows/", sessionMW(adminFlowsSubRouter(cfg.FlowsAPICfg)))
			mux.Handle("/admin/api/flows", sessionMW(AdminFlowsRootHandler(cfg.FlowsAPICfg)))
		}

		// Imagens de fundo dos fluxos (face-flow — FASE 4).
		if cfg.BackgroundImgCfg.Repo != nil {
			mux.Handle("/admin/api/background-images/", sessionMW(adminBgImagesSubRouter(cfg.BackgroundImgCfg)))
			mux.Handle("/admin/api/background-images", sessionMW(AdminBackgroundImagesRootHandler(cfg.BackgroundImgCfg)))
		}

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

// adminDevicesRouter roteia /admin/api/devices/* para os handlers corretos.
//
// Routing table (all require SessionMiddleware — applied by caller):
//
//	/admin/api/devices/{id}                            → AdminDeviceDetailHandler
//	/admin/api/devices/{id}/credentials                → PutDeviceCredentialsHandler
//	/admin/api/devices/{id}/actions/reboot             → PostDeviceRebootHandler
//	/admin/api/devices/{id}/actions/factory-reset      → PostDeviceFactoryResetHandler
//	/admin/api/devices/{id}/time                       → GetDeviceTimeHandler / PutDeviceTimeHandler
//	/admin/api/devices/{id}/doors                      → GetDeviceDoorsHandler
//	/admin/api/devices/{id}/doors/{door_id}/status     → GetDeviceDoorStatusHandler
//	/admin/api/devices/{id}/doors/{door_id}/control    → PostDeviceDoorControlHandler
//	/admin/api/devices/{id}/users                      → GetDeviceUsersHandler / DeleteDeviceUsersHandler
//	/admin/api/devices/{id}/faces                      → DeleteDeviceFacesHandler
//	/admin/api/devices/{id}/webhooks                   → GetDeviceWebhooksHandler (GET) / PostDeviceConfigureWebhookHandler (POST)
//	/admin/api/devices/{id}/webhooks/{webhook_id}      → DeleteDeviceWebhookHandler
// adminDevicesRouter routing table (all paths require SessionMiddleware — applied by caller):
//
//	/admin/api/devices/{id}                                          → AdminDeviceDetailHandler
//	/admin/api/devices/{id}/credentials                              → PutDeviceCredentialsHandler
//	/admin/api/devices/{id}/actions/reboot                          → PostDeviceRebootHandler
//	/admin/api/devices/{id}/actions/factory-reset                   → PostDeviceFactoryResetHandler
//	/admin/api/devices/{id}/time                                    → GetDeviceTimeHandler / PutDeviceTimeHandler
//	/admin/api/devices/{id}/doors                                   → GetDeviceDoorsHandler
//	/admin/api/devices/{id}/doors/{door_id}/status                  → GetDeviceDoorStatusHandler
//	/admin/api/devices/{id}/doors/{door_id}/control                 → PostDeviceDoorControlHandler
//	/admin/api/devices/{id}/users                                   → GetDeviceUsersHandler / DeleteDeviceUsersHandler
//	/admin/api/devices/{id}/faces                                   → DeleteDeviceFacesHandler
//	/admin/api/devices/{id}/webhooks                                → GetDeviceWebhooksHandler (GET) / PostDeviceConfigureWebhookHandler (POST)
//	/admin/api/devices/{id}/webhooks/{webhook_id}                   → DeleteDeviceWebhookHandler
//	/admin/api/devices/{id}/stats                                   → GetDeviceStatsHandler (FR-016)
//	/admin/api/devices/{id}/preferences/auth-mode                   → GetDeviceAuthModeHandler / PutDeviceAuthModeHandler (FR-001/002)
//	/admin/api/devices/{id}/preferences/display                     → GetDeviceDisplayHandler / PutDeviceDisplayHandler (FR-003/004)
//	/admin/api/devices/{id}/preferences/display/thumbnails          → GetDeviceDisplayThumbnailsHandler (FR-005)
//	/admin/api/devices/{id}/preferences/standby-pictures            → GetDeviceStandbyPicturesHandler / PostDeviceStandbyPictureHandler (FR-006/007)
//	/admin/api/devices/{id}/preferences/standby-pictures/{uuid}     → DeleteDeviceStandbyPictureHandler (FR-008)
//	/admin/api/devices/{id}/preferences/standby-pictures/disable    → PutDeviceStandbyDisableHandler (FR-009)
//	/admin/api/devices/{id}/preferences/boot-picture                → PostDeviceBootPictureHandler / DeleteDeviceBootPictureHandler (FR-010/011)
//	/admin/api/devices/{id}/preferences/media                       → GetDeviceMediaHandler / PostDeviceMediaHandler / DeleteDeviceMediaAllHandler (FR-012/013/015)
//	/admin/api/devices/{id}/preferences/media/{id}                  → DeleteDeviceMediaItemHandler (FR-014)
//	/admin/api/devices/{id}/preferences/face-config                 → PutDeviceFaceConfigHandler (FR-017)
//	/admin/api/devices/{id}/preferences/face-capture                → PostDeviceFaceCaptureHandler (FR-018)
func adminDevicesRouter(apiCfg AdminAPIConfig, dcCfg DeviceConfigConfig) http.Handler {
	detailHandler := AdminDeviceDetailHandler(apiCfg)

	// Device-config handlers (FASE 4 — tasks.md §4.2–4.6)
	credentialsH := PutDeviceCredentialsHandler(dcCfg)
	rebootH := PostDeviceRebootHandler(dcCfg)
	factoryResetH := PostDeviceFactoryResetHandler(dcCfg)
	getTimeH := GetDeviceTimeHandler(dcCfg)
	putTimeH := PutDeviceTimeHandler(dcCfg)
	getDoorsH := GetDeviceDoorsHandler(dcCfg)
	getDoorStatusH := GetDeviceDoorStatusHandler(dcCfg)
	doorControlH := PostDeviceDoorControlHandler(dcCfg)
	getUsersH := GetDeviceUsersHandler(dcCfg)
	deleteUsersH := DeleteDeviceUsersHandler(dcCfg)
	deleteFacesH := DeleteDeviceFacesHandler(dcCfg)
	getWebhooksH := GetDeviceWebhooksHandler(dcCfg)
	configureWebhookH := PostDeviceConfigureWebhookHandler(dcCfg)
	deleteWebhookH := DeleteDeviceWebhookHandler(dcCfg)

	// Device-preferences handlers (FASE 2/3 — device-preferences tasks.md §FASE 2/3)
	getStatsH := GetDeviceStatsHandler(dcCfg)
	getAuthModeH := GetDeviceAuthModeHandler(dcCfg)
	putAuthModeH := PutDeviceAuthModeHandler(dcCfg)
	getDisplayH := GetDeviceDisplayHandler(dcCfg)
	putDisplayH := PutDeviceDisplayHandler(dcCfg)
	getDisplayThumbH := GetDeviceDisplayThumbnailsHandler(dcCfg)
	getStandbyPicsH := GetDeviceStandbyPicturesHandler(dcCfg)
	postStandbyPicH := PostDeviceStandbyPictureHandler(dcCfg)
	deleteStandbyPicH := DeleteDeviceStandbyPictureHandler(dcCfg)
	putStandbyDisableH := PutDeviceStandbyDisableHandler(dcCfg)
	postBootPicH := PostDeviceBootPictureHandler(dcCfg)
	deleteBootPicH := DeleteDeviceBootPictureHandler(dcCfg)
	getMediaH := GetDeviceMediaHandler(dcCfg)
	postMediaH := PostDeviceMediaHandler(dcCfg)
	deleteMediaItemH := DeleteDeviceMediaItemHandler(dcCfg)
	deleteMediaAllH := DeleteDeviceMediaAllHandler(dcCfg)
	putFaceConfigH := PutDeviceFaceConfigHandler(dcCfg)
	postFaceCaptureH := PostDeviceFaceCaptureHandler(dcCfg)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract sub-path after /admin/api/devices/{id}/
		_, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok {
			// Path didn't match expected prefix — fall through to detail handler for safety
			detailHandler.ServeHTTP(w, r)
			return
		}

		if len(segs) == 0 {
			// /admin/api/devices/{id}
			detailHandler.ServeHTTP(w, r)
			return
		}

		switch segs[0] {
		case "credentials":
			credentialsH.ServeHTTP(w, r)
		case "actions":
			if len(segs) < 2 {
				adminJSONError(w, http.StatusNotFound, "endpoint não encontrado")
				return
			}
			switch segs[1] {
			case "reboot":
				rebootH.ServeHTTP(w, r)
			case "factory-reset":
				factoryResetH.ServeHTTP(w, r)
			default:
				adminJSONError(w, http.StatusNotFound, "ação desconhecida")
			}
		case "time":
			if r.Method == http.MethodGet {
				getTimeH.ServeHTTP(w, r)
			} else if r.Method == http.MethodPut {
				putTimeH.ServeHTTP(w, r)
			} else {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			}
		case "doors":
			if len(segs) == 1 {
				// GET /doors
				getDoorsH.ServeHTTP(w, r)
			} else if len(segs) >= 3 {
				// GET /doors/{door_id}/status  or  POST /doors/{door_id}/control
				switch segs[2] {
				case "status":
					getDoorStatusH.ServeHTTP(w, r)
				case "control":
					doorControlH.ServeHTTP(w, r)
				default:
					adminJSONError(w, http.StatusNotFound, "endpoint de porta desconhecido")
				}
			} else {
				adminJSONError(w, http.StatusNotFound, "endpoint não encontrado")
			}
		case "users":
			if r.Method == http.MethodGet {
				getUsersH.ServeHTTP(w, r)
			} else if r.Method == http.MethodDelete {
				deleteUsersH.ServeHTTP(w, r)
			} else {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			}
		case "faces":
			deleteFacesH.ServeHTTP(w, r)
		case "webhooks":
			if len(segs) == 1 {
				// GET /webhooks (listar) | POST /webhooks (provisionar/reparar)
				if r.Method == http.MethodPost {
					configureWebhookH.ServeHTTP(w, r)
				} else {
					getWebhooksH.ServeHTTP(w, r)
				}
			} else {
				// DELETE /webhooks/{webhook_id}
				deleteWebhookH.ServeHTTP(w, r)
			}
		case "stats":
			// GET /admin/api/devices/{id}/stats — device capacity stats (FR-016)
			if r.Method != http.MethodGet {
				adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				return
			}
			getStatsH.ServeHTTP(w, r)
		case "preferences":
			// /admin/api/devices/{id}/preferences/{sub} — device preference management
			// (FR-001..015,017,018)
			if len(segs) < 2 {
				adminJSONError(w, http.StatusNotFound, "sub-recurso de preferences não especificado")
				return
			}
			switch segs[1] {
			case "auth-mode":
				// GET/PUT /preferences/auth-mode (FR-001/002)
				if r.Method == http.MethodGet {
					getAuthModeH.ServeHTTP(w, r)
				} else if r.Method == http.MethodPut {
					putAuthModeH.ServeHTTP(w, r)
				} else {
					adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				}
			case "display":
				if len(segs) == 2 {
					// GET/PUT /preferences/display (FR-003/004)
					if r.Method == http.MethodGet {
						getDisplayH.ServeHTTP(w, r)
					} else if r.Method == http.MethodPut {
						putDisplayH.ServeHTTP(w, r)
					} else {
						adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
					}
				} else if len(segs) >= 3 && segs[2] == "thumbnails" {
					// GET /preferences/display/thumbnails (FR-005)
					if r.Method != http.MethodGet {
						adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
						return
					}
					getDisplayThumbH.ServeHTTP(w, r)
				} else {
					adminJSONError(w, http.StatusNotFound, "endpoint de display não encontrado")
				}
			case "standby-pictures":
				if len(segs) == 2 {
					// GET/POST /preferences/standby-pictures (FR-006/007)
					if r.Method == http.MethodGet {
						getStandbyPicsH.ServeHTTP(w, r)
					} else if r.Method == http.MethodPost {
						postStandbyPicH.ServeHTTP(w, r)
					} else {
						adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
					}
				} else if len(segs) >= 3 {
					sub := segs[2]
					if sub == "disable" {
						// PUT /preferences/standby-pictures/disable (FR-009)
						if r.Method != http.MethodPut {
							adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
							return
						}
						putStandbyDisableH.ServeHTTP(w, r)
					} else {
						// DELETE /preferences/standby-pictures/{uuid} (FR-008)
						if r.Method != http.MethodDelete {
							adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
							return
						}
						deleteStandbyPicH.ServeHTTP(w, r)
					}
				} else {
					adminJSONError(w, http.StatusNotFound, "endpoint de standby-pictures não encontrado")
				}
			case "boot-picture":
				// POST /preferences/boot-picture (FR-010) | DELETE /preferences/boot-picture (FR-011)
				if r.Method == http.MethodPost {
					postBootPicH.ServeHTTP(w, r)
				} else if r.Method == http.MethodDelete {
					deleteBootPicH.ServeHTTP(w, r)
				} else {
					adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
				}
			case "media":
				if len(segs) == 2 {
					// GET /preferences/media (FR-012) | POST /preferences/media (FR-013)
					// DELETE /preferences/media (FR-015, bulk) — method dispatch
					if r.Method == http.MethodGet {
						getMediaH.ServeHTTP(w, r)
					} else if r.Method == http.MethodPost {
						postMediaH.ServeHTTP(w, r)
					} else if r.Method == http.MethodDelete {
						deleteMediaAllH.ServeHTTP(w, r)
					} else {
						adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
					}
				} else if len(segs) >= 3 {
					// DELETE /preferences/media/{id} (FR-014)
					if r.Method != http.MethodDelete {
						adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
						return
					}
					deleteMediaItemH.ServeHTTP(w, r)
				} else {
					adminJSONError(w, http.StatusNotFound, "endpoint de media não encontrado")
				}
			case "face-config":
				// PUT /preferences/face-config (FR-017)
				if r.Method != http.MethodPut {
					adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
					return
				}
				putFaceConfigH.ServeHTTP(w, r)
			case "face-capture":
				// POST /preferences/face-capture (FR-018)
				if r.Method != http.MethodPost {
					adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
					return
				}
				postFaceCaptureH.ServeHTTP(w, r)
			default:
				adminJSONError(w, http.StatusNotFound, "recurso de preferences não encontrado")
			}
		default:
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
