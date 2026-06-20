package repository

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// AttendanceEventRepository handles persistence for the attendance_events table.
type AttendanceEventRepository struct {
	pool *pgxpool.Pool
}

// NewAttendanceEventRepository creates a new AttendanceEventRepository.
func NewAttendanceEventRepository(pool *pgxpool.Pool) *AttendanceEventRepository {
	return &AttendanceEventRepository{pool: pool}
}

// InsertIfNotExists inserts an AttendanceEvent or ignores if event_key already exists.
// Returns inserted=true if a new row was created, inserted=false if it was a duplicate.
// raw_payload is stored as JSONB via a parameterized query (plan.md §S2 — no interpolation).
func (r *AttendanceEventRepository) InsertIfNotExists(ctx context.Context, e domain.AttendanceEvent) (bool, error) {
	query := `
		INSERT INTO attendance_events (
			event_key, employee_no_string, federal_document,
			member_id, device_id, event_datetime, attendance_status,
			marked, raw_payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, now())
		ON CONFLICT (event_key) DO NOTHING
	`
	tag, err := r.pool.Exec(ctx, query,
		e.EventKey,
		e.EmployeeNoString,
		e.FederalDocument,
		e.MemberID,
		e.DeviceID,
		e.EventDatetime,
		e.AttendanceStatus,
		e.RawPayload, // stored as JSONB — must be valid JSON bytes
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// MarkAsMarked updates marked=true and marked_at=now() for the given event_key.
func (r *AttendanceEventRepository) MarkAsMarked(ctx context.Context, eventKey string) error {
	query := `UPDATE attendance_events SET marked = true, marked_at = now() WHERE event_key = $1`
	_, err := r.pool.Exec(ctx, query, eventKey)
	return err
}

// ComputeEventKey computes a deterministic SHA-256 hash for a face recognition event.
// Key fields: employeeNoString + eventDatetime (RFC3339) + deviceIdentifier.
// If eventDatetime is zero, uses receivedAt truncated to second + payloadHash (data-model.md §Regra de event_key).
func ComputeEventKey(employeeNoString string, eventDatetime time.Time, deviceIdentifier string) string {
	var input string
	if eventDatetime.IsZero() {
		// Fallback: use receive time truncated to second
		input = fmt.Sprintf("%s|%s|%s", employeeNoString, time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), deviceIdentifier)
	} else {
		input = fmt.Sprintf("%s|%s|%s", employeeNoString, eventDatetime.UTC().Format(time.RFC3339), deviceIdentifier)
	}
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash[:])
}

// ValidateRawPayload ensures the raw payload is valid JSON before storing.
func ValidateRawPayload(payload []byte) (json.RawMessage, error) {
	if !json.Valid(payload) {
		return nil, fmt.Errorf("raw_payload is not valid JSON")
	}
	return json.RawMessage(payload), nil
}
