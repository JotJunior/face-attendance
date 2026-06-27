# Plano de Implementação: face-flow

**Feature**: `face-flow` — Editor de Fluxo por Reconhecimento Facial
**Fase**: plan (onda-003)
**Data**: 2026-06-26
**Spec**: `docs/specs/face-flow/spec.md`
**Data model**: `docs/specs/face-flow/data-model.md`
**Research**: `docs/specs/face-flow/research.md`

---

## Visão Geral

Implementação de um editor visual de fluxogramas no painel admin e um motor de execução
que dispara automaticamente quando o webhook recebe um evento `AccessControllerEvent`.

**Escopo implementável nesta release** (contratos verificados):
- Nós 3, 5, 7, 9 (start, wait, https_call, decision) — sem dependências externas
- Nós 4 e 6 (change_background, qrcode_background) — ISAPI verificado em `client_standby.go`

**Escopo com placeholder** (contratos pendentes):
- Nós 1, 2 (camera_on, camera_off) — BLOCKED_ISAPI: nenhum endpoint verificado
- Nó 8 (send_message) — BLOCKED_API: contrato não fornecido

---

## 1. Arquitetura de Pacotes

```
internal/
  flow/                         # Domínio: entidades + validação de fluxo
    doc.go
    flow.go                     # Flow, FlowNode, FlowEdge, NodeType + typed configs
    validator.go                # Validate() — DFS ciclos, nó start, decision branches
    interpolator.go             # InterpolateVariables(template, ExecutionContext)
  flowengine/
    doc.go
    engine.go                   # Engine.TriggerForDevice(), Execute(), circuitBreak()
    node_wait.go                # executeWait
    node_https.go               # executeHTTPSCall
    node_background.go          # executeChangeBackground, executeQRCodeBackground
    node_blocked.go             # executeBlocked (camera_on/off, send_message → erro)
  repository/
    flow_repository.go          # FlowRepository interface + pgx impl
    background_image_repository.go
    flow_execution_log_repository.go
  http/
    admin_flow_handlers.go      # CRUD de flows + biblioteca de imagens
    admin_flow_handlers_test.go
```

---

## 2. Domínio — `internal/flow`

### 2.1 `flow.go`

Define os tipos principais (ver `data-model.md §Entidades Go`). Funcionalidades:

- `Flow.FindNodeByType(t NodeType) *FlowNode` — procura primeiro nó do tipo
- `Flow.FindNodeByID(id string) *FlowNode` — lookup O(n)
- `Flow.OutgoingEdges(nodeID string) []FlowEdge`
- `Flow.NextNodeID(nodeID string) (string, error)` — para nós com saída única
- `Flow.NextNodeIDByLabel(nodeID, label string) (string, error)` — para decision

### 2.2 `validator.go`

```go
// Validate verifica a estrutura do fluxo antes da publicação (FR-005, FR-022).
// Erros retornados como slice para exibir todos os problemas de uma vez no editor.
func Validate(f *Flow) []ValidationError

type ValidationError struct {
  Code    string // "no_start_node", "multiple_start_nodes", "cycle_detected",
                 // "decision_missing_branch", "dangling_node_reference"
  Message string
  NodeID  string // quando aplicável
}
```

Implementação de Validate:
1. Contar nós com `type=="start"`: deve ser exatamente 1
2. Para cada nó `decision`: contar edges com `from==nodeID`; deve ter
   exatamente 2 (labels "valid" e "invalid")
3. Verificar referências: todo `from`/`to` em `edges` existe em `nodes`
4. Detecção de ciclos via DFS (coloração white/gray/black):
   - Construir adjacência `from → []to`
   - DFS a partir do nó `start`
   - Se encontrar nó gray → ciclo

### 2.3 `interpolator.go`

```go
// InterpolateVariables substitui ocorrências de [variavel] no template.
// Variável ausente no ctx → "". Sintaxe não-[...] → preservada literalmente.
func InterpolateVariables(template string, ctx ExecutionContext) string
```

Implementação: regexp `\[([a-z][a-z0-9._]*)\]` → lookup na tabela de variáveis derivada
do `ExecutionContext`. Nenhuma variável fora do vocabulário definido gera erro; retorna `""`.

---

## 3. Motor de Execução — `internal/flowengine`

### 3.1 `engine.go`

```go
type Engine struct {
  hikClientFor  func(device *domain.Device) (*hikvision.Client, error)
  memberRepo    MemberRepository       // lookup por FederalDocument (CPF)
  logRepo       FlowExecutionLogRepository
  bgImageRepo   BackgroundImageRepository
  flowRepo      FlowRepository
  httpClient    *http.Client           // para nó https_call
}

// TriggerForDevice é chamado pelo webhook handler após processar attendance.
// Não bloqueia: retorna imediatamente após disparar goroutine.
func (e *Engine) TriggerForDevice(
  deviceMACAddress string,
  event *domain.AttendanceEvent,
  member *domain.Member,
  device *domain.Device,
)
```

Sequência em `TriggerForDevice`:
1. Buscar fluxo ativo para o device (`flowRepo.FindActiveByDeviceID`)
2. Se não houver fluxo: return (passthrough silencioso — FR-019)
3. Copiar snapshot do fluxo (campos `Nodes` e `Edges` são slices já desserializados)
4. Disparar `go e.execute(snapshot, event, member, device)`

Sequência em `execute(ctx context.Context, ...)`:
```
start := time.Now()
ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
defer cancel()

execCtx := ExecutionContext{Member: member, Device: device, Event: event}

// Validar snapshot (FR-022)
if errs := flow.Validate(snapshot); len(errs) > 0 {
  e.circuitBreak(ctx, snapshot, device, event, "", fmt.Errorf("validação: %v", errs), start)
  return
}

currentNodeID := snapshot.FindNodeByType(flow.NodeTypeStart).ID

for {
  node := snapshot.FindNodeByID(currentNodeID)
  if node == nil {
    e.circuitBreak(ctx, snapshot, device, event, currentNodeID,
      fmt.Errorf("nó '%s' não encontrado no snapshot"), start)
    return
  }

  if err := e.executeNode(ctx, node, execCtx, device); err != nil {
    e.circuitBreak(ctx, snapshot, device, event, node.ID, err, start)
    return
  }

  nextID, err := nextNodeFor(snapshot, node, execCtx)
  if err != nil {
    e.circuitBreak(ctx, snapshot, device, event, node.ID, err, start)
    return
  }
  if nextID == "" {
    break // fim de fluxo
  }
  currentNodeID = nextID
}

e.logCompleted(ctx, snapshot, device, event, start)
```

`circuitBreak`: loga via `slog.Error(...)` com campos `device_id`, `flow_id`, `node_id`, `error`
e persiste `FlowExecutionLog{status: "circuit_break", ...}`. Ignora violação de unique
em `event_key` (execução concorrente — FR-023).

`logCompleted`: persiste `FlowExecutionLog{status: "completed", ...}`. Idem para unique.

### 3.2 `node_wait.go`

```go
func (e *Engine) executeWait(ctx context.Context, node *flow.FlowNode) error {
  var cfg flow.WaitConfig
  if err := json.Unmarshal(node.Config, &cfg); err != nil {
    return fmt.Errorf("wait: config inválida: %w", err)
  }
  if cfg.DurationSeconds < 1 || cfg.DurationSeconds > 3600 {
    return fmt.Errorf("wait: duration_seconds fora do intervalo [1, 3600]: %d", cfg.DurationSeconds)
  }
  select {
  case <-time.After(time.Duration(cfg.DurationSeconds) * time.Second):
    return nil
  case <-ctx.Done():
    return ctx.Err()
  }
}
```

`time.After` dentro de `select` com `ctx.Done()` garante que o engine para se o
contexto for cancelado durante a espera (evita goroutine leak em fluxos longos).

### 3.3 `node_https.go`

```go
func (e *Engine) executeHTTPSCall(ctx context.Context, node *flow.FlowNode, execCtx ExecutionContext) error {
  var cfg flow.HTTPSCallConfig
  if err := json.Unmarshal(node.Config, &cfg); err != nil {
    return fmt.Errorf("https_call: config inválida: %w", err)
  }

  timeout := cfg.TimeoutSeconds
  if timeout <= 0 { timeout = 30 }             // default 30s (CL-005)
  if timeout > 300 { timeout = 300 }            // cap defensivo

  body := flow.InterpolateVariables(cfg.Body, execCtx)
  method := cfg.Method
  if method == "" { method = "POST" }

  reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
  defer cancel()

  req, err := http.NewRequestWithContext(reqCtx, method, cfg.URL, strings.NewReader(body))
  if err != nil {
    return fmt.Errorf("https_call: criar request: %w", err)
  }
  for k, v := range cfg.Headers {
    req.Header.Set(k, flow.InterpolateVariables(v, execCtx))
  }

  resp, err := e.httpClient.Do(req)
  if err != nil {
    return fmt.Errorf("https_call: %w", err) // inclui timeout → circuit-break (FR-021)
  }
  defer resp.Body.Close()
  io.Copy(io.Discard, resp.Body) // drenar body para reusar conexão
  return nil                     // qualquer status HTTP é aceito (FR-014)
}
```

### 3.4 `node_background.go`

**Nó 4 — change_background** (ISAPI verificado — `client_standby.go`):

```go
func (e *Engine) executeChangeBackground(ctx context.Context, node *flow.FlowNode, device *domain.Device) error {
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

  // Redimensionar para 600×1024 JPEG (requisito firmware — client_bootpic.go L22-24)
  jpegData, err := hikvision.ResizeImageJPEG(data, 600, 1024)
  if err != nil {
    return fmt.Errorf("change_background: redimensionar: %w", err)
  }

  // Upload standby picture
  // SOURCED: client_standby.go:UploadStandbyPicture
  // Endpoint: POST /ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json
  if err := hikClient.UploadStandbyPicture(ctx, img.Name+".jpg", jpegData); err != nil {
    return fmt.Errorf("change_background: upload ISAPI: %w", err)
  }

  // Ativar modo custom standby
  // SOURCED: client_standby.go:EnableCustomStandby
  // Endpoint: PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json
  if err := hikClient.EnableCustomStandby(ctx); err != nil {
    return fmt.Errorf("change_background: ativar standby: %w", err)
  }
  return nil
}
```

**Nó 6 — qrcode_background** (ISAPI verificado; QR code gerado internamente):

```go
func (e *Engine) executeQRCodeBackground(ctx context.Context, node *flow.FlowNode,
    execCtx ExecutionContext, device *domain.Device) error {
  var cfg flow.QRCodeBackgroundConfig
  if err := json.Unmarshal(node.Config, &cfg); err != nil {
    return fmt.Errorf("qrcode_background: config inválida: %w", err)
  }

  content := flow.InterpolateVariables(cfg.ContentTemplate, execCtx)

  // Gerar QR code PNG (github.com/skip2/go-qrcode)
  pngBytes, err := qrcode.Encode(content, qrcode.Medium, 600)
  if err != nil {
    return fmt.Errorf("qrcode_background: gerar QR: %w", err)
  }

  // Redimensionar para 600×1024 JPEG (requisito firmware)
  // SOURCED: internal/hikvision/client_bootpic.go resizeImageJPEG
  jpegData, err := hikvision.ResizeImageJPEG(pngBytes, 600, 1024)
  if err != nil {
    return fmt.Errorf("qrcode_background: redimensionar: %w", err)
  }

  hikClient, err := e.hikClientFor(device)
  if err != nil {
    return fmt.Errorf("qrcode_background: cliente ISAPI: %w", err)
  }

  // Upload + enable (mesmo caminho do nó 4)
  if err := hikClient.UploadStandbyPicture(ctx, "qrcode.jpg", jpegData); err != nil {
    return fmt.Errorf("qrcode_background: upload ISAPI: %w", err)
  }
  return hikClient.EnableCustomStandby(ctx)
}
```

**Exportar `resizeImageJPEG`**: a função atualmente está em `client_bootpic.go` com
letra minúscula. Renomear para `ResizeImageJPEG` (exported) e referenciá-la em
`node_background.go`. Não cria duplicação.

### 3.5 `node_blocked.go`

Nós bloqueados retornam erro descritivo que aciona circuit-break:

```go
func (e *Engine) executeBlocked(node *flow.FlowNode) error {
  switch node.Type {
  case flow.NodeTypeCameraOn, flow.NodeTypeCameraOff:
    return fmt.Errorf("nó '%s' (tipo %s) requer contrato ISAPI não disponível — BLOCKED_ISAPI",
      node.ID, node.Type)
  case flow.NodeTypeSendMessage:
    return fmt.Errorf("nó '%s' (tipo %s) requer contrato de API não disponível — BLOCKED_API",
      node.ID, node.Type)
  }
  return fmt.Errorf("nó '%s': tipo desconhecido '%s'", node.ID, node.Type)
}
```

---

## 4. Repositórios — `internal/repository`

### 4.1 `FlowRepository` (interface)

```go
type FlowRepository interface {
  Create(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
  FindByID(ctx context.Context, id int64) (*flow.Flow, error)
  FindAll(ctx context.Context) ([]*flow.Flow, error)
  FindActiveByDeviceID(ctx context.Context, deviceID int64) (*flow.Flow, error)
  Update(ctx context.Context, f *flow.Flow) (*flow.Flow, error)
  SetStatus(ctx context.Context, id int64, status string) error
  SetDeviceID(ctx context.Context, id int64, deviceID *int64) error
  Delete(ctx context.Context, id int64) error
}
```

Implementação pgx:
- `nodes` e `edges` são serializados/desserializados via `json.Marshal`/`json.Unmarshal`
  nos campos JSONB
- `FindActiveByDeviceID`: `SELECT ... FROM flows WHERE device_id=$1 AND status='active'`
- `SetDeviceID(id, nil)`: SET device_id = NULL (desvincula)
- `SetDeviceID(id, &devID)`: UPDATE com constraint check via UNIQUE na tabela

### 4.2 `BackgroundImageRepository` (interface)

```go
type BackgroundImageRepository interface {
  Create(ctx context.Context, name, filePath string) (*BackgroundImage, error)
  FindByID(ctx context.Context, id int64) (*BackgroundImage, error)
  FindAll(ctx context.Context) ([]*BackgroundImage, error)
  Delete(ctx context.Context, id int64) error
}
```

### 4.3 `FlowExecutionLogRepository` (interface)

```go
type FlowExecutionLogRepository interface {
  // Create insere um log; retorna nil em violação de UNIQUE (idempotência por event_key).
  Create(ctx context.Context, log *FlowExecutionLog) error
  FindByFlowID(ctx context.Context, flowID int64, limit, offset int) ([]*FlowExecutionLog, error)
}
```

Tratamento de unique violation: código de erro pgx `23505` → retornar `nil` (não é
um erro de negócio; é idempotência de execuções concorrentes do mesmo evento).

---

## 5. API HTTP — `internal/http/admin_flow_handlers.go`

### Rotas de fluxos

| Método | Rota | Ação |
|--------|------|------|
| `GET` | `/admin/api/flows` | Lista todos os fluxos (id, name, status, device_id) |
| `POST` | `/admin/api/flows` | Cria fluxo (name; nodes/edges = `[]` por padrão) |
| `GET` | `/admin/api/flows/{id}` | Retorna fluxo completo (com nodes e edges) |
| `PUT` | `/admin/api/flows/{id}` | Atualiza fluxo (name, nodes, edges); valida estrutura |
| `DELETE` | `/admin/api/flows/{id}` | Remove fluxo (desvincula device antes) |
| `PUT` | `/admin/api/flows/{id}/activate` | Define status=active; valida fluxo primeiro |
| `PUT` | `/admin/api/flows/{id}/deactivate` | Define status=inactive |
| `PUT` | `/admin/api/flows/{id}/device` | Vincula device_id (`{"device_id": N}`) |
| `DELETE` | `/admin/api/flows/{id}/device` | Desvincula (device_id=NULL) |
| `GET` | `/admin/api/flows/{id}/logs` | Retorna execution logs paginados |

### Rotas de imagens de background

| Método | Rota | Ação |
|--------|------|------|
| `GET` | `/admin/api/background-images` | Lista imagens disponíveis |
| `POST` | `/admin/api/background-images` | Upload de imagem (multipart/form-data) |
| `DELETE` | `/admin/api/background-images/{id}` | Remove imagem e arquivo em disco |

### Validações na API:

- `PUT /flows/{id}` e `PUT /flows/{id}/activate`: chamar `flow.Validate()` e retornar
  `422 Unprocessable Entity` com lista de erros se houver falhas
- `PUT /flows/{id}/device`: verificar se device existe; verificar se device já tem fluxo
  ativo (retornar `409 Conflict` com mensagem explicativa)
- Upload de imagem: aceitar JPEG/PNG; rejeitar > 5MB; armazenar com nome único
  (`uuid.jpg`); persistir `background_images` no DB

### Roteamento

Seguir o padrão existente em `server.go` (`adminDevicesRouter`):
- Adicionar `adminFlowsRouter` para `ServeMux` path `/admin/api/flows/`
- Adicionar `adminBackgroundImagesRouter` para `/admin/api/background-images/`

### Middleware

Mesmos middlewares do painel admin existente: `AdminAuth` (bearer) + `Session` (cookie).

---

## 6. Integração no Webhook Handler

Arquivo: `internal/http/handlers.go`

**Ponto de integração**: após o bloco de processamento de attendance (linha ~262),
adicionar chamada não-bloqueante ao flow engine:

```go
// Trigger flow engine (não-bloqueante — FR-018/FR-019)
if h.flowEngine != nil && payload.EventType == "AccessControllerEvent" {
  go h.flowEngine.TriggerForDevice(
    payload.MACAddress,
    savedEvent,    // *domain.AttendanceEvent persistido
    resolvedMember, // *domain.Member (nil se não encontrado)
    resolvedDevice, // *domain.Device
  )
}
```

O campo `flowEngine` é injetado no `Server` (ou no `Handler`) em `main.go`,
condicionalmente (nil-safe quando a feature está desabilitada).

**Wiring em `cmd/presenca-facial/main.go`**:
```go
flowRepo    := repository.NewPgxFlowRepository(pool)
bgImageRepo := repository.NewPgxBackgroundImageRepository(pool)
logRepo     := repository.NewPgxFlowExecutionLogRepository(pool)

flowEngine := flowengine.New(flowengine.Config{
  HikClientFor: dbConnResolver.ClientFor,
  FlowRepo:     flowRepo,
  BgImageRepo:  bgImageRepo,
  LogRepo:      logRepo,
  BgImagesDir:  cfg.BackgroundImagesDir,
})
```

---

## 7. SPA Admin — `internal/web/dist`

### Novas telas (hash routing)

| Hash | Tela |
|------|------|
| `#/flows` | Lista de fluxos: tabela com id, nome, status, device vinculado, ações |
| `#/flows/new` | Criar novo fluxo (nome + redireciona para editor) |
| `#/flows/{id}/edit` | Editor visual de canvas |
| `#/flows/{id}/logs` | Log de execuções do fluxo |

### Editor visual (`#/flows/{id}/edit`)

**Tecnologia**: vanilla JS + SVG (sem dependências externas — alinhado com stack existente).

**Estrutura HTML do editor**:
```html
<div id="flow-editor">
  <div id="node-palette">
    <!-- Lista de tipos de nó: arrastar para canvas -->
  </div>
  <div id="canvas-container">
    <svg id="edges-layer" />       <!-- SVG para arestas (setas) -->
    <div id="nodes-layer">          <!-- Divs arrastáveis para nós -->
  </div>
  <div id="config-panel">
    <!-- Configuração do nó selecionado -->
  </div>
</div>
```

**Comportamento do canvas**:
1. **Adicionar nó**: arrastar do palette para o canvas (drag-and-drop com `mousedown`/`mousemove`/`mouseup`)
2. **Mover nó**: arrastar nó existente
3. **Conectar nós**: clicar em porta de saída de um nó, arrastar até porta de entrada de outro
4. **Nó decision**: duas portas de saída (labels "valid" e "invalid")
5. **Selecionar nó**: clicar exibe painel de configuração lateral
6. **Remover nó**: botão no nó ou no painel de configuração
7. **Remover edge**: clicar na aresta e tecla Delete

**Renderização de arestas**: caminhos SVG `<path>` com curva cubic bezier entre as portas
dos nós. Recalculado ao mover nó.

**Nós bloqueados**: renderizados com label "aguardando contrato" (ícone de aviso),
cor diferenciada (cinza), não-selecionáveis para configuração de execução.

**Botão Publicar (Activate)**:
- Envia `PUT /admin/api/flows/{id}` com nodes+edges atuais
- Depois `PUT /admin/api/flows/{id}/activate`
- Exibe erros de validação inline (lista de `ValidationError`)

**Painel de configuração por tipo de nó**:
- `start`: sem campos
- `wait`: campo numérico `duration_seconds`
- `change_background`: dropdown de imagens (GET `/admin/api/background-images`)
- `https_call`: campos url, method (select), headers (key-value pairs), body (textarea), timeout_seconds
- `qrcode_background`: textarea `content_template` com hint de variáveis disponíveis
- `send_message`: textarea `message_template` + aviso "contrato de API pendente"
- `camera_on`, `camera_off`: aviso "contrato ISAPI pendente"
- `decision`: sem campos (bifurcação automática por `event.authorized`)

**Upload de imagem de background**: tela separada ou modal em `#/flows/{id}/edit`:
- `<input type="file" accept="image/jpeg,image/png">`
- POST para `/admin/api/background-images`

---

## 8. Cenários de Teste

### 8.1 Testes unitários — `internal/flow`

| Teste | Cenário |
|-------|---------|
| `TestValidate_NoStart` | Fluxo sem nó start → erro `no_start_node` |
| `TestValidate_MultipleStart` | Dois nós start → erro `multiple_start_nodes` |
| `TestValidate_Cycle` | A→B→C→A → erro `cycle_detected` |
| `TestValidate_DecisionMissingBranch` | Decision com 1 edge → erro `decision_missing_branch` |
| `TestValidate_DecisionMissingLabel` | Decision sem label "valid" ou sem "invalid" → erro |
| `TestValidate_DanglingReference` | Edge aponta para nó inexistente → erro `dangling_node_reference` |
| `TestValidate_ValidFlow` | Fluxo start→wait→decision→(valid→https,invalid→wait2) → sem erros |
| `TestInterpolate_AllVars` | Template com todos os `[user.*]` e `[device.*]` → corretos |
| `TestInterpolate_MissingVar` | `[user.name]` com Member=nil → `""` |
| `TestInterpolate_UnknownVar` | `[foo.bar]` → preservado literalmente |
| `TestInterpolate_InvalidSyntax` | `[incompleto` → preservado literalmente |

### 8.2 Testes unitários — `internal/flowengine`

| Teste | Cenário |
|-------|---------|
| `TestEngine_NoFlow` | Device sem fluxo ativo → TriggerForDevice retorna sem executar |
| `TestEngine_InvalidFlow` | Fluxo com ciclo → circuit-break imediato no validate |
| `TestEngine_WaitNode` | Fluxo start→wait(1s)→end → executa em ~1s |
| `TestEngine_HTTPSCallTimeout` | Servidor não-responsivo + timeout 1s → circuit-break |
| `TestEngine_DecisionValid` | Event.authorized=true → segue ramo "valid" |
| `TestEngine_DecisionInvalid` | Event.authorized=false → segue ramo "invalid" |
| `TestEngine_BlockedNode_Camera` | Fluxo com camera_on → circuit-break com mensagem BLOCKED_ISAPI |
| `TestEngine_BlockedNode_Message` | Fluxo com send_message → circuit-break com mensagem BLOCKED_API |
| `TestEngine_ConcurrentExecutions` | Dois disparos simultâneos do mesmo flow → 2 goroutines independentes |

### 8.3 Testes de integração — `internal/repository` (`//go:build integration`)

| Teste | Cenário |
|-------|---------|
| `TestFlowRepository_CreateFind` | Criar fluxo + buscar por ID |
| `TestFlowRepository_UniqueDeviceID` | Vincular mesmo device em dois fluxos → constraint error |
| `TestFlowRepository_FindActiveByDeviceID` | Fluxo ativo encontrado; fluxo inativo não |
| `TestFlowExecutionLog_IdempotentEventKey` | Inserir dois logs com mesmo event_key → segundo insert nil |

### 8.4 Testes de integração — webhook (`//go:build integration`)

| Teste | Cenário |
|-------|---------|
| `TestWebhook_AccessControllerEvent_TriggersEngine` | POST simulated event → flow engine disparado |
| `TestWebhook_HeartbeatIgnoredByEngine` | Heartbeat event → engine NÃO disparado |
| `TestWebhook_NoFlowForDevice` | Device sem fluxo → webhook 200 sem erro |

---

## 9. Contratos ISAPI — Resumo de Fontes

| Nó | Operação | Endpoint ISAPI | Método | Fonte |
|----|----------|---------------|--------|-------|
| 4 | Upload background | `/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json` | POST multipart | `internal/hikvision/client_standby.go:UploadStandbyPicture` |
| 4 | Ativar background | `/ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` | PUT JSON | `internal/hikvision/client_standby.go:EnableCustomStandby` |
| 6 | (mesmo que nó 4) | — | — | Idem; QR code gerado internamente |
| 1 | Ligar câmera | **BLOCKED_ISAPI** | — | Nenhum endpoint verificado em `t.txt`, `legacy/hik-api/`, `internal/hikvision/` |
| 2 | Desligar câmera | **BLOCKED_ISAPI** | — | Idem |

---

## 10. Bloqueios Humanos Ativos

### BLOQUEIO B-001 — Contrato ISAPI câmera on/off (nós 1 e 2)

**Status**: BLOCKED_ISAPI. Nenhum endpoint ISAPI para habilitar/desabilitar câmera
individualmente foi encontrado em nenhuma fonte verificada do repositório.

**Impacto no plan**: nós `camera_on` e `camera_off` são implementados como
placeholder — renderizados no editor com aviso "aguardando contrato ISAPI",
executados pelo engine com circuit-break imediato (`executeBlocked`).

**Para desbloquear**: operador deve fornecer endpoint ISAPI verificado + payload
(XML ou JSON) para ligar/desligar câmera no device DS-K1T673*.

### BLOQUEIO B-002 — Contrato API disparo de mensagem (nó 8)

**Status**: BLOCKED_API. Parâmetros da API de envio de mensagem não fornecidos.

**Impacto no plan**: nó `send_message` é implementado como placeholder — editor
exibe textarea para `message_template` + aviso "configuração de API pendente",
engine retorna circuit-break imediato.

**Para desbloquear**: operador deve fornecer endpoint, método HTTP, headers
(incluindo auth), schema do payload, e como o `message_template` mapeia no body.

---

## 11. Nova Dependência Go

```
github.com/skip2/go-qrcode  v0.0.0-20200617195104-da1b6568686e
```

Uso exclusivo no nó 6 (`qrcode_background`). Sem dependências transitivas problemáticas.
Alternativa pura-Go sem dependências: `github.com/boombuler/barcode` (mais verbose).

---

## 12. Nova Variável de Ambiente

| Variável | Default | Uso |
|----------|---------|-----|
| `BACKGROUND_IMAGES_DIR` | `./data/background-images` | Diretório de armazenamento de imagens para nó 4 |

Adicionada a `internal/config` (seguindo padrão existente).
