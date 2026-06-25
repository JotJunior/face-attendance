package hikvision

import (
	"context"
	"encoding/xml"
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
func TestCreateAdvertisingMedia_StepCFailReturnsOrphan(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		switch {
		case n == 1 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/MaterialMgr/material"):
			// Step (a): material create — success, return ID
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(200)
			xml.NewEncoder(w).Encode(struct { //nolint:errcheck
				XMLName xml.Name `xml:"Material"`
				ID      string   `xml:"id"`
			}{ID: "orphan-mat-999"})
		case n == 2 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/upload"):
			// Step (b): upload — success
			w.WriteHeader(200)
		case n == 3 && r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/ProgramMgr/program"):
			// Step (c): program create — fail
			w.WriteHeader(500)
		default:
			t.Errorf("unexpected call %d: %s %s", n, r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.CreateAdvertisingMedia(context.Background(), "test.jpg", []byte{0xFF, 0xD8})
	if err == nil {
		t.Fatal("expected error when step (c) fails")
	}

	var createErr *ErrAdvertisingMediaCreate
	if !errors.As(err, &createErr) {
		t.Fatalf("expected *ErrAdvertisingMediaCreate, got %T: %v", err, err)
	}
	if createErr.Step != "c" {
		t.Errorf("expected Step=c, got Step=%q", createErr.Step)
	}
	if createErr.OrphanMaterialID != "orphan-mat-999" {
		t.Errorf("expected OrphanMaterialID=orphan-mat-999, got %q", createErr.OrphanMaterialID)
	}
}

// TestCreateAdvertisingMedia_Success verifies all 5 steps are called and IDs are returned.
func TestCreateAdvertisingMedia_Success(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&callCount, 1))
		w.Header().Set("Content-Type", "application/xml")
		switch n {
		case 1: // Step (a): POST /MaterialMgr/material
			w.WriteHeader(200)
			w.Write([]byte(`<Material><id>mat-42</id></Material>`)) //nolint:errcheck
		case 2: // Step (b): POST /material/mat-42/upload
			w.WriteHeader(200)
		case 3: // Step (c): POST /ProgramMgr/program
			w.WriteHeader(200)
			w.Write([]byte(`<Program><id>prog-7</id></Program>`)) //nolint:errcheck
		case 4: // Step (d): PUT /program/1/page/1
			w.WriteHeader(204)
		case 5: // Step (e): PUT /playSchedule/1
			w.WriteHeader(200)
			w.Write([]byte(`<PlaySchedule><id>sched-1</id></PlaySchedule>`)) //nolint:errcheck
		default:
			t.Errorf("unexpected call %d: %s %s", n, r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	res, err := c.CreateAdvertisingMedia(context.Background(), "ad.jpg", []byte{0xFF, 0xD8})
	if err != nil {
		t.Fatalf("CreateAdvertisingMedia: %v", err)
	}
	if res.MaterialID != "mat-42" {
		t.Errorf("MaterialID = %q", res.MaterialID)
	}
	if res.ProgramID != "prog-7" {
		t.Errorf("ProgramID = %q", res.ProgramID)
	}
	if res.ScheduleID != "sched-1" {
		t.Errorf("ScheduleID = %q", res.ScheduleID)
	}
	if int(atomic.LoadInt32(&callCount)) != 5 {
		t.Errorf("expected 5 ISAPI calls, got %d", callCount)
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
