package domain

import "time"

// ProcessingOutcome tracks per-member per-device ISAPI processing state.
// Maps to the member_processing_status table (data-model.md §ProcessingOutcome).
type ProcessingOutcome struct {
	ID              int64      `json:"id"`
	FederalDocument string     `json:"federal_document"` // CPF digits
	DeviceID        int64      `json:"device_id"`
	UserSynced      bool       `json:"user_synced"`
	FaceUploaded    bool       `json:"face_uploaded"`
	WebhookSet      bool       `json:"webhook_set"`
	LastStage       *string    `json:"last_stage,omitempty"` // user_sync|face_upload|webhook|done
	LastError       *string    `json:"last_error,omitempty"`
	Attempts        int        `json:"attempts"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
