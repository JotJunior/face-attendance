package flowengine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// executeWait implementa o nó wait: aguarda duration_seconds antes de avançar.
// O select com ctx.Done() garante cancelamento limpo em caso de timeout global
// ou circuit-break — sem goroutine leak (spec FR-012, tasks.md §3.2).
func (e *Engine) executeWait(ctx context.Context, node *flow.FlowNode) error {
	var cfg flow.WaitConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return fmt.Errorf("wait: config inválida: %w", err)
	}

	// Validar bounds: [1, 3600] segundos (tasks.md §3.2.1).
	if cfg.DurationSeconds < 1 || cfg.DurationSeconds > 3600 {
		return fmt.Errorf("wait: duration_seconds fora do intervalo [1, 3600]: %d",
			cfg.DurationSeconds)
	}

	// Espera cancelável via ctx.Done() (tasks.md §3.2.2).
	select {
	case <-time.After(time.Duration(cfg.DurationSeconds) * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
