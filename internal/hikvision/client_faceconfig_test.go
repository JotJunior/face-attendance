package hikvision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSetFaceCompareCond_XMLFields verifies that SetFaceCompareCond sends XML
// with maxDistance and all fixed fields (tasks 1.11.3 / SOURCED: Face.php).
func TestSetFaceCompareCond_XMLFields(t *testing.T) {
	var receivedXML string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/AccessControl/FaceCompareCond") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		buf := make([]byte, 2048)
		n, _ := r.Body.Read(buf)
		receivedXML = string(buf[:n])
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.SetFaceCompareCond(context.Background(), 1.5); err != nil {
		t.Fatalf("SetFaceCompareCond: %v", err)
	}

	// Verify maxDistance value
	if !strings.Contains(receivedXML, "<maxDistance>1.5</maxDistance>") {
		t.Errorf("XML missing maxDistance=1.5; got:\n%s", receivedXML)
	}

	// Verify all fixed fields — SOURCED: Face.php:setMaxDistance()
	requiredFields := []string{
		"<pitch>45</pitch>",
		"<yaw>45</yaw>",
		"<leftBorder>0</leftBorder>",
		"<rightBorder>0</rightBorder>",
		"<upBorder>0</upBorder>",
		"<bottomBorder>0</bottomBorder>",
		"<faceScore>0</faceScore>",
		"<faceScoreThreshold1>0</faceScoreThreshold1>",
		"<ROIRegionMode>manual</ROIRegionMode>",
	}
	for _, field := range requiredFields {
		if !strings.Contains(receivedXML, field) {
			t.Errorf("XML missing fixed field %s; got:\n%s", field, receivedXML)
		}
	}
}

// TestSetFaceCompareCond_Idempotent verifies that two calls with the same maxDistance
// produce identical XML bodies (Constitution II / tasks 1.11.3).
func TestSetFaceCompareCond_Idempotent(t *testing.T) {
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 2048)
		n, _ := r.Body.Read(buf)
		bodies = append(bodies, string(buf[:n]))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		if err := c.SetFaceCompareCond(context.Background(), 1.5); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("SetFaceCompareCond NOT idempotent:\nfirst:  %q\nsecond: %q", bodies[0], bodies[1])
	}
}

// TestCaptureFaceData_SSRFBlocked verifies that CaptureFaceData returns ErrSSRFHostMismatch
// when faceDataUrl host differs from the device host (tasks 1.11.5, CHK023).
func TestCaptureFaceData_SSRFBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// faceDataUrl host (10.0.0.1) differs from device host (127.0.0.1)
		w.Write([]byte(`{"faceDataUrl":"http://10.0.0.1/face.jpg"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.CaptureFaceData(context.Background())
	if err == nil {
		t.Fatal("expected SSRF error, got nil")
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Errorf("expected SSRF error, got: %v", err)
	}
}

// TestValidateFaceDataURL_HostMatch verifies that matching hosts return nil (tasks 1.13.3).
func TestValidateFaceDataURL_HostMatch(t *testing.T) {
	if err := validateFaceDataURL("http://192.168.68.107/img.jpg", "192.168.68.107"); err != nil {
		t.Errorf("expected nil for matching host, got: %v", err)
	}
}

// TestValidateFaceDataURL_HostMismatch verifies that different hosts return ErrSSRFHostMismatch (tasks 1.13.3).
func TestValidateFaceDataURL_HostMismatch(t *testing.T) {
	cases := []struct {
		name        string
		faceDataURL string
		deviceHost  string
	}{
		{
			name:        "different IP",
			faceDataURL: "http://10.0.0.1/img.jpg",
			deviceHost:  "192.168.68.107",
		},
		{
			name:        "internal service hostname",
			faceDataURL: "http://internal-service/secret",
			deviceHost:  "192.168.68.107",
		},
		{
			name:        "different subnet",
			faceDataURL: "http://172.16.0.1/face.jpg",
			deviceHost:  "192.168.1.100",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFaceDataURL(tc.faceDataURL, tc.deviceHost)
			if err == nil {
				t.Errorf("expected ErrSSRFHostMismatch for %q vs %q, got nil", tc.faceDataURL, tc.deviceHost)
			}
			if err != ErrSSRFHostMismatch {
				t.Errorf("expected ErrSSRFHostMismatch, got %v", err)
			}
		})
	}
}

// TestValidateFaceDataURL_PortStripped verifies that port numbers in device host are stripped
// before comparison (tasks 1.13.3 — device host format is "ip:port").
func TestValidateFaceDataURL_PortStripped(t *testing.T) {
	// Device host includes port (as per baseURL pattern)
	if err := validateFaceDataURL("http://192.168.68.107/img.jpg", "192.168.68.107:80"); err != nil {
		t.Errorf("expected nil when device host has port, got: %v", err)
	}
}
