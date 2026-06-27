# Research Notes: face-flow

**Feature**: `face-flow`
**Fase**: plan (onda-003)
**Data**: 2026-06-26

---

## 1. Contratos ISAPI verificados — Nós 4 e 6 (DESBLOQUEADOS)

### Fonte primária: `internal/hikvision/client_standby.go`

O Go client já implementa as operações de standby picture necessárias para os nós 4 e 6,
com endpoints verificados no device DS-K1T673* e sourced de `legacy/hik2go`:

| Operação | Endpoint ISAPI | Método | Formato | Função Go |
|----------|---------------|--------|---------|-----------|
| Upload standby picture | `/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json` | POST | multipart (JSON meta + binary JPEG) | `Client.UploadStandbyPicture(ctx, filename, data)` |
| Ativar modo custom standby | `/ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` | PUT | JSON `{standbyPicType: "custom", displayEffect: "stretch", switchingTime: 20}` | `Client.EnableCustomStandby(ctx)` |
| Listar standby pictures | `/ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json` | GET | JSON | `Client.ListStandbyPictures(ctx)` |
| Deletar standby picture | `/ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json` | POST | JSON `{customStandbyPicUUIDList: [...]}` | `Client.DeleteStandbyPicture(ctx, uuid)` |

**Fonte**: `internal/hikvision/client_standby.go` (comentário de origem: `legacy/hik2go/src/Hik2go/Preferences/StandbyPicture.php`)

**Requisitos de imagem verificados** (fonte: `internal/hikvision/client_bootpic.go` L22-24):
- Formato: JPEG
- Resolução: 600×1024 pixels (exigência de firmware; imagens fora → HTTP 400)
- Helper existente: `resizeImageJPEG(data []byte, w, h int)` em `client_bootpic.go`

**Multipart upload payload verificado** (fonte: `client_standby.go:UploadStandbyPicture`):
- Part 1 — field `"UploadCustomStandbyPic"`: JSON `{"filePathType": "multipart", "filePath": "<filename>"}`
- Part 2 — field `"filePath"`: binary JPEG, Content-Type `image/jpeg`

### Impacto no scope do plan:

- **Nó 4 (change_background)**: UNBLOCKED. Usa `UploadStandbyPicture` + `EnableCustomStandby`.
  A biblioteca de imagens pré-cadastradas (FR-024) é armazenada localmente no servidor,
  `background_images` tabela referencia path no filesystem.
- **Nó 6 (qrcode_background)**: UNBLOCKED. Gera QR code PNG internamente,
  redimensiona para 600×1024 JPEG via `resizeImageJPEG`, então `UploadStandbyPicture` +
  `EnableCustomStandby`. Conteúdo do QR code passa por interpolação de variáveis antes.
- **Nós 1, 2 (camera_on/off)**: Permanecem BLOCKED_ISAPI — nenhum endpoint verificado
  encontrado em `t.txt`, `legacy/hik-api/`, `internal/hikvision/` para habilitar/desabilitar
  a câmera individualmente.
- **Nó 8 (send_message)**: Permanece BLOCKED_API — contrato não fornecido.

---

## 2. Domínio Go existente (fontes para o contexto de execução)

### `internal/domain/attendance_event.go`

```go
type AttendanceEvent struct {
  EventKey         string          // SHA-256 hash — chave de idempotência
  EmployeeNoString string          // CPF (employeeNo do device)
  FederalDocument  *string         // CPF digits (11 dígitos)
  MemberID         *int64
  DeviceID         *int64
  EventDatetime    *time.Time
  AttendanceStatus *string         // "authorized" = positivo
  RawPayload       json.RawMessage
}
func (e *AttendanceEvent) IsAuthorized() bool // attendanceStatus == "authorized"
```

### `internal/domain/member.go`

```go
type Member struct {
  ID              int64
  FederalDocument string   // CPF 11 dígitos
  Name            string
  Status          string
  MobileNumber    *string
}
```

### `internal/domain/device.go`

```go
type Device struct {
  ID               int64
  DeviceIdentifier string   // MAC address (identificador estável)
  IPAddress        *string
  MACAddress       *string
  ISAPIUsername    *string
  ISAPIPort        int
  ISAPIPasswordEnc []byte   // AES-GCM blob
}
```

---

## 3. Webhook handler existente — ponto de integração

Fonte: `internal/http/handlers.go`

O webhook handler já:
1. Extrai `payload.EventType` e `payload.MACAddress`
2. Filtra `eventType != "AccessControllerEvent"` → return 200 (linha 162)
3. Calcula `eventKey := repository.ComputeEventKey(employeeNoString, when, MACAddress)` (linha 196)
4. Persiste `AttendanceEvent` no PostgreSQL
5. Chama `gob.Client.MarkAttendance` para eventos autorizados

**Ponto de integração**: após o processamento de attendance (linha ~260), o handler deve disparar
o flow engine em goroutine separada, retornando 200 imediatamente.

---

## 4. Geração de QR code em Go

**Biblioteca recomendada**: `github.com/skip2/go-qrcode`
- Pure Go, sem dependências externas
- API: `qrcode.Encode(content string, level qrcode.RecoveryLevel, size int) ([]byte, error)` → PNG bytes
- License: MIT
- Alternativa: `github.com/boombuler/barcode` (suporte a múltiplos tipos)

**Fluxo nó 6**:
1. Interpolar variáveis no `content_template` (ex: `[user.document]`) → `content`
2. `qrcode.Encode(content, qrcode.Medium, 600)` → PNG bytes
3. `resizeImageJPEG(pngBytes, 600, 1024)` → JPEG 600×1024
4. `hikClient.UploadStandbyPicture(ctx, filename, jpegBytes)`
5. `hikClient.EnableCustomStandby(ctx)`

---

## 5. Motor de execução — modelo de concorrência

**Decisão**: goroutine por execução de fluxo, sem pool de workers. Justificativa:
- Deploy on-premise single instance (sem coordenação multi-pod)
- Execuções esperadas: baixa frequência (reconhecimento facial esporádico)
- Goroutines Go são leves (~2KB stack inicial)
- Circuit-break é por execução (stateless entre execuções)

**Risco de goroutine leak**: o handler passa `context.Background()` para o engine,
não o contexto da requisição HTTP (que cancela após retornar 200). O `ctx` do engine
deve ter timeout máximo proporcional ao fluxo: `context.WithTimeout(ctx, maxFlowDuration)`.
Estimativa conservadora: 5 × (max_nodes=50) × (max_wait=3600s) + buffer → impraticável.
Alternativa: timeout de última instância via `context.WithTimeout(ctx, 30*time.Minute)`.

**Estado entre execuções**: nenhum. O flow engine recebe uma cópia da snapshot do fluxo
(bytes do JSONB decodificados) no início de cada execução; edições posteriores não afetam.

---

## 6. Biblioteca de imagens — armazenamento local

**Decisão**: armazenar imagens no filesystem do servidor no diretório `data/background-images/`
(configurável via env var `BACKGROUND_IMAGES_DIR`, default `./data/background-images`).
A tabela `background_images` armazena o path relativo.

**Justificativa**:
- Deploy on-premise sem S3/object-store
- Consistente com padrão do legacy/hik-api (`storage/` directory)
- Imagens são pequenas (JPEG 600×1024, < 512KB por spec do firmware)

---

## 7. Validação de fluxo — algoritmo DFS para ciclos

Topological sort via DFS (Kahn's algorithm adaptado):
1. Construir grafo de adjacência `from → []to`
2. Computar in-degree de cada nó
3. BFS/DFS: se algum nó permanecer não-visitado → ciclo detectado
4. Verificar nó `start`: count(type==start) == 1
5. Verificar nó `decision`: count(outEdges) == 2, labels == {"valid", "invalid"}
6. Verificar referências: todo `from`/`to` em edges ∈ nodeIDs

---

## 8. Idempotência — FlowExecutionLog

`event_key` = SHA-256(employeeNoString + when + MACAddress) (mesmo algoritmo de
`repository.ComputeEventKey` já existente em `internal/repository`).

Unique constraint em `flow_execution_logs.event_key` garante que dois disparos com
o mesmo `event_key` não gerem dois registros. A execução em goroutine pode rodar duas
vezes (FR-023 — independentes), mas apenas o primeiro a concluir persiste o log. O segundo
recebe erro de violação de unique e ignora (sem circuit-break).

---

## 9. Decisões de clarify incorporadas ao plan

| CL | Decisão | Impacto no plan |
|----|---------|-----------------|
| CL-001 | FlowExecutionLog = tabela PostgreSQL + slog complementar | Schema inclui tabela `flow_execution_logs`; handler loga via `slog` E persiste |
| CL-002 | Editor integra na SPA admin existente (vanilla JS) | Novas rotas hash `#/flows` e `#/flows/{id}/edit` em `internal/web/dist` |
| CL-003 | Edição durante execução: snapshot capturado no início | Engine recebe cópia imutável do flow; edições pós-start não afetam |
| CL-004 | Nó start = tipo dedicado `start` (9.º tipo, decisão do operador) | `NodeType = "start"`, validação exige exatamente 1 |
| CL-005 | timeout HTTPS = campo `timeout_seconds` por nó, default 30s | `HTTPSCallConfig.TimeoutSeconds int`, default 30 |
