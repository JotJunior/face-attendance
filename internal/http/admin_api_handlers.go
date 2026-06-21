package httphandler

// Handlers da API REST do painel de administração web.
// Todos os endpoints desta camada requerem autenticação via SessionMiddleware (cookie HMAC).
// Nenhum handler loga CPF completo nem expõe raw_payload (Constitution §VI, CHK-S13).
// Ref: spec.md §FR-003/004/005/006/007, contracts/admin-api.md, tasks.md §2.5.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
)

// --- Interfaces dos repositórios consumidos pelos handlers admin ---

// memberAdminRepo define os métodos do MemberRepository usados pelos handlers admin.
type memberAdminRepo interface {
	CountMembersWithSelfie(ctx context.Context) (int, error)
	ListMembersPaged(ctx context.Context, q string, cursor int, limit int) ([]domain.MemberView, int, bool, error)
}

// deviceAdminRepo define os métodos do DeviceRepository usados pelos handlers admin.
type deviceAdminRepo interface {
	CountDevicesByActivity(ctx context.Context, thresholdHours int) (active, inactive int, err error)
	ListDevicesAll(ctx context.Context) ([]domain.Device, error)
	GetDeviceByID(ctx context.Context, id int64) (*domain.Device, error)
}

// attendanceAdminRepo define os métodos do AttendanceEventRepository usados pelos handlers admin.
type attendanceAdminRepo interface {
	CountAttendanceSince(ctx context.Context, since time.Time) (int, error)
	ListEventsPaged(ctx context.Context, from, to time.Time, cursor domain.CursorEvt, limit int) ([]domain.EventView, domain.CursorEvt, bool, error)
}

// AdminAPIConfig agrupa as dependências necessárias para os handlers da API admin.
type AdminAPIConfig struct {
	MemberRepo              memberAdminRepo
	DeviceRepo              deviceAdminRepo
	AttendanceRepo          attendanceAdminRepo
	DeviceOfflineThreshold  int // horas — DEVICE_OFFLINE_THRESHOLD_HOURS
	Logger                  *logging.Logger
}

// --- Stats handler ---

// statsResponse é o payload de resposta de GET /admin/api/stats.
// Ref: contracts/admin-api.md §GET /admin/api/stats.
type statsResponse struct {
	MembersWithSelfie          int `json:"members_with_selfie"`
	DevicesActive              int `json:"devices_active"`
	DevicesInactive            int `json:"devices_inactive"`
	AttendanceLast24h          int `json:"attendance_last_24h"`
	DeviceOfflineThresholdHours int `json:"device_offline_threshold_hours"`
}

// AdminStatsHandler serve GET /admin/api/stats.
// Retorna 503 JSON se DB inacessível (CHK-A08).
func AdminStatsHandler(cfg AdminAPIConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		ctx := r.Context()
		since := time.Now().UTC().Add(-24 * time.Hour)

		membersWithSelfie, err := cfg.MemberRepo.CountMembersWithSelfie(ctx)
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		devicesActive, devicesInactive, err := cfg.DeviceRepo.CountDevicesByActivity(ctx, cfg.DeviceOfflineThreshold)
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		attendanceLast24h, err := cfg.AttendanceRepo.CountAttendanceSince(ctx, since)
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		resp := statsResponse{
			MembersWithSelfie:           membersWithSelfie,
			DevicesActive:               devicesActive,
			DevicesInactive:             devicesInactive,
			AttendanceLast24h:           attendanceLast24h,
			DeviceOfflineThresholdHours: cfg.DeviceOfflineThreshold,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// --- Devices handlers ---

// deviceResponse é o item de resposta para um dispositivo no painel admin.
// status é derivado de last_heartbeat_at vs threshold (não armazenado em banco — dec-007).
type deviceResponse struct {
	ID                          int64      `json:"id"`
	DeviceIdentifier            string     `json:"device_identifier"`
	IPAddress                   *string    `json:"ip_address,omitempty"`
	MACAddress                  *string    `json:"mac_address,omitempty"`
	LastHeartbeatAt             *time.Time `json:"last_heartbeat_at,omitempty"`
	Status                      string     `json:"status"` // "active" | "offline"
	WebhookConfigured           bool       `json:"webhook_configured"`
	CreatedAt                   time.Time  `json:"created_at"`
	// Telemetria de hardware via ISAPI deviceInfo (nullable até a 1ª leitura).
	// Credenciais ISAPI NÃO são expostas aqui (segurança).
	SerialNumber                *string    `json:"serial_number,omitempty"`
	Model                       *string    `json:"model,omitempty"`
	FirmwareVersion             *string    `json:"firmware_version,omitempty"`
}

// devicesListResponse é o payload de resposta de GET /admin/api/devices.
type devicesListResponse struct {
	Devices                     []deviceResponse `json:"devices"`
	DeviceOfflineThresholdHours int              `json:"device_offline_threshold_hours"`
}

// deriveDeviceStatus deriva o status de um dispositivo baseado no último heartbeat.
func deriveDeviceStatus(d domain.Device, thresholdHours int) string {
	if d.LastHeartbeatAt == nil {
		return "offline"
	}
	cutoff := time.Now().UTC().Add(-time.Duration(thresholdHours) * time.Hour)
	if d.LastHeartbeatAt.Before(cutoff) {
		return "offline"
	}
	return "active"
}

// toDeviceResponse converte domain.Device para deviceResponse com status derivado.
func toDeviceResponse(d domain.Device, thresholdHours int) deviceResponse {
	return deviceResponse{
		ID:                id64(d.ID),
		DeviceIdentifier:  d.DeviceIdentifier,
		IPAddress:         d.IPAddress,
		MACAddress:        d.MACAddress,
		LastHeartbeatAt:   d.LastHeartbeatAt,
		Status:            deriveDeviceStatus(d, thresholdHours),
		WebhookConfigured: d.WebhookConfigured,
		CreatedAt:         d.CreatedAt,
		SerialNumber:      d.SerialNumber,
		Model:             d.Model,
		FirmwareVersion:   d.FirmwareVersion,
	}
}

// AdminDevicesHandler serve GET /admin/api/devices.
// Lista todos os dispositivos com status derivado (CHK-A08 para erros de DB).
func AdminDevicesHandler(cfg AdminAPIConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		devices, err := cfg.DeviceRepo.ListDevicesAll(r.Context())
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		var items []deviceResponse
		for _, d := range devices {
			items = append(items, toDeviceResponse(d, cfg.DeviceOfflineThreshold))
		}
		if items == nil {
			items = []deviceResponse{} // array vazio válido (FR-009)
		}

		resp := devicesListResponse{
			Devices:                     items,
			DeviceOfflineThresholdHours: cfg.DeviceOfflineThreshold,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// AdminDeviceDetailHandler serve GET /admin/api/devices/{id}.
// Extrai o id do path; retorna 404 JSON se não encontrado; sem histórico (dec-007).
func AdminDeviceDetailHandler(cfg AdminAPIConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		// Extrair {id} do path: /admin/api/devices/42
		idStr := extractLastPathSegment(r.URL.Path)
		deviceID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || deviceID <= 0 {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		device, err := cfg.DeviceRepo.GetDeviceByID(r.Context(), deviceID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				adminJSONError(w, http.StatusNotFound, "dispositivo não encontrado")
				return
			}
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		resp := toDeviceResponse(*device, cfg.DeviceOfflineThreshold)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// --- Members handler ---

// membersListResponse é o payload de resposta de GET /admin/api/members.
type membersListResponse struct {
	Members    []domain.MemberView `json:"members"`
	NextCursor *int                `json:"next_cursor"`
	HasMore    bool                `json:"has_more"`
}

// AdminMembersHandler serve GET /admin/api/members.
// Suporta q= (busca), cursor= (paginação keyset), limit= (default=50, teto=200).
func AdminMembersHandler(cfg AdminAPIConfig) http.Handler {
	const (
		defaultMembersLimit = 50
		maxMembersLimit     = 200
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		q := r.URL.Query().Get("q")
		cursor := queryInt(r, "cursor", 0)
		limit := clampInt(queryInt(r, "limit", defaultMembersLimit), 1, maxMembersLimit)

		members, nextCursor, hasMore, err := cfg.MemberRepo.ListMembersPaged(r.Context(), q, cursor, limit)
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		if members == nil {
			members = []domain.MemberView{} // array vazio válido
		}

		resp := membersListResponse{
			Members: members,
			HasMore: hasMore,
		}
		if hasMore {
			resp.NextCursor = &nextCursor
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// --- Events handler ---

// eventsListResponse é o payload de resposta de GET /admin/api/events.
// nextCursor é nulo na última página.
type eventsListResponse struct {
	Events     []domain.EventView  `json:"events"`
	NextCursor *domain.CursorEvt   `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
}

// AdminEventsHandler serve GET /admin/api/events.
// Suporta from=, to= (RFC3339 ou date YYYY-MM-DD), cursor= (JSON base64 do CursorEvt), limit=.
func AdminEventsHandler(cfg AdminAPIConfig) http.Handler {
	const (
		defaultEventsLimit = 100
		maxEventsLimit     = 500
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		from := parseTimeParam(r.URL.Query().Get("from"))
		to := parseTimeParam(r.URL.Query().Get("to"))
		limit := clampInt(queryInt(r, "limit", defaultEventsLimit), 1, maxEventsLimit)

		// Cursor keyset: JSON {"created_at":"...","id":N} em base64url
		var cursor domain.CursorEvt
		if raw := r.URL.Query().Get("cursor"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &cursor); err != nil {
				adminJSONError(w, http.StatusBadRequest, "cursor inválido")
				return
			}
		}

		events, nextCursor, hasMore, err := cfg.AttendanceRepo.ListEventsPaged(r.Context(), from, to, cursor, limit)
		if err != nil {
			adminJSONError(w, http.StatusServiceUnavailable, "serviço temporariamente indisponível")
			return
		}

		if events == nil {
			events = []domain.EventView{} // array vazio válido
		}

		resp := eventsListResponse{
			Events:  events,
			HasMore: hasMore,
		}
		if hasMore && !nextCursor.IsZero() {
			resp.NextCursor = &nextCursor
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// --- helpers internos ---

// queryInt extrai um parâmetro inteiro do query string com fallback para defaultVal.
func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

// clampInt restringe v ao intervalo [min, max].
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// parseTimeParam parseia um parâmetro de data como RFC3339 ou YYYY-MM-DD.
// Retorna zero time.Time se vazio ou inválido.
func parseTimeParam(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Tentar RFC3339 primeiro
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	// Tentar date-only YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// extractLastPathSegment retorna o segmento final do path URL.
// Ex: "/admin/api/devices/42" → "42"
func extractLastPathSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// id64 converte int64 para int64 (helper semântico para legibilidade).
func id64(id int64) int64 { return id }
