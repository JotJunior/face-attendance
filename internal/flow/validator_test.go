package flow

import (
	"testing"
)

// helper: fluxo mínimo válido start→end
func simpleFlow() *Flow {
	return &Flow{
		Nodes: []FlowNode{
			{ID: "n1", Type: NodeTypeStart},
			{ID: "n2", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "n1", To: "n2"},
		},
	}
}

func TestValidate_NoStart(t *testing.T) {
	f := &Flow{
		Nodes: []FlowNode{{ID: "n1", Type: NodeTypeWait}},
		Edges: []FlowEdge{},
	}
	errs := Validate(f)
	if !hasCode(errs, "no_start_node") {
		t.Errorf("esperado erro no_start_node, obteve: %v", errs)
	}
}

func TestValidate_MultipleStart(t *testing.T) {
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "n1", Type: NodeTypeStart},
			{ID: "n2", Type: NodeTypeStart},
		},
		Edges: []FlowEdge{{From: "n1", To: "n2"}},
	}
	errs := Validate(f)
	if !hasCode(errs, "multiple_start_nodes") {
		t.Errorf("esperado erro multiple_start_nodes, obteve: %v", errs)
	}
}

func TestValidate_Cycle(t *testing.T) {
	// A→B→C→A (ciclo)
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
			{ID: "a", Type: NodeTypeWait},
			{ID: "b", Type: NodeTypeWait},
			{ID: "c", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "s", To: "a"},
			{From: "a", To: "b"},
			{From: "b", To: "c"},
			{From: "c", To: "a"}, // ciclo
		},
	}
	errs := Validate(f)
	if !hasCode(errs, "cycle_detected") {
		t.Errorf("esperado erro cycle_detected, obteve: %v", errs)
	}
}

func TestValidate_DecisionMissingBranch(t *testing.T) {
	// decision com apenas 1 edge de saída
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
			{ID: "d", Type: NodeTypeDecision},
			{ID: "e", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "s", To: "d"},
			{From: "d", To: "e", Label: "valid"}, // falta "invalid"
		},
	}
	errs := Validate(f)
	if !hasCode(errs, "decision_missing_branch") {
		t.Errorf("esperado erro decision_missing_branch, obteve: %v", errs)
	}
}

func TestValidate_DecisionMissingLabel(t *testing.T) {
	// decision com 2 edges mas labels errados
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
			{ID: "d", Type: NodeTypeDecision},
			{ID: "e1", Type: NodeTypeWait},
			{ID: "e2", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "s", To: "d"},
			{From: "d", To: "e1", Label: "yes"},  // label errado
			{From: "d", To: "e2", Label: "no"},   // label errado
		},
	}
	errs := Validate(f)
	if !hasCode(errs, "decision_missing_branch") {
		t.Errorf("esperado erro decision_missing_branch por label ausente, obteve: %v", errs)
	}
}

func TestValidate_DanglingReference(t *testing.T) {
	// edge aponta para nó inexistente
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
		},
		Edges: []FlowEdge{
			{From: "s", To: "fantasma"}, // nó "fantasma" não existe
		},
	}
	errs := Validate(f)
	if !hasCode(errs, "dangling_node_reference") {
		t.Errorf("esperado erro dangling_node_reference, obteve: %v", errs)
	}
}

func TestValidate_ValidFlow(t *testing.T) {
	// Fluxo completo válido com bifurcação decision
	f := &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
			{ID: "d", Type: NodeTypeDecision},
			{ID: "ok", Type: NodeTypeWait},
			{ID: "nok", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "s", To: "d"},
			{From: "d", To: "ok", Label: "valid"},
			{From: "d", To: "nok", Label: "invalid"},
		},
	}
	errs := Validate(f)
	if len(errs) != 0 {
		t.Errorf("esperado fluxo válido sem erros, obteve: %v", errs)
	}
}

// hasCode verifica se a slice de erros contém um erro com o código informado.
func hasCode(errs []ValidationError, code string) bool {
	for _, e := range errs {
		if e.Code == code {
			return true
		}
	}
	return false
}
