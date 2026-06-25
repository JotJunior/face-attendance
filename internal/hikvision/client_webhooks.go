package hikvision

// client_webhooks.go implements ISAPI webhook (HTTP notification host) operations: list and delete.
// SOURCED: legacy/hik-api/app/Service/HikVision/Notification/NotificationService.php
// Endpoints verified:
//   GET    /ISAPI/Event/notification/httpHosts          (NotificationService.php:58)
//   DELETE /ISAPI/Event/notification/httpHosts/{id}    (NotificationService.php:99)

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// WebhookHost is a single HTTP notification host configured on the device.
// SOURCED: NotificationService.php:parseWebhookConfig (L397-429).
type WebhookHost struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Protocol string `json:"protocol"` // "HTTP"
}

// httpHostsXML is the XML envelope returned by GET /ISAPI/Event/notification/httpHosts.
// The ISAPI returns XML (not JSON) for this endpoint on HikVision access control firmware.
type httpHostsXML struct {
	XMLName xml.Name    `xml:"HttpHostNotificationList"`
	Hosts   []hostEntry `xml:"HttpHostNotification"`
}

type hostEntry struct {
	ID           string `xml:"id"`
	URL          string `xml:"url"`
	ProtocolType string `xml:"protocolType"`
}

// ListWebhooks retrieves all HTTP notification hosts configured on the device.
// SOURCED: NotificationService.php:397-429 (parseWebhookConfig).
// Returns empty slice (not nil) if no webhooks are configured.
func (c *Client) ListWebhooks(ctx context.Context) ([]WebhookHost, error) {
	respBody, status, err := c.doRequest(ctx, http.MethodGet,
		"/ISAPI/Event/notification/httpHosts", nil, "")
	if err != nil {
		return nil, fmt.Errorf("hikvision: ListWebhooks: %w", err)
	}
	if status == 404 {
		// Some firmware versions return 404 when no hosts are configured
		return []WebhookHost{}, nil
	}
	if status != 200 {
		return nil, retriableOrNot("ListWebhooks", status, respBody)
	}

	var list httpHostsXML
	if err := xml.Unmarshal(respBody, &list); err != nil {
		return nil, fmt.Errorf("hikvision: ListWebhooks XML parse: %w (body: %.120s)", err, string(respBody))
	}

	hosts := make([]WebhookHost, 0, len(list.Hosts))
	for _, h := range list.Hosts {
		hosts = append(hosts, WebhookHost{
			ID:       h.ID,
			URL:      h.URL,
			Protocol: h.ProtocolType,
		})
	}
	return hosts, nil
}

// ProvisionWebhook (re)configura o HTTP notification host do device para que ele
// POSTe os eventos no endereço público do app (destrava devices que não dão
// heartbeat por estarem sem httpHost ou apontando para o lugar errado).
//
// Contrato SOURCED de legacy/hik2go (Notification.php:7-40), preferido sobre o
// legacy/hik-api porque casa com o parser de eventos do projeto:
//   - PUT /ISAPI/Event/notification/httpHosts com envelope <HttpHostNotificationList>
//     (substitui a lista — idempotente, sem acumular hosts; Princípio II).
//   - parameterFormatType=JSON: o device passa a postar JSON, que é o que o
//     handler de webhook deste projeto parseia (XML quebraria o parse).
// O id é deterministicHostID(device.Host) — estável por device e consistente com
// a detecção do "webhook principal" no DeleteWebhook.
//
// ipAddress/port/path são o ALVO público (onde o device posta): IP do app na LAN,
// porta do servidor HTTP e /webhook/{secret}. Aceita 200/201.
//
// NOTA DE VALIDAÇÃO: o verbo (PUT) e o envelope vêm do hik2go (lib funcional p/
// estes leitores); o legacy/hik-api usava POST + elemento solto. Confirmar contra
// o firmware real (esp. DS-K1T681DBX) na 1ª execução — se 405/methodNotAllowed,
// reavaliar verbo.
func (c *Client) ProvisionWebhook(ctx context.Context, ipAddress string, port int, path string) error {
	if port <= 0 {
		port = 80
	}
	id := deterministicHostID(c.device.Host)

	xmlBody := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<HttpHostNotificationList version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">`+
			`<HttpHostNotification>`+
			`<id>%s</id>`+
			`<url>%s</url>`+
			`<protocolType>HTTP</protocolType>`+
			`<parameterFormatType>JSON</parameterFormatType>`+
			`<addressingFormatType>ipaddress</addressingFormatType>`+
			`<ipAddress>%s</ipAddress>`+
			`<portNo>%d</portNo>`+
			`<httpAuthenticationMethod>none</httpAuthenticationMethod>`+
			`</HttpHostNotification>`+
			`</HttpHostNotificationList>`,
		xmlEscape(id), xmlEscape(path), xmlEscape(ipAddress), port,
	)

	respBody, status, err := c.doRequest(ctx, http.MethodPut, endpointHTTPHosts,
		strings.NewReader(xmlBody), "application/xml")
	if err != nil {
		return fmt.Errorf("hikvision: ProvisionWebhook PUT: %w", err)
	}
	if status == 200 || status == 201 {
		return nil
	}
	return retriableOrNot("ProvisionWebhook PUT", status, respBody)
}

// DeleteWebhook removes the HTTP notification host with the given ID from the device.
// SOURCED: NotificationService.php:92-123 — DELETE /ISAPI/Event/notification/httpHosts/{id}.
// Returns nil on 200/204; NonRetriableError on 4xx (including 404 = already removed, idempotent).
func (c *Client) DeleteWebhook(ctx context.Context, webhookID string) error {
	if webhookID == "" {
		return fmt.Errorf("hikvision: DeleteWebhook: webhookID cannot be empty")
	}
	path := "/ISAPI/Event/notification/httpHosts/" + webhookID
	_, status, err := c.doRequest(ctx, http.MethodDelete, path, nil, "")
	if err != nil {
		return fmt.Errorf("hikvision: DeleteWebhook(%q): %w", webhookID, err)
	}
	if status == 200 || status == 204 {
		return nil
	}
	// 404 = already deleted (idempotent — treat as success)
	if status == 404 {
		return nil
	}
	return retriableOrNot(fmt.Sprintf("DeleteWebhook(%q)", webhookID), status, nil)
}
