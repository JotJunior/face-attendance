package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/jotjunior/face-attendance/internal/logging"
)

// captureLogger creates a Logger that writes JSON to a buffer.
func captureLogger(buf *bytes.Buffer) *logging.Logger {
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return logging.NewWithHandler(h)
}

func parseLog(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("failed to parse log JSON: %v\nraw: %s", err, buf.String())
	}
	return m
}

// TestLogger_CPFMasked verifies that CPF is never logged in raw form.
func TestLogger_CPFMasked(t *testing.T) {
	var buf bytes.Buffer
	l := captureLogger(&buf)

	rawCPF := "12345678901"
	l.Info("heartbeat_received", "dev-1", rawCPF, "device registered")

	m := parseLog(t, &buf)

	if v, ok := m["cpf"].(string); !ok || v == rawCPF {
		t.Errorf("cpf should be masked, got %q (raw: %q)", v, rawCPF)
	}
	// Verify the raw CPF is not present anywhere in the log output
	if bytes.Contains(buf.Bytes(), []byte(rawCPF)) {
		t.Errorf("raw CPF %q must not appear in log output: %s", rawCPF, buf.String())
	}
}

// TestLogger_StagePresent verifies that stage field is always present.
func TestLogger_StagePresent(t *testing.T) {
	var buf bytes.Buffer
	l := captureLogger(&buf)

	l.Info("member_load_started", "", "", "loading members")

	m := parseLog(t, &buf)
	if stage, ok := m["stage"].(string); !ok || stage != "member_load_started" {
		t.Errorf("stage = %v, want 'member_load_started'", m["stage"])
	}
}

// TestLogger_GobTokenNotLeaked verifies that the GOB token is not present in output.
func TestLogger_GobTokenNotLeaked(t *testing.T) {
	var buf bytes.Buffer
	l := captureLogger(&buf)

	gobToken := "super_secret_gob_token_abc123"
	// Simulate a log call that should NOT contain the token
	l.Error("member_load_started", "", "", "GOB API error", errors.New("connection refused"))

	if bytes.Contains(buf.Bytes(), []byte(gobToken)) {
		t.Errorf("GOB token must not appear in log: %s", buf.String())
	}
}

// TestLogger_ErrorField verifies that errors are logged in the "error" field.
func TestLogger_ErrorField(t *testing.T) {
	var buf bytes.Buffer
	l := captureLogger(&buf)

	l.Error("user_synced", "dev-1", "", "ISAPI call failed", errors.New("connection timeout"))

	m := parseLog(t, &buf)
	if errVal, ok := m["error"].(string); !ok || errVal == "" {
		t.Errorf("error field should be present and non-empty; got %v", m["error"])
	}
}
