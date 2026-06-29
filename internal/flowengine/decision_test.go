package flowengine

import (
	"encoding/json"
	"testing"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
)

func TestParseStatusSet(t *testing.T) {
	set := parseStatusSet("200, 201,204, ,abc,")
	for _, want := range []int{200, 201, 204} {
		if !set[want] {
			t.Errorf("esperado %d no conjunto", want)
		}
	}
	if set[500] {
		t.Error("500 não deveria estar no conjunto")
	}
	if len(set) != 3 {
		t.Errorf("esperado 3 entradas válidas, obteve %d (%v)", len(set), set)
	}
}

func TestExtractJSONField(t *testing.T) {
	body := []byte(`{"data":{"result":true,"status":"ACTIVE","count":42,"ratio":1.5,"obj":{"k":"v"}},"items":["a","b"]}`)
	cases := []struct {
		path    string
		want    string
		wantOK  bool
	}{
		{"data.result", "true", true},
		{"data.status", "ACTIVE", true},
		{"data.count", "42", true},
		{"data.ratio", "1.5", true},
		{"data.obj", `{"k":"v"}`, true},
		{"items.0", "a", true},
		{"items.1", "b", true},
		{"items.9", "", false},  // índice fora de faixa
		{"data.missing", "", false},
		{"data.result.x", "", false}, // descer numa folha
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := extractJSONField(body, tc.path)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("path %q: got (%q,%v), want (%q,%v)", tc.path, got, ok, tc.want, tc.wantOK)
		}
	}

	// JSON malformado → não resolve.
	if _, ok := extractJSONField([]byte(`{bad json`), "data"); ok {
		t.Error("JSON malformado deveria retornar ok=false")
	}
}

func TestEvaluateHTTPSDecision(t *testing.T) {
	jsonBody := []byte(`{"data":{"status":"active","result":true}}`)
	cases := []struct {
		name string
		cfg  flow.DecisionConfig
		http *httpCallResult
		want bool
	}{
		{"sem resposta", flow.DecisionConfig{Comparison: flow.DecisionComparisonCode, ExpectedStatus: "200"}, nil, false},
		{"http_code match", flow.DecisionConfig{Comparison: flow.DecisionComparisonCode, ExpectedStatus: "200,201"}, &httpCallResult{statusCode: 201}, true},
		{"http_code miss", flow.DecisionConfig{Comparison: flow.DecisionComparisonCode, ExpectedStatus: "200,201"}, &httpCallResult{statusCode: 500}, false},
		{"value match case/space-insensitive", flow.DecisionConfig{Comparison: flow.DecisionComparisonValue, Field: "data.status", Value: " ACTIVE "}, &httpCallResult{statusCode: 200, body: jsonBody}, true},
		{"value match bool", flow.DecisionConfig{Comparison: flow.DecisionComparisonValue, Field: "data.result", Value: "true"}, &httpCallResult{statusCode: 200, body: jsonBody}, true},
		{"value miss", flow.DecisionConfig{Comparison: flow.DecisionComparisonValue, Field: "data.status", Value: "inactive"}, &httpCallResult{statusCode: 200, body: jsonBody}, false},
		{"value field ausente", flow.DecisionConfig{Comparison: flow.DecisionComparisonValue, Field: "data.nope", Value: "x"}, &httpCallResult{statusCode: 200, body: jsonBody}, false},
		{"comparison inválido", flow.DecisionConfig{Comparison: "x"}, &httpCallResult{statusCode: 200}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evaluateHTTPSDecision(tc.cfg, tc.http); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluateDecision(t *testing.T) {
	e := &Engine{}
	authorized := "authorized"
	denied := "denied"
	mkNode := func(cfg interface{}) *flow.FlowNode {
		var raw json.RawMessage
		if cfg != nil {
			b, _ := json.Marshal(cfg)
			raw = b
		}
		return &flow.FlowNode{ID: "d", Type: flow.NodeTypeDecision, Config: raw}
	}
	ctxWith := func(status *string) flow.ExecutionContext {
		return flow.ExecutionContext{Event: &domain.AttendanceEvent{AttendanceStatus: status}}
	}

	// Config vazia/legada → nil (preserva valor propagado).
	if got := e.evaluateDecision(mkNode(nil), ctxWith(&authorized), nil); got != nil {
		t.Errorf("config vazia: esperado nil, obteve %v", *got)
	}
	if got := e.evaluateDecision(mkNode(map[string]any{}), ctxWith(&authorized), nil); got != nil {
		t.Errorf("config {}: esperado nil, obteve %v", *got)
	}

	// Facial: segue event.IsAuthorized().
	if got := e.evaluateDecision(mkNode(flow.DecisionConfig{Source: flow.DecisionSourceFacial}), ctxWith(&authorized), nil); got == nil || !*got {
		t.Errorf("facial autorizado: esperado true")
	}
	if got := e.evaluateDecision(mkNode(flow.DecisionConfig{Source: flow.DecisionSourceFacial}), ctxWith(&denied), nil); got == nil || *got {
		t.Errorf("facial negado: esperado false")
	}

	// HTTPS: delega a evaluateHTTPSDecision (here apenas o caminho feliz).
	cfg := flow.DecisionConfig{Source: flow.DecisionSourceHTTPS, Comparison: flow.DecisionComparisonCode, ExpectedStatus: "200"}
	if got := e.evaluateDecision(mkNode(cfg), ctxWith(&denied), &httpCallResult{statusCode: 200}); got == nil || !*got {
		t.Errorf("https 200: esperado true mesmo com face negada")
	}

	// Idempotência: mesma entrada → mesmo resultado.
	a := e.evaluateDecision(mkNode(cfg), ctxWith(&denied), &httpCallResult{statusCode: 200})
	b := e.evaluateDecision(mkNode(cfg), ctxWith(&denied), &httpCallResult{statusCode: 200})
	if a == nil || b == nil || *a != *b {
		t.Errorf("decisão deve ser determinística: %v vs %v", a, b)
	}
}
