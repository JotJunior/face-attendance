package flow

import "fmt"

// ValidationError descreve um problema estrutural num fluxo.
type ValidationError struct {
	Code    string // "no_start_node", "multiple_start_nodes", "cycle_detected",
	//              "decision_missing_branch", "dangling_node_reference"
	Message string
	NodeID  string // quando aplicável
}

func (e ValidationError) Error() string {
	if e.NodeID != "" {
		return fmt.Sprintf("%s (nó %s): %s", e.Code, e.NodeID, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Validate verifica a estrutura do fluxo antes da publicação.
// Ref: docs/specs/face-flow/spec.md §FR-005, §FR-022, plan.md §2.2
//
// Checagens:
//  1. Exatamente 1 nó do tipo start (erros: no_start_node / multiple_start_nodes)
//  2. Nós decision devem ter exatamente 2 edges de saída com labels "valid" e "invalid"
//  3. Toda referência from/to em edges deve apontar para um nó existente (dangling_node_reference)
//  4. Detecção de ciclos via DFS coloração white/gray/black a partir do nó start
func Validate(f *Flow) []ValidationError {
	var errs []ValidationError

	// Índice de nós para lookup O(1).
	nodeIndex := make(map[string]*FlowNode, len(f.Nodes))
	for i := range f.Nodes {
		nodeIndex[f.Nodes[i].ID] = &f.Nodes[i]
	}

	// 1. Verificar exatamente 1 nó start.
	startCount := 0
	var startID string
	for _, n := range f.Nodes {
		if n.Type == NodeTypeStart {
			startCount++
			startID = n.ID
		}
	}
	switch {
	case startCount == 0:
		errs = append(errs, ValidationError{
			Code:    "no_start_node",
			Message: "o fluxo deve ter exatamente um nó de início (start)",
		})
	case startCount > 1:
		errs = append(errs, ValidationError{
			Code:    "multiple_start_nodes",
			Message: fmt.Sprintf("o fluxo tem %d nós start; apenas 1 é permitido", startCount),
		})
	}

	// Adjacência para DFS e verificação de referências.
	adj := make(map[string][]string, len(f.Nodes))
	for id := range nodeIndex {
		adj[id] = nil // garantir que todo nó aparece no adj
	}

	for _, e := range f.Edges {
		// 3. Verificar referências dangling.
		if _, ok := nodeIndex[e.From]; !ok {
			errs = append(errs, ValidationError{
				Code:    "dangling_node_reference",
				Message: fmt.Sprintf("edge referencia nó de origem inexistente: %q", e.From),
				NodeID:  e.From,
			})
		}
		if _, ok := nodeIndex[e.To]; !ok {
			errs = append(errs, ValidationError{
				Code:    "dangling_node_reference",
				Message: fmt.Sprintf("edge referencia nó de destino inexistente: %q", e.To),
				NodeID:  e.To,
			})
		}
		adj[e.From] = append(adj[e.From], e.To)
	}

	// 2. Verificar nós decision: exatamente 2 edges com labels "valid" e "invalid".
	for _, n := range f.Nodes {
		if n.Type != NodeTypeDecision {
			continue
		}
		outgoing := f.OutgoingEdges(n.ID)
		hasValid := false
		hasInvalid := false
		for _, e := range outgoing {
			if e.Label == "valid" {
				hasValid = true
			}
			if e.Label == "invalid" {
				hasInvalid = true
			}
		}
		if !hasValid || !hasInvalid || len(outgoing) != 2 {
			errs = append(errs, ValidationError{
				Code:    "decision_missing_branch",
				Message: "nó decision deve ter exatamente 2 edges: uma com label \"valid\" e outra com label \"invalid\"",
				NodeID:  n.ID,
			})
		}
	}

	// 4. Detecção de ciclos via DFS a partir do nó start.
	// Só roda se houver exatamente 1 start (caso contrário os erros 1/2 já foram reportados).
	if startCount == 1 {
		const (
			white = 0 // não visitado
			gray  = 1 // em visita (no stack da DFS)
			black = 2 // visita concluída
		)
		color := make(map[string]int, len(f.Nodes))

		var dfs func(id string)
		dfs = func(id string) {
			color[id] = gray
			for _, neighbor := range adj[id] {
				switch color[neighbor] {
				case gray:
					errs = append(errs, ValidationError{
						Code:    "cycle_detected",
						Message: fmt.Sprintf("ciclo detectado: nó %q aponta para %q que já está na pilha de execução", id, neighbor),
						NodeID:  id,
					})
				case white:
					if _, exists := nodeIndex[neighbor]; exists {
						dfs(neighbor)
					}
				}
			}
			color[id] = black
		}
		dfs(startID)
	}

	return errs
}
