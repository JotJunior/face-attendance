package domain

import "time"

// Member represents a GOB member eligible for facial recognition.
// Fields are derived from the GOB API response (contracts/gob-api.md §Campos verificados).
type Member struct {
	ID             int64      `json:"id"`
	GobID          int64      `json:"gob_id"`
	FederalDocument string    `json:"federal_document"` // CPF digits (11 chars)
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	MobileNumber   *string    `json:"mobile_number,omitempty"`
	URLSelfie      *string    `json:"url_selfie,omitempty"`
	GobCreatedAt   *time.Time `json:"gob_created_at,omitempty"`
	GobUpdatedAt   *time.Time `json:"gob_updated_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// HasSelfie reports whether the member has a selfie URL available for ISAPI upload.
func (m *Member) HasSelfie() bool {
	return m.URLSelfie != nil && *m.URLSelfie != ""
}
