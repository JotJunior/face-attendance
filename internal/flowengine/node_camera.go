package flowengine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
)

// node_camera.go implementa os nós de leitor facial (camera_on / camera_off).
//
// "Ligar/desligar a câmera" significa HABILITAR/DESABILITAR O LEITOR FACIAL — não
// liga/desliga hardware. No HikVision (e no port hik2go) isso é feito alterando o
// verifyMode aceito pelo leitor (VerifyWeekPlanCfg) e o modo de exibição da tela
// (IdentityTerminal/showMode):
//
//   - habilitar  → verifyMode "cardOrFace" (face aceita)  + showMode "normal"
//   - desabilitar→ verifyMode "card"       (face recusada) + showMode "full" (standby/advertising)
//
// SOURCED: legacy/hik2go/examples/1-device/face-enable.php e face-disable.php:
//   face-enable  = AuthMode->update('cardOrFace') + IdentityTerminal->update(show_mode='normal')
//   face-disable = AuthMode->update('card')       + IdentityTerminal->update(show_mode='full')
//
// Reusa operações já verificadas e testadas do cliente ISAPI:
//   - Client.SetVerifyMode      (internal/hikvision/client_authmode.go) — RMW de VerifyWeekPlanCfg
//   - Client.GetIdentityTerminal/PutIdentityTerminal (client_display.go) — RMW de showMode
//
// O operador pode sobrescrever o verifyMode por nó via CameraConfig.verify_mode.

const (
	// defaultVerifyModeOn é o verifyMode default ao habilitar o leitor facial.
	// SOURCED: face-enable.php → AuthMode->update('cardOrFace').
	defaultVerifyModeOn = "cardOrFace"

	// defaultVerifyModeOff é o verifyMode default ao desabilitar o leitor facial.
	// SOURCED: face-disable.php → AuthMode->update('card').
	defaultVerifyModeOff = "card"
)

// resolveVerifyMode retorna o verifyMode efetivo: o configurado no nó quando
// presente; caso contrário, o default do tipo de nó (fallback).
func resolveVerifyMode(cfg flow.CameraConfig, fallback string) string {
	if cfg.VerifyMode != "" {
		return cfg.VerifyMode
	}
	return fallback
}

// resolveShowMode retorna o showMode efetivo: o configurado no nó (normal/full/split)
// quando presente; caso contrário, o default do tipo de nó (fallback).
func resolveShowMode(cfg flow.CameraConfig, fallback string) string {
	if cfg.ShowMode != "" {
		return cfg.ShowMode
	}
	return fallback
}

// executeCameraOn habilita o leitor facial do device (nó camera_on).
func (e *Engine) executeCameraOn(ctx context.Context, node *flow.FlowNode, device *domain.Device) error {
	return e.applyFaceReaderState(ctx, node, device, defaultVerifyModeOn, "normal")
}

// executeCameraOff desabilita o leitor facial do device (nó camera_off).
func (e *Engine) executeCameraOff(ctx context.Context, node *flow.FlowNode, device *domain.Device) error {
	return e.applyFaceReaderState(ctx, node, device, defaultVerifyModeOff, "full")
}

// applyFaceReaderState aplica verifyMode + showMode ao device.
// Idempotente (Constituição §II): SetVerifyMode e PutIdentityTerminal são
// read-modify-write idempotentes; reexecutar com o mesmo estado não altera nada.
// Os timeouts de tela atuais são preservados (lidos via GetIdentityTerminal e
// repassados ao PutIdentityTerminal) — só o showMode é alterado.
func (e *Engine) applyFaceReaderState(
	ctx context.Context,
	node *flow.FlowNode,
	device *domain.Device,
	fallbackVerifyMode, showMode string,
) error {
	var cfg flow.CameraConfig
	if len(node.Config) > 0 {
		if err := json.Unmarshal(node.Config, &cfg); err != nil {
			return fmt.Errorf("%s: config inválida: %w", node.Type, err)
		}
	}
	verifyMode := resolveVerifyMode(cfg, fallbackVerifyMode)
	showMode = resolveShowMode(cfg, showMode)

	hikClient, err := e.hikClientFor(device)
	if err != nil {
		return fmt.Errorf("%s: cliente ISAPI: %w", node.Type, err)
	}

	// 1) verifyMode — aceita/retira face dos modos de verificação.
	// SOURCED: client_authmode.go SetVerifyMode (AuthMode->update($mode)).
	if err := hikClient.SetVerifyMode(ctx, verifyMode); err != nil {
		return fmt.Errorf("%s: SetVerifyMode(%s): %w", node.Type, verifyMode, err)
	}

	// 2) showMode — normal (reconhecimento) vs full (standby/advertising),
	// preservando os timeouts de tela atuais do device.
	// SOURCED: client_display.go GetIdentityTerminal/PutIdentityTerminal
	// (IdentityTerminal->update(show_mode=...)).
	disp, err := hikClient.GetIdentityTerminal(ctx)
	if err != nil {
		return fmt.Errorf("%s: GetIdentityTerminal: %w", node.Type, err)
	}
	if err := hikClient.PutIdentityTerminal(
		ctx,
		disp.ScreenOffTimeout,
		disp.PreviewShowTime,
		disp.StandbyTimeout,
		showMode,
	); err != nil {
		return fmt.Errorf("%s: PutIdentityTerminal(%s): %w", node.Type, showMode, err)
	}

	return nil
}
