package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// FlowRepository define as operações de persistência para fluxos de execução.
// Ref: docs/specs/face-flow/plan.md §4.1
type FlowRepository interface {
	Create(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
	FindByID(ctx context.Context, id int64) (*flow.Flow, error)
	FindAll(ctx context.Context) ([]*flow.Flow, error)
	FindActiveByDeviceID(ctx context.Context, deviceID int64) (*flow.Flow, error)
	Update(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
	SetStatus(ctx context.Context, id int64, status string) error
	SetDeviceID(ctx context.Context, id int64, deviceID *int64) error
	Delete(ctx context.Context, id int64) error
}

// ErrFlowDeviceConflict é retornado quando um device já está vinculado a outro fluxo ativo.
// O handler HTTP mapeia este erro para 409 Conflict.
var ErrFlowDeviceConflict = errors.New("device já vinculado a outro fluxo")

// PgxFlowRepository é a implementação pgx de FlowRepository.
type PgxFlowRepository struct {
	pool *pgxpool.Pool
}

// NewPgxFlowRepository cria um PgxFlowRepository com o pool fornecido.
func NewPgxFlowRepository(pool *pgxpool.Pool) *PgxFlowRepository {
	return &PgxFlowRepository{pool: pool}
}

// Create insere um novo fluxo e retorna o registro com ID e timestamps preenchidos.
func (r *PgxFlowRepository) Create(ctx context.Context, f *flow.Flow) (*flow.Flow, error) {
	nodesJSON, err := json.Marshal(f.Nodes)
	if err != nil {
		return nil, err
	}
	edgesJSON, err := json.Marshal(f.Edges)
	if err != nil {
		return nil, err
	}

	query := `
		INSERT INTO flows (name, status, device_id, nodes, edges, created_at, updated_at)
		VALUES ($1, COALESCE(NULLIF($2, ''), 'inactive'), $3, $4, $5, now(), now())
		RETURNING id, name, status, device_id, nodes, edges, created_at, updated_at
	`
	row := r.pool.QueryRow(ctx, query,
		f.Name,
		f.Status,
		f.DeviceID,
		nodesJSON,
		edgesJSON,
	)
	return scanFlowRow(row)
}

// FindByID retorna o fluxo com o ID fornecido.
// Retorna pgx.ErrNoRows se não encontrado.
func (r *PgxFlowRepository) FindByID(ctx context.Context, id int64) (*flow.Flow, error) {
	query := `
		SELECT id, name, status, device_id, nodes, edges, created_at, updated_at
		FROM flows
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)
	return scanFlowRow(row)
}

// FindAll retorna todos os fluxos ordenados por id.
func (r *PgxFlowRepository) FindAll(ctx context.Context) ([]*flow.Flow, error) {
	query := `
		SELECT id, name, status, device_id, nodes, edges, created_at, updated_at
		FROM flows
		ORDER BY id
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFlowRows(rows)
}

// FindActiveByDeviceID retorna o fluxo ativo vinculado ao device, ou nil se não houver.
// Apenas um fluxo pode estar ativo por device (device_id é UNIQUE na tabela).
func (r *PgxFlowRepository) FindActiveByDeviceID(ctx context.Context, deviceID int64) (*flow.Flow, error) {
	query := `
		SELECT id, name, status, device_id, nodes, edges, created_at, updated_at
		FROM flows
		WHERE device_id = $1 AND status = 'active'
		LIMIT 1
	`
	row := r.pool.QueryRow(ctx, query, deviceID)
	f, err := scanFlowRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return f, err
}

// Update persiste alterações em nome, nodes e edges de um fluxo existente.
func (r *PgxFlowRepository) Update(ctx context.Context, f *flow.Flow) (*flow.Flow, error) {
	nodesJSON, err := json.Marshal(f.Nodes)
	if err != nil {
		return nil, err
	}
	edgesJSON, err := json.Marshal(f.Edges)
	if err != nil {
		return nil, err
	}

	query := `
		UPDATE flows
		SET name = $2, nodes = $3, edges = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, name, status, device_id, nodes, edges, created_at, updated_at
	`
	row := r.pool.QueryRow(ctx, query, f.ID, f.Name, nodesJSON, edgesJSON)
	return scanFlowRow(row)
}

// SetStatus altera o status de um fluxo ("active" ou "inactive").
func (r *PgxFlowRepository) SetStatus(ctx context.Context, id int64, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE flows SET status = $2, updated_at = now() WHERE id = $1`,
		id, status,
	)
	return err
}

// SetDeviceID vincula ou desvincula um device de um fluxo.
// deviceID == nil → SET device_id = NULL (desvincular).
// deviceID != nil → UPDATE; retorna ErrFlowDeviceConflict se o device já está em outro fluxo.
func (r *PgxFlowRepository) SetDeviceID(ctx context.Context, id int64, deviceID *int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE flows SET device_id = $2, updated_at = now() WHERE id = $1`,
		id, deviceID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrFlowDeviceConflict
		}
	}
	return err
}

// Delete remove um fluxo pelo ID.
func (r *PgxFlowRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM flows WHERE id = $1`, id)
	return err
}

// scanFlowRow lê um único registro de fluxo de um pgx.Row.
func scanFlowRow(row pgx.Row) (*flow.Flow, error) {
	var (
		f          flow.Flow
		nodesJSON  []byte
		edgesJSON  []byte
		deviceID   *int64
		createdAt  time.Time
		updatedAt  time.Time
	)
	if err := row.Scan(
		&f.ID,
		&f.Name,
		&f.Status,
		&deviceID,
		&nodesJSON,
		&edgesJSON,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	f.DeviceID = deviceID
	f.CreatedAt = createdAt
	f.UpdatedAt = updatedAt

	if err := json.Unmarshal(nodesJSON, &f.Nodes); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(edgesJSON, &f.Edges); err != nil {
		return nil, err
	}
	if f.Nodes == nil {
		f.Nodes = []flow.FlowNode{}
	}
	if f.Edges == nil {
		f.Edges = []flow.FlowEdge{}
	}
	return &f, nil
}

// scanFlowRows lê múltiplos registros de fluxo de pgx.Rows.
func scanFlowRows(rows pgx.Rows) ([]*flow.Flow, error) {
	var flows []*flow.Flow
	for rows.Next() {
		var (
			f         flow.Flow
			nodesJSON []byte
			edgesJSON []byte
			deviceID  *int64
			createdAt time.Time
			updatedAt time.Time
		)
		if err := rows.Scan(
			&f.ID,
			&f.Name,
			&f.Status,
			&deviceID,
			&nodesJSON,
			&edgesJSON,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}

		f.DeviceID = deviceID
		f.CreatedAt = createdAt
		f.UpdatedAt = updatedAt

		if err := json.Unmarshal(nodesJSON, &f.Nodes); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(edgesJSON, &f.Edges); err != nil {
			return nil, err
		}
		if f.Nodes == nil {
			f.Nodes = []flow.FlowNode{}
		}
		if f.Edges == nil {
			f.Edges = []flow.FlowEdge{}
		}
		flows = append(flows, &f)
	}
	return flows, rows.Err()
}
