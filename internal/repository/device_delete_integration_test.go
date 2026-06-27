//go:build integration
// +build integration

// Testes de integração para remoção de dispositivo (DeleteDevice) e o
// comportamento dos FKs ajustados na migration 000010.
// Execute com: go test -tags integration ./internal/repository/...
package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// TestDeleteDevice_CascadeAndSetNull valida a migration 000010:
//   - device removido;
//   - attendance_events preservado com device_id = NULL (SET NULL);
//   - member_processing_status removido (CASCADE).
func TestDeleteDevice_CascadeAndSetNull(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	cleanupFlows(t, pool)
	ctx := context.Background()

	devRepo := repository.NewDeviceRepository(pool)
	ip, mac := "10.9.9.9", "AA:11:22:33:44:55"
	if err := devRepo.Upsert(ctx, domain.Device{DeviceIdentifier: mac, IPAddress: &ip, MACAddress: &mac}); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	dev, err := devRepo.FindByIdentifier(ctx, mac)
	if err != nil || dev == nil {
		t.Fatalf("find device: %v", err)
	}

	// Dependentes apontando para o device.
	if _, err := pool.Exec(ctx,
		`INSERT INTO attendance_events (event_key, employee_no_string, marked, raw_payload, device_id)
		 VALUES ($1,$2,$3,$4,$5)`,
		"evt-del-1", "123", false, "{}", dev.ID); err != nil {
		t.Fatalf("insert attendance_event: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO member_processing_status (federal_document, device_id, user_synced, face_uploaded, webhook_set, attempts)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		"12345678901", dev.ID, true, true, true, 1); err != nil {
		t.Fatalf("insert member_processing_status: %v", err)
	}

	// Remove o device.
	if err := devRepo.DeleteDevice(ctx, dev.ID); err != nil {
		t.Fatalf("DeleteDevice: %v", err)
	}

	// Device sumiu.
	if got, _ := devRepo.FindByIdentifier(ctx, mac); got != nil {
		t.Error("device ainda existe após DeleteDevice")
	}

	// attendance_event preservado com device_id NULL (SET NULL).
	var cnt, nullCnt int
	if err := pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE device_id IS NULL) FROM attendance_events WHERE event_key=$1`,
		"evt-del-1").Scan(&cnt, &nullCnt); err != nil {
		t.Fatalf("query attendance_events: %v", err)
	}
	if cnt != 1 {
		t.Errorf("attendance_event deveria ser preservado; cnt=%d", cnt)
	}
	if nullCnt != 1 {
		t.Errorf("attendance_event.device_id deveria virar NULL (SET NULL); nullCnt=%d", nullCnt)
	}

	// member_processing_status removido (CASCADE).
	var mps int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM member_processing_status WHERE federal_document=$1`,
		"12345678901").Scan(&mps); err != nil {
		t.Fatalf("query member_processing_status: %v", err)
	}
	if mps != 0 {
		t.Errorf("member_processing_status deveria cascatear (0); cnt=%d", mps)
	}
}

// TestDeleteDevice_NotFound: remover ID inexistente → pgx.ErrNoRows.
func TestDeleteDevice_NotFound(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)
	devRepo := repository.NewDeviceRepository(pool)
	if err := devRepo.DeleteDevice(context.Background(), 999999); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("esperado pgx.ErrNoRows; got %v", err)
	}
}
