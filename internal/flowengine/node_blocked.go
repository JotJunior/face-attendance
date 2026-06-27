package flowengine

import (
	"fmt"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// executeBlocked retorna um erro descritivo para nós com contrato externo pendente.
// O erro aciona o circuit-break no Engine, registrando status "circuit_break" no log.
//
// Nós bloqueados (tasks.md §3.5, spec §Dependências):
//   - camera_on (tipo 1): BLOCKED_ISAPI — nenhum endpoint verificado em t.txt / legacy/hik-api
//   - camera_off (tipo 2): BLOCKED_ISAPI — mesma dependência de contrato
//   - send_message (tipo 8): BLOCKED_API — contrato de API externa não fornecido
//
// Ref: tasks.md §3.5.1, plan.md §3.5, BLOQUEIOS B-001 e B-002.
func (e *Engine) executeBlocked(node *flow.FlowNode) error {
	switch node.Type {
	case flow.NodeTypeCameraOn, flow.NodeTypeCameraOff:
		return fmt.Errorf(
			"nó '%s' (tipo %s) requer contrato ISAPI não disponível — BLOCKED_ISAPI",
			node.ID, node.Type,
		)
	case flow.NodeTypeSendMessage:
		return fmt.Errorf(
			"nó '%s' (tipo %s) requer contrato de API externa não disponível — BLOCKED_API",
			node.ID, node.Type,
		)
	default:
		return fmt.Errorf("nó '%s': tipo desconhecido '%s'", node.ID, node.Type)
	}
}
