package httphandler

// admin_device_preferences_handlers.go implements HTTP admin handlers for HikVision
// device preference management endpoints:
//   - GET/PUT  /preferences/auth-mode                  (FR-001/002)
//   - GET/PUT  /preferences/display                    (FR-003/004)
//   - GET      /preferences/display/thumbnails         (FR-005)
//   - GET/POST /preferences/standby-pictures           (FR-006/007)
//   - DELETE   /preferences/standby-pictures/{uuid}    (FR-008)
//   - PUT      /preferences/standby-pictures/disable   (FR-009)
//   - POST     /preferences/boot-picture               (FR-010)
//   - DELETE   /preferences/boot-picture               (FR-011)
//   - GET/POST /preferences/media                      (FR-012/013)
//   - DELETE   /preferences/media/{id}                 (FR-014)
//   - DELETE   /preferences/media                      (FR-015, bulk)
//   - GET      /stats                                  (FR-016)
//   - PUT      /preferences/face-config                (FR-017)
//   - POST     /preferences/face-capture               (FR-018)
//
// Security controls (FASE 1 tasks 1.12/1.14):
//   - maxUploadBodyBytes: cap for all multipart uploads (20 MB)
//   - validateImageContentType: validates image/* MIME type on upload parts
//
// Auth: all endpoints require AdminAuth via adminDevicesRouter (server.go).
// Log: NEVER log isapi_password, ISAPI tokens or binary content (FR-023).
// Ref: spec.md §FR-001..018, tasks.md §FASE 2.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

const (
	// maxUploadBodyBytes caps all multipart upload request bodies at 20 MB.
	// Applied via http.MaxBytesReader before ParseMultipartForm to prevent DoS
	// through memory exhaustion (plan.md §6.1 F1 / CHK022 / CHK025 / tasks 1.12.1).
	// Value is model-agnostic (dec-010): no per-model pre-validation; this cap
	// only guards against exhaust-memory attacks.
	maxUploadBodyBytes = 20 * 1024 * 1024 // 20 MB
)

// validateImageContentType returns an error if contentType does not start with "image/".
// Applied to the Content-Type of uploaded file parts (spec §FR-022 / tasks 1.14.1).
// Returns nil for valid image/* types (image/jpeg, image/png, image/gif, …).
func validateImageContentType(contentType string) error {
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("arquivo deve ser imagem (image/*), recebido: %q", contentType)
	}
	return nil
}

// isBodyTooLargeError reports whether err was produced by http.MaxBytesReader.
func isBodyTooLargeError(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// readUploadPart applies MaxBytesReader, parses multipart, validates image/*, and reads
// the file field. Returns (filename, data, 0, "") on success or (0, statusCode, errMsg)
// on validation failure. The caller should write the error and return if statusCode != 0.
// This consolidates the repeated multipart+image-validation boilerplate across 3 handlers.
func readUploadPart(w http.ResponseWriter, r *http.Request) (filename string, data []byte, httpSt int, errMsg string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)
	if err := r.ParseMultipartForm(maxUploadBodyBytes); err != nil {
		if isBodyTooLargeError(err) {
			return "", nil, http.StatusRequestEntityTooLarge,
				"corpo da requisição excede o limite de 20 MB; envie uma imagem menor"
		}
		return "", nil, http.StatusBadRequest, "multipart inválido: " + err.Error()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return "", nil, http.StatusBadRequest, "campo 'file' ausente"
	}
	defer file.Close()

	ct := header.Header.Get("Content-Type")
	if err := validateImageContentType(ct); err != nil {
		return "", nil, http.StatusBadRequest, err.Error()
	}

	b, err := io.ReadAll(file)
	if err != nil {
		return "", nil, http.StatusBadRequest, "erro ao ler o arquivo: " + err.Error()
	}

	return header.Filename, b, 0, ""
}

// =============================================================================
// 2.1 — Auth mode: GET /preferences/auth-mode, PUT /preferences/auth-mode
// =============================================================================

// weekPlanResponse is one entry in the auth-mode response (tasks 2.1.1).
type weekPlanResponse struct {
	WeekNo     int    `json:"weekNo"`
	VerifyMode string `json:"verifyMode"`
}

// authModeResponse is the JSON response for GET /preferences/auth-mode.
type authModeResponse struct {
	WeekPlans []weekPlanResponse `json:"weekPlans"`
}

// GetDeviceAuthModeHandler serves GET /admin/api/devices/{id}/preferences/auth-mode.
// Spec §FR-001 / tasks 2.1.1.
// Returns the full weekly verify-mode plan from the device.
func GetDeviceAuthModeHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "consultando modo de verificação", "stage", "auth-mode")

		plan, err := client.GetVerifyMode(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao obter modo de verificação", err, "stage", "auth-mode")
			adminJSONError(w, st, msg)
			return
		}

		items := make([]weekPlanResponse, 0, len(plan.WeekPlanCfgs))
		for _, p := range plan.WeekPlanCfgs {
			items = append(items, weekPlanResponse{WeekNo: p.WeekNo, VerifyMode: p.VerifyMode})
		}

		resp := authModeResponse{WeekPlans: items}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// putAuthModeRequest is the request body for PUT /preferences/auth-mode.
type putAuthModeRequest struct {
	VerifyMode string `json:"verifyMode"`
}

// PutDeviceAuthModeHandler serves PUT /admin/api/devices/{id}/preferences/auth-mode.
// Spec §FR-002 / tasks 2.1.2.
// Applies the given verifyMode to all 7 weekly plans via RMW.
func PutDeviceAuthModeHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		var req putAuthModeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}
		if req.VerifyMode == "" {
			adminJSONError(w, http.StatusBadRequest, "verifyMode é obrigatório")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "alterando modo de verificação",
			"stage", "auth-mode", "mode", req.VerifyMode)

		if err := client.SetVerifyMode(ctx, req.VerifyMode); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao definir modo de verificação", err,
				"stage", "auth-mode", "mode", req.VerifyMode)
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "modo de verificação atualizado",
			"stage", "auth-mode", "mode", req.VerifyMode)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "ok",
			"device_id": deviceID,
		})
	})
}

// =============================================================================
// 2.2 — Display: GET/PUT /preferences/display, GET /preferences/display/thumbnails
// =============================================================================

// displayResponse is the JSON response for GET /preferences/display.
type displayResponse struct {
	ShowMode        string `json:"showMode"`
	ScreenOffTimeout int   `json:"screenOffTimeout"`
	PreviewShowTime int   `json:"previewShowTime"`
	StandbyTimeout  int   `json:"standbyTimeout"`
}

// GetDeviceDisplayHandler serves GET /admin/api/devices/{id}/preferences/display.
// Spec §FR-003 / tasks 2.2.1.
func GetDeviceDisplayHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "consultando configuração de display", "stage", "display")

		disp, err := client.GetIdentityTerminal(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao obter configuração de display", err, "stage", "display")
			adminJSONError(w, st, msg)
			return
		}

		resp := displayResponse{
			ShowMode:        disp.ShowMode,
			ScreenOffTimeout: disp.ScreenOffTimeout,
			PreviewShowTime: disp.PreviewShowTime,
			StandbyTimeout:  disp.StandbyTimeout,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// putDisplayRequest is the request body for PUT /preferences/display.
type putDisplayRequest struct {
	ShowMode        string `json:"showMode"`
	ScreenOffTimeout int   `json:"screenOffTimeout"`
	PreviewShowTime int   `json:"previewShowTime"`
	StandbyTimeout  int   `json:"standbyTimeout"`
}

// validShowModes is the set of allowed showMode values (spec §FR-004, Clarification 2).
var validShowModes = map[string]bool{
	"normal": true,
	"full":   true,
	"split":  true,
}

// PutDeviceDisplayHandler serves PUT /admin/api/devices/{id}/preferences/display.
// Spec §FR-004 / tasks 2.2.2.
// Applies the display settings via RMW (read-modify-write) to preserve read-only fields.
func PutDeviceDisplayHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		var req putDisplayRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}
		if !validShowModes[req.ShowMode] {
			adminJSONError(w, http.StatusBadRequest,
				"showMode inválido; valores aceitos: normal, full, split")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "alterando configuração de display",
			"stage", "display", "showMode", req.ShowMode)

		if err := client.PutIdentityTerminal(ctx,
			req.ScreenOffTimeout, req.PreviewShowTime, req.StandbyTimeout, req.ShowMode,
		); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao definir configuração de display", err,
				"stage", "display", "showMode", req.ShowMode)
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "configuração de display atualizada",
			"stage", "display", "showMode", req.ShowMode)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "ok",
			"device_id": deviceID,
		})
	})
}

// GetDeviceDisplayThumbnailsHandler serves GET /admin/api/devices/{id}/preferences/display/thumbnails.
// Spec §FR-005 / tasks 2.2.3.
// Passes the raw ISAPI JSON response to the client (format defined by device firmware).
func GetDeviceDisplayThumbnailsHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		raw, err := client.GetShowModeThumbnails(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao obter thumbnails de display", err, "stage", "display-thumbnails")
			adminJSONError(w, st, msg)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(raw) //nolint:errcheck
	})
}

// =============================================================================
// 2.3 — Standby pictures: GET, POST, DELETE /{uuid}, PUT disable
// =============================================================================

// standbyPicturesResponse is the JSON response for GET /preferences/standby-pictures.
type standbyPicturesResponse struct {
	Pictures []standbyPictureItem `json:"pictures"`
	Total    int                  `json:"total"`
}

type standbyPictureItem struct {
	UUID     string `json:"uuid"`
	FileName string `json:"fileName"`
}

// GetDeviceStandbyPicturesHandler serves GET /admin/api/devices/{id}/preferences/standby-pictures.
// Spec §FR-006 / tasks 2.3.1.
func GetDeviceStandbyPicturesHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		pics, err := client.ListStandbyPictures(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao listar imagens de standby", err, "stage", "standby-pictures")
			adminJSONError(w, st, msg)
			return
		}

		items := make([]standbyPictureItem, 0, len(pics))
		for _, p := range pics {
			items = append(items, standbyPictureItem{UUID: p.UUID, FileName: p.FileName})
		}

		resp := standbyPicturesResponse{Pictures: items, Total: len(items)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// PostDeviceStandbyPictureHandler serves POST /admin/api/devices/{id}/preferences/standby-pictures.
// Spec §FR-007 / tasks 2.3.2.
// Uploads a picture and enables custom standby. Reports enable failure even after successful upload.
// Security: MaxBytesReader (1.12.2) + validateImageContentType (1.14.2) applied via readUploadPart.
func PostDeviceStandbyPictureHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		filename, data, httpSt, errMsg := readUploadPart(w, r)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		ctx := r.Context()
		_, client, httpSt2, errMsg2 := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt2 != 0 {
			adminJSONError(w, httpSt2, errMsg2)
			return
		}

		cfg.logInfo("preferences", deviceID, "enviando imagem de standby",
			"stage", "standby-pictures", "filename", filename)

		if err := client.UploadStandbyPicture(ctx, filename, data); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao enviar imagem de standby", err,
				"stage", "standby-pictures", "filename", filename)
			adminJSONError(w, st, msg)
			return
		}

		// Enable custom standby after upload. Report failure as part of response even on upload success.
		enableErr := client.EnableCustomStandby(ctx)
		if enableErr != nil {
			cfg.logError("preferences", deviceID, "imagem enviada mas falha ao ativar standby customizado",
				enableErr, "stage", "standby-pictures")
			st, msg := mapISAPIError(enableErr)
			adminJSONError(w, st,
				fmt.Sprintf("imagem enviada mas falha ao ativar standby customizado: %s", msg))
			return
		}

		cfg.logInfo("preferences", deviceID, "imagem de standby enviada e ativada",
			"stage", "standby-pictures", "filename", filename)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "uploaded_and_enabled",
			"device_id": deviceID,
		})
	})
}

// DeleteDeviceStandbyPictureHandler serves DELETE /admin/api/devices/{id}/preferences/standby-pictures/{uuid}.
// Spec §FR-008 / tasks 2.3.3.
// uuid is taken from the path segment after standby-pictures/; validated non-empty and no path traversal.
func DeleteDeviceStandbyPictureHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 2 || segs[0] != "preferences" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		// segs: ["preferences", "standby-pictures", "{uuid}"]
		if len(segs) < 3 || segs[1] != "standby-pictures" {
			adminJSONError(w, http.StatusBadRequest, "uuid ausente")
			return
		}
		uuid := segs[2]
		if uuid == "" || strings.Contains(uuid, "/") || strings.Contains(uuid, "..") {
			adminJSONError(w, http.StatusBadRequest,
				"uuid inválido: não pode ser vazio ou conter caracteres de path traversal")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.DeleteStandbyPicture(ctx, uuid); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao remover imagem de standby", err,
				"stage", "standby-pictures", "uuid", uuid)
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "imagem de standby removida",
			"stage", "standby-pictures", "uuid", uuid)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "deleted",
			"device_id": deviceID,
		})
	})
}

// PutDeviceStandbyDisableHandler serves PUT /admin/api/devices/{id}/preferences/standby-pictures/disable.
// Spec §FR-009 / tasks 2.3.4.
// Restores the default standby (disables custom standby mode).
func PutDeviceStandbyDisableHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.DisableCustomStandby(ctx); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao desativar standby customizado", err,
				"stage", "standby-pictures")
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "standby customizado desativado", "stage", "standby-pictures")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "disabled",
			"device_id": deviceID,
		})
	})
}

// =============================================================================
// 2.4 — Boot picture: POST /preferences/boot-picture, DELETE /preferences/boot-picture
// =============================================================================

// PostDeviceBootPictureHandler serves POST /admin/api/devices/{id}/preferences/boot-picture.
// Spec §FR-010 / tasks 2.4.1.
// Security: MaxBytesReader (1.12.2) + validateImageContentType (1.14.2) via readUploadPart.
// Note: validates image/* (not exclusively JPEG) per tasks 2.4.3.
func PostDeviceBootPictureHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		_, data, httpSt, errMsg := readUploadPart(w, r)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		ctx := r.Context()
		_, client, httpSt2, errMsg2 := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt2 != 0 {
			adminJSONError(w, httpSt2, errMsg2)
			return
		}

		cfg.logInfo("preferences", deviceID, "enviando imagem de boot", "stage", "boot-picture")

		if err := client.UploadBootPicture(ctx, deviceID, data); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao enviar imagem de boot", err,
				"stage", "boot-picture")
			// Provide actionable context: firmware may reject images that are too large or wrong format
			adminJSONError(w, st,
				fmt.Sprintf("falha ao enviar imagem de boot: %s — verifique o tamanho e formato (JPEG recomendado pelo firmware)", msg))
			return
		}

		cfg.logInfo("preferences", deviceID, "imagem de boot enviada", "stage", "boot-picture")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "uploaded",
			"device_id": deviceID,
		})
	})
}

// DeleteDeviceBootPictureHandler serves DELETE /admin/api/devices/{id}/preferences/boot-picture.
// Spec §FR-011 / tasks 2.4.2.
func DeleteDeviceBootPictureHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.DeleteBootPicture(ctx); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao remover imagem de boot", err,
				"stage", "boot-picture")
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "imagem de boot removida", "stage", "boot-picture")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "deleted",
			"device_id": deviceID,
		})
	})
}

// =============================================================================
// 2.5 — Media: GET/POST /preferences/media, DELETE /preferences/media/{id}, DELETE /preferences/media
// =============================================================================

// devicePresentationRepo são os métodos de PresentationMediaRepository usados pelos
// handlers de media/presentation (persistência do show_mode por material).
type devicePresentationRepo interface {
	Upsert(ctx context.Context, deviceID int64, materialID, mode, name string) error
	GetMode(ctx context.Context, deviceID int64, materialID string) (string, error)
	ListModesByDevice(ctx context.Context, deviceID int64) (map[string]string, error)
	Delete(ctx context.Context, deviceID int64, materialID string) error
}

// mediaListResponse is the JSON response for GET /preferences/media.
type mediaListResponse struct {
	Materials []mediaItem `json:"materials"`
	Total     int         `json:"total"`
}

type mediaItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Mode é o show_mode (full/split) persistido para a imagem; "" se desconhecido
	// (ex.: material enviado fora deste app). Deriva do tamanho no upload.
	Mode string `json:"mode,omitempty"`
}

// GetDeviceMediaHandler serves GET /admin/api/devices/{id}/preferences/media.
// Spec §FR-012 / tasks 2.5.1.
func GetDeviceMediaHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		mats, err := client.ListMaterials(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao listar materiais", err, "stage", "media")
			adminJSONError(w, st, msg)
			return
		}

		// Enriquece cada material com o show_mode persistido (full/split), para o
		// editor de fluxo / UI saber em que modo a imagem deve ser aplicada.
		var modes map[string]string
		if cfg.PresentationRepo != nil {
			if m, merr := cfg.PresentationRepo.ListModesByDevice(ctx, deviceID); merr == nil {
				modes = m
			} else {
				cfg.logError("preferences", deviceID, "falha ao carregar modos de presentation", merr, "stage", "media")
			}
		}

		items := make([]mediaItem, 0, len(mats))
		for _, m := range mats {
			items = append(items, mediaItem{ID: m.ID, Name: m.Name, Mode: modes[m.ID]})
		}

		resp := mediaListResponse{Materials: items, Total: len(items)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// PostDeviceMediaHandler serves POST /admin/api/devices/{id}/preferences/media.
// Spec §FR-013 / tasks 2.5.2.
// Executes the 5-step advertising media creation flow.
// On step failure with OrphanMaterialID set, returns actionable response with orphan ID
// so the operator can clean up via DELETE /preferences/media/{id} (Clarification 4).
// Security: MaxBytesReader (1.12.2) + validateImageContentType (1.14.2) via readUploadPart.
func PostDeviceMediaHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		filename, data, httpSt, errMsg := readUploadPart(w, r)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		// Valida a resolução e deriva o show_mode (600x1024→full, 600x704→split).
		// Qualquer outra medida → 400. A imagem vai como está (só transcode p/ JPEG).
		mode, err := hikvision.PresentationModeForImage(data)
		if err != nil {
			adminJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		jpegData, err := hikvision.TranscodeToJPEG(data)
		if err != nil {
			adminJSONError(w, http.StatusBadRequest, "falha ao processar a imagem: "+err.Error())
			return
		}

		ctx := r.Context()
		_, client, httpSt2, errMsg2 := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt2 != 0 {
			adminJSONError(w, httpSt2, errMsg2)
			return
		}

		cfg.logInfo("preferences", deviceID, "criando material publicitário",
			"stage", "media", "filename", filename, "mode", mode)

		result, err := client.CreateAdvertisingMedia(ctx, filename, jpegData)
		if err != nil {
			cfg.logError("preferences", deviceID, "falha ao criar material publicitário", err,
				"stage", "media", "filename", filename)

			// Check for partial failure with orphan material (Clarification 4 / dec-012)
			var mediaErr *hikvision.ErrAdvertisingMediaCreate
			if errors.As(err, &mediaErr) {
				st, msg := mapISAPIError(mediaErr.Cause)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(st)
				json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
					"error":            msg,
					"stage":            mediaErr.Step,
					"orphanMaterialId": mediaErr.OrphanMaterialID,
					"device_id":        deviceID,
				})
				return
			}

			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// Persiste o modo derivado do tamanho, para reaplicar ao selecionar a imagem.
		if cfg.PresentationRepo != nil {
			if perr := cfg.PresentationRepo.Upsert(ctx, deviceID, result.MaterialID, mode, filename); perr != nil {
				cfg.logError("preferences", deviceID, "falha ao persistir modo de presentation", perr,
					"stage", "media", "materialId", result.MaterialID)
			}
		}

		// Ajusta o show_mode do terminal conforme o tamanho (a imagem já virou
		// presentation no passo (d) do CreateAdvertisingMedia). Espelha 1-split/2-full.
		if err := client.SetShowMode(ctx, mode); err != nil {
			cfg.logError("preferences", deviceID, "material criado mas falha ao ajustar show_mode", err,
				"stage", "media", "materialId", result.MaterialID, "mode", mode)
			st, msg := mapISAPIError(err)
			adminJSONError(w, st,
				fmt.Sprintf("imagem enviada mas falha ao ajustar o modo de exibição (%s): %s", mode, msg))
			return
		}

		cfg.logInfo("preferences", deviceID, "material publicitário criado e aplicado",
			"stage", "media", "materialId", result.MaterialID, "mode", mode)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":     "created",
			"materialId": result.MaterialID,
			"programId":  result.ProgramID,
			"scheduleId": result.ScheduleID,
			"mode":       mode,
			"device_id":  deviceID,
		})
	})
}

// PutDevicePresentationHandler serve PUT /admin/api/devices/{id}/preferences/media/{materialId}/presentation.
// Torna um material JÁ existente a imagem de presentation (start-page) do device,
// aplicando também o show_mode persistido (full/split). Espelha switch.php (Presentation.page)
// + 1-split/2-full (show_mode). Ref: legacy presentation/switch.php.
func PutDevicePresentationHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 4 || segs[0] != "preferences" || segs[1] != "media" || segs[3] != "presentation" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		materialID := segs[2]
		if materialID == "" || strings.Contains(materialID, "/") || strings.Contains(materialID, "..") {
			adminJSONError(w, http.StatusBadRequest,
				"id de material inválido: não pode ser vazio ou conter path traversal")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		// Resolve o modo persistido; default "full" se desconhecido (material externo).
		mode := hikvision.ShowModeFull
		name := materialID
		if cfg.PresentationRepo != nil {
			if m, gerr := cfg.PresentationRepo.GetMode(ctx, deviceID, materialID); gerr == nil && m != "" {
				mode = m
			}
		}

		cfg.logInfo("preferences", deviceID, "aplicando imagem como presentation",
			"stage", "presentation", "materialId", materialID, "mode", mode)

		if err := client.ApplyPresentation(ctx, materialID, name, mode); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao aplicar presentation", err,
				"stage", "presentation", "materialId", materialID)
			adminJSONError(w, st, msg)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":     "applied",
			"materialId": materialID,
			"mode":       mode,
			"device_id":  deviceID,
		})
	})
}

// DeleteDeviceMediaItemHandler serves DELETE /admin/api/devices/{id}/preferences/media/{id}.
// Spec §FR-014 / tasks 2.5.3.
// Deletes a single advertising material by ID.
func DeleteDeviceMediaItemHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, segs, ok := deviceConfigPathSegments(r.URL.Path)
		if !ok || len(segs) < 3 || segs[0] != "preferences" || segs[1] != "media" {
			adminJSONError(w, http.StatusBadRequest, "path inválido")
			return
		}
		matID := segs[2]
		if matID == "" || strings.Contains(matID, "/") || strings.Contains(matID, "..") {
			adminJSONError(w, http.StatusBadRequest,
				"id inválido: não pode ser vazio ou conter caracteres de path traversal")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		if err := client.DeleteMaterial(ctx, matID); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao remover material", err,
				"stage", "media", "materialId", matID)
			adminJSONError(w, st, msg)
			return
		}

		// Limpa o modo persistido (best-effort; falha não reverte o delete no device).
		if cfg.PresentationRepo != nil {
			if perr := cfg.PresentationRepo.Delete(ctx, deviceID, matID); perr != nil {
				cfg.logError("preferences", deviceID, "falha ao limpar modo de presentation", perr,
					"stage", "media", "materialId", matID)
			}
		}

		cfg.logInfo("preferences", deviceID, "material removido",
			"stage", "media", "materialId", matID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "deleted",
			"device_id": deviceID,
		})
	})
}

// DeleteDeviceMediaAllHandler serves DELETE /admin/api/devices/{id}/preferences/media (bulk).
// Spec §FR-015 / tasks 2.5.4.
// Deletes all advertising materials; individual failures do not block others.
func DeleteDeviceMediaAllHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "removendo todos os materiais publicitários", "stage", "media")

		if err := client.DeleteAllMaterials(ctx); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao remover todos os materiais", err, "stage", "media")
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "todos os materiais removidos", "stage", "media")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "all_deleted",
			"device_id": deviceID,
		})
	})
}

// =============================================================================
// 2.6 — Stats: GET /stats
// =============================================================================

// deviceStatsResponse is the JSON response for GET /stats (device-level stats).
// US4-AC2: zero and "unavailable" are distinct — never return zeros on device offline.
// Named deviceStatsResponse to avoid conflict with admin_api_handlers.go statsResponse.
type deviceStatsResponse struct {
	Users  deviceStatsUsers  `json:"users"`
	Events deviceStatsEvents `json:"events"`
}

type deviceStatsUsers struct {
	Total int `json:"total"`
	Faces int `json:"faces"`
	Cards int `json:"cards"`
	Max   int `json:"max"`
}

type deviceStatsEvents struct {
	Total int `json:"total"`
	Max   int `json:"max"`
}

// GetDeviceStatsHandler serves GET /admin/api/devices/{id}/stats.
// Spec §FR-016 / tasks 2.6.1.
// Device offline → 504 or 502 via mapISAPIError; NEVER returns zeros by default.
func GetDeviceStatsHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		stats, err := client.GetDeviceStats(ctx)
		if err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao obter estatísticas do dispositivo", err, "stage", "stats")
			// US4-AC2: on error propagate the error, never fabricate zeros
			adminJSONError(w, st, msg)
			return
		}

		resp := deviceStatsResponse{
			Users: deviceStatsUsers{
				Total: stats.Users.Total,
				Faces: stats.Users.Faces,
				Cards: stats.Users.Cards,
				Max:   stats.Users.Max,
			},
			Events: deviceStatsEvents{
				Total: stats.Events.Total,
				Max:   stats.Events.Max,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})
}

// =============================================================================
// 2.7 — Face config: PUT /preferences/face-config, POST /preferences/face-capture
// =============================================================================

// putFaceConfigRequest is the request body for PUT /preferences/face-config.
type putFaceConfigRequest struct {
	MaxDistance float64 `json:"maxDistance"`
}

// PutDeviceFaceConfigHandler serves PUT /admin/api/devices/{id}/preferences/face-config.
// Spec §FR-017 / tasks 2.7.1.
func PutDeviceFaceConfigHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		var req putFaceConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			adminJSONError(w, http.StatusBadRequest, "body JSON inválido")
			return
		}
		if req.MaxDistance <= 0 {
			adminJSONError(w, http.StatusBadRequest, "maxDistance deve ser maior que zero")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "alterando configuração de face",
			"stage", "face-config", "maxDistance", req.MaxDistance)

		if err := client.SetFaceCompareCond(ctx, req.MaxDistance); err != nil {
			st, msg := mapISAPIError(err)
			cfg.logError("preferences", deviceID, "falha ao definir condição de comparação facial", err,
				"stage", "face-config")
			adminJSONError(w, st, msg)
			return
		}

		cfg.logInfo("preferences", deviceID, "configuração de face atualizada",
			"stage", "face-config", "maxDistance", req.MaxDistance)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"result":    "ok",
			"device_id": deviceID,
		})
	})
}

// PostDeviceFaceCaptureHandler serves POST /admin/api/devices/{id}/preferences/face-capture.
// Spec §FR-018 / tasks 2.7.2.
// Returns the captured image as base64 JSON (dec-011 — never exposes URL/IP internals).
// ErrSSRFHostMismatch from client → 502 with security-oriented message.
func PostDeviceFaceCaptureHandler(cfg DeviceConfigConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			adminJSONError(w, http.StatusMethodNotAllowed, "método não permitido")
			return
		}

		deviceID, ok := deviceIDFromPath(r.URL.Path)
		if !ok {
			adminJSONError(w, http.StatusBadRequest, "id de dispositivo inválido")
			return
		}

		ctx := r.Context()
		_, client, httpSt, errMsg := loadDeviceAndISAPIClient(ctx, cfg, deviceID)
		if httpSt != 0 {
			adminJSONError(w, httpSt, errMsg)
			return
		}

		cfg.logInfo("preferences", deviceID, "iniciando captura facial ao vivo", "stage", "face-capture")

		imgData, err := client.CaptureFaceData(ctx)
		if err != nil {
			cfg.logError("preferences", deviceID, "falha na captura facial", err, "stage", "face-capture")
			// SSRF mitigation: ErrSSRFHostMismatch → 502 with explicit security message
			if errors.Is(err, hikvision.ErrSSRFHostMismatch) {
				adminJSONError(w, http.StatusBadGateway,
					"captura bloqueada: URL de imagem do dispositivo aponta para host não autorizado (SSRF)")
				return
			}
			st, msg := mapISAPIError(err)
			adminJSONError(w, st, msg)
			return
		}

		// dec-011: encode as base64, never expose the internal URL or device IP
		encoded := base64.StdEncoding.EncodeToString(imgData)

		cfg.logInfo("preferences", deviceID, "captura facial concluída",
			"stage", "face-capture", "bytes", len(imgData))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"image":     encoded,
			"device_id": deviceID,
		})
	})
}
