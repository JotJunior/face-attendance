package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FlowExecutionLog registra o resultado de uma execução do motor de fluxo.
// A coluna event_key é UNIQUE para garantir idempotência de side-effects (Constituição §II,
// spec FR-023): reprocessar o mesmo evento não duplica o log.
// Ref: docs/specs/face-flow/data-model.md §flow_execution_logs
type FlowExecutionLog struct {
	ID           int64      `json:"id"`
	FlowID       int64      `json:"flow_id"`
	DeviceID     int64      `json:"device_id"`
	EventKey     string     `json:"event_key"`
	Status       string     `json:"status"`        // "completed" | "circuit_break"
	FailedNodeID *string    `json:"failed_node_id,omitempty"` // NULL se completed
	Error        *string    `json:"error,omitempty"`          // NULL se completed
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   time.Time  `json:"finished_at"`
}

// FlowExecutionLogRepository define as operações de persistência para logs de execução.
// Ref: docs/specs/face-flow/plan.md §4.3
type FlowExecutionLogRepository interface {
	// Create insere um log de execução.
	// Retorna nil em violação de UNIQUE em event_key — idempotência de execuções
	// concorrentes do mesmo evento (spec FR-023; Constituição §II).
	Create(ctx context.Context, log *FlowExecutionLog) error
	FindByFlowID(ctx context.Context, flowID int64, limit, offset int) ([]*FlowExecutionLog, error)
}

// PgxFlowExecutionLogRepository é a implementação pgx de FlowExecutionLogRepository.
type PgxFlowExecutionLogRepository struct {
	pool *pgxpool.Pool
}

// NewPgxFlowExecutionLogRepository cria um PgxFlowExecutionLogRepository com o pool fornecido.
func NewPgxFlowExecutionLogRepository(pool *pgxpool.Pool) *PgxFlowExecutionLogRepository {
	return &PgxFlowExecutionLogRepository{pool: pool}
}

// Create insere um FlowExecutionLog.
// Violação de UNIQUE em event_key (pgx code 23505) → retorna nil (não é erro de negócio;
// é idempotência de execuções concorrentes que chegaram ao mesmo event_key — spec FR-023).
func (r *PgxFlowExecutionLogRepository) Create(ctx context.Context, log *FlowExecutionLog) error {
	query := `
		INSERT INTO flow_execution_logs
			(flow_id, device_id, event_key, status, failed_node_id, error, started_at, finished_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.pool.Exec(ctx, query,
		log.FlowID,
		log.DeviceID,
		log.EventKey,
		log.Status,
		log.FailedNodeID,
		log.Error,
		log.StartedAt,
		log.FinishedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// event_key duplicado: execução concorrente do mesmo evento. Não é erro de
			// negócio — idempotência intencional. O primeiro insert que venceu a corrida
			// já registrou o log; ignoramos a violação do segundo.
			return nil
		}
		return err
	}
	return nil
}

// FindByFlowID retorna os logs de execução de um fluxo, paginados.
// Ordenados por started_at DESC (mais recentes primeiro), com limit e offset para paginação.
func (r *PgxFlowExecutionLogRepository) FindByFlowID(
	ctx context.Context,
	flowID int64,
	limit, offset int,
) ([]*FlowExecutionLog, error) {
	query := `
		SELECT id, flow_id, device_id, event_key, status,
		       failed_node_id, error, started_at, finished_at
		FROM flow_execution_logs
		WHERE flow_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, flowID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*FlowExecutionLog
	for rows.Next() {
		var l FlowExecutionLog
		if err := rows.Scan(
			&l.ID,
			&l.FlowID,
			&l.DeviceID,
			&l.EventKey,
			&l.Status,
			&l.FailedNodeID,
			&l.Error,
			&l.StartedAt,
			&l.FinishedAt,
		); err != nil {
			return nil, err
		}
		logs = append(logs, &l)
	}
	return logs, rows.Err()
}

// FindByEventKey verifica se já existe um log com o event_key fornecido e o status indicado.
// Usado pelo guarda de idempotência do motor (tasks.md §3.7.1): antes de executar
// um fluxo, o engine verifica se event_key já foi concluído com sucesso.
// Retorna (nil, nil) se não existir.
func (r *PgxFlowExecutionLogRepository) FindByEventKey(
	ctx context.Context,
	eventKey string,
) (*FlowExecutionLog, error) {
	query := `
		SELECT id, flow_id, device_id, event_key, status,
		       failed_node_id, error, started_at, finished_at
		FROM flow_execution_logs
		WHERE event_key = $1
		LIMIT 1
	`
	rows, err := r.pool.Query(ctx, query, eventKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, rows.Err()
	}
	var l FlowExecutionLog
	if err := rows.Scan(
		&l.ID,
		&l.FlowID,
		&l.DeviceID,
		&l.EventKey,
		&l.Status,
		&l.FailedNodeID,
		&l.Error,
		&l.StartedAt,
		&l.FinishedAt,
	); err != nil {
		return nil, err
	}
	return &l, rows.Err()
}

