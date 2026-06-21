package hikvision_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestCommandToISAPICmd_AllFiveValues verifies the command map covers all 5 ISAPI commands.
// SOURCED: DoorService.php CMD_* constants (L38-46).
func TestControlDoor_RejectsUnknownCommand(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ControlDoor(context.Background(), 1, "blast")
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
	if !strings.Contains(err.Error(), "blast") {
		t.Errorf("error should mention the invalid command: %v", err)
	}
}

// TestControlDoor_SendsXMLForOpen verifies ControlDoor sends <cmd>open</cmd> for "open".
// SOURCED: DoorService.php:sendCommand (L307-311).
func TestControlDoor_SendsXMLForOpen(t *testing.T) {
	var capturedBody string
	var capturedPath string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		b := make([]byte, 1024)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ControlDoor(context.Background(), 1, "open")
	if err != nil {
		t.Fatalf("ControlDoor open: %v", err)
	}
	if !strings.Contains(capturedPath, "/RemoteControl/door/1") {
		t.Errorf("path: got %q, want .../door/1", capturedPath)
	}
	if !strings.Contains(capturedBody, "<cmd>open</cmd>") {
		t.Errorf("body: got %q, want <cmd>open</cmd>", capturedBody)
	}
}

// TestControlDoor_AlwaysOpen_SendsAlwaysOpen verifies the "always_open" → "alwaysOpen" mapping.
func TestControlDoor_AlwaysOpen_SendsAlwaysOpen(t *testing.T) {
	var capturedBody string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 1024)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ControlDoor(context.Background(), 2, "always_open")
	if err != nil {
		t.Fatalf("ControlDoor always_open: %v", err)
	}
	if !strings.Contains(capturedBody, "<cmd>alwaysOpen</cmd>") {
		t.Errorf("body: got %q, want <cmd>alwaysOpen</cmd>", capturedBody)
	}
}

// TestGetDoorCapabilities_ParsesJSON verifies GetDoorCapabilities parses a JSON response.
// SOURCED: DoorService.php:56-97.
func TestGetDoorCapabilities_ParsesJSON(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "Door/capabilities") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"DoorList":{"DoorNo":[{"doorNo":1,"doorName":"Main Door"}]}}`)) //nolint:errcheck
	})
	defer srv.Close()

	doors, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).GetDoorCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetDoorCapabilities: %v", err)
	}
	if len(doors) != 1 {
		t.Fatalf("expected 1 door, got %d", len(doors))
	}
	if doors[0].DoorNo != 1 || doors[0].DoorName != "Main Door" {
		t.Errorf("door: got %+v", doors[0])
	}
}
