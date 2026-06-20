package domain

// ProcessingMessage is the payload published to the RabbitMQ queue "member.processing".
// Keys are camelCase to match the outbound JSON boundary convention (plan.md §Convencoes de Borda).
type ProcessingMessage struct {
	FederalDocument string `json:"federalDocument"` // CPF digits
	Name            string `json:"name"`
	URLSelfie       string `json:"urlSelfie"`
	GobID           int64  `json:"gobId"`
}
