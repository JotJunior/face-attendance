package hikvision

// client_bootpic.go implements ISAPI boot/initialization picture management.
//
// UploadBootPicture — SOURCED from legacy/hik2go/src/Hik2go/Preferences/InitializationScreen.php:9-38 (create method):
//   Multipart POST /ISAPI/System/powerUpPicture?format=json
//   Fields: "picture_info" (JSON: {type:"filePathType", faceLibType:"binay"})
//           "picture_name" (binary with filename "<deviceID>.jpg")
//
// DeleteBootPicture — SOURCED from InitializationScreen.php:40-49 (delete method):
//   DELETE /ISAPI/System/powerUpPicture?format=json

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
)

const (
	// endpointBootPicture is the ISAPI endpoint for power-up (boot) picture management.
	// SOURCED: InitializationScreen.php:13 (create) and :43 (delete).
	endpointBootPicture = "/ISAPI/System/powerUpPicture?format=json"
)

// UploadBootPicture uploads a JPEG image to be displayed on device boot.
// SOURCED: InitializationScreen.php:9-38 (create method).
// Part 1: "picture_info" JSON field with type and faceLibType.
// Part 2: "picture_name" binary field with filename "<deviceID>.jpg".
// deviceID is used as the filename stem per the PHP source.
func (c *Client) UploadBootPicture(ctx context.Context, deviceID int64, data []byte) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Part 1: JSON metadata — SOURCED: InitializationScreen.php:17-24.
	meta, err := json.Marshal(map[string]string{
		"type":        "filePathType",
		"faceLibType": "binay", // note: intentional firmware typo — SOURCED: InitializationScreen.php:22
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
