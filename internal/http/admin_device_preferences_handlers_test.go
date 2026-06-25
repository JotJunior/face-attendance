package httphandler

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

// TestPostDeviceStandbyPictureHandler_BodyTooLarge verifies that a POST with body > 20 MB
// returns 413 Request Entity Too Large (tasks 1.12.4).
func TestPostDeviceStandbyPictureHandler_BodyTooLarge(t *testing.T) {
	// Build a body just over the 20 MB limit
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
	// Write oversizeBytes of zeros
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

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()

	PostDeviceStandbyPictureHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d; body: %s", rr.Code, rr.Body.String())
	}
	// Response must include an actionable message
	if !strings.Contains(rr.Body.String(), "20 MB") {
		t.Errorf("413 response should mention 20 MB limit; got: %s", rr.Body.String())
	}
}

// TestPostDeviceBootPictureHandler_BodyTooLarge verifies 413 for oversized boot picture (tasks 1.12.4).
func TestPostDeviceBootPictureHandler_BodyTooLarge(t *testing.T) {
	oversizeBytes := maxUploadBodyBytes + 1024

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="big.jpg"`}
	h["Content-Type"] = []string{"image/jpeg"}
	part, _ := mw.CreatePart(h)
	part.Write(make([]byte, oversizeBytes)) //nolint:errcheck
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()

	PostDeviceBootPictureHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rr.Code)
	}
}

// TestValidateImageContentType_InvalidType verifies that non-image/* types return 400 (tasks 1.14.3).
func TestValidateImageContentType_InvalidType(t *testing.T) {
	body, ct := buildMultipartBody(t, "application/octet-stream", 100)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	PostDeviceStandbyPictureHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for application/octet-stream, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestValidateImageContentType_PNG verifies that image/png is accepted (tasks 1.14.3).
func TestValidateImageContentType_PNG(t *testing.T) {
	body, ct := buildMultipartBody(t, "image/png", 100)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	PostDeviceStandbyPictureHandler().ServeHTTP(rr, req)

	// 501 = stub handler; not 400 or 413, so content type was accepted
	if rr.Code == http.StatusBadRequest || rr.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("image/png should be accepted, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestValidateImageContentType_JPEG verifies that image/jpeg is accepted (tasks 1.14.3).
func TestValidateImageContentType_JPEG(t *testing.T) {
	body, ct := buildMultipartBody(t, "image/jpeg", 100)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	PostDeviceBootPictureHandler().ServeHTTP(rr, req)

	if rr.Code == http.StatusBadRequest || rr.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("image/jpeg should be accepted, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestValidateImageContentType_MediaHandler verifies content-type validation in PostDeviceMediaHandler.
func TestValidateImageContentType_MediaHandler(t *testing.T) {
	body, ct := buildMultipartBody(t, "application/pdf", 50)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ct)
	rr := httptest.NewRecorder()

	PostDeviceMediaHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for application/pdf, got %d", rr.Code)
	}
}

// TestValidateImageContentType_Unit verifies the helper directly (tasks 1.14.1).
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

// Suppress "imported and not used" for fmt in case tests don't use it directly.
var _ = fmt.Sprintf
