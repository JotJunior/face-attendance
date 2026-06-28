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
	// Usado por client_presentation.go:SetPresentationPage (troca a imagem exibida).
	endpointProgramPage = "/ISAPI/Publish/ProgramMgr/program/1/page/1"
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
// Forma REAL verificada no device DS-K1T673*: o firmware responde
// <ResponseStatus>…<ID>N</ID></ResponseStatus> (statusCode 1 = OK), NÃO um
// elemento <Material> com <id>. A suposição anterior (<Material>/<id>) fazia o
// xml.Unmarshal falhar ("expected <Material> but have <ResponseStatus>").
type materialCreateResponse struct {
	XMLName xml.Name `xml:"ResponseStatus"`
	ID      string   `xml:"ID"`
}

// ErrAdvertisingMediaCreate is returned when UploadMaterial fails at a specific step.
// OrphanMaterialID is set when the material record was created (step a) but the binary
// upload (step b) failed — lets the operator clean up via DeleteMaterial.
type ErrAdvertisingMediaCreate struct {
	Step             string // "a" (create record) or "b" (upload binary)
	Cause            error
	OrphanMaterialID string // non-empty when material was successfully created
}

func (e *ErrAdvertisingMediaCreate) Error() string {
	if e.OrphanMaterialID != "" {
		return fmt.Sprintf("hikvision: UploadMaterial step %s: %v (orphan material id=%s — use DeleteMaterial to clean up)",
			e.Step, e.Cause, e.OrphanMaterialID)
	}
	return fmt.Sprintf("hikvision: UploadMaterial step %s: %v", e.Step, e.Cause)
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

// UploadMaterial cria o registro do material e faz upload do binário (2 passos).
// SOURCED: Media.php:create()+upload(). Retorna o id do material.
//
//	(a) POST Material XML — cria o registro do material
//	(b) POST multipart upload — envia a imagem binária
//
// NÃO mexe em program/page/schedule: aplicar a imagem como presentation é separado
// (client_presentation.go:SetPresentationPage), pois o programa já existe no device e
// recriá-lo dá HTTP 400. Em falha após (a), o erro carrega OrphanMaterialID.
func (c *Client) UploadMaterial(ctx context.Context, filename string, data []byte) (string, error) {
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
		return "", &ErrAdvertisingMediaCreate{Step: "a", Cause: err}
	}
	if statusA != 200 && statusA != 201 {
		return "", &ErrAdvertisingMediaCreate{Step: "a",
			Cause: retriableOrNot("UploadMaterial step a POST", statusA, bodyA)}
	}

	var matResp materialCreateResponse
	if err := xml.Unmarshal(bodyA, &matResp); err != nil {
		return "", &ErrAdvertisingMediaCreate{Step: "a",
			Cause: fmt.Errorf("parse Material response: %w", err)}
	}
	materialID := matResp.ID
	if materialID == "" {
		return "", &ErrAdvertisingMediaCreate{Step: "a",
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
			return "", &ErrAdvertisingMediaCreate{Step: "b",
				Cause:            fmt.Errorf("write field %s: %w", field.name, err),
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
		return "", &ErrAdvertisingMediaCreate{Step: "b",
			Cause:            fmt.Errorf("create file part: %w", err),
			OrphanMaterialID: materialID}
	}
	if _, err := filePart.Write(data); err != nil {
		return "", &ErrAdvertisingMediaCreate{Step: "b",
			Cause:            fmt.Errorf("write file data: %w", err),
			OrphanMaterialID: materialID}
	}
	if err := mw.Close(); err != nil {
		return "", &ErrAdvertisingMediaCreate{Step: "b",
			Cause:            fmt.Errorf("close multipart: %w", err),
			OrphanMaterialID: materialID}
	}

	_, statusB, err := c.doRequest(ctx, http.MethodPost, uploadURL,
		&uploadBuf, mw.FormDataContentType())
	if err != nil {
		return "", &ErrAdvertisingMediaCreate{Step: "b", Cause: err, OrphanMaterialID: materialID}
	}
	if statusB != 200 && statusB != 201 {
		return "", &ErrAdvertisingMediaCreate{Step: "b",
			Cause:            retriableOrNot("UploadMaterial step b POST", statusB, nil),
			OrphanMaterialID: materialID}
	}

	return materialID, nil
}

// DeleteMaterial removes a material by its ID.
// SOURCED: Media.php:delete() — DELETE /ISAPI/Publish/MaterialMgr/material/{id}.
func (c *Client) DeleteMaterial(ctx context.Context, id string) error {
	err := c.deleteMaterialDirect(ctx, id)
	if err == nil {
		return nil
	}
	// O firmware recusa (HTTP 400) deletar um material EM USO por um programa de
	// propaganda (o backgroundPic de uma página o referencia). Solução: apagar o(s)
	// programa(s) que o referenciam — o que libera o material — e retentar uma vez.
	var nre *NonRetriableError
	if !errors.As(err, &nre) || nre.Status != 400 {
		return err
	}
	progs, lerr := c.programsReferencing(ctx, id)
	if lerr != nil || len(progs) == 0 {
		return err // não há programa a liberar (ou falha ao listar) → erro original
	}
	for _, pid := range progs {
		if derr := c.deleteProgram(ctx, pid); derr != nil {
			return fmt.Errorf("hikvision: DeleteMaterial %s: liberar programa %s: %w", id, pid, derr)
		}
	}
	return c.deleteMaterialDirect(ctx, id)
}

// deleteMaterialDirect faz o DELETE cru do material (sem tratar dependência de programa).
func (c *Client) deleteMaterialDirect(ctx context.Context, id string) error {
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

// programsReferencing retorna os ids dos programas cuja página referencia o material
// dado. Estrutura REAL verificada no device DS-K1T673*: a imagem exibida é o
// ProgramList>Program>PageList>Page>WindowsList>Windows>PlayItemList>PlayItem>materialNo
// (NÃO <backgroundPic> — esse campo não existe neste firmware).
func (c *Client) programsReferencing(ctx context.Context, materialID string) ([]string, error) {
	body, status, err := c.doRequest(ctx, http.MethodGet, endpointProgramMgr, nil, "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, retriableOrNot("programsReferencing GET", status, body)
	}
	var list struct {
		Programs []struct {
			ID          string   `xml:"id"`
			MaterialNos []string `xml:"PageList>Page>WindowsList>Windows>PlayItemList>PlayItem>materialNo"`
		} `xml:"Program"`
	}
	if err := xml.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("hikvision: programsReferencing parse: %w", err)
	}
	var ids []string
	for _, p := range list.Programs {
		for _, mn := range p.MaterialNos {
			if mn == materialID {
				ids = append(ids, p.ID)
				break
			}
		}
	}
	return ids, nil
}

// deleteProgram remove um programa de propaganda (libera os materiais que ele usa).
// 404 é idempotente (já removido).
func (c *Client) deleteProgram(ctx context.Context, programID string) error {
	endpoint := fmt.Sprintf("%s/%s", endpointProgramMgr, programID)
	_, status, err := c.doRequest(ctx, http.MethodDelete, endpoint, nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: deleteProgram %s: %w", programID, err)
	}
	if status == 200 || status == 204 || status == 404 {
		return nil
	}
	return retriableOrNot("deleteProgram DELETE "+programID, status, nil)
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
