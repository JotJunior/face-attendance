package hikvision

import (
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUploadBootPicture_MultipartFields verifies that UploadBootPicture sends
// the correct multipart fields (tasks 1.8.4): "picture_info" JSON + "picture_name" binary.
func TestUploadBootPicture_MultipartFields(t *testing.T) {
	var (
		gotPictureInfo string
		gotFilename    string
		gotBinary      []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/System/powerUpPicture" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		ct := r.Header.Get("Content-Type")
		mediaType, params, err := mime.ParseMediaType(ct)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Fatalf("Content-Type is not multipart: %s", ct)
		}

		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("multipart read: %v", err)
			}
			data, _ := io.ReadAll(part)
			switch part.FormName() {
			case "picture_info":
				gotPictureInfo = string(data)
			case "picture_name":
				gotFilename = part.FileName()
				gotBinary = data
			}
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	imgData := []byte{0xFF, 0xD8, 0xFF, 0xE0} // minimal JPEG marker
	c := newTestClient(t, srv)
	if err := c.UploadBootPicture(context.Background(), 42, imgData); err != nil {
		t.Fatalf("UploadBootPicture: %v", err)
	}

	// Verify picture_info JSON — SOURCED: InitializationScreen.php:17-24
	for _, want := range []string{`"type":"filePathType"`, `"faceLibType":"binay"`} {
		if !strings.Contains(gotPictureInfo, want) {
			t.Errorf("picture_info missing %s; got: %s", want, gotPictureInfo)
		}
	}

	// Verify filename is "<deviceID>.jpg" — SOURCED: InitializationScreen.php:29
	if gotFilename != "42.jpg" {
		t.Errorf("expected filename 42.jpg, got %q", gotFilename)
	}

	// Verify binary content passed through unchanged
	if string(gotBinary) != string(imgData) {
		t.Errorf("binary data mismatch: got %v", gotBinary)
	}
}

// TestUploadBootPicture_ErrorStatus verifies that a 500 response returns a retriable error.
func TestUploadBootPicture_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	err := c.UploadBootPicture(context.Background(), 1, []byte{0xFF})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// TestDeleteBootPicture_Path verifies DELETE is sent to the correct path (tasks 1.8.4).
func TestDeleteBootPicture_Path(t *testing.T) {
	var gotMethod, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteBootPicture(context.Background()); err != nil {
		t.Fatalf("DeleteBootPicture: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(gotPath, "/ISAPI/System/powerUpPicture") {
		t.Errorf("unexpected path: %s", gotPath)
	}
}

// TestDeleteBootPicture_404Idempotent verifies 404 (no boot pic set) is treated as success (tasks 1.8.4).
func TestDeleteBootPicture_404Idempotent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"statusCode":404,"statusString":"Not Found"}`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteBootPicture(context.Background()); err != nil {
		t.Errorf("DeleteBootPicture with 404 should be idempotent success, got: %v", err)
	}
}
