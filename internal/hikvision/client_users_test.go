package hikvision_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestListUsers_Page1_Body verifies ListUsers(1, 100) sends searchResultPosition=0, maxResults=100.
// SOURCED: UserService.php:49-92.
func TestListUsers_Page1_Body(t *testing.T) {
	var capturedBody string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"UserInfoSearch":{"numOfMatches":0,"totalMatches":0,"UserInfo":[]}}`)) //nolint:errcheck
	})
	defer srv.Close()

	_, _, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ListUsers(context.Background(), 1, 100)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	// Verify the body contains searchResultPosition=0 and maxResults=100
	var body map[string]any
	if err := json.Unmarshal([]byte(capturedBody), &body); err != nil {
		t.Fatalf("body parse: %v (body: %s)", err, capturedBody)
	}
	cond, _ := body["UserInfoSearchCond"].(map[string]any)
	if cond == nil {
		t.Fatalf("UserInfoSearchCond missing in body: %s", capturedBody)
	}
	pos, _ := cond["searchResultPosition"].(float64)
	if pos != 0 {
		t.Errorf("searchResultPosition: got %v, want 0", pos)
	}
	max, _ := cond["maxResults"].(float64)
	if max != 100 {
		t.Errorf("maxResults: got %v, want 100", max)
	}
	searchID, _ := cond["searchID"].(string)
	if searchID == "" {
		t.Error("searchID should be non-empty")
	}
}

// TestListUsers_Page2_Position verifies ListUsers(2, 50) sends searchResultPosition=50.
func TestListUsers_Page2_Position(t *testing.T) {
	var capturedBody string
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		capturedBody = string(b[:n])
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"UserInfoSearch":{"numOfMatches":0,"totalMatches":100,"UserInfo":[]}}`)) //nolint:errcheck
	})
	defer srv.Close()

	_, _, err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ListUsers(context.Background(), 2, 50)
	if err != nil {
		t.Fatalf("ListUsers page 2: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(capturedBody), &body); err != nil {
		t.Fatalf("body parse: %v", err)
	}
	cond, _ := body["UserInfoSearchCond"].(map[string]any)
	pos, _ := cond["searchResultPosition"].(float64)
	if pos != 50 {
		t.Errorf("searchResultPosition: got %v, want 50 for page=2 perPage=50", pos)
	}
	max, _ := cond["maxResults"].(float64)
	if max != 50 {
		t.Errorf("maxResults: got %v, want 50", max)
	}
}

// TestClearUsers_SendsPUT verifies ClearUsers sends PUT to the correct endpoint.
// SOURCED: UserService.php:269-299.
func TestClearUsers_SendsPUT(t *testing.T) {
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "UserInfo/Clear") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer srv.Close()

	err := hikvision.NewWithHTTPClient(cfg, srv.Client()).ClearUsers(context.Background())
	if err != nil {
		t.Errorf("ClearUsers: expected nil, got %v", err)
	}
}

// TestListUsers_SearchIDDiffersPerCall verifies CHK072: searchID is non-deterministic.
func TestListUsers_SearchIDDiffersPerCall(t *testing.T) {
	searchIDs := make([]string, 0, 3)
	srv, cfg := makeISAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b := make([]byte, 4096)
		n, _ := r.Body.Read(b)
		if err := json.Unmarshal(b[:n], &body); err == nil {
			if cond, ok := body["UserInfoSearchCond"].(map[string]any); ok {
				if id, ok := cond["searchID"].(string); ok {
					searchIDs = append(searchIDs, id)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"UserInfoSearch":{"numOfMatches":0,"totalMatches":0,"UserInfo":[]}}`)) //nolint:errcheck
	})
	defer srv.Close()

	client := hikvision.NewWithHTTPClient(cfg, srv.Client())
	for i := 0; i < 3; i++ {
		_, _, err := client.ListUsers(context.Background(), 1, 10)
		if err != nil {
			t.Fatalf("ListUsers call %d: %v", i, err)
		}
	}

	if len(searchIDs) < 3 {
		t.Fatalf("captured only %d searchIDs", len(searchIDs))
	}
	seen := make(map[string]bool)
	for _, id := range searchIDs {
		if seen[id] {
			t.Errorf("duplicate searchID: %q", id)
		}
		seen[id] = true
	}
}
