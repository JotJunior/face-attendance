package hikvision

// client_stats.go implements ISAPI device statistics aggregation.
//
// GetDeviceStats aggregates 4 ISAPI calls — SOURCED from legacy/hik2go/src/Hik2go/Stats.php:globalStats():
//   (1) GET /ISAPI/AccessControl/UserInfo/Count?format=json       → users.total/faces/cards
//   (2) GET /ISAPI/AccessControl/UserInfo/capabilities?format=json → users.max
//   (3) POST /ISAPI/AccessControl/AcsEventTotalNum?format=json    → events.total
//   (4) GET /ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json → events.max
//
// Field mapping verified against Stats.php:globalStats() — keys sourced from PHP array paths.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	// endpointUserCount — total enrolled users with faces and cards.
	// SOURCED: Stats.php:usersCount() — GET /ISAPI/AccessControl/UserInfo/Count?format=json
	endpointUserCount = "/ISAPI/AccessControl/UserInfo/Count?format=json"

	// endpointUserCapabilities — maximum user capacity.
	// SOURCED: Stats.php:usersCapabilities() — GET /ISAPI/AccessControl/UserInfo/capabilities?format=json
	endpointUserCapabilities = "/ISAPI/AccessControl/UserInfo/capabilities?format=json"

	// endpointEventCount — total access control events.
	// SOURCED: Stats.php:eventsCount() — POST /ISAPI/AccessControl/AcsEventTotalNum?format=json
	endpointEventCount = "/ISAPI/AccessControl/AcsEventTotalNum?format=json"

	// endpointEventCapabilities — maximum event log capacity.
	// SOURCED: Stats.php:eventsCapabilities() — GET /ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json
	endpointEventCapabilities = "/ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json"
)

// UserStats holds enrolled-user statistics from the device.
// SOURCED: Stats.php:globalStats() — users.total/faces/cards/max field names.
type UserStats struct {
	Total int // total enrolled users — SOURCED: UserInfoCount.userNumber
	Faces int // users with enrolled face — SOURCED: UserInfoCount.bindFaceUserNumber
	Cards int // users with enrolled card — SOURCED: UserInfoCount.bindCardUserNumber
	Max   int // device capacity — SOURCED: UserInfo.maxRecordNum
}

// EventStats holds access event log statistics from the device.
// SOURCED: Stats.php:globalStats() — events.total/max field names.
type EventStats struct {
	Total int // total logged events — SOURCED: AcsEventTotalNum.totalNum
	Max   int // device event log capacity — SOURCED: AcsEvent.totalNum.@max
}

// DeviceStats aggregates both user and event statistics.
// Spec §FR-016.
type DeviceStats struct {
	Users  UserStats
	Events EventStats
}

// userCountResponse maps /UserInfo/Count?format=json.
// SOURCED: Stats.php:usersCount() — data.UserInfoCount.{userNumber,bindFaceUserNumber,bindCardUserNumber}
type userCountResponse struct {
	UserInfoCount struct {
		UserNumber          int `json:"userNumber"`
		BindFaceUserNumber  int `json:"bindFaceUserNumber"`
		BindCardUserNumber  int `json:"bindCardUserNumber"`
	} `json:"UserInfoCount"`
}

// userCapabilitiesResponse maps /UserInfo/capabilities?format=json.
// SOURCED: Stats.php:usersCapabilities() — data.UserInfo.maxRecordNum
type userCapabilitiesResponse struct {
	UserInfo struct {
		MaxRecordNum int `json:"maxRecordNum"`
	} `json:"UserInfo"`
}

// eventCountResponse maps /AcsEventTotalNum?format=json (POST response).
// SOURCED: Stats.php:eventsCount() — data.AcsEventTotalNum.totalNum
type eventCountResponse struct {
	AcsEventTotalNum struct {
		TotalNum int `json:"totalNum"`
	} `json:"AcsEventTotalNum"`
}

// eventCapabilitiesResponse maps /AcsEventTotalNum/capabilities?format=json.
// SOURCED: Stats.php:eventsCapabilities() — data.AcsEvent.totalNum.@max
// The "@max" JSON key comes from PHP's XML-to-array conversion (attribute prefix "@").
type eventCapabilitiesResponse struct {
	AcsEvent struct {
		TotalNum struct {
			Max int `json:"@max"`
		} `json:"totalNum"`
	} `json:"AcsEvent"`
}

// GetDeviceStats aggregates 4 ISAPI calls into a single DeviceStats value.
// SOURCED: Stats.php:globalStats() — executes usersCount, usersCapabilities,
// eventsCount, eventsCapabilities then maps to the consolidated struct.
// Calls are sequential (firmware may not handle concurrent digest auth well).
// Error messages include which step failed (tasks 1.10.3).
func (c *Client) GetDeviceStats(ctx context.Context) (*DeviceStats, error) {
	// (1) User count
	bodyUC, statusUC, err := c.doRequest(ctx, http.MethodGet, endpointUserCount, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/Count]: %w", err)
	}
	if statusUC != 200 {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/Count]: %w",
			retriableOrNot("UserInfo/Count", statusUC, bodyUC))
	}
	var ucResp userCountResponse
	if err := json.Unmarshal(bodyUC, &ucResp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/Count] parse: %w", err)
	}

	// (2) User capabilities (max capacity)
	bodyCap, statusCap, err := c.doRequest(ctx, http.MethodGet, endpointUserCapabilities, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/capabilities]: %w", err)
	}
	if statusCap != 200 {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/capabilities]: %w",
			retriableOrNot("UserInfo/capabilities", statusCap, bodyCap))
	}
	var capResp userCapabilitiesResponse
	if err := json.Unmarshal(bodyCap, &capResp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [UserInfo/capabilities] parse: %w", err)
	}

	// (3) Event count — POST with condition body
	// SOURCED: Stats.php:eventsCount() — AcsEventTotalNumCond: {major:0, minor:0}
	eventCondBody := `{"AcsEventTotalNumCond":{"major":0,"minor":0}}`
	bodyEC, statusEC, err := c.doRequest(ctx, http.MethodPost, endpointEventCount,
		strings.NewReader(eventCondBody), "application/json")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum]: %w", err)
	}
	if statusEC != 200 {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum]: %w",
			retriableOrNot("AcsEventTotalNum", statusEC, bodyEC))
	}
	var ecResp eventCountResponse
	if err := json.Unmarshal(bodyEC, &ecResp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum] parse: %w", err)
	}

	// (4) Event capabilities (max log capacity)
	bodyECap, statusECap, err := c.doRequest(ctx, http.MethodGet, endpointEventCapabilities, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum/capabilities]: %w", err)
	}
	if statusECap != 200 {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum/capabilities]: %w",
			retriableOrNot("AcsEventTotalNum/capabilities", statusECap, bodyECap))
	}
	var ecapResp eventCapabilitiesResponse
	if err := json.Unmarshal(bodyECap, &ecapResp); err != nil {
		return nil, fmt.Errorf("hikvision: GetDeviceStats [AcsEventTotalNum/capabilities] parse: %w", err)
	}

	return &DeviceStats{
		Users: UserStats{
			Total: ucResp.UserInfoCount.UserNumber,
			Faces: ucResp.UserInfoCount.BindFaceUserNumber,
			Cards: ucResp.UserInfoCount.BindCardUserNumber,
			Max:   capResp.UserInfo.MaxRecordNum,
		},
		Events: EventStats{
			Total: ecResp.AcsEventTotalNum.TotalNum,
			Max:   ecapResp.AcsEvent.TotalNum.Max,
		},
	}, nil
}
