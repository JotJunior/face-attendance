package hikvision

// client_authmode.go implements ISAPI auth mode (verify week plan) operations.
//
// GetVerifyMode — SOURCED from legacy/hik2go/src/Hik2go/Preferences/AuthMode.php:65-77
//   (infoWeekPlan method):
//   GET /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json
//   Response JSON: {"VerifyWeekPlanCfg":{"WeekPlanCfg":[{"weekNo":N,"verifyMode":"..."}]}}
//
// SetVerifyMode (read-modify-write) — SOURCED from AuthMode.php:19-35 (update method):
//   1. GET current plan via GetVerifyMode
//   2. Substitute verifyMode in ALL WeekPlanCfg entries
//   3. PUT /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json with full plan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// endpointVerifyWeekPlan is declared in client_verify.go (same package).
// SOURCED: AuthMode.php:27 (httpPut url) and AuthMode.php:68 (httpGet url).

// WeekPlanCfg represents a single day's verify mode configuration.
// SOURCED: AuthMode.php:22 — iterates over WeekPlanCfg entries by weekNo.
type WeekPlanCfg struct {
	WeekNo     int    `json:"weekNo"`
	VerifyMode string `json:"verifyMode"`
}

// VerifyWeekPlan is the top-level response from GET VerifyWeekPlanCfg.
// SOURCED: AuthMode.php:22 — accesses $payload['VerifyWeekPlanCfg']['WeekPlanCfg'].
type VerifyWeekPlan struct {
	WeekPlanCfgs []WeekPlanCfg `json:"WeekPlanCfg"`
}

// verifyWeekPlanEnvelope wraps the ISAPI JSON envelope for GET/PUT.
type verifyWeekPlanEnvelope struct {
	VerifyWeekPlanCfg VerifyWeekPlan `json:"VerifyWeekPlanCfg"`
}

// GetVerifyMode fetches the current verify week plan from the device.
// SOURCED: AuthMode.php:65-77 (infoWeekPlan) — GET /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json
func (c *Client) GetVerifyMode(ctx context.Context) (*VerifyWeekPlan, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointVerifyWeekPlan, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetVerifyMode: %w", err)
	}
	if status == 401 {
		return nil, &NonRetriableError{Op: "GetVerifyMode", Status: status}
	}
	if status != 200 {
		return nil, retriableOrNot("GetVerifyMode", status, body)
	}

	var env verifyWeekPlanEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("hikvision: GetVerifyMode parse: %w", err)
	}
	return &env.VerifyWeekPlanCfg, nil
}

// SetVerifyMode applies verifyMode to ALL week plan entries (read-modify-write).
// SOURCED: AuthMode.php:19-35 (update method).
// Idempotent: calling with the same mode twice produces identical payload (Constitution II).
//
// O round-trip preserva o corpo CRU do GET (via map[string]any), mexendo só em
// verifyMode de cada slot. O firmware DS-K1T673* rejeita (HTTP 400) um PUT que não
// traga o documento INTEGRAL — re-serializar a partir de um struct parcial (abordagem
// anterior) perdia campos obrigatórios (enable, week, id, TimeSegment, …) → 400.
// Mesma técnica usada por EnsureFaceVerifyMode (client_verify.go).
func (c *Client) SetVerifyMode(ctx context.Context, mode string) error {
	body, err := c.GetVerifyWeekPlan(ctx)
	if err != nil {
		return fmt.Errorf("hikvision: SetVerifyMode read: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("hikvision: SetVerifyMode parse: %w (body: %.120s)", err, string(body))
	}
	cfg, ok := doc["VerifyWeekPlanCfg"].(map[string]any)
	if !ok {
		return fmt.Errorf("hikvision: SetVerifyMode: resposta sem VerifyWeekPlanCfg (body: %.120s)", string(body))
	}
	plans, ok := cfg["WeekPlanCfg"].([]any)
	if !ok {
		return fmt.Errorf("hikvision: SetVerifyMode: sem WeekPlanCfg (body: %.120s)", string(body))
	}
	// Substitui verifyMode em TODOS os slots — SOURCED: AuthMode.php:22-24.
	for _, p := range plans {
		if m, ok := p.(map[string]any); ok {
			m["verifyMode"] = mode
		}
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("hikvision: SetVerifyMode marshal: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPut, endpointVerifyWeekPlan,
		bytes.NewReader(out), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: SetVerifyMode PUT: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("SetVerifyMode PUT", status, nil)
}
