package hikvision

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestListMaterials_ParsesXML verifies that ListMaterials parses the XML response (tasks 1.9.6).
func TestListMaterials_ParsesXML(t *testing.T) {
	payload := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<MaterialList>` +
		`<Material><id>mat-1</id><materialName>foto1.jpg</materialName></Material>` +
		`<Material><id>mat-2</id><materialName>foto2.jpg</materialName></Material>` +
		`</MaterialList>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISAPI/Publish/MaterialMgr/material" || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(200)
		w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	mats, err := c.ListMaterials(context.Background())
	if err != nil {
		t.Fatalf("ListMaterials: %v", err)
	}
	if len(mats) != 2 {
		t.Fatalf("expected 2 materials, got %d", len(mats))
	}
	if mats[0].ID != "mat-1" || mats[0].Name != "foto1.jpg" {
		t.Errorf("mats[0] = %+v", mats[0])
	}
	if mats[1].ID != "mat-2" {
		t.Errorf("mats[1].ID = %q", mats[1].ID)
	}
}

// TestListMaterials_Empty verifies empty list returns non-nil slice (tasks 1.9.6).
func TestListMaterials_Empty(t *testing.T) {
	payload := `<MaterialList></MaterialList>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(200)
		w.Write([]byte(payload)) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	mats, err := c.ListMaterials(context.Background())
	if err != nil {
		t.Fatalf("ListMaterials: %v", err)
	}
	if mats == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(mats) != 0 {
		t.Errorf("expected 0 materials, got %d", len(mats))
	}
}

// TestCreateAdvertisingMedia_StepCFailReturnsOrphan verifies that a failure at step (c)
// returns ErrAdvertisingMediaCreate with OrphanMaterialID set (tasks 1.9.6 / spec §FR-013).
// TestUploadMaterial_StepBFailReturnsOrphan verifica que, se o upload do binário (b)
// falha após criar o registro (a), o erro carrega o OrphanMaterialID.
func TestUploadMaterial_StepBFailReturnsOrphan(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		switch {
		case n == 1 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/MaterialMgr/material") && !strings.Contains(r.URL.Path, "/upload"):
			// Step (a): material create — success, return ID
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			w.Write([]byte(`<ResponseStatus><ID>orphan-mat-999</ID></ResponseStatus>`)) //nolint:errcheck
		case n == 2 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload"):
			// Step (b): upload — fail
			w.WriteHeader(400)
		default:
			t.Errorf("unexpected call %d: %s %s", n, r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.UploadMaterial(context.Background(), "test.jpg", []byte{0xFF, 0xD8})
	if err == nil {
		t.Fatal("expected error when step (b) fails")
	}

	var createErr *ErrAdvertisingMediaCreate
	if !errors.As(err, &createErr) {
		t.Fatalf("expected *ErrAdvertisingMediaCreate, got %T: %v", err, err)
	}
	if createErr.Step != "b" {
		t.Errorf("expected Step=b, got Step=%q", createErr.Step)
	}
	if createErr.OrphanMaterialID != "orphan-mat-999" {
		t.Errorf("expected OrphanMaterialID=orphan-mat-999, got %q", createErr.OrphanMaterialID)
	}
}

// TestUploadMaterial_Success verifica os 2 passos (a: criar registro, b: upload) e o
// id retornado — UploadMaterial NÃO toca em program/page/schedule.
func TestUploadMaterial_Success(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		w.Header().Set("Content-Type", "application/xml")
		switch n {
		case 1: // Step (a): POST /MaterialMgr/material — firmware responde ResponseStatus/ID
			w.WriteHeader(200)
			w.Write([]byte(`<ResponseStatus><ID>mat-42</ID></ResponseStatus>`)) //nolint:errcheck
		case 2: // Step (b): POST /material/mat-42/upload
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected call %d: %s %s (UploadMaterial deve fazer só 2 chamadas)", n, r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	materialID, err := c.UploadMaterial(context.Background(), "ad.jpg", []byte{0xFF, 0xD8})
	if err != nil {
		t.Fatalf("UploadMaterial: %v", err)
	}
	if materialID != "mat-42" {
		t.Errorf("materialID = %q, want mat-42", materialID)
	}
	if int(atomic.LoadInt32(&callCount)) != 2 {
		t.Errorf("expected 2 ISAPI calls, got %d", callCount)
	}
}

// TestDeleteAllMaterials_CallsDeleteForEach verifies that DeleteAllMaterials
// calls DeleteMaterial for each listed material (tasks 1.9.6).
func TestDeleteAllMaterials_CallsDeleteForEach(t *testing.T) {
	listPayload := `<MaterialList>` +
		`<Material><id>a1</id><materialName>a.jpg</materialName></Material>` +
		`<Material><id>b2</id><materialName>b.jpg</materialName></Material>` +
		`<Material><id>c3</id><materialName>c.jpg</materialName></Material>` +
		`</MaterialList>`

	var deletedIDs []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			w.Write([]byte(listPayload)) //nolint:errcheck
		case http.MethodDelete:
			// Extract ID from path: /ISAPI/Publish/MaterialMgr/material/{id}
			parts := strings.Split(r.URL.Path, "/")
			deletedIDs = append(deletedIDs, parts[len(parts)-1])
			w.WriteHeader(200)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteAllMaterials(context.Background()); err != nil {
		t.Fatalf("DeleteAllMaterials: %v", err)
	}
	if len(deletedIDs) != 3 {
		t.Errorf("expected 3 deletes, got %d: %v", len(deletedIDs), deletedIDs)
	}
	for _, want := range []string{"a1", "b2", "c3"} {
		found := false
		for _, got := range deletedIDs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("material %s was not deleted; deleted: %v", want, deletedIDs)
		}
	}
}

// TestDeleteMaterial_InUse_DeletesProgramThenRetries verifica que, quando o material
// está EM USO por um programa (delete direto → 400), o cliente apaga o programa que o
// referencia (via materialNo no PlayItem) e retenta o delete do material com sucesso.
func TestDeleteMaterial_InUse_DeletesProgramThenRetries(t *testing.T) {
	var matDeletes, progDeletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/MaterialMgr/material/13"):
			matDeletes++
			if matDeletes == 1 {
				w.WriteHeader(http.StatusBadRequest) // em uso
			} else {
				w.WriteHeader(http.StatusOK) // liberado após apagar o programa
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/ProgramMgr/program"):
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<ProgramList><Program><id>1</id><PageList><Page><WindowsList><Windows>` + //nolint:errcheck
				`<PlayItemList><PlayItem><materialNo>13</materialNo></PlayItem></PlayItemList>` +
				`</Windows></WindowsList></Page></PageList></Program>` +
				`<Program><id>9</id><PageList><Page><WindowsList><Windows>` +
				`<PlayItemList><PlayItem><materialNo>99</materialNo></PlayItem></PlayItemList>` +
				`</Windows></WindowsList></Page></PageList></Program></ProgramList>`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/ProgramMgr/program/1"):
			progDeletes++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.DeleteMaterial(context.Background(), "13"); err != nil {
		t.Fatalf("DeleteMaterial: %v", err)
	}
	if matDeletes != 2 {
		t.Errorf("deletes de material = %d, want 2 (1ª 400 + retry)", matDeletes)
	}
	if progDeletes != 1 {
		t.Errorf("deletes de programa = %d, want 1 (só o programa 1 referencia o material 13)", progDeletes)
	}
}
