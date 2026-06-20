package config_test

import (
	"os"
	"testing"

	"github.com/jotjunior/face-attendance/internal/config"
)

func setRequiredVars(t *testing.T) func() {
	t.Helper()
	vars := map[string]string{
		"GOB_STATE_URL":     "https://gob.example.com",
		"GOB_STATE_TOKEN":   "tok_test_secret",
		"ADMIN_TOKEN":       "admin_test_secret",
		"WEBHOOK_PATH_SECRET": "abc123secret",
		"DATABASE_URL":      "postgres://presenca:presenca_dev@localhost:5432/presenca_facial",
		"RABBITMQ_URL":      "amqp://guest:guest@localhost:5672/",
	}
	for k, v := range vars {
		os.Setenv(k, v) //nolint:errcheck
	}
	return func() {
		for k := range vars {
			os.Unsetenv(k) //nolint:errcheck
		}
	}
}

func TestLoad_AllPresent(t *testing.T) {
	cleanup := setRequiredVars(t)
	defer cleanup()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.GobStateURL != "https://gob.example.com" {
		t.Errorf("GobStateURL = %q", cfg.GobStateURL)
	}
	if cfg.MemberSyncIntervalMinutes != 60 {
		t.Errorf("MemberSyncIntervalMinutes default = %d, want 60", cfg.MemberSyncIntervalMinutes)
	}
	if cfg.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts default = %d, want 3", cfg.RetryMaxAttempts)
	}
	if !cfg.RunHTTP {
		t.Error("RunHTTP default should be true")
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// Ensure no relevant env vars are set
	vars := []string{"GOB_STATE_URL", "GOB_STATE_TOKEN", "ADMIN_TOKEN", "WEBHOOK_PATH_SECRET", "DATABASE_URL", "RABBITMQ_URL"}
	for _, v := range vars {
		os.Unsetenv(v) //nolint:errcheck
	}

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() expected error for missing vars, got nil")
	}

	for _, name := range vars {
		if !contains(err.Error(), name) {
			t.Errorf("error should mention missing var %s; got: %v", name, err)
		}
	}
}

func TestLoad_OptionalOverrides(t *testing.T) {
	cleanup := setRequiredVars(t)
	defer cleanup()

	os.Setenv("MEMBER_SYNC_INTERVAL_MINUTES", "30")
	os.Setenv("RETRY_MAX_ATTEMPTS", "5")
	os.Setenv("RUN_SCHEDULER", "false")
	defer func() {
		os.Unsetenv("MEMBER_SYNC_INTERVAL_MINUTES")
		os.Unsetenv("RETRY_MAX_ATTEMPTS")
		os.Unsetenv("RUN_SCHEDULER")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if cfg.MemberSyncIntervalMinutes != 30 {
		t.Errorf("MemberSyncIntervalMinutes = %d, want 30", cfg.MemberSyncIntervalMinutes)
	}
	if cfg.RetryMaxAttempts != 5 {
		t.Errorf("RetryMaxAttempts = %d, want 5", cfg.RetryMaxAttempts)
	}
	if cfg.RunScheduler {
		t.Error("RunScheduler should be false")
	}
}

func TestLoad_ISAPIDevices(t *testing.T) {
	cleanup := setRequiredVars(t)
	defer cleanup()

	os.Setenv("ISAPI_DEVICE_1_HOST", "192.168.1.100")
	os.Setenv("ISAPI_DEVICE_1_USER", "admin")
	os.Setenv("ISAPI_DEVICE_1_PASSWORD", "secret123")
	defer func() {
		os.Unsetenv("ISAPI_DEVICE_1_HOST")
		os.Unsetenv("ISAPI_DEVICE_1_USER")
		os.Unsetenv("ISAPI_DEVICE_1_PASSWORD")
	}()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	if len(cfg.ISAPIDevices) != 1 {
		t.Fatalf("expected 1 ISAPI device, got %d", len(cfg.ISAPIDevices))
	}
	if cfg.ISAPIDevices[0].Host != "192.168.1.100" {
		t.Errorf("ISAPIDevices[0].Host = %q", cfg.ISAPIDevices[0].Host)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
