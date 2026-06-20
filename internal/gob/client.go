// Package gob provides the HTTP client for the GOB State API.
// Contracts: contracts/gob-api.md (all fields verified from t.txt).
package gob

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
)

const (
	membersPath      = "/api/face-detection/members"
	attendancePath   = "/attendance/3ff4708cb695ad1a6e9f87cb714e1f22"
	defaultTimeout   = 30 * time.Second
)

// Client is the GOB API HTTP client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// New creates a Client with the given base URL and bearer token.
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewWithHTTPClient creates a Client using a custom *http.Client (useful for tests).
func NewWithHTTPClient(baseURL, token string, hc *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: hc,
	}
}

// gobMemberResponse is the shape of each item in the GOB members response.
// Field names mirror the GOB JSON keys (verified in contracts/gob-api.md §Campos verificados).
type gobMemberItem struct {
	ID              int64   `json:"id"`
	Status          string  `json:"status"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
	FederalDocument string  `json:"federal_document"`
	Name            string  `json:"name"`
	MobileNumber    *string `json:"mobile_number"`
	URLSelfie       *string `json:"url_selfie"`
}

// gobMembersResponse is the envelope returned by GET /api/face-detection/members.
type gobMembersResponse struct {
	Success bool            `json:"success"`
	Data    []gobMemberItem `json:"data"`
}

// ListMembers fetches all members from the GOB API.
// Returns an error if success != true or if the HTTP status is not 2xx.
// Paginacao: the GOB API currently returns all data in a single response (no pagination
// fields observed in contracts/gob-api.md); if pagination is added, the caller must
// iterate (adaptive behavior — CHK006).
func (c *Client) ListMembers(ctx context.Context) ([]domain.Member, error) {
	url := c.baseURL + membersPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gob: create request: %w", err)
	}
	// Uses Bearer prefix (contracts/gob-api.md §Request: Authorization: Bearer {token})
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gob: do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gob: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gob: HTTP %d from %s", resp.StatusCode, url)
	}

	var envelope gobMembersResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("gob: unmarshal response: %w", err)
	}

	if !envelope.Success {
		return nil, fmt.Errorf("gob: response success=false from %s", url)
	}

	return mapGobMembers(envelope.Data), nil
}

// mapGobMembers converts GOB API items to domain.Member values.
func mapGobMembers(items []gobMemberItem) []domain.Member {
	members := make([]domain.Member, 0, len(items))
	for _, item := range items {
		m := domain.Member{
			GobID:           item.ID,
			FederalDocument: item.FederalDocument,
			Name:            item.Name,
			Status:          item.Status,
			MobileNumber:    item.MobileNumber,
			URLSelfie:       item.URLSelfie,
		}
		// Parse GOB timestamps (ISO 8601 strings)
		if t, err := time.Parse(time.RFC3339Nano, item.CreatedAt); err == nil {
			m.GobCreatedAt = &t
		}
		if t, err := time.Parse(time.RFC3339Nano, item.UpdatedAt); err == nil {
			m.GobUpdatedAt = &t
		}
		members = append(members, m)
	}
	return members
}

// MarkAttendance marks attendance for the given CPF digits via the GOB API.
// The CPF is formatted as "NNN.NNN.NNN-NN" before sending (contracts/gob-api.md §Body).
// The Authorization header does NOT use the Bearer prefix (contracts/gob-api.md §Request note).
func (c *Client) MarkAttendance(ctx context.Context, cpfDigits string) error {
	// Format CPF to masked form before sending (contracts/gob-api.md §Body: cpf: "00.000.000-00")
	masked, err := formatCPFForAttendance(cpfDigits)
	if err != nil {
		return fmt.Errorf("gob: MarkAttendance: invalid CPF: %w", err)
	}

	url := c.baseURL + attendancePath

	bodyJSON := fmt.Sprintf(`{"cpf":%q}`, masked)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		strings.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("gob: MarkAttendance: create request: %w", err)
	}
	// ATTENTION: no "Bearer" prefix for attendance endpoint (contracts/gob-api.md §Note).
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gob: MarkAttendance: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gob: MarkAttendance: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// formatCPFForAttendance validates and formats CPF for the attendance endpoint.
func formatCPFForAttendance(cpfDigits string) (string, error) {
	// Validate: must have exactly 11 digits
	digits := cleanDigits(cpfDigits)
	if len(digits) != 11 {
		return "", fmt.Errorf("CPF must have 11 digits, got %d", len(digits))
	}
	// Format: NNN.NNN.NNN-NN (verified from t.txt:48 and contracts/gob-api.md §Body)
	return fmt.Sprintf("%s.%s.%s-%s", digits[0:3], digits[3:6], digits[6:9], digits[9:11]), nil
}

// cleanDigits strips non-digit characters (mirrors domain.cleanDigits for package isolation).
func cleanDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
