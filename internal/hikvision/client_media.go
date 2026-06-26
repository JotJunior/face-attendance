package hikvision

// client_media.go implements ISAPI advertising media management operations.
//
// ListMaterials — SOURCED from legacy/hik2go/src/Hik2go/Preferences/Media.php:list() (httpGet):
//   GET /ISAPI/Publish/MaterialMgr/material
//
// CreateAdvertisingMedia (5 etapas) — SOURCED from Media.php:create() + upload() + Presentation.php:
//   (a) POST /ISAPI/Publish/MaterialMgr/material         (XML <Material>)
//   (b) POST /ISAPI/Publish/MaterialMgr/material/{ID}/upload  (multipart)
//   (c) POST /ISAPI/Publish/ProgramMgr/program            (XML <Program>)
//   (d) PUT  /ISAPI/Publish/ProgramMgr/program/1/page/1   (XML <Page>)
//   (e) PUT  /ISAPI/Publish/ScheduleMgr/playSchedule/1    (XML <PlaySchedule>)
//
// DeleteMaterial — SOURCED from Media.php:delete():
//   DELETE /ISAPI/Publish/MaterialMgr/material/{id}
//
// DeleteAllMaterials — SOURCED from Media.php:clear():
//   Lists then removes each via DeleteMaterial.

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const (
	// endpointMaterialMgr is the base for material CRUD operations.
	// SOURCED: Media.php:list() and create().
	endpointMaterialMgr = "/ISAPI/Publish/MaterialMgr/material"

	// endpointProgramMgr is the base for program creation.
	// SOURCED: Presentation.php:create().
	endpointProgramMgr = "/ISAPI/Publish/ProgramMgr/program"

	// endpointProgramPage is the page update endpoint (program 1, page 1).
	// SOURCED: Presentation.php:page().
	endpointProgramPage = "/ISAPI/Publish/ProgramMgr/program/1/page/1"

	// endpointPlaySchedule is the schedule update endpoint (schedule 1).
	// SOURCED: Presentation.php:schedule().
	endpointPlaySchedule = "/ISAPI/Publish/ScheduleMgr/playSchedule/1"
)

// Material represents a single advertising media material on the device.
// SOURCED: Media.php list() — ID and Name are the two identifying fields returned.
type Material struct {
	ID   string `xml:"id"`
	Name string `xml:"materialName"`
}

// materialListResponse is the XML envelope for GET /ISAPI/Publish/MaterialMgr/material.
type materialListResponse struct {
	XMLName   xml.Name   `xml:"MaterialList"`
	Materials []Material `xml:"Material"`
}

// materialCreateResponse is the XML envelope returned after POST /material.
// The device assigns an ID; we extract it for subsequent steps.
type materialCreateResponse struct {
	XMLName xml.Name `xml:"Material"`
	ID      string   `xml:"id"`
}

// programCreateResponse is the XML envelope returned after POST /program.
type programCreateResponse struct {
	XMLName xml.Name `xml:"Program"`
	ID      string   `xml:"id"`
}

// scheduleUpdateResponse is the XML envelope returned after PUT /playSchedule/1.
type scheduleUpdateResponse struct {
	XMLName xml.Name `xml:"PlaySchedule"`
	ID      string   `xml:"id"`
}

// AdvertisingMediaResult holds the IDs created during CreateAdvertisingMedia.
// Spec §FR-013 — all three IDs are returned for lifecycle management.
type AdvertisingMediaResult struct {
	MaterialID string
	ProgramID  string
	ScheduleID string
}

// ErrAdvertisingMediaCreate is returned when CreateAdvertisingMedia fails at a specific step.
// OrphanMaterialID is set when the material was created (step a) but a later step failed
// (Clarification 4 / spec §FR-013 — lets the operator clean up via DeleteMaterial).
type ErrAdvertisingMediaCreate struct {
	Step             string // "a", "b", "c", "d", or "e"
	Cause            error
	OrphanMaterialID string // non-empty when material was successfully created
}

func (e *ErrAdvertisingMediaCreate) Error() string {
	if e.OrphanMaterialID != "" {
		return fmt.Sprintf("hikvision: CreateAdvertisingMedia step %s: %v (orphan material id=%s — use DeleteMaterial to clean up)",
			e.Step, e.Cause, e.OrphanMaterialID)
	}
	return fmt.Sprintf("hikvision: CreateAdvertisingMedia step %s: %v", e.Step, e.Cause)
}

func (e *ErrAdvertisingMediaCreate) Unwrap() error { return e.Cause }

// ListMaterials returns all advertising media materials on the device.
// SOURCED: Media.php:list() — GET /ISAPI/Publish/MaterialMgr/material.
// Returns an empty (non-nil) slice when no materials exist.
func (c *Client) ListMaterials(ctx context.Context) ([]Material, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointMaterialMgr, nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: ListMaterials: %w", err)
	}
	if status == 401 {
		return nil, &NonRetriableError{Op: "ListMaterials", Status: status}
	}
	if status != 200 {
		return nil, retriableOrNot("ListMaterials", status, body)
	}

	var resp materialListResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hikvision: ListMaterials parse: %w", err)
	}

	mats := resp.Materials
	if mats == nil {
		mats = []Material{}
	}
	return mats, nil
}

// CreateAdvertisingMedia creates a new advertising media item on the device.
// SOURCED: Media.php:create()+upload() and Presentation.php:create()+page()+schedule().
//
// Executes 5 sequential steps (spec §FR-013):
//
//	(a) POST Material XML — creates the material record
//	(b) POST multipart upload — uploads the binary image
//	(c) POST Program XML — creates the display program
//	(d) PUT Page XML — sets the page layout referencing the material
//	(e) PUT PlaySchedule XML — activates the schedule
//
// On failure at any step, returns *ErrAdvertisingMediaCreate with the step name and
// OrphanMaterialID set if step (a) succeeded (spec §FR-013 Clarification 4).
func (c *Client) CreateAdvertisingMedia(ctx context.Context, filename string, data []byte) (*AdvertisingMediaResult, error) {
	now := time.Now().Format("2006-01-02 15:04:05")

	// Step (a): Create material record
	// SOURCED: Media.php:create() — POST /material with XML <Material>
	materialXML := fmt.Sprintf(`<Material>`+
		`<id>0</id>`+
		`<materialName>%s</materialName>`+
		`<materialRemarks></materialRemarks>`+
		`<materialType>static</materialType>`+
		`<approveState>notApprove</approveState>`+
		`<approveRemarks></approveRemarks>`+
		`<shareProperty>private</shareProperty>`+
		`<uploadUser>admin</uploadUser>`+
		`<uploadTime>%s</uploadTime>`+
		`<orgNo>undefined</orgNo>`+
		`<replaceTerminal></replaceTerminal>`+
		`<StaticMaterial>`+
		`<staticMaterialType>picture</staticMaterialType>`+
		`<picFormat>jpeg</picFormat>`+
		`<fileSize>%d</fileSize>`+
		`</StaticMaterial>`+
		`</Material>`,
		xmlEscape(filename), now, len(data))

	bodyA, statusA, err := c.doRequest(ctx, http.MethodPost, endpointMaterialMgr,
		strings.NewReader(materialXML), "application/xml")
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "a", Cause: err}
	}
	if statusA != 200 && statusA != 201 {
		return nil, &ErrAdvertisingMediaCreate{Step: "a",
			Cause: retriableOrNot("CreateAdvertisingMedia step a POST", statusA, bodyA)}
	}

	var matResp materialCreateResponse
	if err := xml.Unmarshal(bodyA, &matResp); err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "a",
			Cause: fmt.Errorf("parse Material response: %w", err)}
	}
	materialID := matResp.ID
	if materialID == "" {
		return nil, &ErrAdvertisingMediaCreate{Step: "a",
			Cause: errors.New("device returned empty material ID")}
	}

	// Step (b): Upload binary image via multipart
	// SOURCED: Media.php:upload() — POST /material/{ID}/upload with fields: name, type, size + file
	uploadURL := fmt.Sprintf("%s/%s/upload", endpointMaterialMgr, materialID)

	var uploadBuf bytes.Buffer
	mw := multipart.NewWriter(&uploadBuf)

	for _, field := range []struct{ name, value string }{
		{"name", filename},
		{"type", "image"},
		{"size", fmt.Sprintf("%d", len(data))},
	} {
		if err := mw.WriteField(field.name, field.value); err != nil {
			return nil, &ErrAdvertisingMediaCreate{Step: "b",
				Cause: fmt.Errorf("write field %s: %w", field.name, err),
				OrphanMaterialID: materialID}
		}
	}

	// Binary file part — SOURCED: Media.php:upload() files field "file"
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	fileHeader.Set("Content-Type", "image/jpeg")
	filePart, err := mw.CreatePart(fileHeader)
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "b",
			Cause: fmt.Errorf("create file part: %w", err),
			OrphanMaterialID: materialID}
	}
	if _, err := filePart.Write(data); err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "b",
			Cause: fmt.Errorf("write file data: %w", err),
			OrphanMaterialID: materialID}
	}
	if err := mw.Close(); err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "b",
			Cause: fmt.Errorf("close multipart: %w", err),
			OrphanMaterialID: materialID}
	}

	_, statusB, err := c.doRequest(ctx, http.MethodPost, uploadURL,
		&uploadBuf, mw.FormDataContentType())
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "b", Cause: err, OrphanMaterialID: materialID}
	}
	if statusB != 200 && statusB != 201 {
		return nil, &ErrAdvertisingMediaCreate{Step: "b",
			Cause:            retriableOrNot("CreateAdvertisingMedia step b POST", statusB, nil),
			OrphanMaterialID: materialID}
	}

	// Step (c): Create program
	// SOURCED: Presentation.php:create() — POST /program with XML <Program version="2.0">
	programXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<Program version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
		`<id>1</id>`+
		`<programName>%s</programName>`+
		`<programRemarks/>`+
		`<programType>normal</programType>`+
		`<Resolution><imageWidth>580</imageWidth><imageHeight>884</imageHeight></Resolution>`+
		`<PageList><Page><id>1</id>`+
		`<PageBasicInfo><pageName/><switchDuration>1</switchDuration><switchEffect>none</switchEffect></PageBasicInfo>`+
		`</Page></PageList>`+
		`</Program>`,
		xmlEscape(filename))

	bodyC, statusC, err := c.doRequest(ctx, http.MethodPost, endpointProgramMgr,
		strings.NewReader(programXML), "application/xml")
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "c", Cause: err, OrphanMaterialID: materialID}
	}
	if statusC != 200 && statusC != 201 {
		return nil, &ErrAdvertisingMediaCreate{Step: "c",
			Cause:            retriableOrNot("CreateAdvertisingMedia step c POST", statusC, bodyC),
			OrphanMaterialID: materialID}
	}

	var progResp programCreateResponse
	if err := xml.Unmarshal(bodyC, &progResp); err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "c",
			Cause:            fmt.Errorf("parse Program response: %w", err),
			OrphanMaterialID: materialID}
	}
	programID := progResp.ID

	// Step (d): Set page layout referencing the material by ID
	// SOURCED: Presentation.php:page() — PUT /program/1/page/1 with XML <Page version="2.0">
	pageXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<Page version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
		`<id>1</id>`+
		`<programName>%s</programName>`+
		`<programType>normal</programType>`+
		`<PageBasicInfo>`+
		`<pageName>1</pageName>`+
		`<BackgroundColor><RGB>16777215</RGB></BackgroundColor>`+
		`<backgroundPic>%s</backgroundPic>`+
		`<playDurationMode>1</playDurationMode>`+
		`<switchDuration>1</switchDuration>`+
		`<switchEffect>none</switchEffect>`+
		`</PageBasicInfo>`+
		`<WindowsList/>`+
		`</Page>`,
		xmlEscape(filename), xmlEscape(materialID))

	_, statusD, err := c.doRequest(ctx, http.MethodPut, endpointProgramPage,
		strings.NewReader(pageXML), "application/xml")
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "d", Cause: err, OrphanMaterialID: materialID}
	}
	if statusD != 200 && statusD != 204 {
		return nil, &ErrAdvertisingMediaCreate{Step: "d",
			Cause:            retriableOrNot("CreateAdvertisingMedia step d PUT", statusD, nil),
			OrphanMaterialID: materialID}
	}

	// Step (e): Activate the play schedule
	// SOURCED: Presentation.php:schedule() — PUT /playSchedule/1 with XML <PlaySchedule version="2.0">
	scheduleXML := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<PlaySchedule version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">` +
		`<id>1</id>` +
		`<scheduleName>web</scheduleName>` +
		`<scheduleMode>screensaver</scheduleMode>` +
		`<scheduleType>daily</scheduleType>` +
		`<DailySchedule><PlaySpanList><PlaySpan>` +
		`<id>1</id><programNo>1</programNo>` +
		`<TimeRange><beginTime>00:00:00</beginTime><endTime>24:00:00</endTime></TimeRange>` +
		`</PlaySpan></PlaySpanList></DailySchedule>` +
		`</PlaySchedule>`

	bodyE, statusE, err := c.doRequest(ctx, http.MethodPut, endpointPlaySchedule,
		strings.NewReader(scheduleXML), "application/xml")
	if err != nil {
		return nil, &ErrAdvertisingMediaCreate{Step: "e", Cause: err, OrphanMaterialID: materialID}
	}
	if statusE != 200 && statusE != 204 {
		return nil, &ErrAdvertisingMediaCreate{Step: "e",
			Cause:            retriableOrNot("CreateAdvertisingMedia step e PUT", statusE, bodyE),
			OrphanMaterialID: materialID}
	}

	var schedResp scheduleUpdateResponse
	scheduleID := "1" // default when response body is empty (204)
	if len(bodyE) > 0 {
		if err := xml.Unmarshal(bodyE, &schedResp); err == nil && schedResp.ID != "" {
			scheduleID = schedResp.ID
		}
	}
	if programID == "" {
		programID = "1" // default per Presentation.php — program id always starts at 1
	}

	return &AdvertisingMediaResult{
		MaterialID: materialID,
		ProgramID:  programID,
		ScheduleID: scheduleID,
	}, nil
}

// DeleteMaterial removes a material by its ID.
// SOURCED: Media.php:delete() — DELETE /ISAPI/Publish/MaterialMgr/material/{id}.
func (c *Client) DeleteMaterial(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("%s/%s", endpointMaterialMgr, id)
	_, status, err := c.doRequest(ctx, http.MethodDelete, endpoint, nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: DeleteMaterial %s: %w", id, err)
	}
	if status == 200 || status == 204 || status == 404 {
		return nil // 404 = already gone — idempotent
	}
	return retriableOrNot("DeleteMaterial DELETE "+id, status, nil)
}

// DeleteAllMaterials lists all materials and removes each one.
// SOURCED: Media.php:clear() — iterates list() then calls delete() for each.
// Individual delete failures do not block remaining deletions; any errors are
// aggregated and returned at the end.
func (c *Client) DeleteAllMaterials(ctx context.Context) error {
	mats, err := c.ListMaterials(ctx)
	if err != nil {
		return fmt.Errorf("hikvision: DeleteAllMaterials list: %w", err)
	}

	var errs []error
	for _, m := range mats {
		if delErr := c.DeleteMaterial(ctx, m.ID); delErr != nil {
			errs = append(errs, fmt.Errorf("material %s: %w", m.ID, delErr))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("hikvision: DeleteAllMaterials: %d error(s): %v", len(errs), errs)
	}
	return nil
}
