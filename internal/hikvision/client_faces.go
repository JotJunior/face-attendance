package hikvision

// client_faces.go implements ISAPI face library operations.
//
// ClearFaces — SOURCED from legacy FaceService.php:38 (const ENDPOINT_FACE_CLEAR) and
// FaceService.php:283 (method clear()):
//   PUT /ISAPI/AccessControl/ClearPictureCfg?format=json
//   Body JSON: {"ClearPictureCfg":{"ClearFlags":{"facePicture":true,"capOrVerifyPicture":true}}}
//   Success: HTTP 200
//
// NOT /ISAPI/Intelligent/FDLib/FDSearch/Delete — that endpoint deletes ONE face by FPID,
// not the full library. Confirmed by reading FaceService.php constants:
//   ENDPOINT_FACE_DELETE = /ISAPI/Intelligent/FDLib/FDSearch/Delete (L34, delete by FPID)
//   ENDPOINT_FACE_CLEAR  = /ISAPI/AccessControl/ClearPictureCfg    (L38, clear all)

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// ClearFaces removes all face data (face pictures + capture/verify pictures) from the device.
// SOURCED: FaceService.php:38 (ENDPOINT_FACE_CLEAR) + FaceService.php:283 (clear() method).
// Endpoint: PUT /ISAPI/AccessControl/ClearPictureCfg?format=json
// Body: {"ClearPictureCfg":{"ClearFlags":{"facePicture":true,"capOrVerifyPicture":true}}}
func (c *Client) ClearFaces(ctx context.Context) error {
	const body = `{"ClearPictureCfg":{"ClearFlags":{"facePicture":true,"capOrVerifyPicture":true}}}`
	_, status, err := c.doRequest(ctx, http.MethodPut,
		"/ISAPI/AccessControl/ClearPictureCfg?format=json",
		strings.NewReader(body), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: ClearFaces: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("ClearFaces", status, nil)
}
