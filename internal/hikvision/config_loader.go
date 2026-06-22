package hikvision

import (
	"fmt"
	"strconv"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

// LoadDeviceConfig constructs a DeviceConfig from a domain.Device by decrypting the
// stored ISAPI password using the provided Cipher.
//
// CHK007: if cipher is nil (ISAPI_CRED_KEY absent) OR the device has no stored
// credentials, this function returns ErrKeyMissing. All ISAPI action handlers must
// call this function and map ErrKeyMissing → HTTP 503 with an orientative message.
// This centralises the check so no handler silently proceeds without a valid key.
func LoadDeviceConfig(device *domain.Device, cipher *secrets.Cipher) (DeviceConfig, error) {
	if cipher == nil {
		return DeviceConfig{}, ErrKeyMissing
	}
	if device.ISAPIPasswordEnc == nil {
		return DeviceConfig{}, ErrKeyMissing
	}
	username := ""
	if device.ISAPIUsername != nil {
		username = *device.ISAPIUsername
	}
	if username == "" {
		return DeviceConfig{}, ErrKeyMissing
	}

	password, err := cipher.Decrypt(device.ISAPIPasswordEnc)
	if err != nil {
		return DeviceConfig{}, fmt.Errorf("%w: %v", ErrKeyMissing, err)
	}

	port := device.ISAPIPort
	if port <= 0 {
		port = 80
	}

	host := ""
	if device.IPAddress != nil {
		host = *device.IPAddress + ":" + strconv.Itoa(port)
	}
	if host == "" {
		return DeviceConfig{}, fmt.Errorf("hikvision: device sem endereço IP")
	}

	return DeviceConfig{
		Host:     host,
		Username: username,
		Password: password,
	}, nil
}
