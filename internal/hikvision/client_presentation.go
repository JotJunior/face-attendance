package hikvision

// client_presentation.go implementa a troca da imagem de presentation (start-page)
// para um material JÁ existente no device, e o ajuste do show_mode.
//
// SetPresentationPage — SOURCED de legacy/.../presentation/switch.php (Presentation::page):
//   PUT /ISAPI/Publish/ProgramMgr/program/1/page/1 (XML <Page> com <backgroundPic>{media_id}</backgroundPic>)
//   Troca a imagem do programa 1/página 1 para o material indicado (sem reupload).
//
// SetShowMode — read-modify-write sobre IdentityTerminal (client_display.go), igual aos
//   exemplos 1-split.php / 2-full.php, que setam o show_mode junto com a imagem.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// SetPresentationPage aponta a página 1 do programa 1 para o material mediaID (a
// imagem de presentation passa a ser esse material). name é usado como programName.
// SOURCED: Presentation.php:page() — mesmo XML do passo (d) de CreateAdvertisingMedia.
func (c *Client) SetPresentationPage(ctx context.Context, mediaID, name string) error {
	if mediaID == "" {
		return fmt.Errorf("hikvision: SetPresentationPage: mediaID vazio")
	}
	pageXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<Page version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
		`<id>1</id>`+
		`<programName>%s</programName>`+
		`<programType>normal</programType>`+
		`<PageBasicInfo>`+
		`<pageName>1</pageName>`+
		`<BackgroundColor><RGB>16777215</RGB></BackgroundColor>`+
		`<backgroundPic>%s</backgroundPic>`+
		`<playDurationMode>1</playDurationMode>`+
		`<switchDuration>1</switchDuration>`+
		`<switchEffect>none</switchEffect>`+
		`</PageBasicInfo>`+
		`<WindowsList/>`+
		`</Page>`,
		xmlEscape(name), xmlEscape(mediaID))

	_, status, err := c.doRequest(ctx, http.MethodPut, endpointProgramPage,
		strings.NewReader(pageXML), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: SetPresentationPage PUT: %w", err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	return retriableOrNot("SetPresentationPage PUT", status, nil)
}

// SetShowMode ajusta o show_mode do terminal (normal/full/split) preservando os
// demais campos de display (read-modify-write). Mesmo padrão de PutDeviceDisplayHandler.
func (c *Client) SetShowMode(ctx context.Context, mode string) error {
	cur, err := c.GetIdentityTerminal(ctx)
	if err != nil {
		return fmt.Errorf("hikvision: SetShowMode: ler display: %w", err)
	}
	if err := c.PutIdentityTerminal(ctx, cur.ScreenOffTimeout, cur.PreviewShowTime, cur.StandbyTimeout, mode); err != nil {
		return fmt.Errorf("hikvision: SetShowMode: %w", err)
	}
	return nil
}

// ApplyPresentation troca a imagem de presentation para o material mediaID e ajusta
// o show_mode (derivado do tamanho da imagem) — espelha 1-split.php / 2-full.php.
func (c *Client) ApplyPresentation(ctx context.Context, mediaID, name, mode string) error {
	if err := c.SetShowMode(ctx, mode); err != nil {
		return err
	}
	return c.SetPresentationPage(ctx, mediaID, name)
}
