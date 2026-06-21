package hikvision

// client_system.go implements ISAPI system operations: reboot, factory-reset, get/set time.
// SOURCED: legacy/hik-api/app/Service/HikVision/Device/DeviceService.php
// Endpoints verified:
//   PUT /ISAPI/System/reboot       (DeviceService.php:222)
//   PUT /ISAPI/System/factoryReset (DeviceService.php:189)
//   GET /ISAPI/System/time         (DeviceService.php:251)
//   PUT /ISAPI/System/time         (DeviceService.php:292)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// TimeData holds the time configuration retrieved from the device.
// Fields map to ISAPI response keys (DeviceService.php:parseTimeData, L395-406).
type TimeData struct {
	LocalTime string `json:"local_time"` // ISO 8601 e.g. "2026-06-21T14:30:00"
	TimeZone  string `json:"time_zone"`  // e.g. "CST-8:00:00"
	TimeMode  string `json:"time_mode"`  // "manual" | "ntp"
}

// TimeSetRequest is the body for PUT /ISAPI/System/time.
// CHK071: time_mode must be "manual" or "ntp" — validated by the HTTP handler, not here.
// CHK042: NTP mode is [PROPOSTA] — firmware may return 4xx; handler maps to 502.
type TimeSetRequest struct {
	LocalTime string // ISO 8601 datetime (required for manual mode)
	TimeZone  string // e.g. "CST-8:00:00" (optional; device uses current if empty)
	TimeMode  string // "manual" | "ntp"
}

// Reboot sends PUT /ISAPI/System/reboot to the device.
// SOURCED: DeviceService.php:222-246. The body is empty (device requires PUT, not POST).
// Returns nil on 200/204; NonRetriableError on 4xx; retriable error on 5xx/network.
func (c *Client) Reboot(ctx context.Context) error {
	_, status, err := c.doRequest(ctx, http.MethodPut, "/ISAPI/System/reboot", nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: Reboot: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("Reboot", status, nil)
}

// FactoryReset sends PUT /ISAPI/System/factoryReset with body {"mode":"basic"}.
// SOURCED: DeviceService.php:186-217 — uses 'basic' mode (vs 'complete' which wipes network).
// Returns nil on 200/204; NonRetriableError on 4xx.
func (c *Client) FactoryReset(ctx context.Context) error {
	body := `{"mode":"basic"}`
	_, status, err := c.doRequest(ctx, http.MethodPut, "/ISAPI/System/factoryReset",
		strings.NewReader(body), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: FactoryReset: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("FactoryReset", status, nil)
}

// GetTime retrieves the current time configuration from the device.
// SOURCED: DeviceService.php:251-270 (GET) + parseTimeData:395-406 (JSON parse).
// Response is JSON: {"Time":{"localTime":"...","timeZone":"...","timeMode":"..."}}.
func (c *Client) GetTime(ctx context.Context) (*TimeData, error) {
	respBody, status, err := c.doRequest(ctx, http.MethodGet, "/ISAPI/System/time?format=json", nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetTime: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("GetTime", status, respBody)
	}

	// Parse JSON response. The ISAPI wraps data in {"Time":{...}}.
	// SOURCED: parseTimeData (DeviceService.php:395-406) reads data['Time']['localTime'] etc.
	var wrapper struct {
		Time struct {
			LocalTime string `json:"localTime"`
			TimeZone  string `json:"timeZone"`
			TimeMode  string `json:"timeMode"`
		} `json:"Time"`
	}
	if err := json.Unmarshal(respBody, &wrapper); err != nil {
		return nil, fmt.Errorf("hikvision: GetTime JSON parse: %w (body: %.120s)", err, string(respBody))
	}

	td := wrapper.Time
	return &TimeData{
		LocalTime: td.LocalTime,
		TimeZone:  td.TimeZone,
		TimeMode:  td.TimeMode,
	}, nil
}

// SetTime sends PUT /ISAPI/System/time with a JSON body.
// Manual mode is SOURCED (DeviceService.php:278-320: timeMode=manual + localTime + timeZone).
// NTP mode is [PROPOSTA — validar empiricamente (CHK042)]: if the firmware returns 4xx,
// the HTTP handler maps it to 502 with an orientative message.
func (c *Client) SetTime(ctx context.Context, req TimeSetRequest) error {
	payload := map[string]any{
		"Time": map[string]any{
			"timeMode":  req.TimeMode,
			"localTime": req.LocalTime,
			"timeZone":  req.TimeZone,
		},
	}
	b, _ := json.Marshal(payload) //nolint:errcheck
	_, status, err := c.doRequest(ctx, http.MethodPut, "/ISAPI/System/time?format=json",
		strings.NewReader(string(b)), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: SetTime: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("SetTime", status, nil)
}
