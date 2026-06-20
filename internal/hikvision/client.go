// Package hikvision provides the ISAPI HTTP client for HikVision devices.
// Only the 3 operations permitted by Constitution Principle IV and FR-013:
//  1. UpsertUser  — POST/PUT /ISAPI/AccessControl/UserInfo/Modify
//  2. UploadFace  — POST /ISAPI/Intelligent/FDLib/faceDataRecord?format=json
//  3. ConfigureWebhook — POST /ISAPI/Event/notification/httpHosts
//
// All contracts verified from legacy/hik-api (contracts/hikvision-isapi.md).
// Auth: HTTP Digest (github.com/icholy/digest).
package hikvision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/icholy/digest"
)

const (
	endpointUserModify = "/ISAPI/AccessControl/UserInfo/Modify"
	endpointFaceRecord = "/ISAPI/Intelligent/FDLib/faceDataRecord?format=json"
	endpointHTTPHosts  = "/ISAPI/Event/notification/httpHosts"
	defaultTimeout     = 30 * time.Second
)

// DeviceConfig holds credentials and addressing for one HikVision device.
type DeviceConfig struct {
	Host     string // hostname or host:port (no scheme)
	Username string
	Password string // sensitive — never log
}

// Client is the HikVision ISAPI client for one device.
type Client struct {
	device     DeviceConfig
	httpClient *http.Client
}

// New creates a Client for the given device using HTTP Digest auth.
func New(device DeviceConfig) *Client {
	transport := &digest.Transport{
		Username: device.Username,
		Password: device.Password,
	}
	return &Client{
		device: device,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   defaultTimeout,
		},
	}
}

// NewWithHTTPClient creates a Client with a custom *http.Client (for tests).
func NewWithHTTPClient(device DeviceConfig, hc *http.Client) *Client {
	return &Client{device: device, httpClient: hc}
}

// baseURL returns the http:// base URL for the device.
func (c *Client) baseURL() string {
	host := c.device.Host
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}
	return "http://" + host
}

// doRequest executes an HTTP request and returns the response body + status code.
// Errors on network failure; the caller inspects the status code.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) ([]byte, int, error) {
	url := c.baseURL() + path

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("hikvision: create request %s %s: %w", method, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("hikvision: do request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("hikvision: read response body: %w", err)
	}

	return respBody, resp.StatusCode, nil
}

// UpsertUser creates or updates a user on the device.
// Strategy: try POST (create); if 409 conflict (user already exists), retry with PUT.
// XML fields: employeeNo (CPF digits) + name (contracts/hikvision-isapi.md §1).
func (c *Client) UpsertUser(ctx context.Context, cpfDigits, name string) error {
	xmlBody := fmt.Sprintf(
		"<UserInfo><employeeNo>%s</employeeNo><name>%s</name></UserInfo>",
		xmlEscape(cpfDigits),
		xmlEscape(name),
	)

	// Try POST first (create)
	_, status, err := c.doRequest(ctx, http.MethodPost, endpointUserModify,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: UpsertUser POST: %w", err)
	}

	switch {
	case status == 200 || status == 201:
		return nil
	case status == 409:
		// User already exists — try PUT (update)
		_, status2, err2 := c.doRequest(ctx, http.MethodPut, endpointUserModify,
			strings.NewReader(xmlBody), "application/xml")
		if err2 != nil {
			return fmt.Errorf("hikvision: UpsertUser PUT: %w", err2)
		}
		if status2 == 200 || status2 == 204 {
			return nil
		}
		return retriableOrNot("UpsertUser PUT", status2, nil)
	case status >= 400 && status < 500:
		// Non-retriable (e.g. 400 bad XML)
		return &NonRetriableError{Op: "UpsertUser POST", Status: status}
	default:
		// 5xx and others are retriable
		return fmt.Errorf("hikvision: UpsertUser POST: HTTP %d (retriable)", status)
	}
}

// UploadFace downloads the image from imageURL and uploads it to the device
// as a multipart request (contracts/hikvision-isapi.md §2).
func (c *Client) UploadFace(ctx context.Context, cpfDigits, imageURL string) error {
	// Download image
	imageData, mimeType, err := downloadImage(ctx, imageURL)
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace download %s: %w", imageURL, err)
	}

	// Validate MIME type (must be image/jpeg — contracts/hikvision-isapi.md §2, CHK031)
	if !strings.HasPrefix(mimeType, "image/jpeg") {
		return &NonRetriableError{
			Op:  "UploadFace mime_invalid",
			Msg: fmt.Sprintf("face image mime type %q is not image/jpeg; stage=face_upload_mime_invalid", mimeType),
		}
	}

	// Build multipart body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Part 1: FaceDataRecord (JSON)
	faceDataRecord := map[string]string{
		"type":        "concurrent",
		"faceLibType": "blackFD",
		"FDID":        "1",
		"FPID":        cpfDigits,
	}
	faceDataJSON, _ := json.Marshal(faceDataRecord)

	faceField, err := writer.CreateFormField("FaceDataRecord")
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart FaceDataRecord: %w", err)
	}
	if _, err := faceField.Write(faceDataJSON); err != nil {
		return fmt.Errorf("hikvision: UploadFace write FaceDataRecord: %w", err)
	}

	// Part 2: FaceImage (jpeg file)
	filePart, err := writer.CreateFormFile("FaceImage", cpfDigits+".jpg")
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart FaceImage: %w", err)
	}
	if _, err := filePart.Write(imageData); err != nil {
		return fmt.Errorf("hikvision: UploadFace write FaceImage: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart close: %w", err)
	}

	_, status, err := c.doRequest(ctx, http.MethodPost, endpointFaceRecord,
		&buf, writer.FormDataContentType())
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace POST: %w", err)
	}
	if status != 200 {
		return retriableOrNot("UploadFace POST", status, nil)
	}

	return nil
}

// ConfigureWebhook sets the HTTP notification host on the device.
// The id field is deterministic per device (SHA-256 of device host).
// Path includes WEBHOOK_PATH_SECRET so only authorized endpoints receive events (plan.md §S1).
// contracts/hikvision-isapi.md §3.
func (c *Client) ConfigureWebhook(ctx context.Context, webhookURL string) error {
	// Derive stable id from device host (deterministic — no accumulation of duplicate hosts)
	id := deterministicHostID(c.device.Host)

	// Parse webhookURL components for the XML fields
	host, port, path := parseWebhookURL(webhookURL)

	xmlBody := fmt.Sprintf(
		`<HttpHostNotification>`+
			`<id>%s</id>`+
			`<url>%s</url>`+
			`<protocolType>HTTP</protocolType>`+
			`<parameterFormatType>XML</parameterFormatType>`+
			`<addressingFormatType>ipaddress</addressingFormatType>`+
			`<ipAddress>%s</ipAddress>`+
			`<portNo>%s</portNo>`+
			`<path>%s</path>`+
			`<httpAuthenticationMethod>none</httpAuthenticationMethod>`+
			`</HttpHostNotification>`,
		xmlEscape(id),
		xmlEscape(webhookURL),
		xmlEscape(host),
		xmlEscape(port),
		xmlEscape(path),
	)

	_, status, err := c.doRequest(ctx, http.MethodPost, endpointHTTPHosts,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: ConfigureWebhook POST: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}
	return retriableOrNot("ConfigureWebhook POST", status, nil)
}

// downloadImage fetches an image from url and returns (bytes, mimeType, error).
func downloadImage(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	hc := &http.Client{Timeout: 30 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d fetching image from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Detect MIME from Content-Type header; fall back to sniffing
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}

	return data, ct, nil
}

// deterministicHostID returns a short hex ID stable per device host.
func deterministicHostID(host string) string {
	h := sha256.Sum256([]byte(host))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars
}

// parseWebhookURL splits a URL into host, port, path.
// Minimal parser sufficient for known patterns (no complex URL escaping needed).
func parseWebhookURL(rawURL string) (host, port, path string) {
	// Strip scheme
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}

	// Split host:port from path
	pathIdx := strings.Index(s, "/")
	if pathIdx < 0 {
		path = "/"
	} else {
		path = s[pathIdx:]
		s = s[:pathIdx]
	}

	// Split host and port
	if colonIdx := strings.LastIndex(s, ":"); colonIdx >= 0 {
		host = s[:colonIdx]
		port = s[colonIdx+1:]
	} else {
		host = s
		port = "80"
	}

	return host, port, path
}

// xmlEscape performs minimal XML escaping for attribute/element values.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// retriableOrNot returns an appropriate error based on HTTP status.
func retriableOrNot(op string, status int, body []byte) error {
	if status >= 400 && status < 500 {
		return &NonRetriableError{Op: op, Status: status}
	}
	return fmt.Errorf("hikvision: %s: HTTP %d (retriable)", op, status)
}

// NonRetriableError indicates an error that should go directly to DLQ without retrying.
type NonRetriableError struct {
	Op     string
	Status int
	Msg    string
}

func (e *NonRetriableError) Error() string {
	if e.Msg != "" {
		return fmt.Sprintf("hikvision: %s: %s", e.Op, e.Msg)
	}
	return fmt.Sprintf("hikvision: %s: HTTP %d (non-retriable)", e.Op, e.Status)
}

// IsNonRetriable reports whether an error should bypass retry and go straight to DLQ.
func IsNonRetriable(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*NonRetriableError)
	return ok
}
