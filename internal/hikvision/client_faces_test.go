package hikvision_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestClearFaces_ReturnsErrNotImplemented verifies the stub returns ErrNotImplemented.
// CHK-PROPOSTA-9: endpoint not yet empirically verified. Ref: tasks.md §3.5.1/3.5.4.
func TestClearFaces_ReturnsErrNotImplemented(t *testing.T) {
	cfg := hikvision.DeviceConfig{
		Host:     "192.168.1.1",
		Username: "admin",
		Password: "pass",
	}
	err := hikvision.New(cfg).ClearFaces(context.Background())
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
	if !errors.Is(err, hikvision.ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %T: %v", err, err)
	}
	// Verify orientative message is present
	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}
