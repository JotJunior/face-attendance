// Package repository provides PostgreSQL data access for the presenca-facial domain.
// All queries use prepared statements / parameterized queries (plan.md §S2 — no fmt.Sprintf).
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// MemberRepository handles persistence for the members table.
type MemberRepository struct {
	pool *pgxpool.Pool
}

// NewMemberRepository creates a new MemberRepository backed by the given pool.
func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository {
	return &MemberRepository{pool: pool}
}

// Upsert inserts or updates a member using ON CONFLICT (federal_document).
// Idempotent: re-running with the same federal_document updates the row (Principle II).
func (r *MemberRepository) Upsert(ctx context.Context, m domain.Member) error {
	query := `
		INSERT INTO members (
			gob_id, federal_document, name, status,
			mobile_number, url_selfie, gob_created_at, gob_updated_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())
		ON CONFLICT (federal_document) DO UPDATE SET
			gob_id          = EXCLUDED.gob_id,
			name            = EXCLUDED.name,
			status          = EXCLUDED.status,
			mobile_number   = EXCLUDED.mobile_number,
			url_selfie      = EXCLUDED.url_selfie,
			gob_created_at  = EXCLUDED.gob_created_at,
			gob_updated_at  = EXCLUDED.gob_updated_at,
			updated_at      = now()
	`
	_, err := r.pool.Exec(ctx, query,
		m.GobID,
		m.FederalDocument,
		m.Name,
		m.Status,
		m.MobileNumber,
		m.URLSelfie,
		m.GobCreatedAt,
		m.GobUpdatedAt,
	)
	return err
}

// ListWithSelfie returns all members that have a non-empty url_selfie.
func (r *MemberRepository) ListWithSelfie(ctx context.Context) ([]domain.Member, error) {
	query := `
		SELECT id, gob_id, federal_document, name, status,
		       mobile_number, url_selfie, gob_created_at, gob_updated_at,
		       created_at, updated_at
		FROM members
		WHERE url_selfie IS NOT NULL AND url_selfie != ''
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanMembers(rows)
}

// FindByCPF finds a member by their federal_document (CPF digits).
// Returns (nil, nil) if not found.
func (r *MemberRepository) FindByCPF(ctx context.Context, cpfDigits string) (*domain.Member, error) {
	// federal_document é armazenado formatado (ex. "005.149.047-12"), mas o
	// boundary do webhook passa apenas dígitos. Normaliza ambos os lados.
	query := `
		SELECT id, gob_id, federal_document, name, status,
		       mobile_number, url_selfie, gob_created_at, gob_updated_at,
		       created_at, updated_at
		FROM members
		WHERE regexp_replace(federal_document, '[^0-9]', '', 'g') = regexp_replace($1, '[^0-9]', '', 'g')
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, cpfDigits)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members, err := scanMembers(rows)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}
	return &members[0], nil
}

// scanMembers reads all rows into domain.Member values (explicit mapper — no ORM).
func scanMembers(rows pgx.Rows) ([]domain.Member, error) {
	var members []domain.Member
	for rows.Next() {
		var m domain.Member
		if err := rows.Scan(
			&m.ID,
			&m.GobID,
			&m.FederalDocument,
			&m.Name,
			&m.Status,
			&m.MobileNumber,
			&m.URLSelfie,
			&m.GobCreatedAt,
			&m.GobUpdatedAt,
			&m.CreatedAt,
			&m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// scanMember scans a single member row.
func scanMember(row pgx.Row) (*domain.Member, error) {
	var m domain.Member
	err := row.Scan(
		&m.ID,
		&m.GobID,
		&m.FederalDocument,
		&m.Name,
		&m.Status,
		&m.MobileNumber,
		&m.URLSelfie,
		&m.GobCreatedAt,
		&m.GobUpdatedAt,
		&m.CreatedAt,
		&m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// suppress unused warning for scanMember
var _ = scanMember
var _ = time.Now
