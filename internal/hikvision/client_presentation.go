package hikvision

// client_presentation.go implementa a troca da imagem de presentation (start-page)
// para um material JÁ existente no device, e o ajuste do show_mode.
//
// SetPresentationPage — CONTRATO REAL verificado no device DS-K1T673*
//   (GET /ISAPI/Publish/ProgramMgr/program/1/page/1): a imagem EXIBIDA é o
//   <materialNo> do PlayItem dentro de WindowsList/Windows/PlayItemList — NÃO existe
//   <backgroundPic> neste firmware (o port legado do switch.php usava backgroundPic e
//   NÃO trocava a imagem). O programa 1 e o schedule JÁ existem no device (configurados
//   uma vez); trocar a imagem = PUT page/1 mudando o materialNo. Position usa
//   uniformCoordinate (0..1920) → independente do show_mode.
//
// SetShowMode — read-modify-write sobre IdentityTerminal (client_display.go), igual aos
//   exemplos 1-split.php / 2-full.php, que setam o show_mode junto com a imagem.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// presentationPageXMLFmt espelha EXATAMENTE a estrutura de program/1/page/1 retornada
// pelo device (GET), trocando apenas o materialNo. %s = id do material.
const presentationPageXMLFmt = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<Page version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">` +
	`<id>1</id>` +
	`<PageBasicInfo>` +
	`<pageName>1</pageName>` +
	`<BackgroundColor><RGB>16777215</RGB></BackgroundColor>` +
	`<switchDuration>1</switchDuration>` +
	`<switchEffect>none</switchEffect>` +
	`</PageBasicInfo>` +
	`<WindowsList><Windows>` +
	`<id>1</id>` +
	`<Position><positionX>0</positionX><positionY>0</positionY><height>1920</height><width>1920</width></Position>` +
	`<layerNo>1</layerNo>` +
	`<WinMaterialInfo><materialType>static</materialType><staticMaterialType>picture</staticMaterialType></WinMaterialInfo>` +
	`<PlayItemList><PlayItem>` +
	`<id>1</id>` +
	`<materialNo>%s</materialNo>` +
	`<playEffect>none</playEffect>` +
	`<PlayDuration><durationType>materialTime</durationType><duration>1</duration></PlayDuration>` +
	`</PlayItem></PlayItemList>` +
	`</Windows></WindowsList>` +
	`</Page>`

// SetPresentationPage troca a imagem exibida (program 1, página 1) para o material
// materialID, via PUT page/1 mudando o materialNo. Pressupõe que o programa de
// presentation já existe no device (configurado uma vez). CONTRATO REAL (ver topo).
func (c *Client) SetPresentationPage(ctx context.Context, materialID string) error {
	if materialID == "" {
		return fmt.Errorf("hikvision: SetPresentationPage: materialID vazio")
	}
	pageXML := fmt.Sprintf(presentationPageXMLFmt, xmlEscape(materialID))

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

// ApplyPresentation troca a imagem de presentation para o material materialID e ajusta
// o show_mode (derivado do tamanho da imagem) — espelha 1-split.php / 2-full.php.
func (c *Client) ApplyPresentation(ctx context.Context, materialID, mode string) error {
	if err := c.SetShowMode(ctx, mode); err != nil {
		return err
	}
	return c.SetPresentationPage(ctx, materialID)
}
