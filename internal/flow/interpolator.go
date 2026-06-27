package flow

import (
	"fmt"
	"regexp"

	"github.com/jotjunior/face-attendance/internal/domain"
)

// ExecutionContext contém os dados disponíveis para interpolação de variáveis
// durante a execução de um fluxo.
// Ref: docs/specs/face-flow/data-model.md §Contexto de execução
type ExecutionContext struct {
	Member *domain.Member          // nil se membro não encontrado
	Device *domain.Device          // sempre presente (gatilho é do device)
	Event  *domain.AttendanceEvent // sempre presente
}

// varPattern captura ocorrências de [variavel] onde variavel começa com letra
// minúscula e contém apenas letras minúsculas, dígitos, underscores e pontos.
var varPattern = regexp.MustCompile(`\[([a-z][a-z0-9._]*)\]`)

// InterpolateVariables substitui ocorrências de [variavel] no template pelos
// valores do ExecutionContext.
//
// Vocabulário fechado (10 variáveis — data-model.md §Contexto de execução):
//   - [user.name], [user.document], [user.status], [user.mobile]
//   - [device.id], [device.identifier], [device.ip], [device.mac]
//   - [event.authorized], [event.datetime]
//
// Variável ausente no contexto → substituída por "".
// Sintaxe inválida (não casa com o padrão) → preservada literalmente.
func InterpolateVariables(template string, ctx ExecutionContext) string {
	vars := buildVarMap(ctx)
	return varPattern.ReplaceAllStringFunc(template, func(match string) string {
		// Extrai o nome da variável (sem os colchetes).
		sub := varPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		if val, ok := vars[name]; ok {
			return val
		}
		// Variável fora do vocabulário ou ausente no contexto → "".
		return ""
	})
}

// buildVarMap constrói o mapa de variáveis a partir do contexto de execução.
func buildVarMap(ctx ExecutionContext) map[string]string {
	m := make(map[string]string, 10)

	// Variáveis de usuário (só quando Member != nil).
	if ctx.Member != nil {
		m["user.name"] = ctx.Member.Name
		m["user.document"] = ctx.Member.FederalDocument
		m["user.status"] = ctx.Member.Status
		if ctx.Member.MobileNumber != nil {
			m["user.mobile"] = *ctx.Member.MobileNumber
		} else {
			m["user.mobile"] = ""
		}
	} else {
		m["user.name"] = ""
		m["user.document"] = ""
		m["user.status"] = ""
		m["user.mobile"] = ""
	}

	// Variáveis de device (sempre presente).
	if ctx.Device != nil {
		m["device.id"] = fmt.Sprintf("%d", ctx.Device.ID)
		m["device.identifier"] = ctx.Device.DeviceIdentifier
		if ctx.Device.IPAddress != nil {
			m["device.ip"] = *ctx.Device.IPAddress
		} else {
			m["device.ip"] = ""
		}
		if ctx.Device.MACAddress != nil {
			m["device.mac"] = *ctx.Device.MACAddress
		} else {
			m["device.mac"] = ""
		}
	} else {
		m["device.id"] = ""
		m["device.identifier"] = ""
		m["device.ip"] = ""
		m["device.mac"] = ""
	}

	// Variáveis de evento (sempre presente).
	if ctx.Event != nil {
		if ctx.Event.AttendanceStatus != nil && *ctx.Event.AttendanceStatus == "authorized" {
			m["event.authorized"] = "true"
		} else {
			m["event.authorized"] = "false"
		}
		if ctx.Event.EventDatetime != nil {
			m["event.datetime"] = ctx.Event.EventDatetime.UTC().Format("2006-01-02T15:04:05Z")
		} else {
			m["event.datetime"] = ""
		}
	} else {
		m["event.authorized"] = "false"
		m["event.datetime"] = ""
	}

	return m
}
