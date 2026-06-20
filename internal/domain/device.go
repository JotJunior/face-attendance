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
}
