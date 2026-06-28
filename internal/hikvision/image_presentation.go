package hikvision

// image_presentation.go valida e transcodifica imagens de presentation (start-page).
//
// Regra de produto: a imagem de fundo (presentation) do device só pode ter as medidas
// EXATAS 600x1024 ou 600x704. Cada medida mapeia para um show_mode do terminal:
//   - 600x1024 → "full"  (imagem ocupa a tela cheia)
//   - 600x704  → "split" (imagem ocupa a área da tela dividida)
// Qualquer outra resolução é rejeitada. A imagem é enviada COMO ESTÁ — sem resize;
// apenas transcodificada para JPEG (o material no device é picFormat=jpeg).

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
)

// Presentation show_mode values derivados do tamanho da imagem (ver 1-split.php /
// 2-full.php no legado: cada um seta IdentityTerminal show_mode + a imagem).
const (
	ShowModeFull  = "full"
	ShowModeSplit = "split"
)

// presentationSize associa uma resolução EXATA aceita ao show_mode correspondente.
type presentationSize struct {
	w, h int
	mode string
}

// PresentationImageSizes são as medidas aceitas para a imagem de presentation e o
// show_mode que cada uma implica.
var PresentationImageSizes = []presentationSize{
	{600, 1024, ShowModeFull},
	{600, 704, ShowModeSplit},
}

// PresentationModeForImage decodifica apenas o cabeçalho da imagem (DecodeConfig,
// barato) e retorna o show_mode correspondente ao seu tamanho. Retorna erro se a
// resolução não for uma das medidas aceitas. O decoder PNG está registrado em client.go.
func PresentationModeForImage(data []byte) (mode string, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("arquivo não é uma imagem válida (JPEG/PNG): %w", err)
	}
	for _, s := range PresentationImageSizes {
		if cfg.Width == s.w && cfg.Height == s.h {
			return s.mode, nil
		}
	}
	return "", fmt.Errorf("resolução %dx%d não permitida; envie 600x1024 (tela cheia) ou 600x704 (split)", cfg.Width, cfg.Height)
}

// TranscodeToJPEG decodifica a imagem e a recodifica em JPEG SEM redimensionar
// (preserva a resolução original). O material no device exige JPEG; imagens PNG
// válidas são convertidas mantendo as dimensões. Diferente de ResizeImageJPEG
// (boot/QR), aqui nada é escalado/distorcido.
func TranscodeToJPEG(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode imagem: %w", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, src, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}
	return out.Bytes(), nil
}
