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

// FindByID busca um membro pela chave primária. Retorna (nil, nil) se não existir.
// Usado pelo reenvio individual (POST /admin/api/members/{id}/resync), que opera
// por id porque o frontend só conhece o CPF mascarado.
func (r *MemberRepository) FindByID(ctx context.Context, id int64) (*domain.Member, error) {
	query := `
		SELECT id, gob_id, federal_document, name, status,
		       mobile_number, url_selfie, gob_created_at, gob_updated_at,
		       created_at, updated_at
		FROM members
		WHERE id = $1
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, id)
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

// CountMembersWithSelfie conta membros com url_selfie não-nula e não-vazia.
// Usado pelo endpoint GET /admin/api/stats.
func (r *MemberRepository) CountMembersWithSelfie(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM members WHERE url_selfie IS NOT NULL AND url_selfie != ''`,
	).Scan(&count)
	return count, err
}

// ListMembersPaged retorna membros com paginação keyset (cursor por id) e busca opcional.
// q filtra case-insensitive em name e federal_document (ILIKE '%q%').
// cursor é o id da última linha da página anterior (0 = início).
// limit é clampeado ao teto pelo caller (handler 2.5.4).
// Faz LEFT JOIN com member_processing_status para trazer sync_status sem N+1 (CHK-P12).
// Retorna os membros da página, o nextCursor (id do último item) e hasMore.
func (r *MemberRepository) ListMembersPaged(
	ctx context.Context,
	q string,
	cursor int,
	limit int,
) ([]domain.MemberView, int, bool, error) {
	// Buscar limit+1 para detectar hasMore sem query extra
	fetchLimit := limit + 1

	query := `
		SELECT
			m.id,
			m.name,
			m.federal_document,
			m.status,
			mps.last_stage      AS last_failed_stage,
			mps.last_error,
			mps.user_synced,
			mps.face_uploaded,
			mps.webhook_set
		FROM members m
		LEFT JOIN (
			SELECT DISTINCT ON (federal_document)
				federal_document,
				last_stage,
				last_error,
				user_synced,
				face_uploaded,
				webhook_set
			FROM member_processing_status
			ORDER BY federal_document, updated_at DESC
		) mps ON mps.federal_document = m.federal_document
		WHERE ($1 = '' OR m.name ILIKE '%' || $1 || '%' OR m.federal_document ILIKE '%' || $1 || '%')
		  AND ($2 = 0 OR m.id > $2)
		ORDER BY m.id
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, q, cursor, fetchLimit)
	if err != nil {
		return nil, 0, false, err
	}
	defer rows.Close()

	var results []domain.MemberView
	for rows.Next() {
		var (
			id              int64
			name            string
			federalDocument string
			status          string
			lastStage       *string
			lastError       *string
			userSynced      *bool
			faceUploaded    *bool
			webhookSet      *bool
		)
		if err := rows.Scan(
			&id, &name, &federalDocument, &status,
			&lastStage, &lastError,
			&userSynced, &faceUploaded, &webhookSet,
		); err != nil {
			return nil, 0, false, err
		}

		syncStatus := deriveSyncStatusFromJoin(userSynced, faceUploaded, webhookSet, lastError)

		results = append(results, domain.MemberView{
			ID:                    id,
			Name:                  name,
			FederalDocumentMasked: domain.MaskCPF(federalDocument),
			Status:                status,
			SyncStatus:            syncStatus,
			LastFailedStage:       lastStage,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	nextCursor := 0
	if len(results) > 0 {
		nextCursor = int(results[len(results)-1].ID)
	}

	return results, nextCursor, hasMore, nil
}

// deriveSyncStatusFromJoin infere sync_status a partir dos campos do LEFT JOIN.
// NULL nos booleanos indica ausência de linha em member_processing_status (nunca processado).
func deriveSyncStatusFromJoin(userSynced, faceUploaded, webhookSet *bool, lastError *string) string {
	if userSynced == nil {
		return "pending" // sem linha no JOIN = nunca processado
	}
	if *userSynced && *faceUploaded && *webhookSet {
		return "synced"
	}
	if lastError != nil && *lastError != "" {
		return "failed"
	}
	return "pending"
}
