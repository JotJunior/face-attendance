package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// TestMemberJSONTags verifies that Member fields serialize with the expected snake_case JSON keys
// (boundary convention from contracts/gob-api.md).
func TestMemberJSONTags(t *testing.T) {
	now := time.Now()
	selfie := "https://example.com/avatar.jpg"
	mobile := "(27) 99999-9999"
	m := domain.Member{
		ID:              1,
		GobID:           42,
		FederalDocument: "12345678901",
		Name:            "Test User",
		Status:          "REGULAR",
		MobileNumber:    &mobile,
		URLSelfie:       &selfie,
		GobCreatedAt:    &now,
		GobUpdatedAt:    &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal(Member) failed: %v", err)
	}

	s := string(b)
	requiredKeys := []string{
		`"id"`, `"gob_id"`, `"federal_document"`, `"name"`, `"status"`,
		`"mobile_number"`, `"url_selfie"`, `"gob_created_at"`, `"gob_updated_at"`,
		`"created_at"`, `"updated_at"`,
	}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("Member JSON missing key %s; got: %s", key, s)
		}
	}
}

// TestDeviceJSONTags verifies Device JSON field names.
func TestDeviceJSONTags(t *testing.T) {
	now := time.Now()
	ip := "192.168.1.10"
	mac := "AA:BB:CC:DD:EE:FF"
	d := domain.Device{
		ID:                1,
		DeviceIdentifier:  "AA:BB:CC:DD:EE:FF",
		IPAddress:         &ip,
		MACAddress:        &mac,
		LastHeartbeatAt:   &now,
		IsActive:          true,
		WebhookConfigured: false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("json.Marshal(Device) failed: %v", err)
	}

	s := string(b)
	requiredKeys := []string{
		`"id"`, `"device_identifier"`, `"ip_address"`, `"mac_address"`,
		`"last_heartbeat_at"`, `"is_active"`, `"webhook_configured"`,
		`"created_at"`, `"updated_at"`,
	}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("Device JSON missing key %s; got: %s", key, s)
		}
	}
}

// TestAttendanceEventJSONTags verifies AttendanceEvent JSON field names.
func TestAttendanceEventJSONTags(t *testing.T) {
	now := time.Now()
	status := "authorized"
	e := domain.AttendanceEvent{
		ID:               1,
		EventKey:         "abc123",
		EmployeeNoString: "12345678901",
		AttendanceStatus: &status,
		Marked:           false,
		RawPayload:       []byte(`{}`),
		CreatedAt:        now,
	}

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("json.Marshal(AttendanceEvent) failed: %v", err)
	}

	s := string(b)
	requiredKeys := []string{
		`"id"`, `"event_key"`, `"employee_no_string"`, `"attendance_status"`,
		`"marked"`, `"raw_payload"`, `"created_at"`,
	}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("AttendanceEvent JSON missing key %s; got: %s", key, s)
		}
	}
}

// TestProcessingOutcomeJSONTags verifies ProcessingOutcome JSON field names.
func TestProcessingOutcomeJSONTags(t *testing.T) {
	now := time.Now()
	stage := "done"
	o := domain.ProcessingOutcome{
		ID:              1,
		FederalDocument: "12345678901",
		DeviceID:        2,
		UserSynced:      true,
		FaceUploaded:    true,
		WebhookSet:      true,
		LastStage:       &stage,
		Attempts:        1,
		UpdatedAt:       now,
	}

	b, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("json.Marshal(ProcessingOutcome) failed: %v", err)
	}

	s := string(b)
	requiredKeys := []string{
		`"id"`, `"federal_document"`, `"device_id"`, `"user_synced"`,
		`"face_uploaded"`, `"webhook_set"`, `"last_stage"`, `"attempts"`, `"updated_at"`,
	}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("ProcessingOutcome JSON missing key %s; got: %s", key, s)
		}
	}
}

// TestProcessingMessageJSONTags verifies ProcessingMessage uses camelCase keys (outbound RabbitMQ boundary).
func TestProcessingMessageJSONTags(t *testing.T) {
	msg := domain.ProcessingMessage{
		FederalDocument: "12345678901",
		Name:            "Test",
		URLSelfie:       "https://example.com/img.jpg",
		GobID:           99,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal(ProcessingMessage) failed: %v", err)
	}

	s := string(b)
	requiredKeys := []string{`"federalDocument"`, `"name"`, `"urlSelfie"`, `"gobId"`}
	for _, key := range requiredKeys {
		if !strings.Contains(s, key) {
			t.Errorf("ProcessingMessage JSON missing key %s; got: %s", key, s)
		}
	}

	// Ensure snake_case variants are NOT present (drift guard)
	forbiddenKeys := []string{`"federal_document"`, `"url_selfie"`, `"gob_id"`}
	for _, key := range forbiddenKeys {
		if strings.Contains(s, key) {
			t.Errorf("ProcessingMessage JSON must not contain snake_case key %s; got: %s", key, s)
		}
	}
}
