package hikvision_test

import (
	"errors"
	"testing"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

// makeTestCipher creates a Cipher with a test key (32 bytes of 0xAB).
func makeTestCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xAB
	}
	c, err := secrets.NewCipher(key)
	if err != nil {
		t.Fatalf("makeTestCipher: %v", err)
	}
	return c
}

// TestLoadDeviceConfig_ErrKeyMissing_WhenCipherNil verifies CHK007:
// LoadDeviceConfig returns ErrKeyMissing when ISAPI_CRED_KEY is absent (cipher == nil).
func TestLoadDeviceConfig_ErrKeyMissing_WhenCipherNil(t *testing.T) {
	ip := "192.168.1.10"
	user := "admin"
	device := &domain.Device{
		IPAddress:        &ip,
		ISAPIUsername:    &user,
		ISAPIPasswordEnc: []byte("some-encrypted-bytes"),
		ISAPIPort:        80,
	}
	_, err := hikvision.LoadDeviceConfig(device, nil)
	if !errors.Is(err, hikvision.ErrKeyMissing) {
		t.Errorf("expected ErrKeyMissing, got: %v", err)
	}
}

// TestLoadDeviceConfig_ErrKeyMissing_WhenPasswordEncNil verifies CHK007:
// LoadDeviceConfig returns ErrKeyMissing when device has no encrypted password stored.
func TestLoadDeviceConfig_ErrKeyMissing_WhenPasswordEncNil(t *testing.T) {
	c := makeTestCipher(t)
	ip := "192.168.1.10"
	user := "admin"
	device := &domain.Device{
		IPAddress:        &ip,
		ISAPIUsername:    &user,
		ISAPIPasswordEnc: nil, // no credentials stored
		ISAPIPort:        80,
	}
	_, err := hikvision.LoadDeviceConfig(device, c)
	if !errors.Is(err, hikvision.ErrKeyMissing) {
		t.Errorf("expected ErrKeyMissing, got: %v", err)
	}
}

// TestLoadDeviceConfig_ErrKeyMissing_WhenDecryptFails verifies CHK007:
// LoadDeviceConfig wraps decryption errors in ErrKeyMissing (tampered ciphertext).
func TestLoadDeviceConfig_ErrKeyMissing_WhenDecryptFails(t *testing.T) {
	c := makeTestCipher(t)
	ip := "192.168.1.10"
	user := "admin"
	device := &domain.Device{
		IPAddress:        &ip,
		ISAPIUsername:    &user,
		ISAPIPasswordEnc: []byte("not-a-valid-gcm-ciphertext"), // garbage
		ISAPIPort:        80,
	}
	_, err := hikvision.LoadDeviceConfig(device, c)
	if !errors.Is(err, hikvision.ErrKeyMissing) {
		t.Errorf("expected ErrKeyMissing wrapping decrypt error, got: %v", err)
	}
}

// TestLoadDeviceConfig_Success verifies that a valid device + cipher returns a DeviceConfig
// with decrypted credentials and correct host:port.
func TestLoadDeviceConfig_Success(t *testing.T) {
	c := makeTestCipher(t)

	// Encrypt a test password
	enc, err := c.Encrypt("s3cr3t")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	ip := "192.168.1.10"
	user := "admin"
	device := &domain.Device{
		IPAddress:        &ip,
		ISAPIUsername:    &user,
		ISAPIPasswordEnc: enc,
		ISAPIPort:        8080,
	}

	cfg, err := hikvision.LoadDeviceConfig(device, c)
	if err != nil {
		t.Fatalf("LoadDeviceConfig: %v", err)
	}
	if cfg.Username != "admin" {
		t.Errorf("username: got %q, want %q", cfg.Username, "admin")
	}
	if cfg.Password != "s3cr3t" {
		t.Errorf("password: got %q, want %q", cfg.Password, "s3cr3t")
	}
	if cfg.Host != "192.168.1.10:8080" {
		t.Errorf("host: got %q, want %q", cfg.Host, "192.168.1.10:8080")
	}
}
