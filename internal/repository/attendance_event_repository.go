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

// CountAttendanceSince conta eventos de presença criados desde o tempo informado.
// Usado pelo endpoint GET /admin/api/stats (presença nas últimas 24h).
func (r *AttendanceEventRepository) CountAttendanceSince(ctx context.Context, since time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM attendance_events WHERE created_at >= $1`,
		since,
	).Scan(&count)
	return count, err
}

// ListEventsPaged retorna eventos com paginação keyset (created_at DESC, id DESC).
// cursor zero indica início da listagem.
// from/to filtram por created_at; ambos opcionais (zero = sem filtro de data).
// JOIN com devices e members traz device_identifier e member_name sem N+1.
// raw_payload e event_key são excluídos da saída (não expostos ao browser).
// Retorna a página, o nextCursor e hasMore.
func (r *AttendanceEventRepository) ListEventsPaged(
	ctx context.Context,
	from, to time.Time,
	cursor domain.CursorEvt,
	limit int,
) ([]domain.EventView, domain.CursorEvt, bool, error) {
	fetchLimit := limit + 1

	// Construção da query com filtros condicionais via parâmetros nulos.
	// from/to zero são passados como nil para desabilitar o filtro de data.
	var fromArg, toArg interface{}
	if !from.IsZero() {
		fromArg = from
	}
	if !to.IsZero() {
		toArg = to
	}

	// cursor zero = sem filtro de keyset (busca do início)
	var cursorAt interface{}
	var cursorID interface{}
	if !cursor.IsZero() {
		cursorAt = cursor.CreatedAt
		cursorID = cursor.ID
	}

	query := `
		SELECT
			ae.id,
			ae.event_datetime,
			ae.created_at,
			ae.device_id,
			d.device_identifier,
			m.name         AS member_name,
			ae.federal_document,
			ae.attendance_status,
			ae.marked,
			ae.marked_at
		FROM attendance_events ae
		LEFT JOIN devices d  ON d.id  = ae.device_id
		LEFT JOIN members m  ON m.id  = ae.member_id
		WHERE ($1::timestamptz IS NULL OR ae.created_at >= $1)
		  AND ($2::timestamptz IS NULL OR ae.created_at <= $2)
		  AND ($3::timestamptz IS NULL OR $4::bigint IS NULL
		       OR (ae.created_at, ae.id) < ($3, $4))
		ORDER BY ae.created_at DESC, ae.id DESC
		LIMIT $5
	`

	rows, err := r.pool.Query(ctx, query, fromArg, toArg, cursorAt, cursorID, fetchLimit)
	if err != nil {
		return nil, domain.CursorEvt{}, false, err
	}
	defer rows.Close()

	var results []domain.EventView
	for rows.Next() {
		var (
			id               int64
			eventDatetime    *time.Time
			createdAt        time.Time
			deviceID         *int64
			deviceIdentifier *string
			memberName       *string
			federalDocument  *string
			attendanceStatus *string
			marked           bool
			markedAt         *time.Time
		)
		if err := rows.Scan(
			&id, &eventDatetime, &createdAt,
			&deviceID, &deviceIdentifier,
			&memberName, &federalDocument,
			&attendanceStatus, &marked, &markedAt,
		); err != nil {
			return nil, domain.CursorEvt{}, false, err
		}

		// Mascarar CPF antes de expor (SC-006)
		var maskedCPF *string
		if federalDocument != nil {
			s := domain.MaskCPF(*federalDocument)
			maskedCPF = &s
		}

		// Derivar marking_status
		ae := domain.AttendanceEvent{
			AttendanceStatus: attendanceStatus,
			Marked:           marked,
			MarkedAt:         markedAt,
		}
		if deviceID != nil {
			ae.DeviceID = deviceID
		}
		markingStatus := domain.DeriveMarkingStatus(ae)

		results = append(results, domain.EventView{
			ID:                    id,
			EventDatetime:         eventDatetime,
			CreatedAt:             createdAt,
			DeviceID:              deviceID,
			DeviceIdentifier:      deviceIdentifier,
			MemberName:            memberName,
			FederalDocumentMasked: maskedCPF,
			MarkingStatus:         markingStatus,
			MarkedAt:              markedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, domain.CursorEvt{}, false, err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	var nextCursor domain.CursorEvt
	if len(results) > 0 {
		last := results[len(results)-1]
		nextCursor = domain.CursorEvt{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}
	}

	return results, nextCursor, hasMore, nil
}
