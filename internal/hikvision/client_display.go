package hikvision

// client_display.go implements ISAPI IdentityTerminal (display/show mode) operations.
//
// GetIdentityTerminal — SOURCED from legacy/hik2go/src/Hik2go/Preferences/IdentityTerminal.php:21-29
//   (info method):
//   GET /ISAPI/AccessControl/IdentityTerminal  (XML; no ?format=json)
//
// PutIdentityTerminal (read-modify-write) — SOURCED from IdentityTerminal.php:47-104
//   (update method): reads current config, merges configurable fields, preserves all
//   read-only fields and the version attribute, then:
//   PUT /ISAPI/AccessControl/IdentityTerminal  Content-Type: application/xml
//
// GetShowModeThumbnails — SOURCED from IdentityTerminal.php:31-43 (thumbnailList):
//   GET /ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

const (
	// endpointIdentityTerminal is the ISAPI endpoint for the display configuration.
	// SOURCED: IdentityTerminal.php:24 (info) and IdentityTerminal.php:95 (update httpPut).
	endpointIdentityTerminal = "/ISAPI/AccessControl/IdentityTerminal"

	// endpointShowModeThumbnails is the ISAPI endpoint for show mode thumbnails.
	// SOURCED: IdentityTerminal.php:35 (thumbnailList).
	endpointShowModeThumbnails = "/ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json"
)

// identityTerminalXML is the XML structure for GET/PUT of IdentityTerminal.
// Fields are divided into configurable (exposed via the Go API) and read-only
// (preserved verbatim from GET → PUT, per IdentityTerminal.php:60-63 fill-in).
// SOURCED: IdentityTerminal.php:66-91 (XML template fields).
type identityTerminalXML struct {
	XMLName xml.Name `xml:"IdentityTerminal"`
	Version string   `xml:"version,attr"`
	XMLNS   string   `xml:"xmlns,attr"`

	// Read-only hardware/firmware fields — preserved in RMW (IdentityTerminal.php:60-63).
	Camera             string `xml:"camera"`
	FingerPrintModule  string `xml:"fingerPrintModule"`
	FaceAlgorithm      string `xml:"faceAlgorithm"`
	SaveCertifiedImage string `xml:"saveCertifiedImage"`
	ReadInfoOfCard     string `xml:"readInfoOfCard"`
	WorkMode           string `xml:"workMode"`

	// EcoMode is a nested read-only block.
	EcoMode *ecoModeXML `xml:"ecoMode"`

	EnableScreenOff  string `xml:"enableScreenOff"`
	PopUpPreviewWindow string `xml:"popUpPreviewWindow"`

	// Configurable display fields.
	ScreenOffTimeout int    `xml:"screenOffTimeout"`
	ShowMode         string `xml:"showMode"`
	PreviewShowTime  int    `xml:"previewShowTime"`
	StandbyTimeout   int    `xml:"standbyTimeout"`

	// AdvertisingDisplayType controls split vs full in advertising mode.
	// SOURCED: IdentityTerminal.php:48-53 (showMode match → display_type).
	AdvertisingDisplayType string `xml:"advertisingDisplayType"`
}

type ecoModeXML struct {
	Eco                     string `xml:"eco"`
	FaceMatchThreshold1     string `xml:"faceMatchThreshold1"`
	FaceMatchThresholdN     string `xml:"faceMatchThresholdN"`
	ChangeThreshold         string `xml:"changeThreshold"`
	MaskFaceMatchThresholdN string `xml:"maskFaceMatchThresholdN"`
	MaskFaceMatchThreshold1 string `xml:"maskFaceMatchThreshold1"`
}

// IdentityTerminalDisplay exposes only the configurable subset of the terminal display
// settings to callers. Read-only fields stay inside identityTerminalXML for RMW.
type IdentityTerminalDisplay struct {
	// ShowMode is the logical mode: "normal", "full", or "split".
	// SOURCED: IdentityTerminal.php:49-53 — maps to showMode+advertisingDisplayType in XML.
	ShowMode        string
	ScreenOffTimeout int
	PreviewShowTime  int
	StandbyTimeout   int

	// raw holds the full parsed XML for use in read-modify-write.
	raw *identityTerminalXML
}

// GetIdentityTerminal fetches the current display configuration from the device.
// SOURCED: IdentityTerminal.php:21-29 (info method).
// Endpoint: GET /ISAPI/AccessControl/IdentityTerminal (XML; no ?format=json)
func (c *Client) GetIdentityTerminal(ctx context.Context) (*IdentityTerminalDisplay, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointIdentityTerminal, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetIdentityTerminal: %w", err)
	}
	if status == 401 {
		return nil, &NonRetriableError{Op: "GetIdentityTerminal", Status: status}
	}
	if status != 200 {
		return nil, retriableOrNot("GetIdentityTerminal", status, body)
	}

	var raw identityTerminalXML
	if err := xml.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("hikvision: GetIdentityTerminal XML parse: %w", err)
	}

	return &IdentityTerminalDisplay{
		ShowMode:         rawToLogicalShowMode(raw.ShowMode, raw.AdvertisingDisplayType),
		ScreenOffTimeout: raw.ScreenOffTimeout,
		PreviewShowTime:  raw.PreviewShowTime,
		StandbyTimeout:   raw.StandbyTimeout,
		raw:              &raw,
	}, nil
}

// rawToLogicalShowMode converts the ISAPI (showMode, advertisingDisplayType) pair
// to one of the logical modes: "normal", "full", or "split".
// SOURCED: IdentityTerminal.php:49-53 (reverse mapping).
func rawToLogicalShowMode(showMode, displayType string) string {
	switch showMode {
	case "advertising":
		if strings.EqualFold(displayType, "split") {
			return "split"
		}
		return "full"
	default:
		return "normal"
	}
}

// logicalToRawShowMode maps a logical show mode to the ISAPI pair.
// SOURCED: IdentityTerminal.php:49-53 (update method match statement).
func logicalToRawShowMode(mode string) (showMode, displayType string) {
	switch mode {
	case "full":
		return "advertising", "full"
	case "split":
		return "advertising", "split"
	default: // "normal"
		return "normal", "full"
	}
}

// PutIdentityTerminal applies configurable display settings via read-modify-write.
// SOURCED: IdentityTerminal.php:47-104 (update method).
// Idempotent: same inputs produce identical XML (Constitution II / tasks 1.4.2).
func (c *Client) PutIdentityTerminal(ctx context.Context, screenOffTimeout, previewShowTime, standbyTimeout int, showMode string) error {
	disp, err := c.GetIdentityTerminal(ctx)
	if err != nil {
		return fmt.Errorf("hikvision: PutIdentityTerminal read: %w", err)
	}

	raw := disp.raw
	raw.ScreenOffTimeout = screenOffTimeout
	raw.PreviewShowTime = previewShowTime
	raw.StandbyTimeout = standbyTimeout
	raw.ShowMode, raw.AdvertisingDisplayType = logicalToRawShowMode(showMode)

	// Preserve xmlns to satisfy the firmware's schema validation.
	if raw.XMLNS == "" {
		raw.XMLNS = "http://www.isapi.org/ver20/XMLSchema"
	}

	xmlBody, err := xml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("hikvision: PutIdentityTerminal marshal: %w", err)
	}

	// Prepend XML declaration (not emitted by xml.Marshal).
	fullXML := append([]byte(xml.Header), xmlBody...)

	_, status, err := c.doRequest(ctx, http.MethodPut, endpointIdentityTerminal,
		bytes.NewReader(fullXML), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: PutIdentityTerminal PUT: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("PutIdentityTerminal PUT", status, nil)
}

// GetShowModeThumbnails fetches the list of show mode thumbnails from the device.
// SOURCED: IdentityTerminal.php:31-43 (thumbnailList method).
// Returns raw JSON bytes from the ISAPI (passed through to the handler).
// Endpoint: GET /ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json
func (c *Client) GetShowModeThumbnails(ctx context.Context) ([]byte, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointShowModeThumbnails, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: GetShowModeThumbnails: %w", err)
	}
	if status == 200 {
		return body, nil
	}
	if status == 404 {
		return nil, fmt.Errorf("hikvision: GetShowModeThumbnails: endpoint não suportado por este firmware (HTTP 404)")
	}
	return nil, retriableOrNot("GetShowModeThumbnails", status, body)
}
