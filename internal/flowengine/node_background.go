package flowengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/hikvision"
)

// executeChangeBackground implementa o nó change_background (nó tipo 4):
//  1. Busca a imagem de fundo pelo ID configurado no repositório.
//  2. Lê o arquivo de disco a partir de bgImagesDir.
//  3. Redimensiona para 600×1024 JPEG (requisito firmware).
//  4. Faz upload via ISAPI StandbyPictureMgr e ativa o modo custom standby.
//
// ISAPI verificado em internal/hikvision/client_standby.go.
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

	img, err := e.bgImageRepo.FindByID(ctx, cfg.ImageID)
	if err != nil {
		return fmt.Errorf("change_background: imagem %d não encontrada: %w", cfg.ImageID, err)
	}

	data, err := os.ReadFile(filepath.Join(e.bgImagesDir, img.FilePath))
	if err != nil {
		return fmt.Errorf("change_background: ler imagem: %w", err)
	}

	hikClient, err := e.hikClientFor(device)
	if err != nil {
		return fmt.Errorf("change_background: cliente ISAPI: %w", err)
	}

	// Redimensionar para 600×1024 JPEG (requisito de firmware — client_bootpic.go).
	// SOURCED: internal/hikvision/client_bootpic.go ResizeImageJPEG.
	jpegData, err := hikvision.ResizeImageJPEG(data, 600, 1024)
	if err != nil {
		return fmt.Errorf("change_background: redimensionar: %w", err)
	}

	// Upload standby picture.
	// SOURCED: client_standby.go:UploadStandbyPicture
	// Endpoint: POST /ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json
	if err := hikClient.UploadStandbyPicture(ctx, img.Name+".jpg", jpegData); err != nil {
		return fmt.Errorf("change_background: upload ISAPI: %w", err)
	}

	// Ativar modo custom standby.
	// SOURCED: client_standby.go:EnableCustomStandby
	// Endpoint: PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json
	if err := hikClient.EnableCustomStandby(ctx); err != nil {
		return fmt.Errorf("change_background: ativar standby: %w", err)
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
