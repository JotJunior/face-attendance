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
