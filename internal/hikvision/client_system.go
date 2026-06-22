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
	"time"
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

// NTPServerData is the parsed NTP server config read from the device.
type NTPServerData struct {
	HostName             string
	PortNo               int
	AddressingFormatType string
	SynchronizeInterval  int
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
// IMPORTANTE: /ISAPI/System/time IGNORA ?format=json e responde XML neste
// firmware (V4.48.20) — verificado no device 2026-06-22, ex.:
//
//	<Time version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
//	  <timeMode>NTP</timeMode><localTime>2026-...T..-03:00</localTime>
//	  <timeZone>CST+3:00:00</timeZone><IANA>Asia/Shanghai</IANA></Time>
//
// Parse XML primário + fallback JSON (firmwares que honrem format=json).
func (c *Client) GetTime(ctx context.Context) (*TimeData, error) {
	respBody, status, err := c.doRequest(ctx, http.MethodGet, "/ISAPI/System/time", nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetTime: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("GetTime", status, respBody)
	}

	// Primário: XML <Time><timeMode/><localTime/><timeZone/></Time> (com namespace;
	// xml.Unmarshal casa por nome local).
	var xmlData struct {
		LocalTime string `xml:"localTime"`
		TimeZone  string `xml:"timeZone"`
		TimeMode  string `xml:"timeMode"`
	}
	if xmlErr := xml.Unmarshal(respBody, &xmlData); xmlErr == nil &&
		(xmlData.TimeMode != "" || xmlData.LocalTime != "" || xmlData.TimeZone != "") {
		return &TimeData{
			LocalTime: xmlData.LocalTime,
			TimeZone:  xmlData.TimeZone,
			TimeMode:  xmlData.TimeMode,
		}, nil
	}

	// Fallback: JSON {"Time":{...}}.
	var jsonData struct {
		Time struct {
			LocalTime string `json:"localTime"`
			TimeZone  string `json:"timeZone"`
			TimeMode  string `json:"timeMode"`
		} `json:"Time"`
	}
	if jsonErr := json.Unmarshal(respBody, &jsonData); jsonErr == nil {
		return &TimeData{
			LocalTime: jsonData.Time.LocalTime,
			TimeZone:  jsonData.Time.TimeZone,
			TimeMode:  jsonData.Time.TimeMode,
		}, nil
	}

	return nil, fmt.Errorf("hikvision: GetTime: resposta não-parseável como XML nem JSON (body: %.120s)", string(respBody))
}

// ClockDrift calcula o desvio entre o relógio do device (localTime do GetTime) e `now`.
// Só retorna ok=true quando localTime traz OFFSET de fuso (RFC3339, ex.
// "2026-06-21T14:30:00-03:00"): aí o instante é absoluto e o drift é confiável. Sem
// offset (ex. "2026-06-21T14:30:00") NÃO dá pra medir desvio com segurança — não
// inventamos o fuso (Princípio I) — então ok=false e o chamador apenas avisa.
// devTime preserva a Location do offset reportado, p/ formatar uma correção no MESMO fuso.
func ClockDrift(localTime string, now time.Time) (devTime time.Time, drift time.Duration, ok bool) {
	lt := strings.TrimSpace(localTime)
	if lt == "" {
		return time.Time{}, 0, false
	}
	t, err := time.Parse(time.RFC3339, lt)
	if err != nil {
		// time.RFC3339 exige offset; localTime sem offset cai aqui → ok=false.
		return time.Time{}, 0, false
	}
	return t, now.Sub(t), true
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

// GetNTPServer reads the NTP server config for slot id (default 1).
// GET /ISAPI/System/time/ntpServers/{id} → XML <NTPServer>...</NTPServer>
// (mesmas tags do PUT; xml.Unmarshal casa por nome local, tolera namespace).
func (c *Client) GetNTPServer(ctx context.Context, id int) (*NTPServerData, error) {
	if id <= 0 {
		id = 1
	}
	path := fmt.Sprintf("/ISAPI/System/time/ntpServers/%d", id)
	respBody, status, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetNTPServer: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("GetNTPServer", status, respBody)
	}
	var x struct {
		HostName             string `xml:"hostName"`
		PortNo               int    `xml:"portNo"`
		AddressingFormatType string `xml:"addressingFormatType"`
		SynchronizeInterval  int    `xml:"synchronizeInterval"`
	}
	if xmlErr := xml.Unmarshal(respBody, &x); xmlErr != nil {
		return nil, fmt.Errorf("hikvision: GetNTPServer XML parse: %w (body: %.120s)", xmlErr, string(respBody))
	}
	return &NTPServerData{
		HostName:             x.HostName,
		PortNo:               x.PortNo,
		AddressingFormatType: x.AddressingFormatType,
		SynchronizeInterval:  x.SynchronizeInterval,
	}, nil
}
