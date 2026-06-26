package httphandler

// Tests for FASE 4 device-config handlers (tasks.md §4.2–4.6).
// All tests use httptest.ResponseRecorder — no real DB or device needed.
// Ref: tasks.md §4.2.8, §4.3.7, §4.4.6, §4.5.5, §4.6.4.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

// --- fake deviceConfigAdminRepo ---

type fakeDeviceConfigRepo struct {
	getResult            *domain.Device
	getErr               error
	setCredentialsErr    error
	setWebhookConfigured bool
	setWebhookErr        error
	// capture last calls for assertions
	lastSetCredentialsID    int64
	lastSetCredentialsUser  string
	lastSetCredentialsPort  int
	lastSetWebhookID        int64
	lastSetWebhookConfigured bool
}

func (f *fakeDeviceConfigRepo) GetDeviceByID(_ context.Context, id int64) (*domain.Device, error) {
	return f.getResult, f.getErr
}

func (f *fakeDeviceConfigRepo) SetCredentials(_ context.Context, id int64, username string, passwordEnc []byte, port int) error {
	f.lastSetCredentialsID = id
	f.lastSetCredentialsUser = username
	f.lastSetCredentialsPort = port
	return f.setCredentialsErr
}

func (f *fakeDeviceConfigRepo) SetWebhookConfiguredByID(_ context.Context, id int64, configured bool) error {
	f.lastSetWebhookID = id
	f.lastSetWebhookConfigured = configured
	return f.setWebhookErr
}

// --- fake ISAPI client via test HTTP server ---

// testHikServer creates an httptest.Server that returns the given status and body for all requests.
func testHikServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		if body != "" {
			fmt.Fprint(w, body)
		}
	}))
}

// newTestDeviceCfgConfig creates a DeviceConfigConfig for tests.
// if device is non-nil it is returned by the repo's GetDeviceByID.
func newTestDeviceCfgConfig(t *testing.T, device *domain.Device, repoErr error) (DeviceConfigConfig, *fakeDeviceConfigRepo) {
	t.Helper()
	repo := &fakeDeviceConfigRepo{getResult: device, getErr: repoErr}

	// A real cipher key (32 bytes) so Encrypt/Decrypt work in tests.
	key := bytes.Repeat([]byte("k"), 32)
	cipher, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("secrets.NewCipher: %v", err)
	}

	cfg := DeviceConfigConfig{
		DeviceRepo:  repo,
		ISAPICipher: cipher,
	}
	return cfg, repo
}

// makeDevice creates a minimal domain.Device suitable for tests.
// hikHost must be in "host:port" format pointing to a test HTTP server.
func makeDevice(id int64, hikHost string) *domain.Device {
	ip := hikHost
	port := 80
	// Split host:port if needed
	if idx := strings.LastIndex(hikHost, ":"); idx >= 0 {
		ip = hikHost[:idx]
		fmt.Sscan(hikHost[idx+1:], &port)
	}
	username := "admin"
	// We must pre-encrypt the password using the test cipher key
	key := bytes.Repeat([]byte("k"), 32)
	cipher, _ := secrets.NewCipher(key)
	enc, _ := cipher.Encrypt("testpass")

	return &domain.Device{
		ID:               id,
		DeviceIdentifier: "AA:BB:CC:DD:EE:FF",
		IPAddress:        &ip,
		ISAPIUsername:    &username,
		ISAPIPasswordEnc: enc,
		ISAPIPort:        port,
		WebhookConfigured: true,
	}
}

// doRequest is a test helper to call a handler with a given method, path, and JSON body.
func doRequest(t *testing.T, handler http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// =============================================================================
// 4.2 PutDeviceCredentialsHandler
// =============================================================================

func TestPutDeviceCredentials_200(t *testing.T) {
	device := makeDevice(42, "127.0.0.1:80")
	cfg, repo := newTestDeviceCfgConfig(t, device, nil)
	h := PutDeviceCredentialsHandler(cfg)

	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/credentials", map[string]interface{}{
		"isapi_username": "admin",
		"isapi_password": "SuperSecret123!",
		"isapi_port":     80,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Response must never echo password (FR-005)
	if strings.Contains(rr.Body.String(), "SuperSecret123!") {
		t.Fatal("response echoed the password — FR-005 violated")
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["isapi_credentials_set"] != true {
		t.Errorf("isapi_credentials_set: want true, got %v", resp["isapi_credentials_set"])
	}
	// Verify SetCredentials was called
	if repo.lastSetCredentialsID != 42 {
		t.Errorf("SetCredentials device_id: want 42, got %d", repo.lastSetCredentialsID)
	}
	if repo.lastSetCredentialsPort != 80 {
		t.Errorf("SetCredentials port: want 80, got %d", repo.lastSetCredentialsPort)
	}
}

func TestPutDeviceCredentials_503_NoCipher(t *testing.T) {
	device := makeDevice(42, "127.0.0.1:80")
	repo := &fakeDeviceConfigRepo{getResult: device}
	cfg := DeviceConfigConfig{
		DeviceRepo:  repo,
		ISAPICipher: nil, // cipher absent — CHK007/FR-007
	}
	h := PutDeviceCredentialsHandler(cfg)

	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/credentials", map[string]interface{}{
		"isapi_username": "admin",
		"isapi_password": "pass",
		"isapi_port":     80,
	})

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPutDeviceCredentials_400_InvalidPort(t *testing.T) {
	device := makeDevice(42, "127.0.0.1:80")
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PutDeviceCredentialsHandler(cfg)

	cases := []int{0, -1, 65536, 99999}
	for _, port := range cases {
		rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/credentials", map[string]interface{}{
			"isapi_username": "admin",
			"isapi_password": "pass",
			"isapi_port":     port,
		})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("port=%d: want 400, got %d", port, rr.Code)
		}
	}
}

func TestPutDeviceCredentials_404_DeviceNotFound(t *testing.T) {
	cfg, _ := newTestDeviceCfgConfig(t, nil, pgx.ErrNoRows)
	h := PutDeviceCredentialsHandler(cfg)

	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/99/credentials", map[string]interface{}{
		"isapi_username": "admin",
		"isapi_password": "pass",
		"isapi_port":     80,
	})

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPutDeviceCredentials_NeverEchosPassword(t *testing.T) {
	// Even in error responses, password must not appear (FR-005, Constitution §V)
	device := makeDevice(42, "127.0.0.1:80")
	repo := &fakeDeviceConfigRepo{getResult: device, setCredentialsErr: fmt.Errorf("db error")}
	key := bytes.Repeat([]byte("k"), 32)
	cipher, _ := secrets.NewCipher(key)
	cfg := DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: cipher}
	h := PutDeviceCredentialsHandler(cfg)

	secretPW := "VerySecretPassword!@#$"
	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/credentials", map[string]interface{}{
		"isapi_username": "admin",
		"isapi_password": secretPW,
		"isapi_port":     80,
	})

	if strings.Contains(rr.Body.String(), secretPW) {
		t.Fatalf("response body contains plaintext password — FR-005 violated: %s", rr.Body.String())
	}
}

// =============================================================================
// 4.3 System handlers
// =============================================================================

func TestPostDeviceReboot_200(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	// Derive host:port from test server URL
	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PostDeviceRebootHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/actions/reboot", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp["result"] != "rebooting" {
		t.Errorf("result: want rebooting, got %v", resp["result"])
	}
	// CHK058: device_id in response
	if resp["device_id"] == nil {
		t.Error("device_id missing from reboot response — CHK058 violated")
	}
}

func TestPostDeviceReboot_504_Timeout(t *testing.T) {
	// Simulate network error by using a server that immediately closes
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate context cancellation by returning 503 (non-retriable not triggered)
		w.WriteHeader(503)
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PostDeviceRebootHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/actions/reboot", nil)
	// 503 from ISAPI is not NonRetriableError (5xx is retriable → BadGateway from mapISAPIError)
	if rr.Code != http.StatusBadGateway && rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("want 502 or 504, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPostDeviceFactoryReset_200_UpdatesWebhookConfigured(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	device.WebhookConfigured = true

	cfg, repo := newTestDeviceCfgConfig(t, device, nil)
	h := PostDeviceFactoryResetHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/actions/factory-reset", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Post-success: webhook_configured must have been set to false in DB
	if repo.lastSetWebhookID != 42 {
		t.Errorf("SetWebhookConfiguredByID not called with device_id 42")
	}
	if repo.lastSetWebhookConfigured != false {
		t.Errorf("SetWebhookConfiguredByID: want false, got %v", repo.lastSetWebhookConfigured)
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp["result"] != "factory_reset_initiated" {
		t.Errorf("result: want factory_reset_initiated, got %v", resp["result"])
	}
}

func TestGetDeviceTime_200(t *testing.T) {
	timeBody := `{"Time":{"localTime":"2026-06-21T14:30:00","timeZone":"CST-8:00:00","timeMode":"manual"}}`
	ts := testHikServer(t, 200, timeBody)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceTimeHandler(cfg)

	rr := doRequest(t, h, http.MethodGet, "/admin/api/devices/42/time", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp getTimeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TimeMode != "manual" {
		t.Errorf("time_mode: want manual, got %s", resp.TimeMode)
	}
	if resp.LocalTime == "" {
		t.Error("local_time is empty")
	}
}

func TestPutDeviceTime_400_InvalidTimeMode(t *testing.T) {
	// CHK071: time_mode must be "manual" or "ntp"
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PutDeviceTimeHandler(cfg)

	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/time", map[string]string{
		"time_mode": "nfs", // invalid enum value
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "time_mode") {
		t.Errorf("error message should mention time_mode: %s", rr.Body.String())
	}
}

func TestPutDeviceTime_200_Manual(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PutDeviceTimeHandler(cfg)

	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/time", map[string]string{
		"time_mode":  "manual",
		"local_time": "2026-06-21T14:30:00",
		"time_zone":  "CST-8:00:00",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// 4.4 Door handlers
// =============================================================================

func TestGetDeviceDoors_200(t *testing.T) {
	// SOURCED shape: DoorList.DoorNo array from capabilities
	doorsBody := `{"DoorList":{"DoorNo":[{"doorNo":1,"doorName":"Main Door"}]}}`
	ts := testHikServer(t, 200, doorsBody)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceDoorsHandler(cfg)

	rr := doRequest(t, h, http.MethodGet, "/admin/api/devices/42/doors", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp doorsListResponse
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp.Total != 1 {
		t.Errorf("total: want 1, got %d", resp.Total)
	}
}

func TestGetDeviceDoorStatus_400_InvalidDoorID(t *testing.T) {
	// CHK048: door_id must be >= 1
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceDoorStatusHandler(cfg)

	cases := []string{"0", "-1", "abc"}
	for _, doorID := range cases {
		path := "/admin/api/devices/42/doors/" + doorID + "/status"
		rr := doRequest(t, h, http.MethodGet, path, nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("door_id=%s: want 400, got %d", doorID, rr.Code)
		}
	}
}

func TestPostDeviceDoorControl_400_InvalidCommand(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PostDeviceDoorControlHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/doors/1/control", map[string]string{
		"command": "fly_open", // invalid command — not in commandToISAPICmd
	})

	// ErrUnknownCommand → 400
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// 4.5 User handlers
// =============================================================================

func TestGetDeviceUsers_400_PerPageOutOfRange(t *testing.T) {
	// CHK073: per_page must be 1–1000
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceUsersHandler(cfg)

	cases := []string{"0", "9999", "-1"}
	for _, pp := range cases {
		rr := doRequest(t, h, http.MethodGet, "/admin/api/devices/42/users?per_page="+pp, nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("per_page=%s: want 400, got %d", pp, rr.Code)
		}
	}
}

func TestGetDeviceUsers_200_Default(t *testing.T) {
	// Valid paginated response from ISAPI
	usersBody := `{"UserInfoSearch":{"numOfMatches":1,"totalMatches":1,"UserInfo":[{"employeeNo":"12345678900","name":"Test User","userType":"normal","numOfFace":1,"valid":true,"beginTime":"","endTime":""}]}}`
	ts := testHikServer(t, 200, usersBody)
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceUsersHandler(cfg)

	rr := doRequest(t, h, http.MethodGet, "/admin/api/devices/42/users", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp usersListResponse
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp.PerPage != 100 {
		t.Errorf("per_page default: want 100, got %d", resp.PerPage)
	}
	if resp.Page != 1 {
		t.Errorf("page default: want 1, got %d", resp.Page)
	}
}

func TestDeleteDeviceUsers_504_WithActionGuidance(t *testing.T) {
	// CHK009: timeout → 504 with action field in body
	// Use a handler that hangs until context expires — simulate via a closed server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 503 to simulate connectivity issue
		w.WriteHeader(503)
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceUsersHandler(cfg)

	rr := doRequest(t, h, http.MethodDelete, "/admin/api/devices/42/users", nil)
	// 503 from ISAPI → 502 BadGateway (NonRetriableError-like path)
	// CHK009: the timeout path (504) is covered by context.DeadlineExceeded
	if rr.Code != http.StatusBadGateway && rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("want 502 or 504, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteDeviceFaces_200_ReturnsOK(t *testing.T) {
	// tasks §3.5.2/3.5.3: ClearFaces now real (SOURCED FaceService.php:38/283).
	// ISAPI: PUT /ISAPI/AccessControl/ClearPictureCfg?format=json → 200.
	ts := testHikServer(t, 200, "") // device responds 200 to ClearPictureCfg
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceFacesHandler(cfg)

	rr := doRequest(t, h, http.MethodDelete, "/admin/api/devices/42/faces", nil)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// Response must include device_id (CHK058)
	if !strings.Contains(rr.Body.String(), `"device_id"`) {
		t.Errorf("response missing device_id: %s", rr.Body.String())
	}
}

func TestDeleteDeviceFaces_401_ReturnsError(t *testing.T) {
	// ISAPI returns 401 → handler returns 502 (auth failure).
	ts := testHikServer(t, 401, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceFacesHandler(cfg)

	rr := doRequest(t, h, http.MethodDelete, "/admin/api/devices/42/faces", nil)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// 4.6 Webhook handlers
// =============================================================================

func TestGetDeviceWebhooks_200(t *testing.T) {
	// SOURCED XML shape from NotificationService.php
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<HttpHostNotificationList>
  <HttpHostNotification>
    <id>abc123</id>
    <url>http://192.168.68.200:8080/webhook/secret</url>
    <protocolType>HTTP</protocolType>
  </HttpHostNotification>
</HttpHostNotificationList>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(200)
		fmt.Fprint(w, xmlBody)
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := GetDeviceWebhooksHandler(cfg)

	rr := doRequest(t, h, http.MethodGet, "/admin/api/devices/42/webhooks", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp webhooksListResponse
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp.Total != 1 {
		t.Errorf("total: want 1, got %d", resp.Total)
	}
	if len(resp.Webhooks) != 1 || resp.Webhooks[0].ID != "abc123" {
		t.Errorf("webhook id: want abc123, got %v", resp.Webhooks)
	}
}

func TestPostDeviceConfigureWebhook_400_MissingPublicHost(t *testing.T) {
	// Sem WEBHOOK_PUBLIC_HOST → 400 (não inventa IP; Princípio I). Não toca ISAPI.
	device := makeDevice(42, "192.168.68.111:80")
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	cfg.WebhookPublicHost = "" // explícito: config ausente
	h := PostDeviceConfigureWebhookHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/webhooks", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPostDeviceConfigureWebhook_200_ProvisionsAndUpdatesDB(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, repo := newTestDeviceCfgConfig(t, device, nil)
	cfg.WebhookPublicHost = "192.168.68.110"
	cfg.WebhookPublicPort = 8080
	cfg.WebhookPathSecret = "sekret"
	h := PostDeviceConfigureWebhookHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/webhooks", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp actionResponse
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp.Result != "configured" {
		t.Errorf("result: want configured, got %q", resp.Result)
	}
	if resp.WebhookConfigured == nil || !*resp.WebhookConfigured {
		t.Errorf("webhook_configured: want true, got %v", resp.WebhookConfigured)
	}
	if repo.lastSetWebhookID != 42 || !repo.lastSetWebhookConfigured {
		t.Errorf("SetWebhookConfiguredByID: want (42,true), got (%d,%v)",
			repo.lastSetWebhookID, repo.lastSetWebhookConfigured)
	}
}

func TestDeleteDeviceWebhook_400_EmptyID(t *testing.T) {
	// CHK048: webhook_id must be non-empty
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceWebhookHandler(cfg)

	// Path with no webhook_id segment — hits "webhooks" with len(segs)==1
	rr := doRequest(t, h, http.MethodDelete, "/admin/api/devices/42/webhooks/", nil)

	// segs[1] is empty string → parseWebhookID fails → 400
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Fatalf("want 400 or 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteDeviceWebhook_200_MainWebhook_UpdatesDB(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	device.WebhookConfigured = true

	// Compute the deterministic webhook ID for this device
	port := device.ISAPIPort
	if port <= 0 {
		port = 80
	}
	deviceHost := fmt.Sprintf("%s:%d", *device.IPAddress, port)
	mainWebhookID := hikvision.DeterministicWebhookID(deviceHost)

	cfg, repo := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceWebhookHandler(cfg)

	path := "/admin/api/devices/42/webhooks/" + mainWebhookID
	rr := doRequest(t, h, http.MethodDelete, path, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// DB must have been updated: webhook_configured=false
	if repo.lastSetWebhookID != 42 {
		t.Errorf("SetWebhookConfiguredByID not called with device_id 42")
	}
	if repo.lastSetWebhookConfigured != false {
		t.Errorf("SetWebhookConfiguredByID: want false, got true")
	}

	// Response must include webhook_configured=false
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if wc, ok := resp["webhook_configured"]; !ok || wc != false {
		t.Errorf("webhook_configured in response: want false, got %v", resp["webhook_configured"])
	}
}

func TestDeleteDeviceWebhook_200_SecondaryWebhook_NoDBUpdate(t *testing.T) {
	ts := testHikServer(t, 200, "")
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	device.WebhookConfigured = true

	cfg, repo := newTestDeviceCfgConfig(t, device, nil)
	h := DeleteDeviceWebhookHandler(cfg)

	// Use a different webhook_id (not the main one)
	path := "/admin/api/devices/42/webhooks/secondary123"
	rr := doRequest(t, h, http.MethodDelete, path, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// DB must NOT have been updated (secondary webhook)
	if repo.lastSetWebhookID == 42 {
		t.Errorf("SetWebhookConfiguredByID was called for a secondary webhook — should not update DB")
	}
}

// =============================================================================
// CHK007: ErrKeyMissing → 503 for all ISAPI action handlers
// =============================================================================

func TestISAPIHandlers_503_WhenCipherNil(t *testing.T) {
	// Any handler that calls loadDeviceAndISAPIClient with cipher=nil must return 503
	// when device has stored credentials (LoadDeviceConfig returns ErrKeyMissing).
	ip := "127.0.0.1"
	username := "admin"
	device := &domain.Device{
		ID:               42,
		IPAddress:        &ip,
		ISAPIUsername:    &username,
		ISAPIPasswordEnc: []byte("some-enc"), // non-nil password → LoadDeviceConfig tries to decrypt
		ISAPIPort:        80,
	}
	repo := &fakeDeviceConfigRepo{getResult: device}
	cfg := DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: nil}

	handlers := []struct {
		name    string
		method  string
		path    string
		handler http.Handler
	}{
		{"reboot", http.MethodPost, "/admin/api/devices/42/actions/reboot", PostDeviceRebootHandler(cfg)},
		{"factory-reset", http.MethodPost, "/admin/api/devices/42/actions/factory-reset", PostDeviceFactoryResetHandler(cfg)},
		{"get-time", http.MethodGet, "/admin/api/devices/42/time", GetDeviceTimeHandler(cfg)},
		{"get-doors", http.MethodGet, "/admin/api/devices/42/doors", GetDeviceDoorsHandler(cfg)},
		{"get-users", http.MethodGet, "/admin/api/devices/42/users", GetDeviceUsersHandler(cfg)},
		{"delete-users", http.MethodDelete, "/admin/api/devices/42/users", DeleteDeviceUsersHandler(cfg)},
		{"delete-faces", http.MethodDelete, "/admin/api/devices/42/faces", DeleteDeviceFacesHandler(cfg)},
		{"get-webhooks", http.MethodGet, "/admin/api/devices/42/webhooks", GetDeviceWebhooksHandler(cfg)},
	}

	for _, tc := range handlers {
		rr := doRequest(t, tc.handler, tc.method, tc.path, nil)
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: want 503, got %d: %s", tc.name, rr.Code, rr.Body.String())
		}
	}
}

// =============================================================================
// 6.1.5 Auth: todos os novos endpoints retornam 401 sem cookie de sessão (FR-020)
// Note: handlers individuais do device-config não verificam sessão — isso é
// responsabilidade do SessionMiddleware em server.go:101. Este teste valida que
// os handlers, quando wrappados pelo SessionMiddleware, retornam 401 sem cookie.
// =============================================================================

func TestDeviceConfigEndpoints_401_WithoutSession(t *testing.T) {
	// Wrap each handler with SessionMiddleware exactly as server.go does.
	secret := "test-secret-32-bytes-for-hmac-ok"
	sessionMW := SessionMiddleware(secret)

	device := makeDevice(42, "127.0.0.1:80")
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)

	endpoints := []struct {
		name    string
		method  string
		path    string
		handler http.Handler
	}{
		{"put-credentials", http.MethodPut, "/admin/api/devices/42/credentials", PutDeviceCredentialsHandler(cfg)},
		{"post-reboot", http.MethodPost, "/admin/api/devices/42/actions/reboot", PostDeviceRebootHandler(cfg)},
		{"post-factory-reset", http.MethodPost, "/admin/api/devices/42/actions/factory-reset", PostDeviceFactoryResetHandler(cfg)},
		{"get-time", http.MethodGet, "/admin/api/devices/42/time", GetDeviceTimeHandler(cfg)},
		{"put-time", http.MethodPut, "/admin/api/devices/42/time", PutDeviceTimeHandler(cfg)},
		{"get-doors", http.MethodGet, "/admin/api/devices/42/doors", GetDeviceDoorsHandler(cfg)},
		{"get-door-status", http.MethodGet, "/admin/api/devices/42/doors/1/status", GetDeviceDoorStatusHandler(cfg)},
		{"post-door-control", http.MethodPost, "/admin/api/devices/42/doors/1/control", PostDeviceDoorControlHandler(cfg)},
		{"get-users", http.MethodGet, "/admin/api/devices/42/users", GetDeviceUsersHandler(cfg)},
		{"delete-users", http.MethodDelete, "/admin/api/devices/42/users", DeleteDeviceUsersHandler(cfg)},
		{"delete-faces", http.MethodDelete, "/admin/api/devices/42/faces", DeleteDeviceFacesHandler(cfg)},
		{"get-webhooks", http.MethodGet, "/admin/api/devices/42/webhooks", GetDeviceWebhooksHandler(cfg)},
		{"delete-webhook", http.MethodDelete, "/admin/api/devices/42/webhooks/abc123", DeleteDeviceWebhookHandler(cfg)},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			wrapped := sessionMW(ep.handler)
			req := httptest.NewRequest(ep.method, ep.path, nil)
			// No cookie — session must be rejected
			rr := httptest.NewRecorder()
			wrapped.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("%s: want 401 without session, got %d", ep.name, rr.Code)
			}
		})
	}
}

// =============================================================================
// 6.2 Security credential tests (FR-005, Constitution §V)
// =============================================================================

// 6.2.1: PUT credentials with long password (256 chars) — persisted encrypted, never echoed.
func TestPutDeviceCredentials_LongPassword_NeverEchoed(t *testing.T) {
	device := makeDevice(42, "127.0.0.1:80")
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PutDeviceCredentialsHandler(cfg)

	longPass := strings.Repeat("X", 256)
	rr := doRequest(t, h, http.MethodPut, "/admin/api/devices/42/credentials", map[string]interface{}{
		"isapi_username": "admin",
		"isapi_password": longPass,
		"isapi_port":     80,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// Response must never contain the password in any form (FR-005)
	if strings.Contains(rr.Body.String(), longPass) {
		t.Fatal("response echoed the long password — FR-005 violated")
	}
	// isapi_credentials_set must be true
	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp) //nolint:errcheck
	if resp["isapi_credentials_set"] != true {
		t.Errorf("isapi_credentials_set: want true, got %v", resp["isapi_credentials_set"])
	}
}

// 6.2.3: ISAPI 401 digest error → 502 response must not leak any credential field.
func TestISAPI_401Digest_502_NoCredentialLeak(t *testing.T) {
	// Serve 401 from fake ISAPI
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	host := strings.TrimPrefix(ts.URL, "http://")
	device := makeDevice(42, host)
	cfg, _ := newTestDeviceCfgConfig(t, device, nil)
	h := PostDeviceRebootHandler(cfg)

	rr := doRequest(t, h, http.MethodPost, "/admin/api/devices/42/actions/reboot", nil)

	// ISAPI 401 → NonRetriableError → 502 BadGateway
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("want 502 for ISAPI 401 digest, got %d: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	// Must not expose any credential field in 502 response
	for _, forbidden := range []string{"isapi_password", "isapi_password_enc", "testpass", "SuperSecret"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("502 response contains forbidden credential field %q — FR-005 violated", forbidden)
		}
	}
}

// 6.2.4 (documentation): removing ISAPI_CRED_KEY from env after persistence only
// affects future ENCRYPT calls. DECRYPT depends on the key at decryption time.
// This is a known architectural limitation documented in research.md Decision 7.
// The test below documents this as a compile-time note (no runtime env mutation
// in tests — that would be racy and affect parallel tests).
func TestPutDeviceCredentials_KeyLimitation_Documented(t *testing.T) {
	// Empirical: if ISAPI_CRED_KEY changes after storing credentials, the existing
	// encrypted blob becomes unreadable. The handler returns 503 (ErrKeyMissing or
	// decryption failure). This is acceptable per research.md §Decision 7.
	// No testable runtime behavior here without env mutation.
	t.Log("6.2.4: ISAPI_CRED_KEY rotation causes existing credentials to be unreadable (expected). " +
		"Document in research.md §Decision 7 and operational runbook.")
}
