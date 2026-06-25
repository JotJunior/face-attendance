package httphandler

// Handlers HTTP admin para configuração de dispositivos HikVision.
// Implementa os 13 endpoints novos em /admin/api/devices/{id}/*:
//   - PUT    /credentials               (FR-004/005/007)
//   - POST   /actions/reboot            (FR-008/011)
//   - POST   /actions/factory-reset     (FR-009/011)
//   - GET    /time                      (FR-010)
//   - PUT    /time                      (FR-010/CHK071)
//   - GET    /doors                     (FR-012)
//   - GET    /doors/{door_id}/status    (FR-013/CHK048)
//   - POST   /doors/{door_id}/control   (FR-014/015)
//   - GET    /users                     (FR-016/CHK073)
//   - DELETE /users                     (FR-016b/CHK009)
//   - DELETE /faces                     (FR-017)
//   - GET    /webhooks                  (FR-018)
//   - DELETE /webhooks/{webhook_id}     (FR-019/CHK048)
//
// Auth: todos passam por SessionMiddleware via adminDevicesRouter (server.go:86).
// Nunca logar/serializar isapi_password (FR-005, Constitution §V).
// Ref: contracts/admin-api.md, spec.md §FR-004..019, tasks.md §FASE 4.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

// --- deviceConfigAdminRepo: interface with write operations for device-config handlers ---

// deviceConfigAdminRepo extends the read operations of deviceAdminRepo with write methods
// needed by the 13 new endpoints. The concrete implementation is *repository.DeviceRepository.
type deviceConfigAdminRepo interface {
	GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error)
	SetCredentials(ctx context.Context, id int64, username string, passwordEnc []byte, port int) error
	SetWebhookConfiguredByID(ctx context.Context, id int64, configured bool) error
}

// DeviceConfigConfig holds all dependencies for the device-config handlers.
type DeviceConfigConfig struct {
	DeviceRepo  deviceConfigAdminRepo
	ISAPICipher *secrets.Cipher // nil when ISAPI_CRED_KEY absent → 503 on any ISAPI call
	Logger      interface {
		Info(stage, deviceID, cpfRaw, msg string, extra ...any)
		Error(stage, deviceID, cpfRaw, msg string, err error, extra ...any)
	}

	// Provisionamento de webhook (POST /webhooks): endereço público que o device
	// usa para POSTar eventos. WebhookPublicHost vazio → 400 (config ausente).
	WebhookPublicHost string
	WebhookPublicPort int    // default 8080 quando <= 0
	WebhookPathSecret string // compõe o path /webhook/{secret}
}

// --- ISAPI error mapper ---

// mapISAPIError maps hikvision client errors to HTTP status + user-facing message.
// CHK007: ErrKeyMissing → 503. NonRetriableError (4xx from ISAPI) → 502.
// Context timeout/cancelled or network error → 504 (connectivity).
// ErrNotImplemented → 501. ErrUnknownCommand → 400.
func mapISAPIError(err error) (int, string) {
	if err == nil {
		return 0, ""
	}
	if errors.Is(err, hikvision.ErrKeyMissing) {
		return http.StatusServiceUnavailable,
			"credenciais ISAPI não configuradas ou chave de cifragem ausente (ISAPI_CRED_KEY)"
	}
	if errors.Is(err, hikvision.ErrNotImplemented) {
		return http.StatusNotImplemented,
			"funcionalidade não implementada — endpoint ISAPI aguarda verificação empírica no firmware"
	}
	if errors.Is(err, hikvision.ErrUnknownCommand) {
		return http.StatusBadRequest, "comando de porta inválido"
	}
	var nre *hikvision.NonRetriableError
	if errors.As(err, &nre) {
		return http.StatusBadGateway,
			fmt.Sprintf("dispositivo retornou erro de autenticação ou lógica (HTTP %d)", nre.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return http.StatusGatewayTimeout,
			"timeout ao comunicar com o dispositivo — verificar conectividade"
	}
	if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection refused") {
		return http.StatusGatewayTimeout, "timeout ou falha de conexão ao dispositivo"
	}
	return http.StatusBadGateway, "falha ao comunicar com o dispositivo"
}

// loadDeviceAndISAPIClient fetches the device by ID and constructs a hikvision.Client.
// Returns (device, client, 0, "") on success, or (nil, nil, httpStatus, errMsg) on error.
// CHK007: ErrKeyMissing when cipher is nil or credentials absent → caller writes 503.
func loadDeviceAndISAPIClient(
	ctx context.Context,
	cfg DeviceConfigConfig,
	deviceID int64,
) (*domain.Device, *hikvision.Client, int, string) {
	device, err := cfg.DeviceRepo.GetDeviceByID(ctx, deviceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, http.StatusNotFound, "dispositivo não encontrado"
		}
		return nil, nil, http.StatusServiceUnavailable, "serviço temporariamente indisponível"
	}

	devCfg, err := hikvision.LoadDeviceConfig(device, cfg.ISAPICipher)
	if err != nil {
		st, msg := mapISAPIError(err)
		return nil, nil, st, msg
	}

	client := hikvision.New(devCfg)
	return device, client, 0, ""
}

// --- path helpers specific to device-config sub-routes ---

// deviceConfigPathSegments returns path segments after /admin/api/devices/{id}/.
// E.g. "/admin/api/devices/42/doors/1/status" → ["doors","1","status"]
// E.g. "/admin/api/devices/42/credentials"    → ["credentials"]
func deviceConfigPathSegments(path string) (deviceID int64, segs []string, ok bool) {
	const prefix = "/admin/api/devices/"
	rest := strings.TrimPrefix(path, prefix)
	if rest == path { // prefix not found
		return 0, nil, false
	}
	rest = strings.Trim(rest, "/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, nil, false
	}
	id, err := parsePositiveInt64(parts[0])
	if err != nil {
		return 0, nil, false
	}
	if len(parts) == 1 {
		return id, nil, true
	}
	sub := strings.Trim(parts[1], "/")
	if sub == "" {
		return id, nil, true
	}
	return id, strings.Split(sub, "/"), true
}

// parsePositiveInt64 parses s as a positive int64.
func parsePositiveInt64(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	var n int64
	_, err := fmt.Sscan(s, &n)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("not a positive integer: %q", s)
	}
	return n, nil
}

// deviceIDFromPath extracts {id} from /admin/api/devices/{id}/... paths.
func deviceIDFromPath(path string) (int64, bool) {
	id, _, ok := deviceConfigPathSegments(path)
	return id, ok
}

// logInfo logs an info event if logger is configured (best-effort; never panics).
func (cfg DeviceConfigConfig) logInfo(stage string, deviceID int64, msg string, extra ...any) {
	if cfg.Logger == nil {
		return
	}
	cfg.Logger.Info(stage, fmt.Sprintf("%d", deviceID), "", msg, extra...)
}

// logError logs an error event if logger is configured (best-effort; never panics).
func (cfg DeviceConfigConfig) logError(stage string, deviceID int64, msg string, err error, extra ...any) {
	if cfg.Logger == nil {
		return
	}
	cfg.Logger.Error(stage, fmt.Sprintf("%d", deviceID), "", msg, err, extra...)
}

// =============================================================================
// 4.2 — PUT /admin/api/devices/{id}/credentials
// =============================================================================

// putCredentialsRequest is the request body for PUT /credentials.
type putCredentialsRequest struct {
	ISAPIUsername string `json:"isapi_username"`
	ISAPIPassword string `json:"isapi_password"`
	ISAPIPort     int    `json:"isapi_port"`
}

// putCredentialsResponse is the response body — NEVER includes isapi_password (FR-005).
type putCredentialsResponse struct {
	IsapiCredentialsSet bool `json:"isapi_credentials_set"`
	IsapiPort           int  `json:"isapi_port"`
}

// PutDeviceCredentialsHandler serves PUT /admin/api/devices/{id}/credentials.
// FR-004: persist encrypted ISAPI credentials. FR-005: never echo password.
// FR-007 / CHK007: 503 when ISAPI_CRED_KEY absent (cipher nil).
// Log: NEVER include request body (FR-005, plan.md §Recomendações).
func PutDeviceCredentialsHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, _, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		// CHK007 / FR-007: cipher must be present before any write
		if cfg.ISAPICipher == nil {
			adminJSONError(w, http.StatusServiceUnavailable,
				"credenciais ISAPI não podem ser salvas: ISAPI_CRED_KEY ausente no servidor")
			return
		}

		var req putCredentialsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}

		if req.ISAPIUsername == "" {
			adminJSONError(w, http.StatusBadRequest, "isapi_username é obrigatório")
			return
		}
		if req.ISAPIPassword == "" {
			adminJSONError(w, http.StatusBadRequest, "isapi_password é obrigatório")
			return
		}
		// FR-004: validate port range 1–65535
		if req.ISAPIPort < 1 || req.ISAPIPort > 65535 {
			adminJSONError(w, http.StatusBadRequest, "isapi_port deve estar entre 1 e 65535")
			return
		}

		ctx := r.Context()

		// Verify device exists (FR-023)
		_, err := cfg.DeviceRepo.GetDeviceByID(ctx, deviceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				adminJSONError(w, http.StatusNotFound, "dispositivo não encontrado")
				return
			}
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		// Encrypt password — FR-005: never persist plaintext; variable is short-lived, never logged
		enc, err := cfg.ISAPICipher.Encrypt(req.ISAPIPassword)
		if err != nil {
			adminJSONError(w, http.StatusInternalServerError, "falha ao cifrar credencial")
			return
		}

		// Persist via SetCredentials (SOURCED: device_repository.go:248)
		if err := cfg.DeviceRepo.SetCredentials(ctx, deviceID, req.ISAPIUsername, enc, req.ISAPIPort); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				adminJSONError(w, http.StatusNotFound, "dispositivo não encontrado")
				return
			}
			adminJSONError(w, http.StatusServiceUnavailable, "falha ao persistir credenciais")
			return
		}

		// Log: only device_id and stage — NEVER username/password (FR-005, Constitution §V)
		cfg.logInfo("credentials", deviceID, "credenciais ISAPI atualizadas",
			"stage", "credentials", "action", "set_credentials")

		resp := putCredentialsResponse{IsapiCredentialsSet: true, IsapiPort: req.ISAPIPort}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// =============================================================================
// 4.3 — Sistema: reboot, factory-reset, time
// =============================================================================

// PostDeviceRebootHandler serves POST /admin/api/devices/{id}/actions/reboot.
// FR-008: reboot via ISAPI. FR-011: structured log with device_id, stage, action, operator.
func PostDeviceRebootHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		// FR-011: log before action for auditability
		cfg.logInfo("device-config", deviceID, "reboot solicitado",
			"action", "reboot", "stage", "system", "operator", "admin")

		if err := client.Reboot(ctx); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("device-config", deviceID, "reboot falhou", err, "action", "reboot")
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("device-config", deviceID, "reboot iniciado com sucesso",
			"action", "reboot", "stage", "system", "operator", "admin")

		resp := actionResponse{Result: "rebooting", DeviceID: deviceID}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// PostDeviceFactoryResetHandler serves POST /admin/api/devices/{id}/actions/factory-reset.
// FR-009: factory reset via ISAPI. Post-success: webhook_configured=false in DB.
// FR-011: structured log marked as irreversível.
func PostDeviceFactoryResetHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		// FR-011: log before action — irreversível
		cfg.logInfo("device-config", deviceID, "factory-reset solicitado",
			"action", "factory_reset", "stage", "system", "operator", "admin",
			"irreversivel", true)

		if err := client.FactoryReset(ctx); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("device-config", deviceID, "factory-reset falhou", err,
				"action", "factory_reset")
			adminJSONError(w, st, msg)
			return
		}

		// Post-success: update webhook_configured=false (factory reset wipes config)
		if err := cfg.DeviceRepo.SetWebhookConfiguredByID(ctx, deviceID, false); err != nil {
			// Non-fatal: log and continue — ISAPI success is the primary outcome
			cfg.logError("device-config", deviceID, "falha ao atualizar webhook_configured pós factory-reset", err)
		}

		cfg.logInfo("device-config", deviceID, "factory-reset concluído",
			"action", "factory_reset", "stage", "system", "operator", "admin",
			"irreversivel", true, "webhook_configured", false)

		f := false
		resp := actionResponse{Result: "factory_reset_initiated", DeviceID: deviceID, WebhookConfigured: &f}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// getTimeResponse is the response for GET /time.
type getTimeResponse struct {
	LocalTime string `json:"local_time"`
	TimeZone  string `json:"time_zone"`
	TimeMode  string `json:"time_mode"` // normalizado p/ minúsculo ("ntp"|"manual")
	NTPServer string `json:"ntp_server,omitempty"`
	NTPPort   int    `json:"ntp_port,omitempty"`
}

// GetDeviceTimeHandler serves GET /admin/api/devices/{id}/time.
// FR-010: retrieve device time via ISAPI.
func GetDeviceTimeHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		td, err := client.GetTime(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		resp := getTimeResponse{
			LocalTime: td.LocalTime,
			TimeZone:  td.TimeZone,
			TimeMode:  strings.ToLower(td.TimeMode),
		}
		// Em modo NTP, busca o servidor configurado p/ pré-popular o form
		// (best-effort — não falha o GET se a leitura do servidor falhar).
		if strings.EqualFold(td.TimeMode, "ntp") {
			if ns, nerr := client.GetNTPServer(ctx, 1); nerr == nil && ns != nil {
				resp.NTPServer = ns.HostName
				resp.NTPPort = ns.PortNo
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// putTimeRequest is the request body for PUT /time.
// NTP server fields SOURCED from device test (192.168.68.107, 2026-06-21, HTTP 200):
//   ntp_server: hostname or IP of NTP server
//   ntp_port: UDP port, default 123
//   ntp_sync_interval: sync interval in minutes, default 60
//   ntp_addressing_type: "hostname" or "ipaddress", default "hostname"
type putTimeRequest struct {
	TimeMode           string `json:"time_mode"`            // "manual" | "ntp" (CHK071: validated)
	LocalTime          string `json:"local_time"`           // required for manual mode
	TimeZone           string `json:"time_zone"`            // optional
	NTPServer          string `json:"ntp_server"`           // NTP hostname/IP (for ntp mode)
	NTPPort            int    `json:"ntp_port"`             // UDP port, default 123
	NTPSyncInterval    int    `json:"ntp_sync_interval"`    // sync interval minutes, default 60
	NTPAddressingType  string `json:"ntp_addressing_type"`  // "hostname" | "ipaddress", default "hostname"
}

// PutDeviceTimeHandler serves PUT /admin/api/devices/{id}/time.
// FR-010: set device time via ISAPI. CHK071: time_mode enum validated.
// NTP mode: SOURCED from device test (192.168.68.107, 2026-06-21):
//   1. PUT /ISAPI/System/time (XML, timeMode=NTP + timeZone) → 200
//   2. If ntp_server provided: PUT /ISAPI/System/time/ntpServers/1 (XML) → 200
func PutDeviceTimeHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		var req putTimeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}

		// CHK071: validate time_mode enum
		if req.TimeMode != "manual" && req.TimeMode != "ntp" {
			adminJSONError(w, http.StatusBadRequest,
				"time_mode deve ser 'manual' ou 'ntp'")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		setReq := hikvision.TimeSetRequest{
			TimeMode:  req.TimeMode,
			LocalTime: req.LocalTime,
			TimeZone:  req.TimeZone,
		}

		if err := client.SetTime(ctx, setReq); err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// NTP mode: optionally configure NTP server (SOURCED: device test 2026-06-21).
		if req.TimeMode == "ntp" && req.NTPServer != "" {
			ntpReq := hikvision.NTPServerRequest{
				ID:                   1,
				AddressingFormatType: req.NTPAddressingType,
				HostName:             req.NTPServer,
				PortNo:               req.NTPPort,
				SynchronizeInterval:  req.NTPSyncInterval,
			}
			if err := client.SetNTPServer(ctx, ntpReq); err != nil {
				st, msg := mapISAPIError(err)
				adminJSONError(w, st, "erro ao configurar servidor NTP: "+msg)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "ok"}) //nolint:errcheck
	})
}

// =============================================================================
// 4.4 — Doors: capabilities, status, control
// =============================================================================

// doorResponse is a single door item in the capabilities list.
type doorResponse struct {
	DoorID      int    `json:"door_id"`
	DoorName    string `json:"door_name"`
	ReaderCount int    `json:"reader_count"`
}

// doorsListResponse is the response for GET /doors.
type doorsListResponse struct {
	Doors []doorResponse `json:"doors"`
	Total int            `json:"total"`
}

// GetDeviceDoorsHandler serves GET /admin/api/devices/{id}/doors.
// FR-012: list door capabilities from ISAPI.
func GetDeviceDoorsHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		doors, err := client.GetDoorCapabilities(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		items := make([]doorResponse, 0, len(doors))
		for _, d := range doors {
			items = append(items, doorResponse{
				DoorID:      d.DoorNo,
				DoorName:    d.DoorName,
				ReaderCount: 1, // HikVision access control: 1 reader per door (SOURCED: DoorService.php)
			})
		}

		resp := doorsListResponse{Doors: items, Total: len(items)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// doorStatusResponse is the response for GET /doors/{door_id}/status.
type doorStatusResponse struct {
	DoorID       int    `json:"door_id"`
	DoorState    string `json:"door_state"`    // enum observed from device (CHK055: not presumed)
	LockState    string `json:"lock_state"`    // "locked" | "unlocked" | "unknown"
	OpenDuration int    `json:"open_duration"` // seconds; from DoorConfig
}

// GetDeviceDoorStatusHandler serves GET /admin/api/devices/{id}/doors/{door_id}/status.
// FR-013: get door status from ISAPI. CHK048: door_id validated ≥ 1.
func GetDeviceDoorStatusHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		// Path: /admin/api/devices/{id}/doors/{door_id}/status
		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 3 || segs[0] != "doors" || segs[2] != "status" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		doorID, valid := parseDoorID(segs[1])
		if !valid {
			adminJSONError(w, http.StatusBadRequest, "door_id inválido: deve ser inteiro >= 1")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		ds, err := client.GetDoorStatus(ctx, doorID)
		if err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// Also fetch open_duration from door config (best-effort; 0 if unavailable)
		openDuration := 0
		if dc, err := client.GetDoorConfig(ctx, doorID); err == nil {
			openDuration = dc.OpenDuration
		}

		resp := doorStatusResponse{
			DoorID:       ds.DoorNo,
			DoorState:    "unknown", // DoorStatus from ISAPI does not expose door_state directly
			LockState:    ds.LockState,
			OpenDuration: openDuration,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// postDoorControlRequest is the request body for POST /doors/{door_id}/control.
type postDoorControlRequest struct {
	Command string `json:"command"` // "open"|"close"|"always_open"|"always_closed"|"normal"
}

// PostDeviceDoorControlHandler serves POST /admin/api/devices/{id}/doors/{door_id}/control.
// FR-014: control door via ISAPI. FR-015: distinct 504 (connectivity) vs 502 (device logic).
// CHK058: device_id included in response.
func PostDeviceDoorControlHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		// Path: /admin/api/devices/{id}/doors/{door_id}/control
		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 3 || segs[0] != "doors" || segs[2] != "control" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		doorID, valid := parseDoorID(segs[1])
		if !valid {
			adminJSONError(w, http.StatusBadRequest, "door_id inválido: deve ser inteiro >= 1")
			return
		}

		var req postDoorControlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}
		if req.Command == "" {
			adminJSONError(w, http.StatusBadRequest, "command é obrigatório")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.ControlDoor(ctx, doorID, req.Command); err != nil {
			// FR-015: ErrUnknownCommand → 400; connectivity → 504; device logic → 502
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// CHK058: include device_id in response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result":    "ok",
			"command":   req.Command,
			"device_id": deviceID,
		}) //nolint:errcheck
	})
}

// =============================================================================
// 4.5 — Users: list paginado, clear users, clear faces
// =============================================================================

// usersListResponse is the response for GET /users.
type usersListResponse struct {
	Users   []hikvision.DeviceUser `json:"users"`
	Total   int                    `json:"total"`
	Page    int                    `json:"page"`
	PerPage int                    `json:"per_page"`
}

// GetDeviceUsersHandler serves GET /admin/api/devices/{id}/users?page=1&per_page=100.
// FR-016: list users from ISAPI. CHK073: per_page capped 1–1000; default 100.
// CHK072: searchID generated backend-side (never from query param).
// Note: users[] preserves ISAPI camelCase field names (admin-api.md §Nota de borda).
func GetDeviceUsersHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		// CHK073: validate pagination params
		page := queryInt(r, "page", 1)
		perPage := queryInt(r, "per_page", 100)
		if page < 1 {
			adminJSONError(w, http.StatusBadRequest, "page deve ser >= 1")
			return
		}
		if perPage < 1 || perPage > 1000 {
			adminJSONError(w, http.StatusBadRequest, "per_page deve estar entre 1 e 1000")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		// CHK072: searchID generated internally in ListUsers (never from query param)
		users, total, err := client.ListUsers(ctx, page, perPage)
		if err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		if users == nil {
			users = []hikvision.DeviceUser{}
		}

		resp := usersListResponse{
			Users:   users,
			Total:   total,
			Page:    page,
			PerPage: perPage,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// DeleteDeviceUsersHandler serves DELETE /admin/api/devices/{id}/users.
// FR-016b: clear all users from ISAPI (atomic operation — CHK009).
// CHK009: ClearUsers is atomic on ISAPI; timeout → 504 with action guidance.
func DeleteDeviceUsersHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("device-config", deviceID, "limpeza de usuários solicitada",
			"action", "clear_users", "stage", "users", "operator", "admin")

		if err := client.ClearUsers(ctx); err != nil {
			// CHK009: operação atômica no ISAPI; timeout retorna 504 com orientação para verificação manual
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
				strings.Contains(err.Error(), "timeout") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusGatewayTimeout)
				fmt.Fprintf(w, `{"error":"timeout ao limpar usuários","action":"verificar dispositivo manualmente"}`)
				return
			}
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("device-config", deviceID, "usuários removidos com sucesso",
			"action", "clear_users", "stage", "users", "operator", "admin")

		resp := actionResponse{Result: "cleared", DeviceID: deviceID}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// DeleteDeviceFacesHandler serves DELETE /admin/api/devices/{id}/faces.
// FR-017: clear face library from ISAPI.
// SOURCED: FaceService.php:38 (ENDPOINT_FACE_CLEAR) + FaceService.php:283 (clear() method):
//   PUT /ISAPI/AccessControl/ClearPictureCfg?format=json with ClearFlags facePicture+capOrVerifyPicture.
func DeleteDeviceFacesHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.ClearFaces(ctx); err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		resp := actionResponse{Result: "faces_cleared", DeviceID: deviceID}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// =============================================================================
// 4.6 — Webhooks: list e delete
// =============================================================================

// webhookResponse is a single webhook item.
type webhookResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Protocol string `json:"protocol"`
}

// webhooksListResponse is the response for GET /webhooks.
type webhooksListResponse struct {
	Webhooks []webhookResponse `json:"webhooks"`
	Total    int               `json:"total"`
}

// PostDeviceConfigureWebhookHandler serves POST /admin/api/devices/{id}/webhooks.
// Provisiona/repara o HTTP notification host no device para que ele POSTe eventos
// no endereço público do app — destrava leitores que não dão heartbeat por estarem
// sem httpHost (ou apontando para o lugar errado).
//
// Requer WebhookPublicHost configurado (env WEBHOOK_PUBLIC_HOST). Vazio → 400, com
// mensagem explicando a config ausente (nunca inventa o IP — Princípio I).
// Sucesso: webhook_configured=true no banco; resposta { result:"configured", ... }.
func PostDeviceConfigureWebhookHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		if cfg.WebhookPublicHost == "" {
			adminJSONError(w, http.StatusBadRequest,
				"endereço público do webhook não configurado — defina WEBHOOK_PUBLIC_HOST (IP do app alcançável pelos leitores na LAN)")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		port := cfg.WebhookPublicPort
		if port <= 0 {
			port = 8080
		}
		path := "/webhook/" + cfg.WebhookPathSecret

		if err := client.ProvisionWebhook(ctx, cfg.WebhookPublicHost, port, path); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("device-config", deviceID, "falha ao provisionar webhook no device", err)
			adminJSONError(w, st, msg)
			return
		}

		if err := cfg.DeviceRepo.SetWebhookConfiguredByID(ctx, deviceID, true); err != nil {
			cfg.logError("device-config", deviceID, "webhook provisionado mas falha ao gravar webhook_configured", err)
		}
		cfg.logInfo("device-config", deviceID, "webhook provisionado no device")

		t := true
		resp := actionResponse{Result: "configured", DeviceID: deviceID, WebhookConfigured: &t}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// GetDeviceWebhooksHandler serves GET /admin/api/devices/{id}/webhooks.
// FR-018: list HTTP notification hosts from ISAPI.
func GetDeviceWebhooksHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		hooks, err := client.ListWebhooks(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		items := make([]webhookResponse, 0, len(hooks))
		for _, h := range hooks {
			items = append(items, webhookResponse{ID: h.ID, URL: h.URL, Protocol: h.Protocol})
		}

		resp := webhooksListResponse{Webhooks: items, Total: len(items)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// DeleteDeviceWebhookHandler serves DELETE /admin/api/devices/{id}/webhooks/{webhook_id}.
// FR-019: delete webhook from ISAPI. CHK048: webhook_id validated (non-empty, no path traversal).
// Post-success: if webhook_id == deterministicHostID(device.Host) → webhook_configured=false.
// CHK058: webhook_configured and device_id in response.
func DeleteDeviceWebhookHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		// Path: /admin/api/devices/{id}/webhooks/{webhook_id}
		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 2 || segs[0] != "webhooks" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		webhookID, valid := parseWebhookID(segs[1])
		if !valid {
			adminJSONError(w, http.StatusBadRequest,
				"webhook_id inválido: não pode ser vazio ou conter caracteres de path traversal")
			return
		}

		ctx := r.Context()
		device, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.DeleteWebhook(ctx, webhookID); err != nil {
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// FR-019: if deleted webhook is the main system webhook, update webhook_configured=false.
		// The main webhook ID is deterministic per device host (SHA-256 of "ip:port").
		webhookConfigured := device.WebhookConfigured
		if device.IPAddress != nil {
			port := device.ISAPIPort
			if port <= 0 {
				port = 80
			}
			deviceHost := fmt.Sprintf("%s:%d", *device.IPAddress, port)
			if hikvision.DeterministicWebhookID(deviceHost) == webhookID {
				webhookConfigured = false
				if err := cfg.DeviceRepo.SetWebhookConfiguredByID(ctx, deviceID, false); err != nil {
					cfg.logError("device-config", deviceID,
						"falha ao atualizar webhook_configured após remoção do webhook principal", err)
				}
			}
		}

		t := webhookConfigured
		resp := actionResponse{Result: "removed", DeviceID: deviceID, WebhookConfigured: &t}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}
