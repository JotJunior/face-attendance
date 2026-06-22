package hikvision

// client_verify.go implementa o plano semanal de verificação (verify mode por janela).
// SOURCED: legacy/hik-api/old/src/Device/DSK1T673DWX/Preferences/AuthMode.php
//   GET /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json
//   PUT /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json   (read-modify-write)
//
// O verifyMode de cada slot define QUAIS métodos de verificação são aceitos naquela
// janela de horário. Se um slot não aceita face, o leitor RECONHECE o rosto mas NEGA
// o acesso. "faceOrFpOrCardOrPw" inclui face — valor SOURCED de captura real de evento
// num device em operação (legacy/hik-api/t.json + docs/webhook-payload-format.md:
// currentVerifyMode "faceOrFpOrCardOrPw" em grants de face bem-sucedidos).
//
// Princípio I: o PUT é read-modify-write — preserva INTEGRALMENTE o corpo que o device
// devolve e só sobrescreve a chave verifyMode (única tocada pelo legado). Nada da
// estrutura interna é fabricado; um valor recusado pelo firmware falha alto no PUT.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	endpointVerifyWeekPlan = "/ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json"

	// verifyModeFaceIncluded ACEITA face (e tb impressão digital/cartão/senha).
	// SOURCED: currentVerifyMode em grants reais (legacy/hik-api/t.json, device .115).
	verifyModeFaceIncluded = "faceOrFpOrCardOrPw"
)

// GetVerifyWeekPlan retorna o corpo cru (JSON) do plano semanal de verificação (slot 1).
func (c *Client) GetVerifyWeekPlan(ctx context.Context) ([]byte, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointVerifyWeekPlan, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetVerifyWeekPlan: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("GetVerifyWeekPlan", status, body)
	}
	return body, nil
}

// EnsureFaceVerifyMode garante que TODOS os slots do plano semanal de verificação
// aceitam face (verifyMode "faceOrFpOrCardOrPw"), via read-modify-write preservando
// os demais campos. Retorna changed=true quando algo foi alterado (PUT enviado).
// Idempotente: se todos os slots já estão no modo, não escreve.
func (c *Client) EnsureFaceVerifyMode(ctx context.Context) (bool, error) {
	body, err := c.GetVerifyWeekPlan(ctx)
	if err != nil {
		return false, err
	}

	// Preserva a estrutura inteira; só mexe em VerifyWeekPlanCfg.WeekPlanCfg[].verifyMode.
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, fmt.Errorf("hikvision: EnsureFaceVerifyMode parse: %w (body: %.120s)", err, string(body))
	}
	cfg, ok := doc["VerifyWeekPlanCfg"].(map[string]any)
	if !ok {
		return false, fmt.Errorf("hikvision: EnsureFaceVerifyMode: resposta sem VerifyWeekPlanCfg (body: %.120s)", string(body))
	}
	plans, ok := cfg["WeekPlanCfg"].([]any)
	if !ok {
		return false, fmt.Errorf("hikvision: EnsureFaceVerifyMode: sem WeekPlanCfg (body: %.120s)", string(body))
	}

	changed := false
	for _, p := range plans {
		m, ok := p.(map[string]any)
		if !ok {
			continue
		}
		if cur, _ := m["verifyMode"].(string); cur != verifyModeFaceIncluded {
			m["verifyMode"] = verifyModeFaceIncluded
			changed = true
		}
	}
	if !changed {
		return false, nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return false, fmt.Errorf("hikvision: EnsureFaceVerifyMode marshal: %w", err)
	}
	_, status, putErr := c.doRequest(ctx, http.MethodPut, endpointVerifyWeekPlan,
		strings.NewReader(string(out)), "application/json")
	if putErr != nil {
		return false, fmt.Errorf("hikvision: EnsureFaceVerifyMode PUT: %w", putErr)
	}
	if status == 200 || status == 204 {
		return true, nil
	}
	return false, retriableOrNot("EnsureFaceVerifyMode PUT", status, nil)
}
