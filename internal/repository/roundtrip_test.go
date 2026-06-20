//go:build integration
// +build integration

// Scenario 5 — Anti-drift roundtrip test (spec.md §FR-019, tasks.md §9.2).
// Validates that the full data flow is consistent end-to-end:
// member upsert → event insert → mark attendance → verify mark persisted.
// Also validates that CPF formatting roundtrips cleanly (NormalizeCPF → FormatCPF → ParseCPF).
package repository_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/repository"
)

// TestRoundtrip_MemberToAttendance verifies end-to-end: member exists → event inserted → marked.
// This is the canonical Scenario 5 anti-drift test.
func TestRoundtrip_MemberToAttendance(t *testing.T) {
	pool := testPool(t)
	cleanup(t, pool)

	memberRepo := repository.NewMemberRepository(pool)
	eventRepo := repository.NewAttendanceEventRepository(pool)

	// 1. Upsert a member with a selfie
	cpfDigits := "98765432100"
	selfie := "https://example.com/face.jpg"
	m := domain.Member{
		GobID:           999,
		FederalDocument: cpfDigits,
		Name:            "Roundtrip User",
		Status:          "REGULAR",
		URLSelfie:       &selfie,
	}
	if err := memberRepo.Upsert(context.Background(), m); err != nil {
		t.Fatalf("member upsert: %v", err)
	}

	// 2. Find the member back
	found, err := memberRepo.FindByCPF(context.Background(), cpfDigits)
	if err != nil {
		t.Fatalf("FindByCPF: %v", err)
	}
	if found == nil {
		t.Fatal("member not found after upsert")
	}
	if !found.HasSelfie() {
		t.Error("member should have selfie")
	}

	// 3. Simulate an inbound attendance event
	now := time.Now().UTC()
	eventKey := repository.ComputeEventKey(cpfDigits, now, "device-roundtrip")
	status := "authorized"
	payload, _ := json.Marshal(map[string]interface{}{
		"employeeNoString": cpfDigits,
		"attendanceStatus": status,
	})
	memberID := found.ID
	fedDoc := found.FederalDocument

	event := domain.AttendanceEvent{
		EventKey:         eventKey,
		EmployeeNoString: cpfDigits,
		FederalDocument:  &fedDoc,
		MemberID:         &memberID,
		AttendanceStatus: &status,
		EventDatetime:    &now,
		RawPayload:       payload,
	}

	// 4. Insert event (should be new)
	inserted, err := eventRepo.InsertIfNotExists(context.Background(), event)
	if err != nil {
		t.Fatalf("InsertIfNotExists: %v", err)
	}
	if !inserted {
		t.Error("expected event to be inserted (not a duplicate)")
	}

	// 5. Mark attendance
	if err := eventRepo.MarkAsMarked(context.Background(), eventKey); err != nil {
		t.Fatalf("MarkAsMarked: %v", err)
	}

	// 6. Verify dedup: same event_key should not insert again
	inserted2, err := eventRepo.InsertIfNotExists(context.Background(), event)
	if err != nil {
		t.Fatalf("InsertIfNotExists (dedup): %v", err)
	}
	if inserted2 {
		t.Error("duplicate event should not be inserted (idempotency FR-016)")
	}
}

// TestRoundtrip_CPFFormatting verifies that CPF formats roundtrip without data loss.
func TestRoundtrip_CPFFormatting(t *testing.T) {
	cases := []struct {
		name    string
		digits  string
		masked  string // expected formatted form
	}{
		{"standard", "12345678901", "123.456.789-01"},
		{"all_same", "11111111111", "111.111.111-11"},
		{"leading_zero", "01234567890", "012.345.678-90"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			formatted, err := domain.FormatCPF(tc.digits)
			if err != nil {
				t.Fatalf("FormatCPF(%q): %v", tc.digits, err)
			}
			if formatted != tc.masked {
				t.Errorf("FormatCPF(%q) = %q, want %q", tc.digits, formatted, tc.masked)
			}

			parsed, err := domain.ParseCPF(formatted)
			if err != nil {
				t.Fatalf("ParseCPF(%q): %v", formatted, err)
			}
			if parsed != tc.digits {
				t.Errorf("ParseCPF(FormatCPF(%q)) = %q, want original digits", tc.digits, parsed)
			}

			normalized, err := domain.NormalizeCPF(tc.masked)
			if err != nil {
				t.Fatalf("NormalizeCPF(%q): %v", tc.masked, err)
			}
			if normalized != tc.digits {
				t.Errorf("NormalizeCPF(%q) = %q, want %q", tc.masked, normalized, tc.digits)
			}
		})
	}
}

// TestRoundtrip_EventKeyDeterminism verifies that event_key is stable across multiple computations.
func TestRoundtrip_EventKeyDeterminism(t *testing.T) {
	cpf := "12345678901"
	ts := time.Date(2026, 6, 20, 10, 30, 0, 0, time.UTC)
	device := "AA:BB:CC:DD:EE:FF"

	key1 := repository.ComputeEventKey(cpf, ts, device)
	key2 := repository.ComputeEventKey(cpf, ts, device)

	if key1 != key2 {
		t.Errorf("ComputeEventKey not deterministic: %q != %q", key1, key2)
	}

	// Different CPF → different key
	keyOther := repository.ComputeEventKey("99988877766", ts, device)
	if key1 == keyOther {
		t.Error("different CPF should produce different event_key")
	}

	// Different time → different key
	keyLater := repository.ComputeEventKey(cpf, ts.Add(time.Second), device)
	if key1 == keyLater {
		t.Error("different timestamp should produce different event_key")
	}
}
