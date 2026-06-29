package flow

import (
	"encoding/json"
	"testing"
)

// decisionFlow monta um fluxo start→decision com 2 ramos (valid/invalid) e a
// config fornecida no nó decision. config nil = config legada (vazia).
func decisionFlow(t *testing.T, config interface{}) *Flow {
	t.Helper()
	var raw json.RawMessage
	if config != nil {
		b, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		raw = b
	}
	return &Flow{
		Nodes: []FlowNode{
			{ID: "s", Type: NodeTypeStart},
			{ID: "d", Type: NodeTypeDecision, Config: raw},
			{ID: "v", Type: NodeTypeWait},
			{ID: "i", Type: NodeTypeWait},
		},
		Edges: []FlowEdge{
			{From: "s", To: "d"},
			{From: "d", To: "v", Label: "valid"},
			{From: "d", To: "i", Label: "invalid"},
		},
	}
}

func TestValidateDecision_EmptyConfigIsLegacyValid(t *testing.T) {
	// Config nil e config {} devem ser válidas (comportamento legado).
	for _, cfg := range []interface{}{nil, map[string]any{}} {
		errs := Validate(decisionFlow(t, cfg))
		if len(errs) != 0 {
			t.Errorf("config %v: esperado válido, obteve: %v", cfg, errs)
		}
	}
}

func TestValidateDecision_Facial(t *testing.T) {
	errs := Validate(decisionFlow(t, DecisionConfig{Source: DecisionSourceFacial}))
	if len(errs) != 0 {
		t.Errorf("facial: esperado válido, obteve: %v", errs)
	}
}

func TestValidateDecision_HTTPCodeValid(t *testing.T) {
	cfg := DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonCode, ExpectedStatus: "200, 201,204"}
	errs := Validate(decisionFlow(t, cfg))
	if len(errs) != 0 {
		t.Errorf("http_code válido: obteve: %v", errs)
	}
}

func TestValidateDecision_ResponseValueValid(t *testing.T) {
	cfg := DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonValue, Field: "data.result", Value: "true"}
	errs := Validate(decisionFlow(t, cfg))
	if len(errs) != 0 {
		t.Errorf("response_value válido: obteve: %v", errs)
	}
}

func TestValidateDecision_Invalid(t *testing.T) {
	cases := []struct {
		name string
		cfg  DecisionConfig
		code string
	}{
		{"source desconhecido", DecisionConfig{Source: "magic"}, "decision_invalid_source"},
		{"https sem comparison", DecisionConfig{Source: DecisionSourceHTTPS}, "decision_invalid_comparison"},
		{"https comparison inválido", DecisionConfig{Source: DecisionSourceHTTPS, Comparison: "x"}, "decision_invalid_comparison"},
		{"http_code vazio", DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonCode}, "decision_invalid_status"},
		{"http_code não-numérico", DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonCode, ExpectedStatus: "200,abc"}, "decision_invalid_status"},
		{"http_code fora de faixa", DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonCode, ExpectedStatus: "99"}, "decision_invalid_status"},
		{"response_value sem field", DecisionConfig{Source: DecisionSourceHTTPS, Comparison: DecisionComparisonValue, Value: "true"}, "decision_invalid_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := Validate(decisionFlow(t, tc.cfg))
			if !hasCode(errs, tc.code) {
				t.Errorf("esperado erro %q, obteve: %v", tc.code, errs)
			}
		})
	}
}
