package domain

import "time"

// Device represents a HikVision access control terminal registered via heartbeat.
// Fields are derived from the webhook payload (contracts/inbound-http.md §2).
type Device struct {
	ID                 int64      `json:"id"`
	DeviceIdentifier   string     `json:"device_identifier"` // stable identifier (MAC address)
	IPAddress          *string    `json:"ip_address,omitempty"`
	MACAddress         *string    `json:"mac_address,omitempty"`
	LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
	IsActive           bool       `json:"is_active"`
	WebhookConfigured  bool       `json:"webhook_configured"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// Telemetria de hardware obtida via ISAPI deviceInfo (nullable até a 1ª leitura).
	SerialNumber    *string `json:"serial_number,omitempty"`
	Model           *string `json:"model,omitempty"`
	FirmwareVersion *string `json:"firmware_version,omitempty"`

	// Conexão ISAPI persistida no banco (substitui ISAPI_DEVICE_{N}_* do .env).
	// ISAPIPasswordEnc é o blob AES-GCM (nonce||ciphertext); nunca serializado em JSON.
	ISAPIUsername    *string `json:"isapi_username,omitempty"`
	ISAPIPort        int     `json:"isapi_port,omitempty"`
	ISAPIPasswordEnc []byte  `json:"-"`
}
