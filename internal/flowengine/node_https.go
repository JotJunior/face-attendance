package flowengine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jotjunior/face-attendance/internal/flow"
)

// sealedSentinel é o valor-sentinela que indica que o header real está cifrado
// em Flow.SealedConfig (tasks.md §3.8, migration 000009).
const sealedSentinel = "__sealed__"

// privateRanges são as faixas de IP bloqueadas pelo guarda SSRF (tasks.md §3.3.3).
// Compiladas em init() para evitar parse repetido a cada chamada.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",      // RFC 1918 classe A
		"172.16.0.0/12",   // RFC 1918 classe B
		"192.168.0.0/16",  // RFC 1918 classe C
		"169.254.0.0/16",  // IPv4 link-local (APIPA)
		"127.0.0.0/8",     // IPv4 loopback
		"100.64.0.0/10",   // CGNAT (RFC 6598)
		"::1/128",         // IPv6 loopback
		"fc00::/7",        // IPv6 ULA (private)
		"fe80::/10",       // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			privateRanges = append(privateRanges, network)
		}
	}
}

// isPrivateIP verifica se o IP pertence a alguma faixa bloqueada.
func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// checkSSRF resolve o hostname da URL e verifica se algum IP resolvido está
// em faixa privada/loopback/link-local. Retorna erro se bloqueado (CHK001).
// Ref: tasks.md §3.3.3, security CHK001.
func checkSSRF(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("https_call: URL inválida: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("https_call: URL sem host")
	}

	// Se o host já é um IP literal, verificar diretamente.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("https_call: SSRF bloqueado: alvo em range proibido (%s)", ip)
		}
		return nil
	}

	// Resolver hostname para IPs e verificar todos.
	addrs, err := net.LookupHost(host)
	if err != nil {
		// Falha de resolução não bloqueia — o erro virá na chamada HTTP.
		return nil
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("https_call: SSRF bloqueado: alvo em range proibido (%s → %s)", host, ip)
		}
	}
	return nil
}

// executeHTTPSCall implementa o nó https_call:
//   - Interpola variáveis no body e nos headers (incluindo decifração de selados).
//   - Aplica guarda SSRF antes de disparar a requisição.
//   - Aceita qualquer status HTTP (FR-014).
//   - Loga apenas error_code — sem URL com parâmetros nem body (task 3.9.3).
//
// Ref: tasks.md §3.3, plan.md §3.3.
func (e *Engine) executeHTTPSCall(
	ctx context.Context,
	node *flow.FlowNode,
	execCtx flow.ExecutionContext,
	snapshot *flow.Flow,
) error {
	var cfg flow.HTTPSCallConfig
	if err := json.Unmarshal(node.Config, &cfg); err != nil {
		return fmt.Errorf("https_call: config inválida: %w", err)
	}

	// Timeout por nó: default 30s, cap 300s (CL-005, tasks.md §3.3.2).
	timeout := cfg.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}

	method := cfg.Method
	if method == "" {
		method = "POST"
	}

	// Interpolar variáveis no body (sem logar o resultado — task 3.9.3).
	body := flow.InterpolateVariables(cfg.Body, execCtx)

	// Guarda SSRF: resolver hostname e bloquear IPs internos (task 3.3.3).
	// Usa e.ssrfChecker (injetável) em vez de chamar checkSSRF diretamente,
	// para permitir substituição em testes sem comprometer a defesa de produção.
	if err := e.ssrfChecker(cfg.URL); err != nil {
		return err
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, cfg.URL, strings.NewReader(body))
	if err != nil {
		// Logar apenas error_code, sem URL (pode conter parâmetros sensíveis — task 3.9.3).
		return fmt.Errorf("https_call: criar_requisição_falhou")
	}

	// Processar headers: interpolar variáveis e decifrar valores selados (task 3.8.3).
	for headerName, headerValue := range cfg.Headers {
		resolvedValue, err := e.resolveHeaderValue(node.ID, headerName, headerValue, execCtx, snapshot)
		if err != nil {
			// Não logar o valor real — logar apenas o nome do header (sem conteúdo).
			return fmt.Errorf("https_call: resolver_header_falhou: %s", headerName)
		}
		req.Header.Set(headerName, resolvedValue)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		// Logar apenas error_code — sem URL nem body (task 3.9.3).
		return fmt.Errorf("https_call: requisição_falhou")
	}
	defer resp.Body.Close()
	// Drenar body para reutilizar conexão HTTP (tasks.md §3.3.2).
	_, _ = io.Copy(io.Discard, resp.Body)

	// Aceitar qualquer status HTTP (FR-014) — não há tratamento de erro por status.
	return nil
}

// resolveHeaderValue retorna o valor final de um header após interpolação e decifração.
// Se o valor for o sentinela "__sealed__", busca o blob cifrado em snapshot.SealedConfig
// e decifra via e.cipher. Caso contrário, interpola variáveis normalmente.
// Ref: tasks.md §3.8.3.
func (e *Engine) resolveHeaderValue(
	nodeID, headerName, headerValue string,
	execCtx flow.ExecutionContext,
	snapshot *flow.Flow,
) (string, error) {
	if headerValue != sealedSentinel {
		// Header comum: interpolar variáveis.
		return flow.InterpolateVariables(headerValue, execCtx), nil
	}

	// Header selado: decifrar via sealed_config.
	if e.cipher == nil {
		return "", fmt.Errorf("cipher não configurado; não é possível decifrar header selado")
	}
	if snapshot.SealedConfig == nil {
		return "", fmt.Errorf("sealed_config ausente no fluxo para header selado")
	}

	// Chave em sealed_config: "<node_id>.<header_name>" (tasks.md §3.8.1).
	sealedKey := nodeID + "." + headerName
	encoded, ok := snapshot.SealedConfig[sealedKey]
	if !ok {
		return "", fmt.Errorf("sealed_config: chave '%s' não encontrada", sealedKey)
	}

	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("sealed_config: base64 inválido para '%s'", sealedKey)
	}

	plaintext, err := e.cipher.Decrypt(blob)
	if err != nil {
		// Não incluir detalhes do blob no erro — pode conter material sensível.
		return "", fmt.Errorf("sealed_config: decifração falhou para '%s'", sealedKey)
	}

	// Nunca logar o valor decifrado (task 3.8.3, 3.9.3).
	return plaintext, nil
}
