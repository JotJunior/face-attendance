package hikvision

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeTestJPEG gera um JPEG válido e decodificável de wxh para os testes de upload
// (o UploadBootPicture decodifica + redimensiona, então bytes falsos não servem).
func makeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 80, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatalf("makeTestJPEG: %v", err)
	}
	return buf.Bytes()
}

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

	imgData := makeTestJPEG(t, 300, 300) // imagem real, será redimensionada p/ 600x1024
	c := newTestClient(t, srv)
	if err := c.UploadBootPicture(context.Background(), 42, imgData); err != nil {
		t.Fatalf("UploadBootPicture: %v", err)
	}

	// Verify picture_info JSON — contrato REAL do firmware (filePathType=binary).
	for _, want := range []string{`"filePathType":"binary"`, `"applyType":"powerUpPicture"`} {
		if !strings.Contains(gotPictureInfo, want) {
			t.Errorf("picture_info missing %s; got: %s", want, gotPictureInfo)
		}
	}

	// Verify filename is "<deviceID>.jpg".
	if gotFilename != "42.jpg" {
		t.Errorf("expected filename 42.jpg, got %q", gotFilename)
	}

	// O upload redimensiona para 600x1024 JPEG (medida exigida pelo firmware).
	decoded, _, err := image.Decode(bytes.NewReader(gotBinary))
	if err != nil {
		t.Fatalf("binário enviado não é imagem válida: %v", err)
	}
	if b := decoded.Bounds(); b.Dx() != bootPicWidth || b.Dy() != bootPicHeight {
		t.Errorf("dimensões enviadas = %dx%d, esperado %dx%d", b.Dx(), b.Dy(), bootPicWidth, bootPicHeight)
	}
}

// TestUploadBootPicture_ErrorStatus verifies that a 500 response returns a retriable error.
func TestUploadBootPicture_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	err := c.UploadBootPicture(context.Background(), 1, makeTestJPEG(t, 100, 100))
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
