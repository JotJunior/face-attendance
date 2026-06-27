package flowengine

import (
	"fmt"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// executeBlocked retorna um erro descritivo para nós com contrato externo pendente.
// O erro aciona o circuit-break no Engine, registrando status "circuit_break" no log.
//
// Nó bloqueado restante (tasks.md §7.3, spec §Dependências):
//   - send_message (tipo 8): BLOCKED_API — contrato de API externa não fornecido pelo operador
//
// Os nós camera_on/camera_off foram DESBLOQUEADOS (ver node_camera.go) após o
// contrato de leitor facial ser verificado no hik2go (face-enable/face-disable).
//
// Ref: tasks.md §7.3, plan.md §BLOQUEIO B-002.
func (e *Engine) executeBlocked(node *flow.FlowNode) error {
	switch node.Type {
	case flow.NodeTypeSendMessage:
		return fmt.Errorf(
			"nó '%s' (tipo %s) requer contrato de API externa não disponível — BLOCKED_API",
			node.ID, node.Type,
		)
	default:
		return fmt.Errorf("nó '%s': tipo desconhecido '%s'", node.ID, node.Type)
	}
}
