//go:build integration
// +build integration

// Integration tests for the repository layer.
// Require a running PostgreSQL instance (run via docker-compose).
// Run with: go test -tags integration ./internal/repository/...
package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/repository"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://presenca:presenca_dev@localhost:5432/presenca_facial?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func cleanup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"TRUNCATE member_processing_status, attendance_events, members, devices RESTART IDENTITY CASCADE")
	if err != nil {
		t.Logf("cleanup warning: %v", err)
	}
}

// ---------- Members ----------

func TestMemberRepository_Upsert(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	repo := repository.NewMemberRepository(pool)

	selfie := "https://example.com/avatar.jpg"
	m := domain.Member{
		GobID:           100,
		FederalDocument: "12345678901",
		Name:            "Test User",
		Status:          "REGULAR",
		URLSelfie:       &selfie,
	}

	// Insert
	if err := repo.Upsert(context.Background(), m); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	// Update (same federal_document, different name)
	m2 := m
	m2.Name = "Updated Name"
	if err := repo.Upsert(context.Background(), m2); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	// Verify only one row
	found, err := repo.FindByCPF(context.Background(), "12345678901")
	if err != nil {
		t.Fatalf("FindByCPF: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find member")
	}
	if found.Name != "Updated Name" {
		t.Errorf("name = %q, want 'Updated Name'", found.Name)
	}
}

func TestMemberRepository_ListWithSelfie(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	repo := repository.NewMemberRepository(pool)

	selfie := "https://example.com/a.jpg"
	// Member with selfie
	repo.Upsert(context.Background(), domain.Member{ //nolint:errcheck
		GobID: 1, FederalDocument: "11111111111", Name: "A", Status: "REGULAR", URLSelfie: &selfie,
	})
	// Member without selfie
	repo.Upsert(context.Background(), domain.Member{ //nolint:errcheck
		GobID: 2, FederalDocument: "22222222222", Name: "B", Status: "REGULAR",
	})

	members, err := repo.ListWithSelfie(context.Background())
	if err != nil {
		t.Fatalf("ListWithSelfie: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member with selfie, got %d", len(members))
	}
}

// ---------- Devices ----------

func TestDeviceRepository_UpsertAndList(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	repo := repository.NewDeviceRepository(pool)

	ip := "192.168.1.100"
	mac := "AA:BB:CC:DD:EE:FF"
	d := domain.Device{
		DeviceIdentifier: "AA:BB:CC:DD:EE:FF",
		IPAddress:        &ip,
		MACAddress:       &mac,
	}

	// First heartbeat (insert)
	if err := repo.Upsert(context.Background(), d); err != nil {
		t.Fatalf("Upsert (first): %v", err)
	}

	// Second heartbeat (update, no duplicate)
	if err := repo.Upsert(context.Background(), d); err != nil {
		t.Fatalf("Upsert (second): %v", err)
	}

	// Verify only one row
	active, err := repo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("expected 1 active device, got %d", len(active))
	}
	if active[0].DeviceIdentifier != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("device_identifier = %q", active[0].DeviceIdentifier)
	}
}

// ---------- AttendanceEvents ----------

func TestAttendanceEventRepository_InsertAndDedup(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	repo := repository.NewAttendanceEventRepository(pool)

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]string{"test": "payload"})
	status := "authorized"
	e := domain.AttendanceEvent{
		EventKey:         repository.ComputeEventKey("12345678901", now, "AA:BB:CC:DD:EE:FF"),
		EmployeeNoString: "12345678901",
		AttendanceStatus: &status,
		RawPayload:       payload,
	}

	// Insert
	inserted, err := repo.InsertIfNotExists(context.Background(), e)
	if err != nil {
		t.Fatalf("InsertIfNotExists (first): %v", err)
	}
	if !inserted {
		t.Error("expected inserted=true on first insert")
	}

	// Duplicate (same event_key)
	inserted2, err := repo.InsertIfNotExists(context.Background(), e)
	if err != nil {
		t.Fatalf("InsertIfNotExists (dup): %v", err)
	}
	if inserted2 {
		t.Error("expected inserted=false on duplicate")
	}
}

func TestAttendanceEventRepository_MarkAsMarked(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	repo := repository.NewAttendanceEventRepository(pool)

	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]string{"x": "1"})
	eventKey := repository.ComputeEventKey("99988877766", now, "device-1")
	status := "authorized"
	e := domain.AttendanceEvent{
		EventKey:         eventKey,
		EmployeeNoString: "99988877766",
		AttendanceStatus: &status,
		RawPayload:       payload,
	}

	repo.InsertIfNotExists(context.Background(), e) //nolint:errcheck

	if err := repo.MarkAsMarked(context.Background(), eventKey); err != nil {
		t.Fatalf("MarkAsMarked: %v", err)
	}
}

func TestComputeEventKey_Deterministic(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	k1 := repository.ComputeEventKey("12345678901", now, "device-1")
	k2 := repository.ComputeEventKey("12345678901", now, "device-1")
	if k1 != k2 {
		t.Errorf("ComputeEventKey not deterministic: %q != %q", k1, k2)
	}

	k3 := repository.ComputeEventKey("12345678901", now, "device-2")
	if k1 == k3 {
		t.Error("different deviceIdentifier should produce different key")
	}
}

// ---------- ProcessingOutcome ----------

func TestProcessingOutcomeRepository_UpsertAndFind(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)

	// Need a device first (FK constraint)
	devRepo := repository.NewDeviceRepository(pool)
	ip := "192.168.1.1"
	mac := "BB:BB:BB:BB:BB:BB"
	devRepo.Upsert(context.Background(), domain.Device{ //nolint:errcheck
		DeviceIdentifier: "BB:BB:BB:BB:BB:BB", IPAddress: &ip, MACAddress: &mac,
	})
	dev, _ := devRepo.FindByIdentifier(context.Background(), "BB:BB:BB:BB:BB:BB")
	if dev == nil {
		t.Fatal("device not found after upsert")
	}

	repo := repository.NewProcessingOutcomeRepository(pool)
	stage := "user_sync"
	o := domain.ProcessingOutcome{
		FederalDocument: "11100011100",
		DeviceID:        dev.ID,
		UserSynced:      false,
		FaceUploaded:    false,
		WebhookSet:      false,
		LastStage:       &stage,
		Attempts:        1,
	}

	// Insert
	if err := repo.UpsertOutcome(context.Background(), o); err != nil {
		t.Fatalf("UpsertOutcome (insert): %v", err)
	}

	// Update (increment attempts)
	o.UserSynced = true
	o.Attempts = 2
	stageDone := "done"
	o.LastStage = &stageDone
	if err := repo.UpsertOutcome(context.Background(), o); err != nil {
		t.Fatalf("UpsertOutcome (update): %v", err)
	}

	found, err := repo.FindByMemberDevice(context.Background(), "11100011100", dev.ID)
	if err != nil {
		t.Fatalf("FindByMemberDevice: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find processing outcome")
	}
	if found.Attempts != 2 {
		t.Errorf("attempts = %d, want 2", found.Attempts)
	}
	if !found.UserSynced {
		t.Error("user_synced should be true")
	}
	if found.LastStage == nil || *found.LastStage != "done" {
		t.Errorf("last_stage = %v, want 'done'", found.LastStage)
	}
}

// suppress unused
var _ = fmt.Sprintf
