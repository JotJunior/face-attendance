# Data Model: face-flow

**Feature**: `face-flow`
**Fase**: plan (onda-003)
**Data**: 2026-06-26

---

## Entidades novas

### `flows`

Representa um fluxo configurado pelo admin.

```sql
CREATE TABLE flows (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT        NOT NULL,
  status      TEXT        NOT NULL DEFAULT 'inactive'
                          CHECK (status IN ('active', 'inactive')),
  device_id   BIGINT      UNIQUE REFERENCES devices(id) ON DELETE SET NULL,
  nodes       JSONB       NOT NULL DEFAULT '[]',
  edges       JSONB       NOT NULL DEFAULT '[]',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_flows_status ON flows(status);
CREATE INDEX idx_flows_device_id ON flows(device_id) WHERE device_id IS NOT NULL;
```

**Constraint 1:1**: `device_id UNIQUE` — PostgreSQL permite múltiplos NULLs em UNIQUE,
portanto fluxos não vinculados (device_id=NULL) coexistem; ao vincular, a constraint
garante que nenhum outro fluxo já ocupe o mesmo device_id.

**nodes** — array JSONB de `FlowNode`:
```json
[
  {
    "id": "node-uuid-1",
    "type": "start",
    "config": {},
    "x": 100,
    "y": 50
  },
  {
    "id": "node-uuid-2",
    "type": "https_call",
    "config": {
      "url": "https://api.example.com/notify",
      "method": "POST",
      "headers": {"Authorization": "Bearer [user.document]"},
      "body": "{\"cpf\": \"[user.document]\", \"device\": \"[device.id]\"}",
      "timeout_seconds": 30
    },
    "x": 250,
    "y": 50
  }
]
```

**edges** — array JSONB de `FlowEdge`:
```json
[
  {"from": "node-uuid-1", "to": "node-uuid-2", "label": ""},
  {"from": "node-uuid-3", "to": "node-uuid-4", "label": "valid"},
  {"from": "node-uuid-3", "to": "node-uuid-5", "label": "invalid"}
]
```

`label` só é relevante em edges que saem de nó do tipo `decision`
(`"valid"` ou `"invalid"`). Para todos os demais tipos, `label` é string vazia.

---

### `background_images`

Biblioteca de imagens disponíveis para seleção no nó 4 (change_background).

```sql
CREATE TABLE background_images (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT        NOT NULL,
  file_path   TEXT        NOT NULL,   -- path relativo a BACKGROUND_IMAGES_DIR
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

---

### `flow_execution_logs`

Registro de cada execução do motor de fluxo. Idempotência via `event_key`.

```sql
CREATE TABLE flow_execution_logs (
  id              BIGSERIAL   PRIMARY KEY,
  flow_id         BIGINT      NOT NULL REFERENCES flows(id),
  device_id       BIGINT      NOT NULL REFERENCES devices(id),
  event_key       TEXT        NOT NULL,
  status          TEXT        NOT NULL
                              CHECK (status IN ('completed', 'circuit_break')),
  failed_node_id  TEXT,       -- node.id do nó que falhou (NULL se completed)
  error           TEXT,       -- mensagem de erro (NULL se completed)
  started_at      TIMESTAMPTZ NOT NULL,
  finished_at     TIMESTAMPTZ NOT NULL,
  CONSTRAINT uq_flow_execution_logs_event_key UNIQUE (event_key)
);

CREATE INDEX idx_flow_execution_logs_flow_id    ON flow_execution_logs(flow_id);
CREATE INDEX idx_flow_execution_logs_device_id  ON flow_execution_logs(device_id);
CREATE INDEX idx_flow_execution_logs_started_at ON flow_execution_logs(started_at DESC);
```

`event_key` = SHA-256(employeeNoString + eventDatetime + MACAddress),
mesmo algoritmo de `repository.ComputeEventKey` já existente.

---

## Tipos de nó — schema de config por tipo

| `type` | Config JSON | Campos obrigatórios |
|--------|-------------|---------------------|
| `start` | `{}` | — |
| `camera_on` | `{}` | — (BLOCKED_ISAPI) |
| `camera_off` | `{}` | — (BLOCKED_ISAPI) |
| `wait` | `{"duration_seconds": N}` | `duration_seconds` (int, 1..3600) |
| `change_background` | `{"image_id": N}` | `image_id` (int, FK background_images.id) |
| `https_call` | `{"url": "...", "method": "GET\|POST\|PUT\|PATCH", "headers": {}, "body": "...", "timeout_seconds": 30}` | `url`, `method` |
| `qrcode_background` | `{"content_template": "..."}` | `content_template` |
| `decision` | `{}` | — |
| `send_message` | `{"message_template": "..."}` | `message_template` (BLOCKED_API) |

Regras de validação por tipo:
- `wait.duration_seconds`: inteiro entre 1 e 3600
- `https_call.url`: deve começar com `https://` (enforcement em runtime; admin pode configurar `http://` mas recebe aviso)
- `https_call.timeout_seconds`: inteiro entre 1 e 300; default 30 quando ausente
- `change_background.image_id`: FK válido em `background_images`; validado no save, não no runtime
- `qrcode_background.content_template`: string não-vazia

---

## Entidades Go — `internal/flow`

```go
package flow

import (
  "encoding/json"
  "time"
)

// NodeType enumera os 9 tipos de nó suportados.
type NodeType string

const (
  NodeTypeStart            NodeType = "start"
  NodeTypeCameraOn         NodeType = "camera_on"      // BLOCKED_ISAPI
  NodeTypeCameraOff        NodeType = "camera_off"     // BLOCKED_ISAPI
  NodeTypeWait             NodeType = "wait"
  NodeTypeChangeBackground NodeType = "change_background"
  NodeTypeHTTPSCall        NodeType = "https_call"
  NodeTypeQRCodeBackground NodeType = "qrcode_background"
  NodeTypeDecision         NodeType = "decision"
  NodeTypeSendMessage      NodeType = "send_message"   // BLOCKED_API
)

// FlowNode representa um nó no fluxo.
type FlowNode struct {
  ID     string          `json:"id"`
  Type   NodeType        `json:"type"`
  Config json.RawMessage `json:"config"`
  X      float64         `json:"x"`
  Y      float64         `json:"y"`
}

// FlowEdge representa uma conexão entre dois nós.
type FlowEdge struct {
  From  string `json:"from"`
  To    string `json:"to"`
  Label string `json:"label"` // "valid"|"invalid" para nó decision; "" para demais
}

// Flow representa um fluxo completo.
type Flow struct {
  ID        int64      `json:"id"`
  Name      string     `json:"name"`
  Status    string     `json:"status"` // "active" | "inactive"
  DeviceID  *int64     `json:"device_id,omitempty"`
  Nodes     []FlowNode `json:"nodes"`
  Edges     []FlowEdge `json:"edges"`
  CreatedAt time.Time  `json:"created_at"`
  UpdatedAt time.Time  `json:"updated_at"`
}

// Configs typed (para decode seguro no engine):

type WaitConfig struct {
  DurationSeconds int `json:"duration_seconds"`
}

type ChangeBackgroundConfig struct {
  ImageID int64 `json:"image_id"`
}

type HTTPSCallConfig struct {
  URL            string            `json:"url"`
  Method         string            `json:"method"`
  Headers        map[string]string `json:"headers"`
  Body           string            `json:"body"`
  TimeoutSeconds int               `json:"timeout_seconds"` // default 30
}

type QRCodeBackgroundConfig struct {
  ContentTemplate string `json:"content_template"`
}

type SendMessageConfig struct {
  MessageTemplate string `json:"message_template"`
}
```

---

## Contexto de execução — variáveis disponíveis

```go
package flowengine

import "github.com/jotjunior/face-attendance/internal/domain"

// ExecutionContext holds data available for variable interpolation.
type ExecutionContext struct {
  Member *domain.Member         // nil se membro não encontrado
  Device *domain.Device         // sempre presente (gatilho é do device)
  Event  *domain.AttendanceEvent // sempre presente
}
```

Mapeamento de variáveis (vocabulário de interpolação):

| Variável | Campo Go | Disponibilidade |
|----------|----------|-----------------|
| `[user.name]` | `ctx.Member.Name` | Quando Member != nil |
| `[user.document]` | `ctx.Member.FederalDocument` | Quando Member != nil |
| `[user.status]` | `ctx.Member.Status` | Quando Member != nil |
| `[user.mobile]` | `*ctx.Member.MobileNumber` | Quando Member != nil e campo preenchido |
| `[device.id]` | `ctx.Device.ID` | Sempre |
| `[device.identifier]` | `ctx.Device.DeviceIdentifier` | Sempre |
| `[device.ip]` | `*ctx.Device.IPAddress` | Quando disponível |
| `[device.mac]` | `*ctx.Device.MACAddress` | Quando disponível |
| `[event.authorized]` | `"true"` ou `"false"` | Sempre |
| `[event.datetime]` | `*ctx.Event.EventDatetime` ISO 8601 | Quando presente |

Variável ausente no contexto → substituída por `""`. Sintaxe inválida → preservada literalmente.

---

## Migration

Arquivo: `migrations/000008_create_flows.up.sql`

Inclui:
1. `flows` table + índices
2. `background_images` table
3. `flow_execution_logs` table + índices + unique constraint `event_key`

Rollback: `migrations/000008_create_flows.down.sql`
```sql
DROP TABLE IF EXISTS flow_execution_logs;
DROP TABLE IF EXISTS background_images;
DROP TABLE IF EXISTS flows;
```
