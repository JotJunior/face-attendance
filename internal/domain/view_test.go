package domain

// Testes unitários para view.go: maskCPF, DeriveSyncStatus, DeriveMarkingStatus.
// Ref: tasks.md §2.4.6, spec.md §FR-011, §SC-006.

import (
	"testing"
)

func TestMaskCPF(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "CPF formatado padrão",
			input: "005.149.047-12",
			want:  "***.149.047-**",
		},
		{
			name:  "CPF apenas dígitos",
			input: "00514904712",
			want:  "***.149.047-**",
		},
		{
			name:  "CPF vazio",
			input: "",
			want:  "***.***.***-**",
		},
		{
			name:  "CPF mal-formatado (menos de 11 dígitos)",
			input: "123",
			want:  "***.***.***-**",
		},
		{
			name:  "CPF com 11 zeros",
			input: "000.000.000-00",
			want:  "***.000.000-**",
		},
		{
			name:  "CPF com 12 dígitos (inválido)",
			input: "123456789012",
			want:  "***.***.***-**",
		},
		{
			name:  "CPF com caracteres não numéricos",
			input: "abc.def.ghi-jk",
			want:  "***.***.***-**",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MaskCPF(tc.input)
			if got != tc.want {
				t.Errorf("MaskCPF(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestMaskCPF_NeverExposesRaw(t *testing.T) {
	cpf := "005.149.047-12"
	masked := MaskCPF(cpf)
	// Os 3 primeiros dígitos (005) não devem aparecer no resultado
	if len(masked) > 3 && masked[:3] == "005" {
		t.Errorf("MaskCPF expôs os 3 primeiros dígitos do CPF: %q", masked)
	}
	// Os 2 últimos dígitos (12) não devem aparecer
	if len(masked) > 2 && masked[len(masked)-2:] == "12" {
		t.Errorf("MaskCPF expôs os 2 últimos dígitos verificadores: %q", masked)
	}
}

func TestDeriveSyncStatus(t *testing.T) {
	noError := ""
	hasError := "falha ao sincronizar face"

	cases := []struct {
		name    string
		outcome *ProcessingOutcome
		want    string
	}{
		{
			name:    "nil outcome — sem linha no processing_status",
			outcome: nil,
			want:    "pending",
		},
		{
			name: "todos os campos true — sincronizado",
			outcome: &ProcessingOutcome{
				UserSynced:   true,
				FaceUploaded: true,
				WebhookSet:   true,
			},
			want: "synced",
		},
		{
			name: "com erro — failed",
			outcome: &ProcessingOutcome{
				UserSynced:   false,
				FaceUploaded: false,
				WebhookSet:   false,
				LastError:    &hasError,
			},
			want: "failed",
		},
		{
			name: "parcialmente sincronizado sem erro — pending",
			outcome: &ProcessingOutcome{
				UserSynced:   true,
				FaceUploaded: false,
				WebhookSet:   false,
				LastError:    &noError,
			},
			want: "pending",
		},
		{
			name: "erro vazio — pending (não failed)",
			outcome: &ProcessingOutcome{
				LastError: &noError,
			},
			want: "pending",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveSyncStatus(tc.outcome)
			if got != tc.want {
				t.Errorf("DeriveSyncStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeriveMarkingStatus(t *testing.T) {
	authorized := "authorized"
	notAuthorized := "denied"
	memberID := int64(42)

	cases := []struct {
		name  string
		event AttendanceEvent
		want  string
	}{
		{
			name:  "marcado — marked",
			event: AttendanceEvent{Marked: true},
			want:  "marked",
		},
		{
			name: "não autorizado pelo dispositivo — unauthorized",
			event: AttendanceEvent{
				Marked:           false,
				AttendanceStatus: &notAuthorized,
			},
			want: "unauthorized",
		},
		{
			name: "autorizado mas membro não identificado — failed",
			event: AttendanceEvent{
				Marked:           false,
				AttendanceStatus: &authorized,
				MemberID:         nil,
			},
			want: "failed",
		},
		{
			name: "autorizado com membro identificado mas não marcado — pending",
			event: AttendanceEvent{
				Marked:           false,
				AttendanceStatus: &authorized,
				MemberID:         &memberID,
			},
			want: "pending",
		},
		{
			name: "attendance_status nil — trata como não autorizado? não — sem membro = failed",
			event: AttendanceEvent{
				Marked:           false,
				AttendanceStatus: nil,
				MemberID:         nil,
			},
			want: "failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveMarkingStatus(tc.event)
			if got != tc.want {
				t.Errorf("DeriveMarkingStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCursorEvt_IsZero(t *testing.T) {
	zero := CursorEvt{}
	if !zero.IsZero() {
		t.Error("CursorEvt vazio deve retornar IsZero=true")
	}

	// Cursor com ID não-zero
	nonZero := CursorEvt{ID: 42}
	if nonZero.IsZero() {
		t.Error("CursorEvt com ID=42 não deve retornar IsZero=true")
	}
}
