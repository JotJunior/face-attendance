package domain

// DTOs de view para a interface de administração web.
// Expostos pelo pacote admin HTTP — nunca contêm CPF completo (SC-006, Constitution §VI).
// maskCPF garante que federal_document cru nunca trafegue ao browser.
// Ref: spec.md §FR-011, §SC-006, tasks.md §2.4, contracts/admin-api.md.

import "time"

// MemberView é o DTO de saída para listagem de membros no painel admin.
// federal_document_masked substitui o CPF cru (SC-006).
type MemberView struct {
	ID                    int64   `json:"id"`
	Name                  string  `json:"name"`
	FederalDocumentMasked string  `json:"federal_document_masked"`
	Status                string  `json:"status"`
	SyncStatus            string  `json:"sync_status"`
	LastFailedStage       *string `json:"last_failed_stage,omitempty"`
}

// EventView é o DTO de saída para listagem de eventos no painel admin.
// raw_payload e event_key excluídos (não expostos ao browser).
// federal_document_masked substitui o CPF cru (SC-006).
type EventView struct {
	ID                    int64      `json:"id"`
	EventDatetime         *time.Time `json:"event_datetime,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	DeviceID              *int64     `json:"device_id,omitempty"`
	DeviceIdentifier      *string    `json:"device_identifier,omitempty"`
	MemberName            *string    `json:"member_name,omitempty"`
	FederalDocumentMasked *string    `json:"federal_document_masked,omitempty"`
	MarkingStatus         string     `json:"marking_status"`
	MarkedAt              *time.Time `json:"marked_at,omitempty"`
}

// CursorEvt é o cursor de paginação keyset para ListEventsPaged.
// Combina created_at + id para ordenação composta (created_at DESC, id DESC).
// Valor zero (CreatedAt.IsZero() == true) indica início da listagem.
type CursorEvt struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

// IsZero reporta se o cursor representa o início (sem cursor anterior).
func (c CursorEvt) IsZero() bool {
	return c.CreatedAt.IsZero() && c.ID == 0
}

// maskCPF mascara um CPF retornando o formato ***.NNN.NNN-**
// substituindo os 3 primeiros dígitos e os 2 últimos dígitos verificadores.
// CPF esperado no formato "NNN.NNN.NNN-NN" (14 chars com pontuação) ou
// "NNNNNNNNNNN" (11 dígitos sem pontuação). Em caso de formato inesperado,
// retorna string de asteriscos para garantir que nenhum CPF cru vaze.
func maskCPF(cpf string) string {
	// Remover pontuação para normalizar
	digits := make([]byte, 0, 11)
	for i := 0; i < len(cpf); i++ {
		if cpf[i] >= '0' && cpf[i] <= '9' {
			digits = append(digits, cpf[i])
		}
	}

	if len(digits) != 11 {
		// CPF com formato inesperado — retornar totalmente mascarado
		return "***.***.***-**"
	}

	// Formato: ***.NNN.NNN-**
	// Posição: [0..2]=mascara, [3..5]=visível, [6..8]=visível, [9..10]=mascara
	return "***." +
		string(digits[3:6]) + "." +
		string(digits[6:9]) + "-**"
}

// MaskCPF é a versão exportada de maskCPF para uso nos repositórios e handlers.
// Garante que o CPF mascarado seja calculado num único ponto (sem duplicação).
func MaskCPF(cpf string) string {
	return maskCPF(cpf)
}

// DeriveSyncStatus converte um ProcessingOutcome em status de sincronização legível.
// Retorna "synced", "failed" ou "pending".
func DeriveSyncStatus(outcome *ProcessingOutcome) string {
	if outcome == nil {
		return "pending"
	}
	if outcome.UserSynced && outcome.FaceUploaded && outcome.WebhookSet {
		return "synced"
	}
	if outcome.LastError != nil && *outcome.LastError != "" {
		return "failed"
	}
	return "pending"
}

// DeriveMarkingStatus converte um AttendanceEvent em status de marcação legível.
// Retorna "marked", "pending", "failed" ou "unauthorized".
func DeriveMarkingStatus(event AttendanceEvent) string {
	if event.Marked {
		return "marked"
	}
	// Evento não autorizado pelo dispositivo
	if event.AttendanceStatus != nil && *event.AttendanceStatus != "authorized" {
		return "unauthorized"
	}
	// Evento autorizado mas não marcado (pode ser falha ou pendente)
	if event.MemberID == nil {
		// Membro não identificado — considerado failed (não há como marcar)
		return "failed"
	}
	return "pending"
}
