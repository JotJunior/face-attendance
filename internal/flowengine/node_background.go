package flowengine

import (
	"context"
	"encoding/json"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// executeChangeBackground implementa o nó change_background (nó tipo 4):
//  1. Decodifica a config (media_id — mídia já provisionada no device).
//  2. Confirma que a mídia referenciada ainda existe no device.
//  3. Aplica a mídia como imagem de presentation e ajusta o show_mode (full/split).
//
// A imagem NÃO é (re)enviada aqui: ela é gerenciada no device (mídia/MaterialMgr) pelo
// editor de fluxo (modal "Imagens de fundo"). Aplicar = Presentation.page(media_id) +
// show_mode derivado do tamanho — espelha legacy presentation/switch.php + 1-split/2-full.
//
// ISAPI verificado em internal/hikvision/client_presentation.go e client_media.go.
// Ref: tasks.md §3.4.2, plan.md §3.4.
func (e *Engine) executeChangeBackground(
	ctx context.Context,
	node *flow.FlowNode,
	device *domain.Device,
) error {
	var cfg flow.ChangeBackgroundConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return fmt.Errorf("change_background: config inválida: %w", err)
	}
	if cfg.MediaID == "" {
		return fmt.Errorf("change_background: media_id não configurado")
	}

	mode := cfg.Mode
	if mode == "" {
		mode = hikvision.ShowModeFull
	}
	name := cfg.Name
	if name == "" {
		name = cfg.MediaID
	}

	hikClient, err := e.hikClientFor(device)
	if err != nil {
		return fmt.Errorf("change_background: cliente ISAPI: %w", err)
	}

	// Confirma que a mídia referenciada ainda está no device antes de aplicar.
	// SOURCED: client_media.go:ListMaterials
	mats, err := hikClient.ListMaterials(ctx)
	if err != nil {
		return fmt.Errorf("change_background: listar mídias do device: %w", err)
	}
	found := false
	for _, m := range mats {
		if m.ID == cfg.MediaID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("change_background: mídia %s não está mais no device", cfg.MediaID)
	}

	// Aplica a mídia como presentation + show_mode.
	// SOURCED: client_presentation.go:ApplyPresentation
	if err := hikClient.ApplyPresentation(ctx, cfg.MediaID, name, mode); err != nil {
		return fmt.Errorf("change_background: aplicar presentation: %w", err)
	}

	return nil
}

// executeQRCodeBackground implementa o nó qrcode_background (nó tipo 6):
//  1. Interpola content_template com o ExecutionContext.
//  2. Gera QR code PNG via github.com/skip2/go-qrcode.
//  3. Redimensiona para 600×1024 JPEG (requisito firmware).
//  4. Faz upload via ISAPI StandbyPictureMgr e ativa o modo custom standby.
//
// Ref: tasks.md §3.4.3, plan.md §3.4.
func (e *Engine) executeQRCodeBackground(
	ctx context.Context,
	node *flow.FlowNode,
	execCtx flow.ExecutionContext,
	device *domain.Device,
) error {
	var cfg flow.QRCodeBackgroundConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return fmt.Errorf("qrcode_background: config inválida: %w", err)
	}

	// Interpolar variáveis no template de conteúdo do QR code.
	content := flow.InterpolateVariables(cfg.ContentTemplate, execCtx)

	// Gerar QR code PNG 600×600 px (github.com/skip2/go-qrcode).
	// O tamanho 600 é escolhido para ocupar bem a largura ao redimensionar para 600×1024.
	pngBytes, err := qrcode.Encode(content, qrcode.Medium, 600)
	if err != nil {
		return fmt.Errorf("qrcode_background: gerar QR: %w", err)
	}

	// Redimensionar para 600×1024 JPEG (requisito de firmware).
	// SOURCED: internal/hikvision/client_bootpic.go ResizeImageJPEG.
	jpegData, err := hikvision.ResizeImageJPEG(pngBytes, 600, 1024)
	if err != nil {
		return fmt.Errorf("qrcode_background: redimensionar: %w", err)
	}

	hikClient, err := e.hikClientFor(device)
	if err != nil {
		return fmt.Errorf("qrcode_background: cliente ISAPI: %w", err)
	}

	// Upload + enable standby (mesmo caminho do nó change_background).
	// SOURCED: client_standby.go:UploadStandbyPicture, EnableCustomStandby.
	if err := hikClient.UploadStandbyPicture(ctx, "qrcode.jpg", jpegData); err != nil {
		return fmt.Errorf("qrcode_background: upload ISAPI: %w", err)
	}

	return hikClient.EnableCustomStandby(ctx)
}
