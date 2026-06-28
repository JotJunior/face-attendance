package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PresentationMediaRepository persiste o show_mode (full/split) de cada material de
// presentation de um device. A lista ISAPI de materiais não traz as dimensões, então
// guardamos aqui o modo derivado do tamanho no upload, para reaplicá-lo ao selecionar
// a imagem. Ref: migration 000011.
type PresentationMediaRepository interface {
	// Upsert grava (ou atualiza) o modo e nome de um material de um device.
	Upsert(ctx context.Context, deviceID int64, materialID, mode, name string) error
	// GetMode retorna o modo persistido de um material ("" se não houver registro).
	GetMode(ctx context.Context, deviceID int64, materialID string) (string, error)
	// ListModesByDevice retorna um mapa material_id → mode para um device.
	ListModesByDevice(ctx context.Context, deviceID int64) (map[string]string, error)
	// Delete remove o registro de um material (idempotente).
	Delete(ctx context.Context, deviceID int64, materialID string) error
}

// PgxPresentationMediaRepository é a implementação pgx de PresentationMediaRepository.
type PgxPresentationMediaRepository struct {
	pool *pgxpool.Pool
}

// NewPgxPresentationMediaRepository cria o repositório com o pool fornecido.
func NewPgxPresentationMediaRepository(pool *pgxpool.Pool) *PgxPresentationMediaRepository {
	return &PgxPresentationMediaRepository{pool: pool}
}

// Upsert insere ou atualiza o modo/nome de um material (chave: device_id + material_id).
func (r *PgxPresentationMediaRepository) Upsert(ctx context.Context, deviceID int64, materialID, mode, name string) error {
	query := `
		INSERT INTO presentation_media (device_id, material_id, mode, name, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (device_id, material_id)
		DO UPDATE SET mode = EXCLUDED.mode, name = EXCLUDED.name, updated_at = now()
	`
	_, err := r.pool.Exec(ctx, query, deviceID, materialID, mode, name)
	return err
}

// GetMode retorna o modo persistido ("" se não houver registro para o material).
func (r *PgxPresentationMediaRepository) GetMode(ctx context.Context, deviceID int64, materialID string) (string, error) {
	var mode string
	err := r.pool.QueryRow(ctx,
		`SELECT mode FROM presentation_media WHERE device_id = $1 AND material_id = $2`,
		deviceID, materialID).Scan(&mode)
	if err != nil {
		return "", err
	}
	return mode, nil
}

// ListModesByDevice retorna um mapa material_id → mode para o device.
func (r *PgxPresentationMediaRepository) ListModesByDevice(ctx context.Context, deviceID int64) (map[string]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT material_id, mode FROM presentation_media WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var id, mode string
		if err := rows.Scan(&id, &mode); err != nil {
			return nil, err
		}
		out[id] = mode
	}
	return out, rows.Err()
}

// Delete remove o registro de um material (idempotente — não falha se ausente).
func (r *PgxPresentationMediaRepository) Delete(ctx context.Context, deviceID int64, materialID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM presentation_media WHERE device_id = $1 AND material_id = $2`,
		deviceID, materialID)
	return err
}
