package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackgroundImage representa uma imagem de fundo disponível para o nó change_background.
// Ref: docs/specs/face-flow/data-model.md §background_images
type BackgroundImage struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	FilePath  string    `json:"file_path"` // path relativo a BACKGROUND_IMAGES_DIR
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BackgroundImageRepository define as operações de persistência para imagens de fundo.
// Ref: docs/specs/face-flow/plan.md §4.2
type BackgroundImageRepository interface {
	Create(ctx context.Context, name, filePath string) (*BackgroundImage, error)
	FindByID(ctx context.Context, id int64) (*BackgroundImage, error)
	FindAll(ctx context.Context) ([]*BackgroundImage, error)
	Delete(ctx context.Context, id int64) error
}

// PgxBackgroundImageRepository é a implementação pgx de BackgroundImageRepository.
type PgxBackgroundImageRepository struct {
	pool *pgxpool.Pool
}

// NewPgxBackgroundImageRepository cria um PgxBackgroundImageRepository com o pool fornecido.
func NewPgxBackgroundImageRepository(pool *pgxpool.Pool) *PgxBackgroundImageRepository {
	return &PgxBackgroundImageRepository{pool: pool}
}

// Create insere um novo registro de imagem de fundo e retorna a entidade com ID e timestamps.
func (r *PgxBackgroundImageRepository) Create(ctx context.Context, name, filePath string) (*BackgroundImage, error) {
	query := `
		INSERT INTO background_images (name, file_path, created_at, updated_at)
		VALUES ($1, $2, now(), now())
		RETURNING id, name, file_path, created_at, updated_at
	`
	row := r.pool.QueryRow(ctx, query, name, filePath)
	return scanBackgroundImageRow(row)
}

// FindByID retorna a imagem de fundo com o ID fornecido.
// Retorna pgx.ErrNoRows se não encontrada.
func (r *PgxBackgroundImageRepository) FindByID(ctx context.Context, id int64) (*BackgroundImage, error) {
	query := `
		SELECT id, name, file_path, created_at, updated_at
		FROM background_images
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)
	return scanBackgroundImageRow(row)
}

// FindAll retorna todas as imagens de fundo ordenadas por id.
func (r *PgxBackgroundImageRepository) FindAll(ctx context.Context) ([]*BackgroundImage, error) {
	query := `
		SELECT id, name, file_path, created_at, updated_at
		FROM background_images
		ORDER BY id
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []*BackgroundImage
	for rows.Next() {
		img, err := scanBackgroundImageRow(rows)
		if err != nil {
			return nil, err
		}
		images = append(images, img)
	}
	return images, rows.Err()
}

// Delete remove o registro de imagem de fundo do banco.
// A remoção do arquivo em disco fica a cargo do handler HTTP (tasks.md §2.2.3).
func (r *PgxBackgroundImageRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM background_images WHERE id = $1`, id)
	return err
}

// scanBackgroundImageRow lê um único registro de imagem de fundo.
// Aceita tanto pgx.Row (QueryRow) quanto pgx.Rows (via interface pgx.Row).
func scanBackgroundImageRow(row pgx.Row) (*BackgroundImage, error) {
	var img BackgroundImage
	if err := row.Scan(
		&img.ID,
		&img.Name,
		&img.FilePath,
		&img.CreatedAt,
		&img.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &img, nil
}
