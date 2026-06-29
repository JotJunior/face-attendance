package httphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// DeviceRegistrar is the interface the heartbeat handler uses to register devices
// and resolver o id do device (para vincular ao evento de presença).
type DeviceRegistrar interface {
	Upsert(ctx context.Context, d domain.Device) error
	FindByMAC(ctx context.Context, mac string) (*domain.Device, error)
}

// MemberFinder is the interface the webhook handler uses to look up members by CPF.
type MemberFinder interface {
	FindByCPF(ctx context.Context, cpfDigits string) (*domain.Member, error)
}

// AttendanceMarker is the interface the webhook handler uses to mark attendance.
type AttendanceMarker interface {
	MarkAttendance(ctx context.Context, cpfDigits string) error
}

// EventRecorder records attendance events to the database.
type EventRecorder interface {
	InsertIfNotExists(ctx context.Context, e domain.AttendanceEvent) (bool, error)
	MarkAsMarked(ctx context.Context, eventKey string) error
}

// HealthChecker checks connectivity to PostgreSQL and RabbitMQ.
type HealthChecker interface {
	PingDB(ctx context.Context) error
	PingRabbitMQ() error
}

// Scheduler triggers a member load cycle.
type Scheduler interface {
	RunMemberLoadCycle(ctx context.Context) error
}

// FlowEngine é a interface do motor de fluxo que o webhook aciona após processar o evento.
// Nil-safe: o handler verifica se o campo é nil antes de chamar (feature desabilitada se nil).
// Ref: tasks.md §4.4.1, plan.md §6.
type FlowEngine interface {
	TriggerForDevice(macAddress string, event *domain.AttendanceEvent, member *domain.Member, device *domain.Device)
}

// EventHandler handles inbound HikVision events (webhook + heartbeat via single route — dec-038).
type EventHandler struct {
	deviceRepo  DeviceRegistrar
	memberRepo  MemberFinder
	gobClient   AttendanceMarker
	eventRepo   EventRecorder
	flowEngine  FlowEngine // nil = motor de fluxo desabilitado (tasks.md §4.4.2)
	logger      *logging.Logger

	// gobDirectMarkEnabled liga/desliga a marcação direta de presença no GOB feita
	// aqui no webhook (FR-015). Default false: a marcação é delegada ao nó https_call
	// do fluxo. Injetado em main.go via SetGobDirectMarkEnabled a partir da config.
	gobDirectMarkEnabled bool
}

// NewEventHandler creates an EventHandler.
func NewEventHandler(
	deviceRepo DeviceRegistrar,
	memberRepo MemberFinder,
	gobClient AttendanceMarker,
	eventRepo EventRecorder,
	logger *logging.Logger,
) *EventHandler {
	return &EventHandler{
		deviceRepo: deviceRepo,
		memberRepo: memberRepo,
		gobClient:  gobClient,
		eventRepo:  eventRepo,
		logger:     logger,
	}
}

// SetFlowEngine injeta o motor de fluxo no handler.
// Deve ser chamado em main.go após construir o engine (tasks.md §4.4.2).
// Nil-safe: handler continua funcionando normalmente se flowEngine for nil.
func (h *EventHandler) SetFlowEngine(eng FlowEngine) {
	h.flowEngine = eng
}

// SetGobDirectMarkEnabled liga/desliga a marcação direta de presença no GOB pelo
// webhook (FR-015). Quando false (padrão), o webhook não chama MarkAttendance — a
// chamada que vale passa a ser o nó https_call do fluxo. Setar true restaura o
// comportamento legado. Deve ser chamado em main.go a partir da config.
func (h *EventHandler) SetGobDirectMarkEnabled(enabled bool) {
	h.gobDirectMarkEnabled = enabled
}

// hikPayload is the shape of the inbound HikVision event (contracts/inbound-http.md §1).
type hikPayload struct {
	EventType   string          `json:"eventType"`
	IPAddress   string          `json:"ipAddress"`
	MACAddress  string          `json:"macAddress"`
	DateTime    string          `json:"dateTime"`
	AccessControllerEvent *struct {
		MajorEventType   interface{} `json:"majorEventType"`
		SubEventType     interface{} `json:"subEventType"`
		Name             string      `json:"name"`
		EmployeeNoString string      `json:"employeeNoString"`
		CardReaderNo     interface{} `json:"cardReaderNo"`
		DoorNo           interface{} `json:"doorNo"`
		CurrentVerifyMode string     `json:"currentVerifyMode"`
		AttendanceStatus string      `json:"attendanceStatus"`
	} `json:"AccessControllerEvent"`
	EventNotificationAlert *struct {
		DateTime         string `json:"dateTime"`
		EmployeeNoString string `json:"employeeNoString"`
		AttendanceStatus string `json:"attendanceStatus"`
		DoorNo           interface{} `json:"doorNo"`
	} `json:"EventNotificationAlert"`
}

// ServeHTTP handles all inbound HikVision events.
// Registers/updates device on every request. Processes attendance for AccessControllerEvent.
func (h *EventHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deviceID := DeviceIPFromContext(ctx)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("attendance_event_received", deviceID, "", "read body failed", err)
		w.WriteHeader(http.StatusOK) // always 200
		return
	}

	// HikVision posta o evento como multipart/form-data (parte "event_log" com
	// Content-Type application/json); algumas configs postam JSON puro.
	eventJSON, err := extractEventJSON(r.Header.Get("Content-Type"), rawBody)
	if err != nil {
		h.logger.Warn("attendance_event_received", deviceID, "", "could not extract event payload, ignoring")
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload hikPayload
	if err := json.Unmarshal(eventJSON, &payload); err != nil {
		h.logger.Warn("attendance_event_received", deviceID, "", "payload not JSON, ignoring")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Variáveis para o disparo aditivo do motor de fluxo (tasks.md §4.4.1).
	// O defer garante que o trigger ocorre após toda a lógica de presença,
	// inclusive nos caminhos de retorno antecipado — sem alterar o comportamento atual.
	var (
		flowTriggerReady bool                    // true somente após insert de novo evento (não duplicata)
		flowEvent        *domain.AttendanceEvent // preenchido ao construir o evento
		flowMember       *domain.Member          // preenchido após FindByCPF
		flowDevice       *domain.Device          // preenchido após FindByMAC
	)
	defer func() {
		// Apenas AccessControllerEvent aciona o motor; heartbeats e outros eventos não (FR-018).
		if h.flowEngine != nil && flowTriggerReady {
			go h.flowEngine.TriggerForDevice(payload.MACAddress, flowEvent, flowMember, flowDevice)
		}
	}()

	// Register/update device (liveness) — FR-001/FR-002
	var deviceDBID *int64
	if payload.MACAddress != "" || payload.IPAddress != "" {
		identifier := payload.MACAddress
		if identifier == "" {
			identifier = payload.IPAddress
		}
		ip := payload.IPAddress
		mac := payload.MACAddress
		d := domain.Device{
			DeviceIdentifier: identifier,
			IPAddress:        strPtr(ip),
			MACAddress:       strPtr(mac),
		}
		if upsertErr := h.deviceRepo.Upsert(ctx, d); upsertErr != nil {
			h.logger.Error("heartbeat_received", identifier, "", "device upsert failed", upsertErr)
		} else {
			h.logger.Info("heartbeat_received", identifier, "", "device registered/updated")
		}
		// Resolve o id do device (registrado por MAC) para vincular ao evento de
		// presença — assim a coluna "Dispositivo" mostra de qual leitor veio o acesso.
		if dev, findErr := h.deviceRepo.FindByMAC(ctx, identifier); findErr == nil && dev != nil {
			deviceDBID = &dev.ID
			flowDevice = dev // para o motor de fluxo (tasks.md §4.4.1)
		}
	}

	// Process attendance only for AccessControllerEvent
	if payload.EventType != "AccessControllerEvent" {
		w.WriteHeader(http.StatusOK)
		return
	}

	employeeNoString := extractEmployeeNo(payload)
	attendanceStatus := extractAttendanceStatus(payload)

	if !domain.ValidateCPF(employeeNoString) {
		h.logger.Warn("attendance_event_received", deviceID, "", "invalid or missing employeeNoString")
		w.WriteHeader(http.StatusOK)
		return
	}

	cpfDigits, err := domain.NormalizeCPF(employeeNoString)
	if err != nil {
		h.logger.Warn("attendance_event_received", deviceID, "", "cannot normalize CPF")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse event datetime
	var eventDatetime *time.Time
	if dt := extractDateTime(payload); dt != "" {
		if t, err := time.Parse(time.RFC3339, dt); err == nil {
			eventDatetime = &t
		}
	}

	// Compute event key for dedup (FR-016)
	when := time.Time{}
	if eventDatetime != nil {
		when = *eventDatetime
	}
	eventKey := repository.ComputeEventKey(employeeNoString, when, payload.MACAddress)

	// Look up member
	member, err := h.memberRepo.FindByCPF(ctx, cpfDigits)
	if err != nil {
		h.logger.Error("attendance_event_received", deviceID, cpfDigits, "db lookup failed", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	flowMember = member // para o motor de fluxo (nil se membro não encontrado)

	rawPayload := eventJSON
	if !json.Valid(rawPayload) {
		rawPayload = []byte(`{}`)
	}

	var memberID *int64
	var federalDoc *string
	if member != nil {
		memberID = &member.ID
		federalDoc = &member.FederalDocument
	}

	// Acesso concedido (face autenticada): mesma regra usada para marcar presença.
	// O motor de fluxo usa isto (event.Authorized) para ramificar a decisão — não dá
	// para depender só de attendanceStatus, que é vazio em vários firmwares.
	granted := accessGranted(payload, attendanceStatus)

	event := domain.AttendanceEvent{
		EventKey:         eventKey,
		EmployeeNoString: employeeNoString,
		FederalDocument:  federalDoc,
		MemberID:         memberID,
		DeviceID:         deviceDBID,
		AttendanceStatus: strPtr(attendanceStatus),
		Authorized:       &granted,
		EventDatetime:    eventDatetime,
		RawPayload:       rawPayload,
	}
	flowEvent = &event // para o motor de fluxo (tasks.md §4.4.1)

	inserted, err := h.eventRepo.InsertIfNotExists(ctx, event)
	if err != nil {
		h.logger.Error("attendance_event_received", deviceID, cpfDigits, "event insert failed", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if !inserted {
		h.logger.Info("attendance_deduped", deviceID, cpfDigits, "duplicate event ignored")
		w.WriteHeader(http.StatusOK)
		return
		// flowTriggerReady permanece false: evento duplicado não aciona o motor
		// (o guarda de idempotência do engine pularia de qualquer forma — tasks.md §3.7.1).
	}
	// Evento novo (não duplicata): autorizar disparo do motor de fluxo via defer.
	flowTriggerReady = true

	if member == nil {
		h.logger.Warn("attendance_unknown_member", deviceID, cpfDigits, "member not found; event recorded but not marked")
		w.WriteHeader(http.StatusOK)
		return
	}

	if !granted {
		h.logger.Info("attendance_event_received", deviceID, cpfDigits, "evento não é acesso concedido; não marca")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Marcação de presença DIRETA no GOB (FR-015). Desligável por config: quando
	// gobDirectMarkEnabled é false (padrão), o webhook não marca aqui — a chamada
	// que vale passa a ser o nó https_call do fluxo, acionado pelo defer acima.
	if !h.gobDirectMarkEnabled {
		h.logger.Info("attendance_marked", deviceID, cpfDigits, "marcação direta no GOB desabilitada; delegada ao fluxo (https_call)")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Mark attendance in GOB (FR-015)
	if err := h.gobClient.MarkAttendance(ctx, cpfDigits); err != nil {
		h.logger.Error("attendance_marked", deviceID, cpfDigits, "GOB MarkAttendance failed", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Record mark
	if err := h.eventRepo.MarkAsMarked(ctx, eventKey); err != nil {
		h.logger.Error("attendance_marked", deviceID, cpfDigits, "MarkAsMarked DB update failed", err)
	}

	h.logger.Info("attendance_marked", deviceID, cpfDigits, "attendance marked successfully")
	w.WriteHeader(http.StatusOK)
}

// HealthHandler implements GET /health (spec.md §FR-019).
type HealthHandler struct {
	checker HealthChecker
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(checker HealthChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbStatus := "ok"
	rabbitStatus := "ok"
	overall := "ok"
	httpStatus := http.StatusOK

	if err := h.checker.PingDB(ctx); err != nil {
		dbStatus = "error"
		overall = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	if err := h.checker.PingRabbitMQ(); err != nil {
		rabbitStatus = "error"
		overall = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"status":   overall,
		"db":       dbStatus,
		"rabbitmq": rabbitStatus,
	})
}

// AdminSyncHandler implements POST /admin/sync (spec.md §FR-021-INFRA-SCHED).
type AdminSyncHandler struct {
	scheduler  Scheduler
	serializer *SyncSerializer
	logger     *logging.Logger
}

// NewAdminSyncHandler creates an AdminSyncHandler.
func NewAdminSyncHandler(scheduler Scheduler, serializer *SyncSerializer, logger *logging.Logger) *AdminSyncHandler {
	return &AdminSyncHandler{
		scheduler:  scheduler,
		serializer: serializer,
		logger:     logger,
	}
}

func (h *AdminSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	acquired, release := h.serializer.TryAcquire()
	if !acquired {
		http.Error(w, `{"error":"sync already in progress"}`, http.StatusConflict)
		return
	}

	// Log audit trail (CHK034)
	callerIP := extractClientIP(r)
	h.logger.Info("admin_sync_triggered", callerIP, "", "manual sync triggered",
		slog.String("trigger_type", "manual"),
		slog.String("caller_ip", callerIP),
	)

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"}) //nolint:errcheck

	// Run asynchronously
	go func() {
		defer release()
		if err := h.scheduler.RunMemberLoadCycle(context.Background()); err != nil {
			h.logger.Error("member_load_started", "", "", "sync cycle failed", err)
		}
	}()
}

// --- helpers ---

// extractEventJSON retorna o JSON do evento de um request HikVision.
// HikVision posta multipart/form-data; o NOME da parte JSON varia conforme o
// firmware do terminal ("event_log" em uns, "AccessControllerEvent" em outros),
// e há partes de imagem junto. Algumas configs postam JSON puro. Retorna os bytes
// JSON da primeira parte que for JSON (por nome conhecido, Content-Type ou conteúdo).
func extractEventJSON(contentType string, body []byte) ([]byte, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		// Não-multipart: trata o corpo como JSON direto.
		return body, nil
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("multipart sem boundary")
	}
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(part)
		if readErr != nil {
			return nil, readErr
		}
		if isEventJSONPart(part.FormName(), part.Header.Get("Content-Type"), data) {
			return data, nil
		}
	}
	return nil, fmt.Errorf("nenhuma parte JSON de evento encontrada no multipart")
}

// isEventJSONPart decide se uma parte multipart é o JSON do evento — robusto ao
// nome (que varia por firmware), aceitando também por Content-Type ou pelo
// conteúdo começar com '{' (descarta partes de imagem/binárias).
func isEventJSONPart(name, contentType string, data []byte) bool {
	switch name {
	case "event_log", "AccessControllerEvent", "EventNotificationAlert":
		return true
	}
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	t := bytes.TrimSpace(data)
	return len(t) > 0 && t[0] == '{'
}

// accessGranted reporta se o evento representa um acesso concedido a ser marcado.
// Marca quando o device opera em attendance mode (attendanceStatus "authorized")
// OU quando é uma face autenticada com sucesso (AccessControllerEvent
// majorEventType=5, subEventType=75 — verificado contra o dispositivo real;
// heartbeats vêm como major=2/sub=38 e não marcam).
func accessGranted(p hikPayload, attendanceStatus string) bool {
	if attendanceStatus == "authorized" {
		return true
	}
	if p.AccessControllerEvent == nil {
		return false
	}
	major, okMajor := toInt(p.AccessControllerEvent.MajorEventType)
	sub, okSub := toInt(p.AccessControllerEvent.SubEventType)
	return okMajor && okSub && major == 5 && sub == 75
}

// toInt coage um valor JSON (float64/int/string) para int.
func toInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		return i, err == nil
	}
	return 0, false
}

func extractEmployeeNo(p hikPayload) string {
	if p.AccessControllerEvent != nil && p.AccessControllerEvent.EmployeeNoString != "" {
		return p.AccessControllerEvent.EmployeeNoString
	}
	if p.EventNotificationAlert != nil {
		return p.EventNotificationAlert.EmployeeNoString
	}
	return ""
}

func extractAttendanceStatus(p hikPayload) string {
	if p.AccessControllerEvent != nil {
		return p.AccessControllerEvent.AttendanceStatus
	}
	if p.EventNotificationAlert != nil {
		return p.EventNotificationAlert.AttendanceStatus
	}
	return ""
}

func extractDateTime(p hikPayload) string {
	if p.DateTime != "" {
		return p.DateTime
	}
	if p.EventNotificationAlert != nil && p.EventNotificationAlert.DateTime != "" {
		return p.EventNotificationAlert.DateTime
	}
	return ""
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

