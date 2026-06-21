package hikvision_test

import (
	"errors"
	"testing"

	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// TestErrKeyMissing_IsDistinct verifies ErrKeyMissing is a distinct sentinel error.
// CHK007: the error must be identifiable via errors.Is for handler 503 mapping.
func TestErrKeyMissing_IsDistinct(t *testing.T) {
	if hikvision.ErrKeyMissing == nil {
		t.Fatal("ErrKeyMissing is nil")
	}
	if !errors.Is(hikvision.ErrKeyMissing, hikvision.ErrKeyMissing) {
		t.Error("errors.Is(ErrKeyMissing, ErrKeyMissing) should be true")
	}
	if errors.Is(hikvision.ErrKeyMissing, hikvision.ErrNotImplemented) {
		t.Error("ErrKeyMissing should not match ErrNotImplemented")
	}
}

// TestErrNotImplemented_MessageContainsStub verifies stub error message is orientative.
func TestErrNotImplemented_MessageContainsStub(t *testing.T) {
	if hikvision.ErrNotImplemented == nil {
		t.Fatal("ErrNotImplemented is nil")
	}
	msg := hikvision.ErrNotImplemented.Error()
	if msg == "" {
		t.Error("ErrNotImplemented message should not be empty")
	}
}

// TestErrUnknownCommand_IsDistinct verifies ErrUnknownCommand is a sentinel error.
func TestErrUnknownCommand_IsDistinct(t *testing.T) {
	if hikvision.ErrUnknownCommand == nil {
		t.Fatal("ErrUnknownCommand is nil")
	}
	if errors.Is(hikvision.ErrUnknownCommand, hikvision.ErrKeyMissing) {
		t.Error("ErrUnknownCommand should not match ErrKeyMissing")
	}
}
