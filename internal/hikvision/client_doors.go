package hikvision

// client_doors.go implements ISAPI door operations: capabilities, status, control, config.
// SOURCED: legacy/hik-api/app/Service/HikVision/Door/DoorService.php
// Endpoints verified:
//   GET  /ISAPI/AccessControl/Door/capabilities?format=json    (DoorService.php:67)
//   POST /ISAPI/AccessControl/Door/Status?format=json          (DoorService.php:110)
//   PUT  /ISAPI/AccessControl/RemoteControl/door/{N}           (DoorService.php:313)
//   GET  /ISAPI/AccessControl/Door/{N}?format=json             (DoorService.php:179)

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DoorInfo holds capability information for one door returned by GetDoorCapabilities.
// SOURCED: DoorService.php:parseDoorList.
type DoorInfo struct {
	DoorNo   int    `json:"doorNo"`
	DoorName string `json:"doorName"`
}

// DoorStatus holds the current status of one door.
// SOURCED: DoorService.php:parseDoorStatus.
type DoorStatus struct {
	DoorNo    int    `json:"doorNo"`
	LockState string `json:"lockState"` // "locked" | "unlocked" | "unknown"
}

// DoorConfig holds the editable configuration of one door.
// SOURCED: DoorService.php:parseDoorConfig (L430).
type DoorConfig struct {
	DoorNo       int `json:"doorNo"`
	OpenDuration int `json:"openDuration"` // seconds the door stays open after unlock
}

// commandToISAPICmd maps our API command names to ISAPI cmd values.
// SOURCED: DoorService.php CMD_* constants (L38-46).
var commandToISAPICmd = map[string]string{
	"open":         "open",
	"close":        "close",
	"always_open":  "alwaysOpen",
	"always_closed": "alwaysClosed",
	"normal":       "normalOpen",
}

// GetDoorCapabilities retrieves the list of doors available on the device.
// SOURCED: DoorService.php:56-97 — GET /ISAPI/AccessControl/Door/capabilities?format=json.
func (c *Client) GetDoorCapabilities(ctx context.Context) ([]DoorInfo, error) {
	respBody, status, err := c.doRequest(ctx, http.MethodGet,
		"/ISAPI/AccessControl/Door/capabilities?format=json", nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorCapabilities: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("GetDoorCapabilities", status, respBody)
	}

	// Parse JSON: {"DoorList":{"DoorNo":[{"doorNo":1,"doorName":"Door1"},...]},...}
	// The ISAPI may use different envelope shapes; handle gracefully.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorCapabilities JSON: %w (body: %.120s)", err, string(respBody))
	}

	// Try DoorList.DoorNo array first
	var doors []DoorInfo
	if dl, ok := raw["DoorList"]; ok {
		var doorList struct {
			DoorNo []struct {
				DoorNo   int    `json:"doorNo"`
				DoorName string `json:"doorName"`
			} `json:"DoorNo"`
		}
		if err := json.Unmarshal(dl, &doorList); err == nil {
			for _, d := range doorList.DoorNo {
				doors = append(doors, DoorInfo{DoorNo: d.DoorNo, DoorName: d.DoorName})
			}
		}
	}
	// Fallback: single-door devices may return a flat {"doorNo":1,...}
	if len(doors) == 0 {
		var single struct {
			DoorNo   int    `json:"doorNo"`
			DoorName string `json:"doorName"`
		}
		if err := json.Unmarshal(respBody, &single); err == nil && single.DoorNo > 0 {
			doors = append(doors, DoorInfo{DoorNo: single.DoorNo, DoorName: single.DoorName})
		}
	}
	return doors, nil
}

// GetDoorStatus retrieves the current lock state of a specific door.
// SOURCED: DoorService.php:99-146 — POST /ISAPI/AccessControl/Door/Status?format=json.
func (c *Client) GetDoorStatus(ctx context.Context, doorID int) (*DoorStatus, error) {
	payload := map[string]any{
		"DoorStatusList": map[string]any{
			"DoorStatus": []map[string]any{
				{"doorID": doorID},
			},
		},
	}
	b, _ := json.Marshal(payload) //nolint:errcheck
	respBody, status, err := c.doRequest(ctx, http.MethodPost,
		"/ISAPI/AccessControl/Door/Status?format=json",
		strings.NewReader(string(b)), "application/json")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorStatus(%d): %w", doorID, err)
	}
	if status != 200 {
		return nil, retriableOrNot(fmt.Sprintf("GetDoorStatus(%d)", doorID), status, respBody)
	}

	// Parse: {"DoorStatusList":{"DoorStatus":[{"doorNo":1,"lockState":"locked"}]}}
	var resp struct {
		DoorStatusList struct {
			DoorStatus []struct {
				DoorNo    int    `json:"doorNo"`
				LockState string `json:"lockState"`
			} `json:"DoorStatus"`
		} `json:"DoorStatusList"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorStatus JSON: %w (body: %.120s)", err, string(respBody))
	}
	for _, ds := range resp.DoorStatusList.DoorStatus {
		if ds.DoorNo == doorID {
			return &DoorStatus{DoorNo: ds.DoorNo, LockState: ds.LockState}, nil
		}
	}
	// Fallback: return first entry if doorNo matching fails (single-door)
	if len(resp.DoorStatusList.DoorStatus) > 0 {
		ds := resp.DoorStatusList.DoorStatus[0]
		return &DoorStatus{DoorNo: ds.DoorNo, LockState: ds.LockState}, nil
	}
	return nil, fmt.Errorf("hikvision: GetDoorStatus(%d): door not found in response", doorID)
}

// ControlDoor sends a control command to the specified door.
// SOURCED: DoorService.php:sendCommand (L307-311) — PUT with XML <RemoteControlDoor><cmd>...</cmd>.
// Returns ErrUnknownCommand if command is not in commandToISAPICmd.
func (c *Client) ControlDoor(ctx context.Context, doorID int, command string) error {
	isapiCmd, ok := commandToISAPICmd[command]
	if !ok {
		return fmt.Errorf("%w: %q (valid: open, close, always_open, always_closed, normal)",
			ErrUnknownCommand, command)
	}

	xmlBody := fmt.Sprintf(
		`<RemoteControlDoor version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
			`<cmd>%s</cmd>`+
			`</RemoteControlDoor>`,
		xmlEscape(isapiCmd),
	)
	path := fmt.Sprintf("/ISAPI/AccessControl/RemoteControl/door/%d", doorID)
	_, status, err := c.doRequest(ctx, http.MethodPut, path,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: ControlDoor(%d, %q): %w", doorID, command, err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot(fmt.Sprintf("ControlDoor(%d,%q)", doorID, command), status, nil)
}

// GetDoorConfig retrieves the configuration (e.g., openDuration) for a specific door.
// SOURCED: DoorService.php:getConfig (L168-208) — GET /ISAPI/AccessControl/Door/{N}?format=json.
func (c *Client) GetDoorConfig(ctx context.Context, doorID int) (*DoorConfig, error) {
	path := fmt.Sprintf("/ISAPI/AccessControl/Door/%d?format=json", doorID)
	respBody, status, err := c.doRequest(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorConfig(%d): %w", doorID, err)
	}
	if status != 200 {
		return nil, retriableOrNot(fmt.Sprintf("GetDoorConfig(%d)", doorID), status, respBody)
	}

	// Parse: {"Door":{"doorNo":1,"openDuration":5,...}}
	var resp struct {
		Door struct {
			DoorNo       int `json:"doorNo"`
			OpenDuration int `json:"openDuration"`
		} `json:"Door"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDoorConfig JSON: %w (body: %.120s)", err, string(respBody))
	}
	return &DoorConfig{
		DoorNo:       resp.Door.DoorNo,
		OpenDuration: resp.Door.OpenDuration,
	}, nil
}
