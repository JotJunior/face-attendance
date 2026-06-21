package hikvision

import (
	"strings"
	"testing"
)

// TestNewSearchID_Format verifies UUID v4 string format: 8-4-4-4-12.
func TestNewSearchID_Format(t *testing.T) {
	id := newSearchID()
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 parts, got %d: %q", len(parts), id)
	}
	lens := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != lens[i] {
			t.Errorf("part[%d] len: got %d, want %d (value: %q)", i, len(p), lens[i], p)
		}
	}
}

// TestNewSearchID_NonDeterministic verifies CHK072: searchID is different each call.
// Probabilistically certain: 2 UUIDs v4 from crypto/rand matching would require
// 2^128 birthday collision — impossible in practice.
func TestNewSearchID_NonDeterministic(t *testing.T) {
	ids := make(map[string]struct{}, 10)
	for i := 0; i < 10; i++ {
		id := newSearchID()
		if _, dup := ids[id]; dup {
			t.Errorf("duplicate searchID generated: %q", id)
		}
		ids[id] = struct{}{}
	}
}

// TestNewSearchID_Version4 verifies UUID version nibble = 4.
func TestNewSearchID_Version4(t *testing.T) {
	for i := 0; i < 5; i++ {
		id := newSearchID()
		// 3rd group (index [2]), first char must be '4'
		parts := strings.Split(id, "-")
		if len(parts[2]) < 1 || parts[2][0] != '4' {
			t.Errorf("version nibble: got %q, want '4' in %q", string(parts[2][0]), id)
		}
	}
}
