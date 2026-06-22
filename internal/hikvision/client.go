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
	"encoding/xml"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registra o decoder PNG para image.Decode
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/icholy/digest"
)

const (
	// Criação de usuário: POST /Record; atualização: PUT /Modify (ambos JSON).
	// Este firmware recusa POST em /Modify (methodNotAllowed) — contrato verificado
	// contra o dispositivo real, não o legacy (que usava XML+POST em /Modify).
	endpointUserCreate = "/ISAPI/AccessControl/UserInfo/Record?format=json"
	endpointUserModify = "/ISAPI/AccessControl/UserInfo/Modify?format=json"
	endpointFaceRecord = "/ISAPI/Intelligent/FDLib/faceDataRecord?format=json"
	endpointHTTPHosts  = "/ISAPI/Event/notification/httpHosts"
	// Sem ?format=json: este endpoint responde XML por padrão neste firmware
	// (verificado no legacy DeviceService::parseDeviceInfo). FetchDeviceInfo
	// aceita XML ou JSON para robustez.
	endpointDeviceInfo = "/ISAPI/System/deviceInfo"
	defaultTimeout     = 30 * time.Second
)

// DeviceInfo é o subconjunto de /ISAPI/System/deviceInfo que persistimos no banco
// (a identidade de hardware estável — serial — que o heartbeat não carrega).
type DeviceInfo struct {
	DeviceName      string
	Model           string
	SerialNumber    string
	FirmwareVersion string
}

// FetchDeviceInfo lê GET /ISAPI/System/deviceInfo e retorna serial/model/firmware.
// Aceita XML (padrão do firmware) ou JSON. Best-effort: o chamador trata erro
// (device offline) sem falhar o fluxo principal.
func (c *Client) FetchDeviceInfo(ctx context.Context) (*DeviceInfo, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointDeviceInfo, nil, "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("hikvision: deviceInfo HTTP %d (body: %.120s)", status, string(body))
	}

	di := &DeviceInfo{}
	if t := bytes.TrimSpace(body); len(t) > 0 && t[0] == '{' {
		// JSON: {"DeviceInfo":{...}}
		var w struct {
			DeviceInfo struct {
				DeviceName      string `json:"deviceName"`
				Model           string `json:"model"`
				SerialNumber    string `json:"serialNumber"`
				FirmwareVersion string `json:"firmwareVersion"`
			} `json:"DeviceInfo"`
		}
		if err := json.Unmarshal(body, &w); err != nil {
			return nil, fmt.Errorf("hikvision: deviceInfo JSON parse: %w", err)
		}
		di.DeviceName, di.Model = w.DeviceInfo.DeviceName, w.DeviceInfo.Model
		di.SerialNumber, di.FirmwareVersion = w.DeviceInfo.SerialNumber, w.DeviceInfo.FirmwareVersion
	} else {
		// XML: <DeviceInfo><serialNumber>…</serialNumber>…</DeviceInfo>
		var w struct {
			XMLName         xml.Name `xml:"DeviceInfo"`
			DeviceName      string   `xml:"deviceName"`
			Model           string   `xml:"model"`
			SerialNumber    string   `xml:"serialNumber"`
			FirmwareVersion string   `xml:"firmwareVersion"`
		}
		if err := xml.Unmarshal(body, &w); err != nil {
			return nil, fmt.Errorf("hikvision: deviceInfo XML parse: %w", err)
		}
		di.DeviceName, di.Model = w.DeviceName, w.Model
		di.SerialNumber, di.FirmwareVersion = w.SerialNumber, w.FirmwareVersion
	}

	if di.SerialNumber == "" && di.Model == "" && di.FirmwareVersion == "" {
		return nil, fmt.Errorf("hikvision: deviceInfo vazio/sem campos reconhecidos")
	}
	return di, nil
}

// isapiStatus é o corpo JSON de status retornado pela ISAPI.
type isapiStatus struct {
	StatusCode    int    `json:"statusCode"`
	SubStatusCode string `json:"subStatusCode"`
	ErrorMsg      string `json:"errorMsg"`
}

// parseSubStatus extrai o subStatusCode de uma resposta ISAPI JSON ("" se não parsear).
func parseSubStatus(body []byte) string {
	var s isapiStatus
	if err := json.Unmarshal(body, &s); err != nil {
		return ""
	}
	return s.SubStatusCode
}

// buildUserJSON monta o corpo de criação/atualização de usuário.
// userType e Valid são obrigatórios neste firmware (verificado contra o device).
func buildUserJSON(employeeNo, name string) string {
	payload := map[string]any{
		"UserInfo": map[string]any{
			"employeeNo": employeeNo,
			"name":       name,
			"userType":   "normal",
			"Valid": map[string]any{
				"enable":    true,
				"beginTime": "2020-01-01T00:00:00",
				"endTime":   "2037-12-31T23:59:59",
				"timeType":  "local",
			},
			// doorRight é a LISTA de portas que o usuário pode abrir. Sem ela, há
			// firmware/config em que o leitor RECONHECE a face mas NEGA o acesso
			// ("sem permissão"), pois RightPlan só amarra o HORÁRIO por porta, não a
			// permissão de porta em si. SOURCED do payload provado no device real:
			// legacy/hik-api/old/src/Device/DSK1T673DWX/User.php:81 ('doorRight'=>'1,2').
			"doorRight":    "1,2",
			"localUIRight": false,
			// Vincula o template de horário semanal (porta 1, plano 1 = 24/7).
			// Sem planTemplateNo o leitor recusa o acesso com "Duração inválida".
			// O firmware exige planTemplateNo como STRING (número é rejeitado).
			"RightPlan": []map[string]any{
				{"doorNo": 1, "planTemplateNo": "1"},
			},
		},
	}
	b, _ := json.Marshal(payload) //nolint:errcheck
	return string(b)
}

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
	body := buildUserJSON(cpfDigits, name)

	// 1) Cria via POST /Record
	respBody, status, err := c.doRequest(ctx, http.MethodPost, endpointUserCreate,
		strings.NewReader(body), "application/json")
	if err != nil {
		return fmt.Errorf("hikvision: UpsertUser POST: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}

	// 2) Já existe (HTTP 400 + employeeNoAlreadyExist) → atualiza via PUT /Modify
	if status == 400 && parseSubStatus(respBody) == "employeeNoAlreadyExist" {
		respBody2, status2, err2 := c.doRequest(ctx, http.MethodPut, endpointUserModify,
			strings.NewReader(body), "application/json")
		if err2 != nil {
			return fmt.Errorf("hikvision: UpsertUser PUT: %w", err2)
		}
		if status2 == 200 || status2 == 204 {
			return nil
		}
		return retriableOrNot("UpsertUser PUT "+parseSubStatus(respBody2), status2, respBody2)
	}

	if status >= 400 && status < 500 {
		return &NonRetriableError{Op: "UpsertUser POST " + parseSubStatus(respBody), Status: status}
	}
	return fmt.Errorf("hikvision: UpsertUser POST: HTTP %d (retriable)", status)
}

// UploadFace downloads the image from imageURL and uploads it to the device
// as a multipart request (contracts/hikvision-isapi.md §2).
func (c *Client) UploadFace(ctx context.Context, cpfDigits, imageURL string) error {
	// Download image
	imageData, mimeType, err := downloadImage(ctx, imageURL)
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace download %s: %w", imageURL, err)
	}

	// O dispositivo aceita JPEG e PNG (verificado); só rejeitamos não-imagem.
	if !strings.HasPrefix(mimeType, "image/") {
		return &NonRetriableError{
			Op:  "UploadFace mime_invalid",
			Msg: fmt.Sprintf("conteúdo %q não é imagem; stage=face_upload_mime_invalid", mimeType),
		}
	}

	// Transcodifica para JPEG: PNGs grandes do GOB (ex. 400x400 > 200KB) estouram
	// o limite de tamanho do leitor (resposta badJsonContent/faceURL). JPEG q85
	// reduz para dezenas de KB, dentro do limite. image.Decode cobre png/jpeg.
	srcImg, _, decErr := image.Decode(bytes.NewReader(imageData))
	if decErr != nil {
		return &NonRetriableError{
			Op:  "UploadFace decode",
			Msg: fmt.Sprintf("imagem inválida (%s): %v; stage=face_upload_decode", mimeType, decErr),
		}
	}
	var jpegBuf bytes.Buffer
	if encErr := jpeg.Encode(&jpegBuf, srcImg, &jpeg.Options{Quality: 85}); encErr != nil {
		return fmt.Errorf("hikvision: UploadFace jpeg encode: %w", encErr)
	}
	imageData = jpegBuf.Bytes()

	// Build multipart body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Part 1: FaceDataRecord (JSON)
	faceDataRecord := map[string]string{
		"faceLibType": "blackFD",
		"FDID":        "1",
		"FPID":        cpfDigits,
	}
	faceDataJSON, _ := json.Marshal(faceDataRecord) //nolint:errcheck

	faceField, err := writer.CreateFormField("FaceDataRecord")
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart FaceDataRecord: %w", err)
	}
	if _, err := faceField.Write(faceDataJSON); err != nil {
		return fmt.Errorf("hikvision: UploadFace write FaceDataRecord: %w", err)
	}

	// Part 2: FaceImage (arquivo, com content-type real detectado)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="FaceImage"; filename="%s.jpg"`, cpfDigits))
	header.Set("Content-Type", "image/jpeg")
	filePart, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart FaceImage: %w", err)
	}
	if _, err := filePart.Write(imageData); err != nil {
		return fmt.Errorf("hikvision: UploadFace write FaceImage: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("hikvision: UploadFace multipart close: %w", err)
	}

	respBody, status, err := c.doRequest(ctx, http.MethodPost, endpointFaceRecord,
		&buf, writer.FormDataContentType())
	if err != nil {
		return fmt.Errorf("hikvision: UploadFace POST: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}
	// Face já cadastrada para o usuário → idempotente (sucesso)
	if status == 400 && parseSubStatus(respBody) == "deviceUserAlreadyExistFace" {
		return nil
	}
	return retriableOrNot("UploadFace POST "+parseSubStatus(respBody), status, respBody)
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

// DeterministicWebhookID returns the stable webhook host ID for the given device host string.
// Used by admin handlers to detect when the primary system webhook is being removed (FR-019).
// host must be in "ip:port" format (same as DeviceConfig.Host built by LoadDeviceConfig).
func DeterministicWebhookID(host string) string {
	return deterministicHostID(host)
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
