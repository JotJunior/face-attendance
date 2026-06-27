package flowengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// sendMessageTimeout é o timeout da chamada HTTP de disparo de mensagem.
const sendMessageTimeout = 30 * time.Second

// executeSendMessage implementa o nó send_message (nó tipo 8).
//
// Contrato SOURCED do operador (curl multipart):
//
//	POST <SENDER_URL>  (multipart/form-data)
//	  appkey  = <SENDER_APP_KEY>   (segredo, do .env)
//	  authkey = <SENDER_AUTH_KEY>  (segredo, do .env)
//	  to      = <telefone do destinatário>  (config.to interpolada; ex.: [user.mobile])
//	  message = <texto>                     (config.message_template interpolada)
//
// As credenciais/endpoint vêm do .env (MessageSenderConfig). O destinatário e o
// texto vêm do nó, com variáveis interpoladas a partir do ExecutionContext.
//
// Princípio I (zero fabricação): se a API não estiver configurada (.env ausente),
// o nó falha com erro claro — aciona circuit-break, sem inventar credencial/rota.
// Logging: nunca registrar appkey/authkey/message/to (segredos + PII) — apenas
// códigos de erro genéricos.
func (e *Engine) executeSendMessage(ctx context.Context, node *flow.FlowNode, execCtx flow.ExecutionContext) error {
	var cfg flow.SendMessageConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return fmt.Errorf("send_message: config inválida: %w", err)
	}

	if e.messageSender == nil || e.messageSender.URL == "" {
		return fmt.Errorf("send_message: API de mensagem não configurada (defina SENDER_URL/SENDER_APP_KEY/SENDER_AUTH_KEY no .env)")
	}

	// Interpolar destinatário e mensagem (sem logar o resultado — PII/segredo).
	to := strings.TrimSpace(flow.InterpolateVariables(cfg.To, execCtx))
	message := flow.InterpolateVariables(cfg.MessageTemplate, execCtx)
	if to == "" {
		return fmt.Errorf("send_message: destinatário vazio após interpolação (verifique o campo 'to', ex.: [user.mobile])")
	}

	// Montar multipart/form-data com os 4 campos do contrato.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fields := []struct{ k, v string }{
		{"appkey", e.messageSender.AppKey},
		{"authkey", e.messageSender.AuthKey},
		{"to", to},
		{"message", message},
	}
	for _, f := range fields {
		if err := w.WriteField(f.k, f.v); err != nil {
			// Nunca incluir o valor no erro (pode ser segredo/PII) — só o nome do campo.
			return fmt.Errorf("send_message: montar_form_falhou: campo %s", f.k)
		}
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("send_message: fechar_form_falhou")
	}

	reqCtx, cancel := context.WithTimeout(ctx, sendMessageTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, e.messageSender.URL, &buf)
	if err != nil {
		// Não logar a URL — pode conter credencial em querystring.
		return fmt.Errorf("send_message: criar_requisição_falhou")
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send_message: requisição_falhou")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drena para reuso de conexão

	// Sucesso = 2xx. Outros status acionam circuit-break (sem expor corpo/segredo).
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("send_message: API retornou status %d", resp.StatusCode)
	}
	return nil
}
