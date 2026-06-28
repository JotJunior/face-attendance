package flow

import (
	"encoding/json"
	"fmt"
	"time"
)

// NodeType enumera os 9 tipos de nó suportados pelo motor de fluxo.
// Ref: docs/specs/face-flow/data-model.md §Tipos de nó
type NodeType string

const (
	NodeTypeStart            NodeType = "start"
	NodeTypeCameraOn         NodeType = "camera_on"          // habilita leitor facial (verifyMode + showMode)
	NodeTypeCameraOff        NodeType = "camera_off"         // desabilita leitor facial (verifyMode + showMode)
	NodeTypeWait             NodeType = "wait"
	NodeTypeChangeBackground NodeType = "change_background"
	NodeTypeHTTPSCall        NodeType = "https_call"
	NodeTypeQRCodeBackground NodeType = "qrcode_background"
	NodeTypeDecision         NodeType = "decision"
	NodeTypeSendMessage      NodeType = "send_message"       // BLOCKED_API
)

// FlowNode representa um nó no grafo de fluxo.
type FlowNode struct {
	ID     string          `json:"id"`
	Type   NodeType        `json:"type"`
	Config json.RawMessage `json:"config"`
	X      float64         `json:"x"`
	Y      float64         `json:"y"`
}

// FlowEdge representa uma conexão direcional entre dois nós.
// O campo Label só é relevante em edges que saem de um nó decision
// ("valid" ou "invalid"). Para todos os demais tipos, Label é string vazia.
type FlowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"` // "valid"|"invalid" para nó decision; "" para demais
}

// Flow representa um fluxo completo configurado pelo admin.
type Flow struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Status    string     `json:"status"` // "active" | "inactive"
	DeviceID  *int64     `json:"device_id,omitempty"`
	Nodes     []FlowNode `json:"nodes"`
	Edges     []FlowEdge `json:"edges"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// SealedConfig armazena segredos cifrados (AES-256-GCM) de headers sensíveis do nó https_call.
	// Chave: "<node_id>.<header_name>" — valor: bytes cifrados codificados em base64.
	// Persistido em flows.sealed_config (JSONB). Nunca serializado em JSON de resposta da API
	// (o handler mascara com "__secret__:***" antes de retornar — tasks.md §3.8.2).
	// Ref: tasks.md §3.8, migration 000009.
	SealedConfig map[string]string `json:"sealed_config,omitempty"`
}

// Configs tipadas para decode seguro no motor de execução:

// WaitConfig é a configuração do nó wait.
type WaitConfig struct {
	DurationSeconds int `json:"duration_seconds"`
}

// ChangeBackgroundConfig é a configuração do nó change_background.
// A imagem é uma MÍDIA (presentation/start-page) já provisionada no device selecionado;
// o nó a referencia por media_id. Mode (full/split) deriva do tamanho e é reaplicado
// ao executar. Name é só para exibição/programName.
type ChangeBackgroundConfig struct {
	MediaID string `json:"media_id"`
	Mode    string `json:"mode,omitempty"`
	Name    string `json:"name,omitempty"`
}

// HTTPSCallConfig é a configuração do nó https_call.
type HTTPSCallConfig struct {
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"` // default 30; cap 300
}

// QRCodeBackgroundConfig é a configuração do nó qrcode_background.
type QRCodeBackgroundConfig struct {
	ContentTemplate string `json:"content_template"`
}

// SendMessageConfig é a configuração do nó send_message.
// To é o destinatário (telefone) — suporta variáveis (ex.: "[user.mobile]").
// MessageTemplate é o texto, também com variáveis. As credenciais/endpoint da API
// (appkey/authkey/URL) vêm do .env (instance-level), não do fluxo.
// SOURCED: contrato multipart fornecido pelo operador (appkey/authkey/to/message).
type SendMessageConfig struct {
	To              string `json:"to"`
	MessageTemplate string `json:"message_template"`
}

// CameraConfig é a configuração dos nós de leitor facial (camera_on / camera_off).
// VerifyMode sobrescreve o verifyMode aplicado a TODOS os slots do VerifyWeekPlanCfg;
// quando vazio, o motor usa o default do tipo de nó (camera_on→"cardOrFace",
// camera_off→"card").
// SOURCED: legacy/hik2go/examples/1-device/face-{enable,disable}.php
//   (AuthMode->update($mode)).
type CameraConfig struct {
	VerifyMode string `json:"verify_mode,omitempty"`
}

// FindNodeByType retorna o primeiro nó do tipo t, ou nil se não encontrado.
func (f *Flow) FindNodeByType(t NodeType) *FlowNode {
	for i := range f.Nodes {
		if f.Nodes[i].Type == t {
			return &f.Nodes[i]
		}
	}
	return nil
}

// FindNodeByID retorna o nó com o ID fornecido, ou nil se não encontrado.
func (f *Flow) FindNodeByID(id string) *FlowNode {
	for i := range f.Nodes {
		if f.Nodes[i].ID == id {
			return &f.Nodes[i]
		}
	}
	return nil
}

// OutgoingEdges retorna todas as edges que partem do nó nodeID.
func (f *Flow) OutgoingEdges(nodeID string) []FlowEdge {
	var out []FlowEdge
	for _, e := range f.Edges {
		if e.From == nodeID {
			out = append(out, e)
		}
	}
	return out
}

// NextNodeID retorna o ID do próximo nó para nós com saída única (não decision).
// Retorna erro se não houver exatamente uma edge de saída.
func (f *Flow) NextNodeID(nodeID string) (string, error) {
	edges := f.OutgoingEdges(nodeID)
	if len(edges) == 0 {
		return "", nil // fim do fluxo
	}
	if len(edges) != 1 {
		return "", fmt.Errorf("nó %q tem %d edges de saída; esperado exatamente 1 para este tipo", nodeID, len(edges))
	}
	return edges[0].To, nil
}

// NextNodeIDByLabel retorna o ID do próximo nó para nós decision, filtrando pelo label.
// Retorna erro se não encontrar edge com o label fornecido.
func (f *Flow) NextNodeIDByLabel(nodeID, label string) (string, error) {
	for _, e := range f.OutgoingEdges(nodeID) {
		if e.Label == label {
			return e.To, nil
		}
	}
	return "", fmt.Errorf("nó decision %q não possui edge com label %q", nodeID, label)
}
