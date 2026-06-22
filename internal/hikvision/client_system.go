package hikvision

// client_system.go implements ISAPI system operations: reboot, factory-reset, get/set time, NTP.
// SOURCED: legacy/hik-api/app/Service/HikVision/Device/DeviceService.php
// Endpoints verified:
//   PUT /ISAPI/System/reboot              (DeviceService.php:222)
//   PUT /ISAPI/System/factoryReset        (DeviceService.php:189)
//   GET /ISAPI/System/time                (DeviceService.php:251)
//   PUT /ISAPI/System/time                (DeviceService.php:292)
//
// NTP endpoints — SOURCED from device test (192.168.68.107, HTTP 200, 2026-06-21):
//   PUT /ISAPI/System/time                (XML, timeMode=NTP + timeZone) → 200
//   PUT /ISAPI/System/time/ntpServers/{id} (XML, NTPServer shape) → 200
// NOT /ISAPI/System/Network/NTPServers (different endpoint, rejected 404 on tested firmware).

import (
	"context"
	"encoding/json"
	"encoding/xml"
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
// NTP mode is SOURCED from device test (192.168.68.107, 2026-06-21): XML body with
// timeMode=NTP + timeZone → 200. Additional NTP server config via SetNTPServer.
type TimeSetRequest struct {
	LocalTime string // ISO 8601 datetime (required for manual mode)
	TimeZone  string // e.g. "CST-8:00:00" (optional; device uses current if empty)
	TimeMode  string // "manual" | "ntp"
}

// NTPServerRequest is the body for PUT /ISAPI/System/time/ntpServers/{id}.
// SOURCED from device test (192.168.68.107, 2026-06-21, HTTP 200).
// addressingFormatType: "hostname" or "ipaddress".
// portNo default: 123. synchronizeInterval in minutes (e.g. 60).
type NTPServerRequest struct {
	ID                   int    // NTP server slot ID (1-based)
	AddressingFormatType string // "hostname" or "ipaddress"
	HostName             string // hostname or IP of the NTP server
	PortNo               int    // UDP port, default 123
	SynchronizeInterval  int    // sync interval in minutes, e.g. 60
}

// ntpServerXML is the XML shape for PUT /ISAPI/System/time/ntpServers/{id}.
// SOURCED from device test (192.168.68.107, 2026-06-21, HTTP 200).
type ntpServerXML struct {
	XMLName              xml.Name `xml:"NTPServer"`
	ID                   int      `xml:"id"`
	AddressingFormatType string   `xml:"addressingFormatType"`
	HostName             string   `xml:"hostName"`
	PortNo               int      `xml:"portNo"`
	SynchronizeInterval  int      `xml:"synchronizeInterval"`
}

// setTimeXML is the XML shape for PUT /ISAPI/System/time (NTP mode).
// SOURCED from device test (192.168.68.107, 2026-06-21, HTTP 200).
type setTimeXML struct {
	XMLName  xml.Name `xml:"Time"`
	TimeMode string   `xml:"timeMode"`
	TimeZone string   `xml:"timeZone"`
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

// SetTime sends PUT /ISAPI/System/time to the device.
// Manual mode: SOURCED — DeviceService.php:278-320, JSON body {Time:{timeMode,localTime,timeZone}}.
// NTP mode: SOURCED — device test (192.168.68.107, 2026-06-21, HTTP 200):
//
//	XML Content-Type, body <Time><timeMode>NTP</timeMode><timeZone>{tz}</timeZone></Time>
//	(The device accepts plain timeMode=NTP via XML; JSON was not accepted in NTP mode on
//	this firmware — use XML for NTP, JSON for manual.)
func (c *Client) SetTime(ctx context.Context, req TimeSetRequest) error {
	if strings.EqualFold(req.TimeMode, "ntp") {
		// NTP mode: XML body (SOURCED from device test 192.168.68.107 2026-06-21).
		tz := req.TimeZone
		xmlPayload := setTimeXML{TimeMode: "NTP", TimeZone: tz}
		b, err := xml.Marshal(xmlPayload)
		if err != nil {
			return fmt.Errorf("hikvision: SetTime: marshal XML: %w", err)
		}
		_, status, reqErr := c.doRequest(ctx, http.MethodPut, "/ISAPI/System/time",
			strings.NewReader(xml.Header+string(b)), "application/xml")
		if reqErr != nil {
			return fmt.Errorf("hikvision: SetTime (NTP): %w", reqErr)
		}
		if status == 200 || status == 204 {
			return nil
		}
		return retriableOrNot("SetTime(NTP)", status, nil)
	}

	// Manual mode: JSON body (SOURCED DeviceService.php:278-320).
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

// SetNTPServer sends PUT /ISAPI/System/time/ntpServers/{id} with an XML body.
// SOURCED from device test (192.168.68.107, 2026-06-21, HTTP 200).
// portNo default 123. synchronizeInterval in minutes (e.g. 60).
// addressingFormatType: "hostname" or "ipaddress".
func (c *Client) SetNTPServer(ctx context.Context, req NTPServerRequest) error {
	id := req.ID
	if id <= 0 {
		id = 1 // default slot
	}
	portNo := req.PortNo
	if portNo <= 0 {
		portNo = 123
	}
	syncInterval := req.SynchronizeInterval
	if syncInterval <= 0 {
		syncInterval = 60
	}
	addrType := req.AddressingFormatType
	if addrType == "" {
		addrType = "hostname"
	}

	xmlPayload := ntpServerXML{
		ID:                   id,
		AddressingFormatType: addrType,
		HostName:             req.HostName,
		PortNo:               portNo,
		SynchronizeInterval:  syncInterval,
	}
	b, err := xml.Marshal(xmlPayload)
	if err != nil {
		return fmt.Errorf("hikvision: SetNTPServer: marshal XML: %w", err)
	}

	path := fmt.Sprintf("/ISAPI/System/time/ntpServers/%d", id)
	_, status, reqErr := c.doRequest(ctx, http.MethodPut, path,
		strings.NewReader(xml.Header+string(b)), "application/xml")
	if reqErr != nil {
		return fmt.Errorf("hikvision: SetNTPServer: %w", reqErr)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("SetNTPServer", status, nil)
}
