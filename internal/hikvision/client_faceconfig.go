package hikvision

// client_faceconfig.go implements ISAPI face comparison configuration and face capture.
//
// SetFaceCompareCond — SOURCED from legacy/hik2go/src/Hik2go/Face.php:setMaxDistance():
//   PUT /ISAPI/AccessControl/FaceCompareCond with XML <FaceCompareCond version="2.0">
//   Fields: pitch=45, yaw=45, leftBorder=0, rightBorder=0, upBorder=0, bottomBorder=0,
//           faceScore=0, maxDistance=<param>, faceScoreThreshold1=0, ROIRegionMode=manual
//
// CaptureFaceData — SOURCED from Face.php:capture():
//   POST /ISAPI/AccessControl/CaptureFaceData with XML <CaptureFaceDataCond>
//   Extracts faceDataUrl, validates host == device host (SSRF mitigation — CHK023),
//   downloads image via downloadImage with io.LimitReader (10 MB cap — F1 mitigation).
//
// validateFaceDataURL — SSRF helper (tasks 1.13.1/1.13.2/1.13.3).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	// endpointFaceCompareCond sets face comparison conditions.
	// SOURCED: Face.php:setMaxDistance() — PUT /ISAPI/AccessControl/FaceCompareCond
	endpointFaceCompareCond = "/ISAPI/AccessControl/FaceCompareCond"

	// endpointCaptureFaceData triggers face capture and returns a URL.
	// SOURCED: Face.php:capture() — POST /ISAPI/AccessControl/CaptureFaceData
	endpointCaptureFaceData = "/ISAPI/AccessControl/CaptureFaceData"

	// maxFaceDataBytes is the download cap for CaptureFaceData (F1 mitigation, plan.md §6.1).
	maxFaceDataBytes = 10 * 1024 * 1024 // 10 MB
)

// ErrSSRFHostMismatch is returned when CaptureFaceData receives a faceDataUrl whose
// host does not match the device host (SSRF mitigation — CHK023, plan.md §6.1 F2).
var ErrSSRFHostMismatch = errors.New("hikvision: CaptureFaceData: faceDataUrl host does not match device host")

// SetFaceCompareCond sets the face comparison conditions on the device.
// SOURCED: Face.php:setMaxDistance() — PUT /ISAPI/AccessControl/FaceCompareCond
//
// Fixed fields (all values verified from Face.php XML template):
//
//	pitch=45, yaw=45, leftBorder=0, rightBorder=0, upBorder=0, bottomBorder=0,
//	faceScore=0, faceScoreThreshold1=0, ROIRegionMode=manual
//
// Idempotent: same maxDistance always produces identical XML (Constitution II / tasks 1.11.3).
func (c *Client) SetFaceCompareCond(ctx context.Context, maxDistance float64) error {
	// XML template — SOURCED: Face.php:setMaxDistance() lines 103-116
	// All fixed fields match the PHP XML template exactly.
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<FaceCompareCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
		`<pitch>45</pitch>`+
		`<yaw>45</yaw>`+
		`<leftBorder>0</leftBorder>`+
		`<rightBorder>0</rightBorder>`+
		`<upBorder>0</upBorder>`+
		`<bottomBorder>0</bottomBorder>`+
		`<faceScore>0</faceScore>`+
		`<maxDistance>%g</maxDistance>`+
		`<faceScoreThreshold1>0</faceScoreThreshold1>`+
		`<ROIRegionMode>manual</ROIRegionMode>`+
		`</FaceCompareCond>`,
		maxDistance)

	_, status, err := c.doRequest(ctx, http.MethodPut, endpointFaceCompareCond,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: SetFaceCompareCond PUT: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("SetFaceCompareCond PUT", status, nil)
}

// captureFaceDataResponse is the JSON response from POST /CaptureFaceData.
// SOURCED: Face.php:capture() — accesses data.faceDataUrl
type captureFaceDataResponse struct {
	FaceDataURL string `json:"faceDataUrl"`
}

// CaptureFaceData triggers a face capture on the device and returns the image bytes.
// SOURCED: Face.php:capture() — POST /ISAPI/AccessControl/CaptureFaceData then httpDownload.
//
// Security controls:
//   - SSRF (CHK023, plan.md §6.1 F2): validates that faceDataUrl host == device host before download.
//   - DoS (F1): download capped at 10 MB via io.LimitReader.
func (c *Client) CaptureFaceData(ctx context.Context) ([]byte, error) {
	// XML body — SOURCED: Face.php:capture() literal XML string
	xmlBody := `<CaptureFaceDataCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">` +
		`<captureInfrared>false</captureInfrared>` +
		`<dataType>url</dataType>` +
		`</CaptureFaceDataCond>`

	body, status, err := c.doRequest(ctx, http.MethodPost, endpointCaptureFaceData,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return nil, fmt.Errorf("hikvision: CaptureFaceData POST: %w", err)
	}
	if status != 200 {
		return nil, retriableOrNot("CaptureFaceData POST", status, body)
	}

	var resp captureFaceDataResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hikvision: CaptureFaceData parse response: %w", err)
	}
	if resp.FaceDataURL == "" {
		return nil, fmt.Errorf("hikvision: CaptureFaceData: device returned empty faceDataUrl")
	}

	// SSRF check — CHK023, plan.md §6.1 F2
	deviceHost := c.device.Host
	if colonIdx := strings.LastIndex(deviceHost, ":"); colonIdx > 0 {
		deviceHost = deviceHost[:colonIdx] // strip port from host:port
	}
	if err := validateFaceDataURL(resp.FaceDataURL, deviceHost); err != nil {
		// Log the attempt (no URL logged to avoid leaking internal IPs in audit logs)
		return nil, fmt.Errorf("hikvision: CaptureFaceData SSRF check (device=%s): %w",
			deviceHost, ErrSSRFHostMismatch)
	}

	// Download with 10 MB cap — plan.md §6.1 F1 (DoS mitigation)
	imgData, _, err := downloadImageLimited(ctx, resp.FaceDataURL, maxFaceDataBytes)
	if err != nil {
		return nil, fmt.Errorf("hikvision: CaptureFaceData download: %w", err)
	}
	return imgData, nil
}

// validateFaceDataURL checks that faceDataURL's host (without port) matches deviceHost.
// SSRF mitigation — CHK023, plan.md §6.1 F2 (tasks 1.13.1).
// Returns ErrSSRFHostMismatch if hosts differ.
func validateFaceDataURL(faceDataURL, deviceHost string) error {
	parsed, err := url.Parse(faceDataURL)
	if err != nil {
		return ErrSSRFHostMismatch
	}
	// Extract host without port
	host := parsed.Hostname() // net/url strips port
	if host == "" {
		return ErrSSRFHostMismatch
	}

	// Normalize both to bare IP/hostname for comparison (strip port if present in deviceHost)
	normalDevice := deviceHost
	if h, _, err := net.SplitHostPort(deviceHost); err == nil {
		normalDevice = h
	}

	if !strings.EqualFold(host, normalDevice) {
		return ErrSSRFHostMismatch
	}
	return nil
}

// downloadImageLimited fetches an image from url, capping the download at maxBytes.
// Uses io.LimitReader to prevent reading more than maxBytes from the response body.
// Returns (bytes, mimeType, error).
func downloadImageLimited(ctx context.Context, imageURL string, maxBytes int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	hc := &http.Client{Timeout: 30 * 1e9} // 30s
	resp, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d fetching face data image", resp.StatusCode)
	}

	// F1 mitigation: cap download at maxBytes
	limited := io.LimitReader(resp.Body, maxBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	return data, ct, nil
}
