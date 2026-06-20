package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// ProcessingOutcomeRepository handles persistence for member_processing_status.
type ProcessingOutcomeRepository struct {
	pool *pgxpool.Pool
}

// NewProcessingOutcomeRepository creates a new ProcessingOutcomeRepository.
func NewProcessingOutcomeRepository(pool *pgxpool.Pool) *ProcessingOutcomeRepository {
	return &ProcessingOutcomeRepository{pool: pool}
}

// UpsertOutcome inserts or updates the processing state for a member×device pair.
// ON CONFLICT (federal_document, device_id) — idempotent (Principle II).
func (r *ProcessingOutcomeRepository) UpsertOutcome(ctx context.Context, o domain.ProcessingOutcome) error {
	query := `
		INSERT INTO member_processing_status (
			federal_document, device_id,
			user_synced, face_uploaded, webhook_set,
			last_stage, last_error, attempts, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (federal_document, device_id) DO UPDATE SET
			user_synced   = EXCLUDED.user_synced,
			face_uploaded = EXCLUDED.face_uploaded,
			webhook_set   = EXCLUDED.webhook_set,
			last_stage    = EXCLUDED.last_stage,
			last_error    = EXCLUDED.last_error,
			attempts      = EXCLUDED.attempts,
			updated_at    = now()
	`
	_, err := r.pool.Exec(ctx, query,
		o.FederalDocument,
		o.DeviceID,
		o.UserSynced,
		o.FaceUploaded,
		o.WebhookSet,
		o.LastStage,
		o.LastError,
		o.Attempts,
	)
	return err
}

// FindByMemberDevice returns the processing state for a member×device pair.
// Returns (nil, nil) if not found.
func (r *ProcessingOutcomeRepository) FindByMemberDevice(ctx context.Context, cpfDigits string, deviceID int64) (*domain.ProcessingOutcome, error) {
	query := `
		SELECT id, federal_document, device_id,
		       user_synced, face_uploaded, webhook_set,
		       last_stage, last_error, attempts, updated_at
		FROM member_processing_status
		WHERE federal_document = $1 AND device_id = $2
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, cpfDigits, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, rows.Err()
	}

	var o domain.ProcessingOutcome
	if err := rows.Scan(
		&o.ID,
		&o.FederalDocument,
		&o.DeviceID,
		&o.UserSynced,
		&o.FaceUploaded,
		&o.WebhookSet,
		&o.LastStage,
		&o.LastError,
		&o.Attempts,
		&o.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &o, rows.Err()
}
