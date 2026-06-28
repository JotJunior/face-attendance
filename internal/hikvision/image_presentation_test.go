package hikvision

import (
	"bytes"
	"image"
	"testing"
)

// TestPresentationModeForImage verifica a regra de medidas e o show_mode derivado:
// 600x1024 → full, 600x704 → split; qualquer outra resolução retorna erro.
func TestPresentationModeForImage(t *testing.T) {
	cases := []struct {
		w, h     int
		wantMode string
		wantErr  bool
	}{
		{600, 1024, ShowModeFull, false},
		{600, 704, ShowModeSplit, false},
		{600, 1200, "", true},
		{600, 754, "", true},
		{800, 1024, "", true},
		{300, 300, "", true},
	}
	for _, tc := range cases {
		mode, err := PresentationModeForImage(makeTestJPEG(t, tc.w, tc.h))
		if tc.wantErr {
			if err == nil {
				t.Errorf("%dx%d: esperava erro, obteve nil (mode=%q)", tc.w, tc.h, mode)
			}
			continue
		}
		if err != nil {
			t.Errorf("%dx%d: esperava ok, obteve erro: %v", tc.w, tc.h, err)
		}
		if mode != tc.wantMode {
			t.Errorf("%dx%d: mode = %q, esperado %q", tc.w, tc.h, mode, tc.wantMode)
		}
	}
}

// TestTranscodeToJPEG verifica que a imagem é recodificada em JPEG preservando as
// dimensões (sem resize).
func TestTranscodeToJPEG(t *testing.T) {
	out, err := TranscodeToJPEG(makeTestJPEG(t, 600, 704))
	if err != nil {
		t.Fatalf("TranscodeToJPEG: %v", err)
	}
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("saída não decodifica: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("formato = %q, esperado jpeg", format)
	}
	if b := img.Bounds(); b.Dx() != 600 || b.Dy() != 704 {
		t.Errorf("dimensões = %dx%d, esperado 600x704 (sem resize)", b.Dx(), b.Dy())
	}
}
