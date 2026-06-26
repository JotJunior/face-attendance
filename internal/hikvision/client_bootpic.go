package hikvision

// client_bootpic.go implements ISAPI boot/initialization picture management.
//
// UploadBootPicture — POST multipart /ISAPI/System/powerUpPicture?format=json.
// Contrato REAL verificado no device DS-K1T673* (o port do hik2go estava errado):
//   - "picture_info" (JSON): {filePathType:"binary", applyType:"powerUpPicture"}
//     (o port antigo mandava {type:"filePathType", faceLibType:"binay"} → HTTP 400).
//   - "picture_name" (binary image/jpeg, filename "<deviceID>.jpg").
//   - capabilities exigem JPG, resolução EXATA 600x1024, fileSize <= 512 KB; por isso
//     a imagem é redimensionada para 600x1024 JPEG antes do upload (imagem fora da
//     medida → HTTP 400 badParameters).
//
// DeleteBootPicture — DELETE /ISAPI/System/powerUpPicture?format=json.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/textproto"

	"golang.org/x/image/draw"
)

const (
	// endpointBootPicture is the ISAPI endpoint for power-up (boot) picture management.
	endpointBootPicture = "/ISAPI/System/powerUpPicture?format=json"

	// Resolução EXATA exigida pelo firmware para a imagem de boot (capabilities
	// pictureResolution=["600*1024"]). Imagens fora disso são rejeitadas (HTTP 400).
	bootPicWidth  = 600
	bootPicHeight = 1024
)

// resizeImageJPEG decodifica a imagem enviada e a redimensiona para wxh JPEG.
// Usado para boot e standby (mesma tela 600x1024) — o firmware exige a medida exata
// da tela, e imagens fora disso são rejeitadas (HTTP 400). Aceita JPEG/PNG
// (image.Decode). Distorce o aspecto se necessário — a imagem ocupa a tela inteira.
// Constituição: nenhum dado fabricado; só transforma o pixel buffer.
func resizeImageJPEG(data []byte, w, h int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode imagem: %w", err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}
	return out.Bytes(), nil
}

// UploadBootPicture uploads a JPEG image to be displayed on device boot.
// SOURCED: InitializationScreen.php:9-38 (create method).
// Part 1: "picture_info" JSON field with type and faceLibType.
// Part 2: "picture_name" binary field with filename "<deviceID>.jpg".
// deviceID is used as the filename stem per the PHP source.
func (c *Client) UploadBootPicture(ctx context.Context, deviceID int64, data []byte) error {
	// Redimensiona para 600x1024 JPEG (medida exigida pelo firmware; fora disso → 400).
	data, err := resizeImageJPEG(data, bootPicWidth, bootPicHeight)
	if err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture: %w", err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Part 1: JSON metadata — contrato REAL do firmware (capabilities ImportCap:
	// filePathType ∈ {binary,URL}, applyType=powerUpPicture).
	meta, err := json.Marshal(map[string]string{
		"filePathType": "binary",
		"applyType":    "powerUpPicture",
	})
	if err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture marshal meta: %w", err)
	}
	jsonPart, err := w.CreateFormField("picture_info")
	if err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture create meta field: %w", err)
	}
	if _, err := jsonPart.Write(meta); err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture write meta: %w", err)
	}

	// Part 2: binary image — SOURCED: InitializationScreen.php:26-34 (files field "picture_name").
	// Filename is "<deviceID>.jpg" per InitializationScreen.php:29.
	filename := fmt.Sprintf("%d.jpg", deviceID)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="picture_name"; filename=%q`, filename))
	header.Set("Content-Type", "image/jpeg")
	filePart, err := w.CreatePart(header)
	if err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture create file part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture write file: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture multipart close: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPost, endpointBootPicture,
		&buf, w.FormDataContentType())
	if err != nil {
		return fmt.Errorf("hikvision: UploadBootPicture POST: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}
	return retriableOrNot("UploadBootPicture POST", status, nil)
}

// DeleteBootPicture removes the current boot (power-up) picture from the device.
// SOURCED: InitializationScreen.php:40-49 (delete method).
// Idempotent: 404 from firmware (picture already absent) is treated as success.
func (c *Client) DeleteBootPicture(ctx context.Context) error {
	_, status, err := c.doRequest(ctx, http.MethodDelete, endpointBootPicture, nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: DeleteBootPicture DELETE: %w", err)
	}
	if status == 200 || status == 204 || status == 404 {
		// 404 = no boot picture set — idempotent success.
		return nil
	}
	return retriableOrNot("DeleteBootPicture DELETE", status, nil)
}
