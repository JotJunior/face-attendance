package httphandler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// DeviceRegistrar is the interface the heartbeat handler uses to register devices.
type DeviceRegistrar interface {
	Upsert(ctx context.Context, d domain.Device) error
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

// EventHandler handles inbound HikVision events (webhook + heartbeat via single route — dec-038).
type EventHandler struct {
	deviceRepo      DeviceRegistrar
	memberRepo      MemberFinder
	gobClient       AttendanceMarker
	eventRepo       EventRecorder
	logger          *logging.Logger
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

	var payload hikPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		h.logger.Warn("attendance_event_received", deviceID, "", "payload not JSON, ignoring")
		w.WriteHeader(http.StatusOK)
		return
	}

	// Register/update device (liveness) — FR-001/FR-002
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

	rawPayload := rawBody
	if !json.Valid(rawPayload) {
		rawPayload = []byte(`{}`)
	}

	var memberID *int64
	var federalDoc *string
	if member != nil {
		memberID = &member.ID
		federalDoc = &member.FederalDocument
	}

	event := domain.AttendanceEvent{
		EventKey:         eventKey,
		EmployeeNoString: employeeNoString,
		FederalDocument:  federalDoc,
		MemberID:         memberID,
		AttendanceStatus: strPtr(attendanceStatus),
		EventDatetime:    eventDatetime,
		RawPayload:       rawPayload,
	}

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
	}

	if member == nil {
		h.logger.Warn("attendance_unknown_member", deviceID, cpfDigits, "member not found; event recorded but not marked")
		w.WriteHeader(http.StatusOK)
		return
	}

	if attendanceStatus != "authorized" {
		h.logger.Info("attendance_event_received", deviceID, cpfDigits, "attendanceStatus not authorized; not marking")
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

