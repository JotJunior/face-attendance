package flowengine

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jotjunior/face-attendance/internal/domain"
	"github.com/jotjunior/face-attendance/internal/flow"
	"github.com/jotjunior/face-attendance/internal/hikvision"
	"github.com/jotjunior/face-attendance/internal/logging"
	"github.com/jotjunior/face-attendance/internal/repository"
	"github.com/jotjunior/face-attendance/internal/secrets"
)

const (
	// globalTimeout é o tempo máximo de uma execução completa de fluxo.
	// Ref: tasks.md §3.6.2, performance CHK009.
	globalTimeout = 30 * time.Minute

	// defaultSemaphoreSize é o número máximo de goroutines de execução simultâneas.
	// Ref: tasks.md §3.6.1, performance CHK005.
	defaultSemaphoreSize = 10
)

// FlowRepo é o subconjunto de repository.FlowRepository usado pelo engine.
// Interface local para evitar acoplamento desnecessário e facilitar testes.
type FlowRepo interface {
	FindActiveByDeviceID(ctx context.Context, deviceID int64) (*flow.Flow, error)
}

// ExecutionLogRepo é o subconjunto de FlowExecutionLogRepository usado pelo engine.
// Inclui FindByEventKey para o guarda de idempotência (tasks.md §3.7).
type ExecutionLogRepo interface {
	Create(ctx context.Context, log *repository.FlowExecutionLog) error
	FindByEventKey(ctx context.Context, eventKey string) (*repository.FlowExecutionLog, error)
}

// BackgroundImageRepo é o subconjunto de BackgroundImageRepository usado pelo engine.
type BackgroundImageRepo interface {
	FindByID(ctx context.Context, id int64) (*repository.BackgroundImage, error)
}

// MessageSenderConfig são as credenciais/endpoint da API de disparo de mensagem,
// usadas pelo nó send_message. Contrato (multipart): POST URL com campos
// appkey/authkey/to/message. Valores vêm do .env (SENDER_URL/SENDER_APP_KEY/
// SENDER_AUTH_KEY). AppKey/AuthKey são SEGREDOS — nunca devem ser logados.
// SOURCED: contrato fornecido pelo operador (curl multipart).
type MessageSenderConfig struct {
	URL     string
	AppKey  string
	AuthKey string
}

// Config agrupa as dependências do Engine para injeção no construtor.
type Config struct {
	// HikClientFor retorna um cliente ISAPI para o device fornecido.
	// Usado pelos nós change_background e qrcode_background.
	HikClientFor func(device *domain.Device) (*hikvision.Client, error)

	// MessageSender são as credenciais da API de mensagem (nó send_message).
	// Se nil ou com URL vazia, o nó send_message falha com erro claro (não fabrica).
	MessageSender *MessageSenderConfig

	FlowRepo    FlowRepo
	LogRepo     ExecutionLogRepo
	BgImageRepo BackgroundImageRepo

	// BgImagesDir é o diretório base onde as imagens de fundo estão armazenadas.
	BgImagesDir string

	// HTTPClient é o cliente HTTP usado pelo nó https_call.
	// Se nil, um cliente padrão sem timeout extra é criado.
	HTTPClient *http.Client

	// SemaphoreSize limita goroutines de execução simultâneas (default 10).
	SemaphoreSize int

	// Cipher é o cifrador AES-256-GCM usado para decifrar headers selados do nó https_call.
	// Se nil, headers selados com sentinela "__sealed__" não são decifrados (retornam erro).
	// Ref: tasks.md §3.8.3, internal/secrets.
	Cipher *secrets.Cipher

	// SSRFChecker é uma função que valida uma URL bruta antes de disparar a requisição HTTP.
	// Se nil, usa a implementação padrão (checkSSRF) que bloqueia IPs internos/loopback/link-local.
	// Exposto para facilitar testes unitários que usam httptest.Server em loopback.
	// Em produção, NUNCA passar nil intencionalmente sem razão de segurança documentada.
	SSRFChecker func(rawURL string) error

	Logger *logging.Logger
}

// Engine executa fluxos disparados por eventos de reconhecimento facial.
// Ref: docs/specs/face-flow/plan.md §3.1, tasks.md §3.1.
type Engine struct {
	hikClientFor  func(device *domain.Device) (*hikvision.Client, error)
	messageSender *MessageSenderConfig
	flowRepo      FlowRepo
	logRepo       ExecutionLogRepo
	bgImageRepo   BackgroundImageRepo
	bgImagesDir   string
	httpClient    *http.Client
	sema          chan struct{} // semáforo de goroutines (bound de concorrência)
	cipher        *secrets.Cipher
	ssrfChecker   func(rawURL string) error
	logger        *logging.Logger
}

// New cria um Engine com as dependências fornecidas em cfg.
// Ref: tasks.md §3.1.1.
func New(cfg Config) *Engine {
	size := cfg.SemaphoreSize
	if size <= 0 {
		size = defaultSemaphoreSize
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	ssrfFn := cfg.SSRFChecker
	if ssrfFn == nil {
		ssrfFn = checkSSRF // implementação padrão em node_https.go
	}
	return &Engine{
		hikClientFor:  cfg.HikClientFor,
		messageSender: cfg.MessageSender,
		flowRepo:      cfg.FlowRepo,
		logRepo:       cfg.LogRepo,
		bgImageRepo:   cfg.BgImageRepo,
		bgImagesDir:   cfg.BgImagesDir,
		httpClient:    client,
		sema:          make(chan struct{}, size),
		cipher:        cfg.Cipher,
		ssrfChecker:   ssrfFn,
		logger:        cfg.Logger,
	}
}

// TriggerForDevice é chamado pelo webhook handler após processar um AttendanceEvent.
// Retorna imediatamente (não bloqueia) após disparar a goroutine de execução.
//
// Ref: tasks.md §3.1.2, plan.md §3.1 (TriggerForDevice).
func (e *Engine) TriggerForDevice(
	deviceMACAddress string,
	event *domain.AttendanceEvent,
	member *domain.Member,
	device *domain.Device,
) {
	if device == nil || event == nil {
		return
	}

	// Buscar fluxo ativo para o device — passthrough silencioso se não houver (FR-019).
	activeFlow, err := e.flowRepo.FindActiveByDeviceID(context.Background(), device.ID)
	if err != nil || activeFlow == nil {
		return
	}

	// Copiar snapshot do fluxo: edicoes do admin só afetam execuções subsequentes
	// (decisão do clarify — tasks.md §3.1.2).
	snapshot := *activeFlow
	nodesCopy := make([]flow.FlowNode, len(activeFlow.Nodes))
	copy(nodesCopy, activeFlow.Nodes)
	snapshot.Nodes = nodesCopy
	edgesCopy := make([]flow.FlowEdge, len(activeFlow.Edges))
	copy(edgesCopy, activeFlow.Edges)
	snapshot.Edges = edgesCopy
	// SealedConfig é um map — copiar para evitar race condition caso o admin atualize.
	if len(activeFlow.SealedConfig) > 0 {
		sc := make(map[string]string, len(activeFlow.SealedConfig))
		for k, v := range activeFlow.SealedConfig {
			sc[k] = v
		}
		snapshot.SealedConfig = sc
	}

	// Bound de concorrência: tentativa não-bloqueante (task 3.6.1).
	// Se o semáforo estiver cheio, o evento é descartado com log de aviso.
	select {
	case e.sema <- struct{}{}:
	default:
		if e.logger != nil {
			deviceIDStr := fmt.Sprintf("%d", device.ID)
			e.logger.Warn("flowengine.trigger", deviceIDStr, "",
				"bound de concorrência atingido — evento descartado",
				"flow_id", snapshot.ID,
			)
		}
		return
	}

	go func() {
		defer func() { <-e.sema }()
		e.execute(context.Background(), &snapshot, event, member, device)
	}()
}

// execute é o coração do motor: percorre o grafo do fluxo nó a nó.
// Qualquer erro ou timeout dispara circuit-break e reseta o estado (FR-021/FR-022).
// Ref: tasks.md §3.1.3, plan.md §3.1 (execute).
func (e *Engine) execute(
	ctx context.Context,
	snapshot *flow.Flow,
	event *domain.AttendanceEvent,
	member *domain.Member,
	device *domain.Device,
) {
	start := time.Now()

	// Timeout global — circuit-break registra "timeout global atingido" (task 3.6.2).
	ctx, cancel := context.WithTimeout(ctx, globalTimeout)
	defer cancel()

	deviceIDStr := fmt.Sprintf("%d", device.ID)

	// Guarda de idempotência: se event_key já foi completado, skip silencioso (task 3.7.1).
	// at-least-once com deduplicação por event_key; side-effects não se repetem se a
	// execução anterior concluiu com sucesso (Constituição §II, spec FR-023).
	if event.EventKey != "" && e.logRepo != nil {
		existing, err := e.logRepo.FindByEventKey(ctx, event.EventKey)
		if err == nil && existing != nil && existing.Status == "completed" {
			if e.logger != nil {
				e.logger.Info("flowengine.execute", deviceIDStr, "",
					"event_key já completado — skip idempotente",
					"event_key", event.EventKey,
				)
			}
			return
		}
	}

	execCtx := flow.ExecutionContext{
		Member: member,
		Device: device,
		Event:  event,
	}

	// Validar snapshot antes de executar (FR-022).
	if errs := flow.Validate(snapshot); len(errs) > 0 {
		e.circuitBreak(ctx, snapshot, device, event,
			"", fmt.Errorf("validação do fluxo falhou: %v", errs), start)
		return
	}

	startNode := snapshot.FindNodeByType(flow.NodeTypeStart)
	if startNode == nil {
		e.circuitBreak(ctx, snapshot, device, event,
			"", fmt.Errorf("nó start não encontrado no snapshot"), start)
		return
	}

	currentNodeID := startNode.ID

	// decisionValue é o RESULTADO AVALIÁVEL corrente que um nó de decisão consome.
	// Default: reconhecimento facial (evento autorizado). É sobrescrito pelo
	// resultado do nó anterior quando este produz um booleano — hoje, o https_call
	// (true se 200-204, false caso contrário). Assim a decisão ramifica tanto por
	// "face não reconhecida" quanto por "webhook não retornou 200-204".
	decisionValue := event != nil && event.IsAuthorized()

	for {
		// Verificar cancelamento/timeout antes de cada nó.
		if ctx.Err() != nil {
			e.circuitBreak(ctx, snapshot, device, event,
				currentNodeID, fmt.Errorf("timeout global atingido"), start)
			return
		}

		node := snapshot.FindNodeByID(currentNodeID)
		if node == nil {
			e.circuitBreak(ctx, snapshot, device, event,
				currentNodeID,
				fmt.Errorf("nó '%s' não encontrado no snapshot", currentNodeID),
				start)
			return
		}

		result, err := e.executeNode(ctx, node, execCtx, device, snapshot)
		if err != nil {
			e.circuitBreak(ctx, snapshot, device, event, node.ID, err, start)
			return
		}
		if result != nil {
			decisionValue = *result // nó produziu booleano (ex.: https_call 200-204)
		}

		nextID, err := nextNodeFor(snapshot, node, decisionValue)
		if err != nil {
			e.circuitBreak(ctx, snapshot, device, event, node.ID, err, start)
			return
		}
		if nextID == "" {
			break // fim do fluxo
		}
		currentNodeID = nextID
	}

	e.logCompleted(ctx, snapshot, device, event, start)
}

// executeNode despacha a execução de um nó pelo seu tipo.
// snapshot é passado para acesso ao SealedConfig (nó https_call com segredos).
// executeNode executa o nó e retorna, opcionalmente, um RESULTADO AVALIÁVEL
// (*bool) que alimenta o próximo nó de decisão. Hoje só o nó https_call produz
// esse resultado (true se a resposta foi 200-204; false caso contrário); os
// demais nós retornam nil (não alteram o valor de decisão corrente).
func (e *Engine) executeNode(
	ctx context.Context,
	node *flow.FlowNode,
	execCtx flow.ExecutionContext,
	device *domain.Device,
	snapshot *flow.Flow,
) (*bool, error) {
	switch node.Type {
	case flow.NodeTypeStart:
		return nil, nil // nó start: nenhuma ação, apenas ponto de entrada
	case flow.NodeTypeWait:
		return nil, e.executeWait(ctx, node)
	case flow.NodeTypeHTTPSCall:
		return e.executeHTTPSCall(ctx, node, execCtx, snapshot)
	case flow.NodeTypeChangeBackground:
		return nil, e.executeChangeBackground(ctx, node, device)
	case flow.NodeTypeQRCodeBackground:
		return nil, e.executeQRCodeBackground(ctx, node, execCtx, device)
	case flow.NodeTypeDecision:
		return nil, nil // decision: nenhuma ação; nextNodeFor cuida do roteamento pelo label
	case flow.NodeTypeCameraOn:
		return nil, e.executeCameraOn(ctx, node, device)
	case flow.NodeTypeCameraOff:
		return nil, e.executeCameraOff(ctx, node, device)
	case flow.NodeTypeSendMessage:
		return nil, e.executeSendMessage(ctx, node, execCtx)
	default:
		return nil, fmt.Errorf("tipo de nó desconhecido: %q", node.Type)
	}
}

// nextNodeFor determina o ID do próximo nó a ser executado.
// Para nós decision, consulta AttendanceEvent.IsAuthorized() para escolher o ramo.
// nextNodeFor resolve o próximo nó. Para o nó de decisão, ramifica pelo
// decisionValue corrente (resultado do nó anterior): true → aresta "valid",
// false → aresta "invalid".
func nextNodeFor(snapshot *flow.Flow, node *flow.FlowNode, decisionValue bool) (string, error) {
	switch node.Type {
	case flow.NodeTypeDecision:
		if decisionValue {
			return snapshot.NextNodeIDByLabel(node.ID, "valid")
		}
		return snapshot.NextNodeIDByLabel(node.ID, "invalid")
	default:
		return snapshot.NextNodeID(node.ID)
	}
}

// circuitBreak loga o erro e persiste FlowExecutionLog com status "circuit_break".
// Ignora violação de UNIQUE em event_key (execução concorrente — FR-023).
// Ref: tasks.md §3.1.4, 3.9.2.
func (e *Engine) circuitBreak(
	ctx context.Context,
	snapshot *flow.Flow,
	device *domain.Device,
	event *domain.AttendanceEvent,
	nodeID string,
	err error,
	start time.Time,
) {
	deviceIDStr := fmt.Sprintf("%d", device.ID)

	// Mascarar CPF — nunca logar CPF em claro (task 3.9.2, Constituição §logging).
	cpfRaw := ""
	if event != nil && event.FederalDocument != nil {
		cpfRaw = *event.FederalDocument
	}

	// Logar apenas flow_id, device_id, node_id, error_code — sem body interpolado
	// nem headers de autenticação (task 3.9.2).
	if e.logger != nil {
		e.logger.Error("flowengine.circuit_break", deviceIDStr, cpfRaw,
			"circuit-break: execução interrompida",
			err,
			"flow_id", snapshot.ID,
			"node_id", nodeID,
		)
	}

	if e.logRepo == nil || event == nil {
		return
	}

	now := time.Now()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	var failedNodeID *string
	if nodeID != "" {
		s := nodeID
		failedNodeID = &s
	}

	logEntry := &repository.FlowExecutionLog{
		FlowID:       snapshot.ID,
		DeviceID:     device.ID,
		EventKey:     event.EventKey,
		Status:       "circuit_break",
		FailedNodeID: failedNodeID,
		Error:        &errMsg,
		StartedAt:    start,
		FinishedAt:   now,
	}
	// Ignorar violação de UNIQUE em event_key — idempotência de execuções concorrentes (FR-023).
	_ = e.logRepo.Create(ctx, logEntry)
}

// logCompleted persiste FlowExecutionLog com status "completed".
// Ignora violação de UNIQUE em event_key (FR-023).
// Ref: tasks.md §3.1.5.
func (e *Engine) logCompleted(
	ctx context.Context,
	snapshot *flow.Flow,
	device *domain.Device,
	event *domain.AttendanceEvent,
	start time.Time,
) {
	if e.logRepo == nil || event == nil {
		return
	}

	now := time.Now()
	logEntry := &repository.FlowExecutionLog{
		FlowID:     snapshot.ID,
		DeviceID:   device.ID,
		EventKey:   event.EventKey,
		Status:     "completed",
		StartedAt:  start,
		FinishedAt: now,
	}
	// Ignorar violação de UNIQUE em event_key — idempotência (FR-023).
	_ = e.logRepo.Create(ctx, logEntry)
}
