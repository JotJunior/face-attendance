// Package logging provides structured JSON logging with mandatory fields
// and PII masking (spec.md §FR-018, Constitution Principle V/VI).
package logging

import (
	"context"
	"log/slog"
	"os"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// Logger wraps slog.Logger and enforces field conventions:
//   - cpf: always masked via domain.MaskCPFForLog (never raw digits)
//   - stage: identifies the processing stage
//   - device_id: correlates to devices.id or device_identifier
//   - error: error message (string)
//
// Secrets (GOB_STATE_TOKEN, ISAPI passwords) must NEVER be passed as args.
type Logger struct {
	inner *slog.Logger
}

// New creates a Logger writing JSON to stdout.
func New() *Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return &Logger{inner: slog.New(h)}
}

// NewWithHandler creates a Logger with the provided handler (useful for tests).
func NewWithHandler(h slog.Handler) *Logger {
	return &Logger{inner: slog.New(h)}
}

// Info logs at INFO level with the mandatory field set.
// cpfRaw is automatically masked; pass empty string if not applicable.
func (l *Logger) Info(stage, deviceID, cpfRaw, msg string, extra ...any) {
	l.inner.Info(msg, l.buildArgs(stage, deviceID, cpfRaw, extra)...)
}

// Error logs at ERROR level with the mandatory field set.
func (l *Logger) Error(stage, deviceID, cpfRaw, msg string, err error, extra ...any) {
	args := l.buildArgs(stage, deviceID, cpfRaw, extra)
	if err != nil {
		args = append(args, slog.String("error", err.Error()))
	}
	l.inner.Error(msg, args...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(stage, deviceID, cpfRaw, msg string, extra ...any) {
	l.inner.Warn(msg, l.buildArgs(stage, deviceID, cpfRaw, extra)...)
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(stage, deviceID, cpfRaw, msg string, extra ...any) {
	l.inner.Debug(msg, l.buildArgs(stage, deviceID, cpfRaw, extra)...)
}

// InfoCtx logs at INFO with a context (future: trace propagation).
func (l *Logger) InfoCtx(ctx context.Context, stage, deviceID, cpfRaw, msg string, extra ...any) {
	l.inner.InfoContext(ctx, msg, l.buildArgs(stage, deviceID, cpfRaw, extra)...)
}

// ErrorCtx logs at ERROR with a context.
func (l *Logger) ErrorCtx(ctx context.Context, stage, deviceID, cpfRaw, msg string, err error, extra ...any) {
	args := l.buildArgs(stage, deviceID, cpfRaw, extra)
	if err != nil {
		args = append(args, slog.String("error", err.Error()))
	}
	l.inner.ErrorContext(ctx, msg, args...)
}

// buildArgs constructs the mandatory slog Attr list.
// cpfRaw is masked unconditionally; empty string produces masked empty.
func (l *Logger) buildArgs(stage, deviceID, cpfRaw string, extra []any) []any {
	masked := domain.MaskCPFForLog(cpfRaw)
	args := []any{
		slog.String("stage", stage),
		slog.String("device_id", deviceID),
		slog.String("cpf", masked),
	}
	args = append(args, extra...)
	return args
}
