package flow

import (
	"testing"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// ptrStr retorna ponteiro para string — helper de teste.
func ptrStr(s string) *string { return &s }

// ptrTime retorna ponteiro para time.Time — helper de teste.
func ptrTime(t time.Time) *time.Time { return &t }

// ptrStatus retorna ponteiro para status string — helper de teste.
func ptrStatus(s string) *string { return &s }

func makeCtx() ExecutionContext {
	mobile := "11999990000"
	ip := "192.168.1.50"
	mac := "AA:BB:CC:DD:EE:FF"
	auth := "authorized"
	dt := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	return ExecutionContext{
		Member: &domain.Member{
			Name:            "João Silva",
			FederalDocument: "12345678901",
			Status:          "active",
			MobileNumber:    &mobile,
		},
		Device: &domain.Device{
			ID:               42,
			DeviceIdentifier: "DS-K1T673DWX",
			IPAddress:        &ip,
			MACAddress:       &mac,
		},
		Event: &domain.AttendanceEvent{
			AttendanceStatus: &auth,
			EventDatetime:    &dt,
		},
	}
}

func TestInterpolate_AllVars(t *testing.T) {
	ctx := makeCtx()
	cases := map[string]string{
		"[user.name]":       "João Silva",
		"[user.document]":   "12345678901",
		"[user.status]":     "active",
		"[user.mobile]":     "11999990000",
		"[device.id]":       "42",
		"[device.identifier]": "DS-K1T673DWX",
		"[device.ip]":       "192.168.1.50",
		"[device.mac]":      "AA:BB:CC:DD:EE:FF",
		"[event.authorized]": "true",
		"[event.datetime]":  "2025-03-15T10:30:00Z",
	}
	for tmpl, want := range cases {
		got := InterpolateVariables(tmpl, ctx)
		if got != want {
			t.Errorf("InterpolateVariables(%q) = %q, quero %q", tmpl, got, want)
		}
	}
}

func TestInterpolate_MissingVar(t *testing.T) {
	// member sem mobile
	ctx := ExecutionContext{
		Member: &domain.Member{Name: "Ana"},
		Device: &domain.Device{ID: 1, DeviceIdentifier: "dev1"},
		Event:  &domain.AttendanceEvent{},
	}
	got := InterpolateVariables("Cel: [user.mobile]", ctx)
	if got != "Cel: " {
		t.Errorf("variável ausente deve render vazio, obteve: %q", got)
	}
}

func TestInterpolate_UnknownVar(t *testing.T) {
	ctx := makeCtx()
	// variável fora do vocabulário → render como ""
	got := InterpolateVariables("X=[foo.bar]", ctx)
	if got != "X=" {
		t.Errorf("variável fora do vocabulário deve render vazio, obteve: %q", got)
	}
}

func TestInterpolate_InvalidSyntax(t *testing.T) {
	ctx := makeCtx()
	// [123nao] não casa com o padrão ([a-z][a-z0-9._]*]) → preservado literalmente
	got := InterpolateVariables("ok=[123nao]", ctx)
	if got != "ok=[123nao]" {
		t.Errorf("sintaxe inválida deve ser preservada literalmente, obteve: %q", got)
	}
}

func TestInterpolate_Unauthorized(t *testing.T) {
	status := "denied"
	ctx := ExecutionContext{
		Member: nil,
		Device: &domain.Device{ID: 1, DeviceIdentifier: "d"},
		Event:  &domain.AttendanceEvent{AttendanceStatus: &status},
	}
	got := InterpolateVariables("[event.authorized]", ctx)
	if got != "false" {
		t.Errorf("status denied deve render false, obteve: %q", got)
	}
}
