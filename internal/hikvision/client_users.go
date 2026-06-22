package hikvision

// client_users.go implements ISAPI user operations: paginated list and clear.
// SOURCED: legacy/hik-api/app/Service/HikVision/User/UserService.php
// Endpoints verified:
//   POST /ISAPI/AccessControl/UserInfo/Search  (UserService.php:60)
//   PUT  /ISAPI/AccessControl/UserInfo/Clear   (UserService.php:272)
//
// CHK072 (security): searchID is always generated internally via newSearchID() (uuid.go).
// It is NEVER accepted from HTTP requests to prevent ISAPI injection.
// CHK009: ClearUsers is atomic on the ISAPI — no partial rollback is possible.
// Timeout returns a Go context error; the handler maps it to 504 with action guidance.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DeviceUser is a single user record returned by ListUsers.
// SOURCED: UserService.php:parseUserList — fields from ISAPI UserInfo JSON.
type DeviceUser struct {
	EmployeeNo string `json:"employeeNo"` // CPF digits (identity)
	Name       string `json:"name"`
	UserType   string `json:"userType"` // "normal" | "admin"
	NumOfFace  int    `json:"numOfFace"`
	Valid      bool   `json:"valid"`
	BeginTime  string `json:"beginTime"` // ISO 8601
	EndTime    string `json:"endTime"`   // ISO 8601
}

// ListUsers returns a page of users from the device.
// SOURCED: UserService.php:49-92 — POST /ISAPI/AccessControl/UserInfo/Search with
// searchResultPosition = (page-1)*perPage and maxResults = perPage.
// searchID is generated internally (CHK072 — see uuid.go:newSearchID).
// Returns (users, totalCount, error). totalCount may be 0 if not reported by firmware.
func (c *Client) ListUsers(ctx context.Context, page, perPage int) ([]DeviceUser, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 100
	}

	// CHK072: searchID gerado internamente — nunca aceitar do request HTTP (CHK072 — sem injeção ISAPI)
	payload := map[string]any{
		"UserInfoSearchCond": map[string]any{
			"searchID":             newSearchID(),
			"searchResultPosition": (page - 1) * perPage,
			"maxResults":           perPage,
		},
	}
	b, _ := json.Marshal(payload) //nolint:errcheck
	respBody, status, err := c.doRequest(ctx, http.MethodPost,
		"/ISAPI/AccessControl/UserInfo/Search?format=json",
		strings.NewReader(string(b)), "application/json")
	if err != nil {
		return nil, 0, fmt.Errorf("hikvision: ListUsers: %w", err)
	}
	if status != 200 {
		return nil, 0, retriableOrNot("ListUsers", status, respBody)
	}

	// Parse: {"UserInfoSearch":{"responseStatusStrg":"OK","numOfMatches":N,"totalMatches":M,
	//         "UserInfo":[{"employeeNo":"...","name":"...",...}]}}
	var resp struct {
		UserInfoSearch struct {
			NumOfMatches  int          `json:"numOfMatches"`
			TotalMatches  int          `json:"totalMatches"`
			UserInfo      []DeviceUser `json:"UserInfo"`
		} `json:"UserInfoSearch"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, 0, fmt.Errorf("hikvision: ListUsers JSON: %w (body: %.120s)", err, string(respBody))
	}
	users := resp.UserInfoSearch.UserInfo
	if users == nil {
		users = []DeviceUser{}
	}
	return users, resp.UserInfoSearch.TotalMatches, nil
}

// ClearUsers removes all users from the device via PUT /ISAPI/AccessControl/UserInfo/Clear.
// SOURCED: UserService.php:269-299 — body is empty; 200 or 204 = success.
// CHK009: this operation is ATOMIC on the ISAPI — no partial rollback is possible.
// On timeout the Go context error propagates; the HTTP handler maps it to 504
// with {"error":"...","action":"verificar dispositivo manualmente"}.
func (c *Client) ClearUsers(ctx context.Context) error {
	_, status, err := c.doRequest(ctx, http.MethodPut,
		"/ISAPI/AccessControl/UserInfo/Clear", nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: ClearUsers: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("ClearUsers", status, nil)
}
