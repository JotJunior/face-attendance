package domain

import (
	"encoding/json"
	"time"
)

// AttendanceEvent records a face recognition event received from a HikVision device.
// event_key provides idempotency (FR-016, data-model.md §AttendanceEvent).
type AttendanceEvent struct {
	ID                 int64           `json:"id"`
	EventKey           string          `json:"event_key"` // SHA-256 hash; see repository.ComputeEventKey
	EmployeeNoString   string          `json:"employee_no_string"`
	FederalDocument    *string         `json:"federal_document,omitempty"` // CPF digits; nil if unknown
	MemberID           *int64          `json:"member_id,omitempty"`
	DeviceID           *int64          `json:"device_id,omitempty"`
	EventDatetime      *time.Time      `json:"event_datetime,omitempty"`
	AttendanceStatus   *string         `json:"attendance_status,omitempty"` // "authorized" = positive
	Marked             bool            `json:"marked"`
	MarkedAt           *time.Time      `json:"marked_at,omitempty"`
	RawPayload         json.RawMessage `json:"raw_payload"`
	CreatedAt          time.Time       `json:"created_at"`
}

// IsAuthorized reports whether the event represents a positive facial recognition result.
func (e *AttendanceEvent) IsAuthorized() bool {
	return e.AttendanceStatus != nil && *e.AttendanceStatus == "authorized"
}
