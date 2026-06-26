package hikvision

// client_standby.go implements ISAPI standby picture management operations.
//
// ListStandbyPictures — SOURCED from legacy/hik2go/src/Hik2go/Preferences/StandbyPicture.php:91-100
//   (list method):
//   GET /ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json
//
// UploadStandbyPicture — SOURCED from StandbyPicture.php:9-39 (create method):
//   Multipart POST /ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json
//   Fields: "UploadCustomStandbyPic" (JSON) + "filePath" (binary)
//
// EnableCustomStandby / DisableCustomStandby — SOURCED from StandbyPicture.php:73-88 (enable)
//   and StandbyPicture.php:57-71 (disable):
//   PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json
//
// DeleteStandbyPicture — SOURCED from StandbyPicture.php:41-54 (delete):
//   POST /ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

const (
	// endpointStandbyPicList fetches the custom standby picture list.
	// SOURCED: StandbyPicture.php:94 (list method URL).
	endpointStandbyPicList = "/ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json"

	// endpointStandbyPicUpload uploads a new custom standby picture.
	// SOURCED: StandbyPicture.php:14 (create method URL).
	endpointStandbyPicUpload = "/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json"

	// endpointStandbyPicDisplayParams sets the standby display mode.
	// SOURCED: StandbyPicture.php:62 (disable) and StandbyPicture.php:77 (enable).
	endpointStandbyPicDisplayParams = "/ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json"

	// endpointStandbyPicDelete deletes a custom standby picture by UUID.
	// SOURCED: StandbyPicture.php:44 (delete method URL).
	endpointStandbyPicDelete = "/ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json"
)

// StandbyPicture represents one entry in the custom standby picture list.
// SOURCED: StandbyPicture.php — UUID and FileName are the two identifiers.
type StandbyPicture struct {
	UUID     string `json:"uuid"`
	FileName string `json:"fileName"`
}

// standbyPicListResponse is the JSON envelope for GetCustomStandbyPicList.
// Forma REAL do firmware (verificada no device DS-K1T673*): "customStandbyPicList"
// é um ARRAY direto de pictures, NÃO um objeto wrapper com "CustomStandbyPic"
// aninhado. A suposição anterior (wrapper) fazia o json.Unmarshal falhar → 502.
// NOTA: o shape do ELEMENTO (StandbyPicture) não pôde ser verificado — a lista
// estava vazia no device. Confirmar as chaves quando houver uma standby pic custom
// cadastrada (provável customStandbyPicUUID, alinhado ao corpo do delete).
type standbyPicListResponse struct {
	CustomStandbyPicList []StandbyPicture `json:"customStandbyPicList"`
}

// ListStandbyPictures returns the list of custom standby pictures on the device.
// SOURCED: StandbyPicture.php:91-100 (list method).
// Returns an empty (non-nil) slice when the list is empty.
func (c *Client) ListStandbyPictures(ctx context.Context) ([]StandbyPicture, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointStandbyPicList, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: ListStandbyPictures: %w", err)
	}
	if status == 401 {
		return nil, &NonRetriableError{Op: "ListStandbyPictures", Status: status}
	}
	if status != 200 {
		return nil, retriableOrNot("ListStandbyPictures", status, body)
	}

	var resp standbyPicListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hikvision: ListStandbyPictures parse: %w", err)
	}

	pics := resp.CustomStandbyPicList
	if pics == nil {
		pics = []StandbyPicture{} // guarantee non-nil slice
	}
	return pics, nil
}

// UploadStandbyPicture uploads a picture as a new custom standby image.
// SOURCED: StandbyPicture.php:9-39 (create method — multipart with two fields).
// Field 1: "UploadCustomStandbyPic" (JSON: {filePathType, filePath})
// Field 2: "filePath" (binary image data with filename)
func (c *Client) UploadStandbyPicture(ctx context.Context, filename string, data []byte) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Part 1: JSON metadata — SOURCED: StandbyPicture.php:18-24.
	meta, err := json.Marshal(map[string]string{
		"filePathType": "multipart",
		"filePath":     filename,
	})
	if err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture marshal meta: %w", err)
	}
	jsonPart, err := w.CreateFormField("UploadCustomStandbyPic")
	if err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture create meta field: %w", err)
	}
	if _, err := jsonPart.Write(meta); err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture write meta: %w", err)
	}

	// Part 2: binary image — SOURCED: StandbyPicture.php:25-32 (files field "filePath").
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="filePath"; filename=%q`, filename))
	header.Set("Content-Type", "application/octet-stream")
	filePart, err := w.CreatePart(header)
	if err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture create file part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture write file: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture multipart close: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPost, endpointStandbyPicUpload,
		&buf, w.FormDataContentType())
	if err != nil {
		return fmt.Errorf("hikvision: UploadStandbyPicture POST: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}
	return retriableOrNot("UploadStandbyPicture POST", status, nil)
}

// EnableCustomStandby switches the device to display custom standby pictures.
// SOURCED: StandbyPicture.php:73-88 (enable method).
// Idempotent: always sends the same body regardless of current state (Constitution II).
func (c *Client) EnableCustomStandby(ctx context.Context) error {
	return c.putStandbyDisplayParams(ctx, "custom")
}

// DisableCustomStandby reverts the device to its default standby display.
// SOURCED: StandbyPicture.php:57-71 (disable method).
// Idempotent: always sends the same body regardless of current state (Constitution II / tasks 1.7.5).
func (c *Client) DisableCustomStandby(ctx context.Context) error {
	return c.putStandbyDisplayParams(ctx, "default")
}

// putStandbyDisplayParams sends the standby display mode PUT request.
// SOURCED: StandbyPicture.php:62 and StandbyPicture.php:77 — same endpoint, different standbyPicType.
func (c *Client) putStandbyDisplayParams(ctx context.Context, standbyPicType string) error {
	payload := map[string]interface{}{
		"standbyPicType": standbyPicType,
		"displayEffect":  "stretch",
		"switchingTime":  20,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("hikvision: putStandbyDisplayParams marshal: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPut, endpointStandbyPicDisplayParams,
		strings.NewReader(string(data)), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: putStandbyDisplayParams PUT (%s): %w", standbyPicType, err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("putStandbyDisplayParams PUT "+standbyPicType, status, nil)
}

// DeleteStandbyPicture removes a custom standby picture by its UUID.
// SOURCED: StandbyPicture.php:41-54 (delete method).
func (c *Client) DeleteStandbyPicture(ctx context.Context, uuid string) error {
	payload := map[string]interface{}{
		"customStandbyPicUUIDList": []map[string]string{
			{"customStandbyPicUUID": uuid},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("hikvision: DeleteStandbyPicture marshal: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPost, endpointStandbyPicDelete,
		strings.NewReader(string(data)), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: DeleteStandbyPicture POST: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("DeleteStandbyPicture POST", status, nil)
}
