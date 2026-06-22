package hikvision_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestClearFaces_200_ReturnsNil verifies ClearFaces returns nil on HTTP 200.
// SOURCED: FaceService.php:38 (ENDPOINT_FACE_CLEAR) + FaceService.php:283 (clear()).
func TestClearFaces_200_ReturnsNil(t *testing.T) {
	var capturedPath, capturedMethod, capturedBody string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		b := make([]byte, 512)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ClearFaces(context.Background())
	if err != nil {
		t.Fatalf("ClearFaces: expected nil, got %v", err)
	}
	// Verify endpoint path (SOURCED: ENDPOINT_FACE_CLEAR = /ISAPI/AccessControl/ClearPictureCfg)
	if !strings.Contains(capturedPath, "/ISAPI/AccessControl/ClearPictureCfg") {
		t.Errorf("unexpected path: %q — expected /ISAPI/AccessControl/ClearPictureCfg", capturedPath)
	}
	if capturedMethod != http.MethodPut {
		t.Errorf("expected PUT, got %q", capturedMethod)
	}
	// Verify body contains the ClearFlags shape (SOURCED: FaceService.php:283-295)
	if !strings.Contains(capturedBody, "ClearPictureCfg") {
		t.Errorf("body missing ClearPictureCfg: %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "facePicture") {
		t.Errorf("body missing facePicture flag: %q", capturedBody)
	}
	if !strings.Contains(capturedBody, "capOrVerifyPicture") {
		t.Errorf("body missing capOrVerifyPicture flag: %q", capturedBody)
	}
}

// TestClearFaces_401_ReturnsNonRetriableError verifies ClearFaces returns NonRetriableError on 401.
func TestClearFaces_401_ReturnsNonRetriableError(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ClearFaces(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !hikvision.IsNonRetriable(err) {
		t.Errorf("expected NonRetriableError for 401, got %T: %v", err, err)
	}
}
