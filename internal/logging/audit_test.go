package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/jotjunior/face-attendance/internal/logging"
)

// bufHandler captures log records as JSON lines for inspection.
type bufHandler struct {
	buf *bytes.Buffer
}

func newBufHandler() *bufHandler {
	return &bufHandler{buf: &bytes.Buffer{}}
}

func (h *bufHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *bufHandler) Handle(_ context.Context, r slog.Record) error {
	m := map[string]interface{}{
		"msg":  r.Message,
		"level": r.Level.String(),
	}
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.Any()
		return true
	})
	enc := json.NewEncoder(h.buf)
	return enc.Encode(m)
}

func (h *bufHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *bufHandler) WithGroup(name string) slog.Handler {
	return h
}

// TestLogger_MasksCPFInLogs verifies that the logger never emits a raw CPF.
func TestLogger_MasksCPFInLogs(t *testing.T) {
	h := newBufHandler()
	logger := logging.NewWithHandler(h)

	rawCPF := "12345678901"
	maskedPrefix := "***.***.***-" // MaskCPFForLog produces ***.***.***-NN

	logger.Info("test_stage", "device1", rawCPF, "some event")

	output := h.buf.String()
	if strings.Contains(output, rawCPF) {
		t.Errorf("log output must not contain raw CPF %q; got: %s", rawCPF, output)
	}
	if !strings.Contains(output, maskedPrefix) {
		t.Errorf("log output should contain masked CPF prefix %q; got: %s", maskedPrefix, output)
	}
}

// TestLogger_MandatoryFields verifies stage, device_id, and cpf fields are present.
func TestLogger_MandatoryFields(t *testing.T) {
	h := newBufHandler()
	logger := logging.NewWithHandler(h)

	logger.Info("my_stage", "dev-42", "12345678901", "hello")

	output := h.buf.String()

	requiredKeys := []string{"stage", "device_id", "cpf"}
	for _, key := range requiredKeys {
		if !strings.Contains(output, key) {
			t.Errorf("log output missing mandatory key %q; got: %s", key, output)
		}
	}
}

// TestLogger_ErrorIncludesErrField verifies that Error logs include an "err" field.
func TestLogger_ErrorIncludesErrField(t *testing.T) {
	h := newBufHandler()
	logger := logging.NewWithHandler(h)

	logger.Error("test_stage", "dev-1", "", "something failed", context.DeadlineExceeded)

	output := h.buf.String()
	if !strings.Contains(output, "err") {
		t.Errorf("error log should include 'err' field; got: %s", output)
	}
}

// TestLogger_EmptyCPFDoesNotPanic verifies graceful handling of empty CPF input.
func TestLogger_EmptyCPFDoesNotPanic(t *testing.T) {
	h := newBufHandler()
	logger := logging.NewWithHandler(h)

	// Should not panic even with empty or malformed CPF
	logger.Info("test_stage", "dev-1", "", "empty cpf test")
	logger.Info("test_stage", "dev-1", "short", "short cpf test")
}
