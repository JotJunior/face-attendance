package httphandler

// actionResponse is the standard payload for ISAPI action endpoints (reboot, factory-reset,
// door control, DELETE users/faces/webhooks, etc.).
// CHK058: all action responses include device_id for traceability.
// Optional fields use omitempty — handlers set only the fields relevant to each action.
type actionResponse struct {
	Result   string `json:"result"`
	DeviceID int64  `json:"device_id"`
	// Optional per-action fields
	WebhookConfigured *bool `json:"webhook_configured,omitempty"` // factory-reset: false after reset
}
