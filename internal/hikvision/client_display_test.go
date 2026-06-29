package hikvision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sampleIdentityTerminalXML returns a realistic IdentityTerminal XML response.
// The version attribute and all read-only fields are included so the RMW test
// can verify they survive round-trip.
const sampleIdentityTerminalXML = `<?xml version="1.0" encoding="UTF-8"?>
<IdentityTerminal version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
<camera>true</camera>
<fingerPrintModule>false</fingerPrintModule>
<faceAlgorithm>H264</faceAlgorithm>
<saveCertifiedImage>true</saveCertifiedImage>
<readInfoOfCard>false</readInfoOfCard>
<workMode>accessControl</workMode>
<ecoMode>
<eco>true</eco>
<faceMatchThreshold1>75</faceMatchThreshold1>
<faceMatchThresholdN>70</faceMatchThresholdN>
<changeThreshold>10</changeThreshold>
<maskFaceMatchThresholdN>60</maskFaceMatchThresholdN>
<maskFaceMatchThreshold1>65</maskFaceMatchThreshold1>
</ecoMode>
<enableScreenOff>true</enableScreenOff>
<screenOffTimeout>600</screenOffTimeout>
<showMode>normal</showMode>
<popUpPreviewWindow>true</popUpPreviewWindow>
<previewShowTime>10</previewShowTime>
<standbyTimeout>30</standbyTimeout>
<advertisingDisplayType>full</advertisingDisplayType>
</IdentityTerminal>`

// TestGetIdentityTerminal_ParsesFields checks that GetIdentityTerminal correctly
// parses the configurable fields from the XML response (tasks 1.3.3).
func TestGetIdentityTerminal_ParsesFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/AccessControl/IdentityTerminal" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(200)
		w.Write([]byte(sampleIdentityTerminalXML)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	disp, err := c.GetIdentityTerminal(context.Background())
	if err != nil {
		t.Fatalf("GetIdentityTerminal: %v", err)
	}
	if disp.ShowMode != "normal" {
		t.Errorf("ShowMode = %q, want %q", disp.ShowMode, "normal")
	}
	if disp.ScreenOffTimeout != 600 {
		t.Errorf("ScreenOffTimeout = %d, want 600", disp.ScreenOffTimeout)
	}
	if disp.PreviewShowTime != 10 {
		t.Errorf("PreviewShowTime = %d, want 10", disp.PreviewShowTime)
	}
	if disp.StandbyTimeout != 30 {
		t.Errorf("StandbyTimeout = %d, want 30", disp.StandbyTimeout)
	}
	if disp.raw == nil {
		t.Fatal("raw field must not be nil (needed for RMW)")
	}
	if disp.raw.Camera != "true" {
		t.Errorf("read-only Camera = %q, want %q", disp.raw.Camera, "true")
	}
	if disp.raw.Version != "2.0" {
		t.Errorf("version attr = %q, want %q", disp.raw.Version, "2.0")
	}
}

// TestGetIdentityTerminal_401_NonRetriable verifies 401 → NonRetriableError (tasks 1.3.3).
func TestGetIdentityTerminal_401_NonRetriable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetIdentityTerminal(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsNonRetriable(err) {
		t.Errorf("expected NonRetriableError, got %T: %v", err, err)
	}
}

// TestShowModeMapping checks the logical→ISAPI mapping (tasks 1.4.3).
func TestShowModeMapping(t *testing.T) {
	cases := []struct {
		logical   string
		wantShow  string
		wantType  string
	}{
		{"normal", "normal", "full"},
		{"full", "advertising", "full"},
		{"split", "advertising", "split"},
	}
	for _, tc := range cases {
		sm, dt := logicalToRawShowMode(tc.logical)
		if sm != tc.wantShow || dt != tc.wantType {
			t.Errorf("logicalToRawShowMode(%q) = (%q, %q), want (%q, %q)",
				tc.logical, sm, dt, tc.wantShow, tc.wantType)
		}
	}
}

// TestPutIdentityTerminal_PreservesReadOnlyFields verifies that PutIdentityTerminal
// preserves read-only fields from the GET response in the PUT body (tasks 1.4.2 / 1.4.3).
func TestPutIdentityTerminal_PreservesReadOnlyFields(t *testing.T) {
	var putBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			w.Write([]byte(sampleIdentityTerminalXML)) //nolint:errcheck
		case http.MethodPut:
			var buf [1 << 20]byte
			n, _ := r.Body.Read(buf[:])
			putBody = make([]byte, n)
			copy(putBody, buf[:n])
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.PutIdentityTerminal(context.Background(), 300, 5, 60, "full"); err != nil {
		t.Fatalf("PutIdentityTerminal: %v", err)
	}

	body := string(putBody)
	// Read-only fields must survive round-trip.
	for _, want := range []string{
		"<camera>true</camera>",
		"<fingerPrintModule>false</fingerPrintModule>",
		"<faceAlgorithm>H264</faceAlgorithm>",
		"version=\"2.0\"",
	} {
		if !containsStr(body, want) {
			t.Errorf("PUT body missing %q", want)
		}
	}
	// Configurable fields must be updated.
	for _, want := range []string{
		"<screenOffTimeout>300</screenOffTimeout>",
		"<previewShowTime>5</previewShowTime>",
		"<standbyTimeout>60</standbyTimeout>",
		"<showMode>advertising</showMode>",
		"<advertisingDisplayType>full</advertisingDisplayType>",
	} {
		if !containsStr(body, want) {
			t.Errorf("PUT body missing updated field %q", want)
		}
	}
}

// TestPutIdentityTerminal_OmitsAbsentPopUpPreviewWindow verifica que, quando o device
// NÃO retorna popUpPreviewWindow no GET (caso real do DS-K1T673*), o PUT também NÃO o
// emite — emitir <popUpPreviewWindow></popUpPreviewWindow> faz o firmware dar HTTP 400.
func TestPutIdentityTerminal_OmitsAbsentPopUpPreviewWindow(t *testing.T) {
	// GET sem popUpPreviewWindow (estrutura real verificada no device .116).
	const getXML = `<?xml version="1.0" encoding="UTF-8"?>` +
		`<IdentityTerminal version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">` +
		`<camera>C270</camera><fingerPrintModule>ALIWARD</fingerPrintModule>` +
		`<faceAlgorithm>DeepLearn</faceAlgorithm><saveCertifiedImage>enable</saveCertifiedImage>` +
		`<readInfoOfCard>serialNo</readInfoOfCard><workMode>accessControlMode</workMode>` +
		`<enableScreenOff>true</enableScreenOff><screenOffTimeout>60</screenOffTimeout>` +
		`<showMode>advertising</showMode><previewShowTime>5</previewShowTime>` +
		`<standbyTimeout>30</standbyTimeout><advertisingDisplayType>split</advertisingDisplayType>` +
		`</IdentityTerminal>`

	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			w.Write([]byte(getXML)) //nolint:errcheck
		case http.MethodPut:
			var buf [1 << 20]byte
			n, _ := r.Body.Read(buf[:])
			putBody = string(buf[:n])
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.PutIdentityTerminal(context.Background(), 60, 5, 30, "full"); err != nil {
		t.Fatalf("PutIdentityTerminal: %v", err)
	}
	if containsStr(putBody, "popUpPreviewWindow") {
		t.Errorf("PUT NÃO deveria conter popUpPreviewWindow quando o GET o omite; body:\n%s", putBody)
	}
}

// TestPutIdentityTerminal_Idempotent verifies that two identical calls produce
// identical PUT bodies (Constitution II / tasks 1.4.2).
func TestPutIdentityTerminal_Idempotent(t *testing.T) {
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			w.Write([]byte(sampleIdentityTerminalXML)) //nolint:errcheck
		case http.MethodPut:
			var buf [1 << 20]byte
			n, _ := r.Body.Read(buf[:])
			bodies = append(bodies, string(buf[:n]))
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		if err := c.PutIdentityTerminal(context.Background(), 600, 10, 30, "normal"); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 PUT bodies, got %d", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Error("PutIdentityTerminal is NOT idempotent: bodies differ")
	}
}

// TestGetShowModeThumbnails_Returns200 verifies pass-through of the JSON body (tasks 1.5.2).
func TestGetShowModeThumbnails_Returns200(t *testing.T) {
	const payload = `{"ShowModeThumbnailsList":[]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	data, err := c.GetShowModeThumbnails(context.Background())
	if err != nil {
		t.Fatalf("GetShowModeThumbnails: %v", err)
	}
	if string(data) != payload {
		t.Errorf("got %q, want %q", string(data), payload)
	}
}

// TestGetShowModeThumbnails_404_Error verifies 404 returns a traceable error (tasks 1.5.2).
func TestGetShowModeThumbnails_404_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.GetShowModeThumbnails(context.Background())
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
