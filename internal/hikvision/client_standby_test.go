package hikvision

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListStandbyPictures_TwoPictures verifies parsing of a non-empty list (tasks 1.6.3).
func TestListStandbyPictures_TwoPictures(t *testing.T) {
	// Forma REAL do firmware: array direto; elemento = customStandbyPicUUID + filePath.
	payload := `{"customStandbyPicList":[` +
		`{"customStandbyPicUUID":"uuid-1","filePath":"a.jpg"},` +
		`{"customStandbyPicUUID":"uuid-2","filePath":"b.jpg"}` +
		`]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	pics, err := c.ListStandbyPictures(context.Background())
	if err != nil {
		t.Fatalf("ListStandbyPictures: %v", err)
	}
	if len(pics) != 2 {
		t.Fatalf("expected 2 pictures, got %d", len(pics))
	}
	if pics[0].UUID != "uuid-1" || pics[0].FileName != "a.jpg" {
		t.Errorf("pic[0] = %+v", pics[0])
	}
	if pics[1].UUID != "uuid-2" || pics[1].FileName != "b.jpg" {
		t.Errorf("pic[1] = %+v", pics[1])
	}
}

// TestListStandbyPictures_Empty verifies an empty list returns non-nil empty slice (tasks 1.6.3).
func TestListStandbyPictures_Empty(t *testing.T) {
	// Forma REAL verificada no device: {"customStandbyPicList":[]} (array direto).
	payload := `{"customStandbyPicList":[]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	pics, err := c.ListStandbyPictures(context.Background())
	if err != nil {
		t.Fatalf("ListStandbyPictures: %v", err)
	}
	if pics == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(pics) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(pics))
	}
}

// TestUploadStandbyPicture_MultipartFields verifies that the upload request
// sends the correct multipart fields (tasks 1.7.6).
func TestUploadStandbyPicture_MultipartFields(t *testing.T) {
	var gotMetaJSON string
	var gotFileBytes []byte
	var gotFileName string
	var gotFileCT string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("next part: %v", err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "UploadCustomStandbyPic":
				gotMetaJSON = string(data)
			case "filePath":
				gotFileBytes = data
				gotFileName = part.FileName()
				gotFileCT = part.Header.Get("Content-Type")
			}
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.UploadStandbyPicture(context.Background(), "standby.jpg", makeTestJPEG(t, 300, 300)); err != nil {
		t.Fatalf("UploadStandbyPicture: %v", err)
	}

	// Verify JSON metadata field.
	var meta map[string]string
	if err := json.Unmarshal([]byte(gotMetaJSON), &meta); err != nil {
		t.Fatalf("parse meta JSON: %v", err)
	}
	if meta["filePathType"] != "multipart" {
		t.Errorf("filePathType = %q, want %q", meta["filePathType"], "multipart")
	}
	if meta["filePath"] != "standby.jpg" {
		t.Errorf("filePath = %q, want %q", meta["filePath"], "standby.jpg")
	}

	// O file part DEVE ser image/jpeg (octet-stream → HTTP 400 no firmware).
	if gotFileCT != "image/jpeg" {
		t.Errorf("file Content-Type = %q, want image/jpeg", gotFileCT)
	}
	if gotFileName != "standby.jpg" {
		t.Errorf("filename = %q, want %q", gotFileName, "standby.jpg")
	}
	// A imagem é redimensionada para 600x1024 JPEG antes do upload.
	decoded, _, err := image.Decode(bytes.NewReader(gotFileBytes))
	if err != nil {
		t.Fatalf("binário enviado não é imagem válida: %v", err)
	}
	if b := decoded.Bounds(); b.Dx() != 600 || b.Dy() != 1024 {
		t.Errorf("dimensões enviadas = %dx%d, esperado 600x1024", b.Dx(), b.Dy())
	}
}

// TestDeleteStandbyPicture_SendsUUID verifies that DeleteStandbyPicture sends
// the correct UUID in the request body (tasks 1.7.6).
func TestDeleteStandbyPicture_SendsUUID(t *testing.T) {
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteStandbyPicture(context.Background(), "uuid-abc-123"); err != nil {
		t.Fatalf("DeleteStandbyPicture: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	list, ok := payload["customStandbyPicUUIDList"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("customStandbyPicUUIDList unexpected: %v", payload)
	}
	entry, ok := list[0].(map[string]interface{})
	if !ok {
		t.Fatalf("list[0] unexpected type")
	}
	if entry["customStandbyPicUUID"] != "uuid-abc-123" {
		t.Errorf("uuid = %v, want %q", entry["customStandbyPicUUID"], "uuid-abc-123")
	}
}

// TestEnableDisableCustomStandby_Bodies verifies that enable and disable
// send the correct and distinct bodies (tasks 1.7.6).
func TestEnableDisableCustomStandby_Bodies(t *testing.T) {
	var enableBody, disableBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		// Distinguish calls by path (same endpoint, caller sets type).
		// We capture both in sequence since tests are sequential.
		if enableBody == nil {
			enableBody = data
		} else {
			disableBody = data
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.EnableCustomStandby(context.Background()); err != nil {
		t.Fatalf("EnableCustomStandby: %v", err)
	}
	if err := c.DisableCustomStandby(context.Background()); err != nil {
		t.Fatalf("DisableCustomStandby: %v", err)
	}

	var en, dis map[string]interface{}
	if err := json.Unmarshal(enableBody, &en); err != nil {
		t.Fatalf("parse enable body: %v", err)
	}
	if err := json.Unmarshal(disableBody, &dis); err != nil {
		t.Fatalf("parse disable body: %v", err)
	}
	if en["standbyPicType"] != "custom" {
		t.Errorf("enable standbyPicType = %v, want %q", en["standbyPicType"], "custom")
	}
	if dis["standbyPicType"] != "default" {
		t.Errorf("disable standbyPicType = %v, want %q", dis["standbyPicType"], "default")
	}
}

// TestDisableCustomStandby_Idempotent verifies that two calls to DisableCustomStandby
// produce identical bodies (Constitution II / tasks 1.7.5).
func TestDisableCustomStandby_Idempotent(t *testing.T) {
	var bodies []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	for i := 0; i < 2; i++ {
		if err := c.DisableCustomStandby(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	if len(bodies) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(bodies))
	}
	if bodies[0] != bodies[1] {
		t.Errorf("DisableCustomStandby NOT idempotent: %q vs %q", bodies[0], bodies[1])
	}
}

// Ensure multipart package is used (prevents "imported and not used" error in test file).
var _ = multipart.NewWriter
