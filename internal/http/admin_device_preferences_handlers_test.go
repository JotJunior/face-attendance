package httphandler

// Tests for FASE 2 device-preferences handlers (device-preferences tasks.md §FASE 2).
// All tests use httptest.ResponseRecorder + fake ISAPI server — no real DB or device needed.
// Shared helpers (fakeDeviceConfigRepo, makeDevice, testHikServer, doRequest, etc.) are
// defined in admin_device_config_handlers_test.go (same package).
// Ref: tasks.md §2.1.4, §2.2.4, §2.3.5, §2.4.3, §2.5.5, §2.6.2, §2.7.3, §2.8.4.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

// --- multipart helpers ---

// buildMultipartBody creates a multipart body with a single file field "file".
func buildMultipartBody(t *testing.T, contentType string, size int) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="test.jpg"`}
	h["Content-Type"] = []string{contentType}
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := io.Copy(part, io.LimitReader(bytes.NewReader(make([]byte, size)), int64(size))); err != nil {
		t.Fatalf("write part: %v", err)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// buildOverSizeMultipart creates a body > 20 MB to trigger MaxBytesReader.
func buildOverSizeMultipart(t *testing.T) (*bytes.Buffer, string) {
	t.Helper()
	oversizeBytes := maxUploadBodyBytes + 1024*1024 // 21 MB

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="big.jpg"`}
	h["Content-Type"] = []string{"image/jpeg"}
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	chunk := make([]byte, 65536)
	written := 0
	for written < oversizeBytes {
		n := len(chunk)
		if written+n > oversizeBytes {
			n = oversizeBytes - written
		}
		part.Write(chunk[:n]) //nolint:errcheck
		written += n
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// newNotFoundCfg returns a config where GetDeviceByID returns pgx.ErrNoRows → 404.
func newNotFoundCfg(t *testing.T) DeviceConfigConfig {
	t.Helper()
	repo := &fakeDeviceConfigRepo{getErr: pgx.ErrNoRows}
	key := bytes.Repeat([]byte("k"), 32)
	cipher, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("secrets.NewCipher: %v", err)
	}
	return DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: cipher}
}

// preferencesHikServer starts a test ISAPI server that dispatches by path.
// handlers is a map from path substring to response (status, body).
// Unmatched paths return 404.
type hikPathResponse struct {
	status int
	body   string
	ct     string // Content-Type, defaults to application/json
}

func preferencesHikServer(t *testing.T, dispatch map[string]hikPathResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		for substr, resp := range dispatch {
			if strings.Contains(path, substr) {
				ct := resp.ct
				if ct == "" {
					ct = "application/json"
				}
				w.Header().Set("Content-Type", ct)
				w.WriteHeader(resp.status)
				if resp.body != "" {
					fmt.Fprint(w, resp.body)
				}
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

// newPreferencesCfg returns a DeviceConfigConfig using the given test ISAPI server.
// Includes a real Cipher so LoadDeviceConfig can decrypt the test password (makeDevice encrypts it).
func newPreferencesCfg(t *testing.T, srv *httptest.Server) DeviceConfigConfig {
	t.Helper()
	device := makeDevice(1, srv.Listener.Addr().String())
	repo := &fakeDeviceConfigRepo{getResult: device}
	key := bytes.Repeat([]byte("k"), 32)
	cipher, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("secrets.NewCipher: %v", err)
	}
	return DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: cipher}
}

// =============================================================================
// validateImageContentType (tasks 1.14.1)
// =============================================================================

func TestValidateImageContentType_Unit(t *testing.T) {
	valid := []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp"}
	for _, ct := range valid {
		if err := validateImageContentType(ct); err != nil {
			t.Errorf("validateImageContentType(%q) = %v, want nil", ct, err)
		}
	}

	invalid := []string{"application/octet-stream", "text/plain", "video/mp4", "", "APPLICATION/JPEG"}
	for _, ct := range invalid {
		if err := validateImageContentType(ct); err == nil {
			t.Errorf("validateImageContentType(%q) = nil, want error", ct)
		}
	}
}

// TestMaxUploadBodyBytes_Value verifies the constant is exactly 20 MB (tasks 1.12.1).
func TestMaxUploadBodyBytes_Value(t *testing.T) {
	const expected = 20 * 1024 * 1024
	if maxUploadBodyBytes != expected {
		t.Errorf("maxUploadBodyBytes = %d, want %d (20 MB)", maxUploadBodyBytes, expected)
	}
}

// =============================================================================
// GetDeviceAuthModeHandler (tasks 2.1.1 / 2.1.4)
// =============================================================================

func TestGetDeviceAuthModeHandler_OK(t *testing.T) {
	body := `{"VerifyWeekPlanCfg":{"WeekPlanCfg":[{"weekNo":1,"verifyMode":"face"},{"weekNo":2,"verifyMode":"face"}]}}`
	srv := testHikServer(t, 200, body)
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/1/preferences/auth-mode", nil)
	rr := httptest.NewRecorder()
	GetDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp authModeResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.WeekPlans) != 2 {
		t.Errorf("expected 2 weekPlans, got %d", len(resp.WeekPlans))
	}
}

func TestGetDeviceAuthModeHandler_DeviceNotFound(t *testing.T) {
	cfg := newNotFoundCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/9999/preferences/auth-mode", nil)
	rr := httptest.NewRecorder()
	GetDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestGetDeviceAuthModeHandler_ISAPITimeout(t *testing.T) {
	// Use a server that closes immediately → connection refused → 504/502
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.Listener.Addr().String()
	srv.Close()

	device := makeDevice(1, addr)
	repo := &fakeDeviceConfigRepo{getResult: device}
	key := bytes.Repeat([]byte("k"), 32)
	cipher, _ := secrets.NewCipher(key)
	cfg := DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: cipher}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/1/preferences/auth-mode", nil)
	rr := httptest.NewRecorder()
	GetDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusGatewayTimeout && rr.Code != http.StatusBadGateway {
		t.Errorf("expected 504 or 502 for offline device, got %d", rr.Code)
	}
}

// =============================================================================
// PutDeviceAuthModeHandler (tasks 2.1.2 / 2.1.4)
// =============================================================================

func TestPutDeviceAuthModeHandler_OK(t *testing.T) {
	// ISAPI responds 200 for both GET (RMW read) and PUT
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"VerifyWeekPlanCfg":{"WeekPlanCfg":[{"weekNo":1,"verifyMode":"card"}]}}`)
		} else {
			fmt.Fprint(w, `{"statusCode":200}`)
		}
	}))
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	body := bytes.NewBufferString(`{"verifyMode":"face"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/auth-mode", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestPutDeviceAuthModeHandler_EmptyMode(t *testing.T) {
	// Validation fires BEFORE device lookup: any cfg works.
	cfg := newNotFoundCfg(t)
	body := bytes.NewBufferString(`{"verifyMode":""}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/auth-mode", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty verifyMode, got %d", rr.Code)
	}
}

func TestPutDeviceAuthModeHandler_DeviceNotFound(t *testing.T) {
	cfg := newNotFoundCfg(t)
	body := bytes.NewBufferString(`{"verifyMode":"face"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/auth-mode", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceAuthModeHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// GetDeviceDisplayHandler (tasks 2.2.1 / 2.2.4)
// =============================================================================

const displayXML = `<?xml version="1.0" encoding="UTF-8"?>
<IdentityTerminal version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<showMode>normal</showMode>
<advertisingDisplayType>full</advertisingDisplayType>
<screenOffTimeout>30</screenOffTimeout>
<previewShowTime>5</previewShowTime>
<standbyTimeout>10</standbyTimeout>
</IdentityTerminal>`

func TestGetDeviceDisplayHandler_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, displayXML)
	}))
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/1/preferences/display", nil)
	rr := httptest.NewRecorder()
	GetDeviceDisplayHandler(cfg).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp displayResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ShowMode != "normal" {
		t.Errorf("showMode = %q, want %q", resp.ShowMode, "normal")
	}
	if resp.ScreenOffTimeout != 30 {
		t.Errorf("screenOffTimeout = %d, want 30", resp.ScreenOffTimeout)
	}
}

// =============================================================================
// PutDeviceDisplayHandler (tasks 2.2.2 / 2.2.4)
// =============================================================================

func TestPutDeviceDisplayHandler_InvalidShowMode(t *testing.T) {
	// Validation fires BEFORE device lookup: any cfg works.
	cfg := newNotFoundCfg(t)
	body := bytes.NewBufferString(`{"showMode":"invalid","screenOffTimeout":30,"previewShowTime":5,"standbyTimeout":10}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/display", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceDisplayHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid showMode, got %d", rr.Code)
	}
}

func TestPutDeviceDisplayHandler_ValidModes(t *testing.T) {
	modes := []string{"normal", "full", "split"}
	for _, mode := range modes {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				if r.Method == http.MethodGet {
					fmt.Fprint(w, displayXML)
				} else {
					fmt.Fprint(w, `{}`)
				}
			}))
			defer srv.Close()

			cfg := newPreferencesCfg(t, srv)
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"showMode":%q,"screenOffTimeout":30,"previewShowTime":5,"standbyTimeout":10}`, mode))
			req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/display", body)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			PutDeviceDisplayHandler(cfg).ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("mode=%q: expected 200, got %d; body: %s", mode, rr.Code, rr.Body.String())
			}
		})
	}
}

// =============================================================================
// PostDeviceStandbyPictureHandler — body too large / content-type (tasks 2.3.5 / 1.12.4)
// =============================================================================

func TestPostDeviceStandbyPictureHandler_BodyTooLarge(t *testing.T) {
	buf, ct := buildOverSizeMultipart(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/standby-pictures", buf)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	// MaxBytesReader fires before device lookup — any non-nil repo works
	PostDeviceStandbyPictureHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}).ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "20 MB") {
		t.Errorf("413 response should mention 20 MB limit; got: %s", rr.Body.String())
	}
}

func TestPostDeviceStandbyPictureHandler_InvalidContentType(t *testing.T) {
	body, ct := buildMultipartBody(t, "application/octet-stream", 100)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/standby-pictures", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	PostDeviceStandbyPictureHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-image content type, got %d", rr.Code)
	}
}

func TestPostDeviceBootPictureHandler_BodyTooLarge(t *testing.T) {
	buf, ct := buildOverSizeMultipart(t)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/boot-picture", buf)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	PostDeviceBootPictureHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}).ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rr.Code)
	}
}

func TestPostDeviceBootPictureHandler_InvalidContentType(t *testing.T) {
	body, ct := buildMultipartBody(t, "text/plain", 100)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/boot-picture", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	PostDeviceBootPictureHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for text/plain, got %d", rr.Code)
	}
}

// TestPostDeviceBootPictureHandler_PNGAccepted verifies image/png is accepted (tasks 2.4.3).
// Validation must pass (not 400/413); device lookup will fail with 404 (no device in repo).
func TestPostDeviceBootPictureHandler_PNGAccepted(t *testing.T) {
	body, ct := buildMultipartBody(t, "image/png", 100)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/boot-picture", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	PostDeviceBootPictureHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{getErr: pgx.ErrNoRows}}).ServeHTTP(rr, req)
	// 404 = device not found (content-type passed → device lookup failed)
	if rr.Code == http.StatusBadRequest || rr.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("image/png should pass content-type gate; got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestValidateImageContentType_MediaHandler verifies content-type validation in PostDeviceMediaHandler.
func TestValidateImageContentType_MediaHandler(t *testing.T) {
	body, ct := buildMultipartBody(t, "application/pdf", 50)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/media", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()
	PostDeviceMediaHandler(DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for application/pdf, got %d", rr.Code)
	}
}

// =============================================================================
// GetDeviceStatsHandler (tasks 2.6.1 / 2.6.2)
// =============================================================================

func TestGetDeviceStatsHandler_AggregatesCorrectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "UserInfo/Count"):
			fmt.Fprint(w, `{"UserInfoCount":{"userNumber":100,"bindFaceUserNumber":80,"bindCardUserNumber":20}}`)
		case strings.Contains(path, "UserInfo/capabilities"):
			fmt.Fprint(w, `{"UserInfo":{"maxRecordNum":3000}}`)
		case strings.Contains(path, "AcsEventTotalNum/capabilities"):
			fmt.Fprint(w, `{"AcsEventTotalNum":{"@max":100000}}`)
		case strings.Contains(path, "AcsEventTotalNum"):
			fmt.Fprint(w, `{"AcsEventTotalNum":{"totalNum":5000}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/1/stats", nil)
	rr := httptest.NewRecorder()
	GetDeviceStatsHandler(cfg).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var resp deviceStatsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Users.Total != 100 {
		t.Errorf("users.total = %d, want 100", resp.Users.Total)
	}
	if resp.Users.Max != 3000 {
		t.Errorf("users.max = %d, want 3000", resp.Users.Max)
	}
}

// TestGetDeviceStatsHandler_DeviceOffline_NeverReturnsZeros verifies US4-AC2.
func TestGetDeviceStatsHandler_DeviceOffline_NeverReturnsZeros(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.Listener.Addr().String()
	srv.Close() // close immediately → connection refused

	device := makeDevice(1, addr)
	repo := &fakeDeviceConfigRepo{getResult: device}
	key := bytes.Repeat([]byte("k"), 32)
	cipher, _ := secrets.NewCipher(key)
	cfg := DeviceConfigConfig{DeviceRepo: repo, ISAPICipher: cipher}

	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/1/stats", nil)
	rr := httptest.NewRecorder()
	GetDeviceStatsHandler(cfg).ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Error("device offline must not return 200 with fabricated zeros (US4-AC2)")
	}
	if rr.Code != http.StatusGatewayTimeout && rr.Code != http.StatusBadGateway {
		t.Errorf("expected 504 or 502 for offline device, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetDeviceStatsHandler_DeviceNotFound(t *testing.T) {
	cfg := newNotFoundCfg(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/devices/9999/stats", nil)
	rr := httptest.NewRecorder()
	GetDeviceStatsHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// PutDeviceFaceConfigHandler (tasks 2.7.1 / 2.7.3)
// =============================================================================

func TestPutDeviceFaceConfigHandler_NegativeDistance(t *testing.T) {
	// Validation fires BEFORE device lookup: any cfg works.
	cfg := newNotFoundCfg(t)
	body := bytes.NewBufferString(`{"maxDistance":-1.0}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/face-config", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceFaceConfigHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for maxDistance -1, got %d", rr.Code)
	}
}

func TestPutDeviceFaceConfigHandler_ZeroDistance(t *testing.T) {
	// Validation fires BEFORE device lookup: repo not reached; any cfg works.
	cfg := newNotFoundCfg(t)
	body := bytes.NewBufferString(`{"maxDistance":0}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/face-config", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceFaceConfigHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for maxDistance 0, got %d", rr.Code)
	}
}

func TestPutDeviceFaceConfigHandler_ValidDistance(t *testing.T) {
	srv := testHikServer(t, 200, `{}`)
	defer srv.Close()
	cfg := newPreferencesCfg(t, srv)

	body := bytes.NewBufferString(`{"maxDistance":1.5}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/face-config", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	PutDeviceFaceConfigHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// PostDeviceFaceCaptureHandler (tasks 2.7.2 / 2.7.3)
// =============================================================================

// TestPostDeviceFaceCaptureHandler_ResponseDoesNotExposeURL verifies dec-011:
// response must not contain internal URL or IP — only base64 image field.
func TestPostDeviceFaceCaptureHandler_ResponseDoesNotExposeURL(t *testing.T) {
	imageData := []byte("FAKEJPEGDATA")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "CaptureFaceData") {
			// faceDataUrl points back to same server host (SSRF check passes)
			host := r.Host
			faceURL := "http://" + host + "/fake-face.jpg"
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<CaptureFaceDataResponse><faceDataUrl>%s</faceDataUrl></CaptureFaceDataResponse>`, faceURL)
		} else {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(imageData) //nolint:errcheck
		}
	}))
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/face-capture", nil)
	rr := httptest.NewRecorder()
	PostDeviceFaceCaptureHandler(cfg).ServeHTTP(rr, req)

	body := rr.Body.String()
	if strings.Contains(body, "faceDataUrl") {
		t.Errorf("response must not expose internal faceDataUrl field; got: %s", body)
	}
	if strings.Contains(body, "http://") {
		t.Errorf("response must not expose raw URL; got: %s", body)
	}
}

// TestPostDeviceFaceCaptureHandler_SSRFBlocked verifies ErrSSRFHostMismatch → 502 (tasks 2.7.3).
func TestPostDeviceFaceCaptureHandler_SSRFBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "CaptureFaceData") {
			w.Header().Set("Content-Type", "application/xml")
			// URL pointing to a different host → SSRF blocked
			fmt.Fprint(w, `<CaptureFaceDataResponse><faceDataUrl>http://10.0.0.1/face.jpg</faceDataUrl></CaptureFaceDataResponse>`)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	cfg := newPreferencesCfg(t, srv)
	req := httptest.NewRequest(http.MethodPost, "/admin/api/devices/1/preferences/face-capture", nil)
	rr := httptest.NewRecorder()
	PostDeviceFaceCaptureHandler(cfg).ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Errorf("SSRF-blocked capture should not return 200; body: %s", rr.Body.String())
	}
	// Must be 5xx (bad gateway), not a client error
	if rr.Code < 500 {
		t.Errorf("SSRF block should yield 5xx status, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// Transversal: device not found → 404 for GET handlers (tasks 2.8.4)
// =============================================================================

func TestAnyPreferencesHandler_DeviceNotFound_Returns404(t *testing.T) {
	notFoundCfg := newNotFoundCfg(t)

	tests := []struct {
		name    string
		method  string
		handler http.Handler
	}{
		{"GetAuthMode", http.MethodGet, GetDeviceAuthModeHandler(notFoundCfg)},
		{"GetDisplay", http.MethodGet, GetDeviceDisplayHandler(notFoundCfg)},
		{"GetStandbyPics", http.MethodGet, GetDeviceStandbyPicturesHandler(notFoundCfg)},
		{"GetMedia", http.MethodGet, GetDeviceMediaHandler(notFoundCfg)},
		{"GetStats", http.MethodGet, GetDeviceStatsHandler(notFoundCfg)},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/admin/api/devices/9999/preferences/auth-mode", nil)
			rr := httptest.NewRecorder()
			tc.handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusNotFound {
				t.Errorf("%s: expected 404, got %d; body: %s", tc.name, rr.Code, rr.Body.String())
			}
		})
	}
}

// =============================================================================
// PutDeviceStandbyDisableHandler (tasks 2.3.5)
// =============================================================================

func TestPutDeviceStandbyDisableHandler_OK(t *testing.T) {
	srv := testHikServer(t, 200, `{}`)
	defer srv.Close()
	cfg := newPreferencesCfg(t, srv)

	req := httptest.NewRequest(http.MethodPut, "/admin/api/devices/1/preferences/standby-pictures/disable", nil)
	rr := httptest.NewRecorder()
	PutDeviceStandbyDisableHandler(cfg).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// DeleteDeviceStandbyPictureHandler — path validation (tasks 2.3.5)
// =============================================================================

func TestDeleteDeviceStandbyPictureHandler_PathTraversalRejected(t *testing.T) {
	cfg := DeviceConfigConfig{DeviceRepo: &fakeDeviceConfigRepo{}}
	req := httptest.NewRequest(http.MethodDelete,
		"/admin/api/devices/1/preferences/standby-pictures/../etc/passwd", nil)
	rr := httptest.NewRecorder()
	DeleteDeviceStandbyPictureHandler(cfg).ServeHTTP(rr, req)
	// path traversal → 400 or 404
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusNotFound {
		t.Errorf("path traversal should return 400 or 404, got %d", rr.Code)
	}
}

// Suppress unused import warning.
var _ = hikvision.ErrSSRFHostMismatch
