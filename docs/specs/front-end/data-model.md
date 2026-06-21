# Data Model: Interface de Administração Web (`front-end`)

> **Sem novas tabelas (dec-007, score 3).** Esta feature **reusa as entidades
> existentes** definidas em `migrations/000001`–`000004` e nos domain structs em
> `internal/domain/*.go`. Os campos abaixo foram extraídos do **código real**
> (verificados file:line) — nenhum nome inventado (Constitution Princípio I).
> A única entidade nova é a **Session**, que é **stateless** (token assinado, sem
> persistência) — não há tabela para ela.

---

## Entity: Device  *(EXISTENTE — `migrations/000002`, `internal/domain/device.go`)*

Dispositivo HikVision registrado. A UI lista (tela Dispositivos) e exibe detalhe.

| Field | Type (DB) | JSON tag | Constraints | Notes |
|-------|-----------|----------|-------------|-------|
| id | BIGSERIAL | `id` | PK | |
| device_identifier | VARCHAR(64) | `device_identifier` | NOT NULL, UNIQUE | MAC ou IP estável |
| ip_address | INET | `ip_address` (omitempty) | nullable | |
| mac_address | VARCHAR(17) | `mac_address` (omitempty) | nullable | |
| last_heartbeat_at | TIMESTAMPTZ | `last_heartbeat_at` (omitempty) | nullable | **coluna única** — sem histórico (dec-007) |
| is_active | BOOLEAN | `is_active` | NOT NULL, default true | |
| webhook_configured | BOOLEAN | `webhook_configured` | NOT NULL, default false | |
| created_at | TIMESTAMPTZ | `created_at` | NOT NULL, default now() | data de registro |
| updated_at | TIMESTAMPTZ | `updated_at` | NOT NULL, default now() | |

**Status offline (derivado, não persistido)**: `offline` quando
`last_heartbeat_at < now() - DEVICE_OFFLINE_THRESHOLD_HOURS` (dec-012/015). O
limiar vem da env var **[NOVO]** e é exposto via `GET /admin/api/stats`. A
comparação ocorre no frontend usando `last_heartbeat_at` + o limiar retornado
(dec-012); o backend também pode pré-calcular o status no DTO de listagem.

**Detalhe de dispositivo (dec-007)**: exibe ID, `mac_address`, `ip_address`,
`created_at` (registro), `last_heartbeat_at`, `webhook_configured` e o status
calculado. **Sem série temporal de heartbeats** — a tabela só tem
`last_heartbeat_at`.

---

## Entity: Member  *(EXISTENTE — `migrations/000001`, `internal/domain/member.go`)*

Membro carregado da GOB. A UI lista (tela Membros) com busca + paginação.

| Field | Type (DB) | JSON tag | Constraints | Notes |
|-------|-----------|----------|-------------|-------|
| id | BIGSERIAL | `id` | PK | cursor de paginação |
| gob_id | BIGINT | `gob_id` | NOT NULL | id na GOB |
| federal_document | VARCHAR(14) | `federal_document` | NOT NULL, UNIQUE | **CPF — mascarado na UI (FR-011)** |
| name | TEXT | `name` | NOT NULL | |
| status | VARCHAR(32) | `status` | NOT NULL | status GOB |
| mobile_number | VARCHAR(32) | `mobile_number` (omitempty) | nullable | |
| url_selfie | TEXT | `url_selfie` (omitempty) | nullable | membro com selfie = contador do dashboard |
| gob_created_at | TIMESTAMPTZ | `gob_created_at` (omitempty) | nullable | |
| gob_updated_at | TIMESTAMPTZ | `gob_updated_at` (omitempty) | nullable | |
| created_at | TIMESTAMPTZ | `created_at` | NOT NULL, default now() | |
| updated_at | TIMESTAMPTZ | `updated_at` | NOT NULL, default now() | |

> **CPF nunca trafega cru** (Decision 4 do research): o DTO de view da UI **[NOVO]**
> expõe `federal_document_masked` (string), não `federal_document`.

---

## Entity: ProcessingOutcome  *(EXISTENTE — `migrations/000003` tabela `member_processing_status`, `internal/domain/processing_outcome.go`)*

Estado de sincronização de um membro em um dispositivo. Alimenta a coluna
"sincronização" da tela Membros.

| Field | Type (DB) | JSON tag | Constraints | Notes |
|-------|-----------|----------|-------------|-------|
| id | BIGSERIAL | `id` | PK | |
| federal_document | VARCHAR(14) | `federal_document` | NOT NULL | CPF (mascarar se exposto) |
| device_id | BIGINT | `device_id` | NOT NULL, FK → devices(id) | |
| user_synced | BOOLEAN | `user_synced` | NOT NULL, default false | usuário criado no device (ISAPI) |
| face_uploaded | BOOLEAN | `face_uploaded` | NOT NULL, default false | face enviada |
| webhook_set | BOOLEAN | `webhook_set` | NOT NULL, default false | |
| last_stage | VARCHAR(32) | `last_stage` (omitempty) | nullable | última etapa (`user_sync`/`face_upload`/...) |
| last_error | TEXT | `last_error` (omitempty) | nullable | erro da última falha (FR-005) |
| attempts | INT | `attempts` | NOT NULL, default 0 | |
| updated_at | TIMESTAMPTZ | `updated_at` | NOT NULL, default now() | |

**Unique**: `(federal_document, device_id)`. Estado "sincronizado" da UI (FR-005) =
`user_synced AND face_uploaded`; estado de falha = `last_error` não-nulo, exibindo
`last_stage` que falhou.

---

## Entity: AttendanceEvent  *(EXISTENTE — `migrations/000004`, `internal/domain/attendance_event.go`)*

Evento de reconhecimento facial recebido. Alimenta a tela Logs.

| Field | Type (DB) | JSON tag | Constraints | Notes |
|-------|-----------|----------|-------------|-------|
| id | BIGSERIAL | `id` | PK | cursor (com created_at) |
| event_key | VARCHAR(128) | `event_key` | NOT NULL, UNIQUE | hash de dedup (SHA-256) |
| employee_no_string | VARCHAR(32) | `employee_no_string` | NOT NULL | CPF cru do device (mascarar) |
| federal_document | VARCHAR(14) | `federal_document` (omitempty) | nullable | CPF normalizado (mascarar) |
| member_id | BIGINT | `member_id` (omitempty) | nullable, FK → members(id) | |
| device_id | BIGINT | `device_id` (omitempty) | nullable, FK → devices(id) | |
| event_datetime | TIMESTAMPTZ | `event_datetime` (omitempty) | nullable | quando reconheceu |
| attendance_status | VARCHAR(32) | `attendance_status` (omitempty) | nullable | `"authorized"` = positivo |
| marked | BOOLEAN | `marked` | NOT NULL, default false | marcado na GOB |
| marked_at | TIMESTAMPTZ | `marked_at` (omitempty) | nullable | quando marcou |
| raw_payload | JSONB | `raw_payload` | NOT NULL | payload bruto HikVision — **não exibir na UI** |
| created_at | TIMESTAMPTZ | `created_at` | NOT NULL, default now() | recebido em; ordenação dos logs |

**Status de marcação GOB exibido na UI (FR-006)**: derivado de `marked` +
`marked_at` + `attendance_status`:
- `marked = true` → "marcado" (com `marked_at`)
- `marked = false` e `attendance_status` positivo → "pendente" / "falhou"
- `attendance_status` não-autorizado → "não autorizado"

> `raw_payload` (JSONB) **não é exposto** na UI — é diagnóstico interno e pode
> conter CPF cru. Os DTOs de view **[NOVO]** o omitem.

---

## Entity: Session  *(NOVA — stateless, sem tabela)*

Sessão autenticada do operador da UI. **Não persiste** — é um token assinado
carregado em cookie httpOnly (research Decision 3).

| Field | Type | Onde vive | Notes |
|-------|------|-----------|-------|
| sub | string | claim no token | identificador do admin (= `ADMIN_USERNAME`) |
| exp | int (unix seconds) | claim no token | expiração = emissão + `ADMIN_SESSION_TTL_HOURS` |
| signature | bytes (HMAC-SHA256) | sufixo do token | assinatura com `ADMIN_SESSION_SECRET` |

**Não há linha em banco**: a validade é auto-contida no token (`exp`) e a
integridade pela assinatura HMAC. Logout = expirar o cookie (Max-Age=0). Não
armazena nenhum dado de membro/dispositivo.

### Relationships

- `ProcessingOutcome` N:1 `Device` via `device_id` *(EXISTENTE — FK)*
- `AttendanceEvent` N:1 `Member` via `member_id` *(EXISTENTE — FK, nullable)*
- `AttendanceEvent` N:1 `Device` via `device_id` *(EXISTENTE — FK, nullable)*
- `ProcessingOutcome` correlaciona-se a `Member` por `federal_document` (CPF) *(EXISTENTE — chave natural, não FK)*
- `Session` **não** se relaciona a nenhuma entidade de dados (isolada por design)

### State Transitions

**Device (status derivado, não persistido)**:
```
active (last_heartbeat_at recente) ⇄ offline (last_heartbeat_at < now - threshold)
```

**AttendanceEvent (marcação GOB — campo `marked`)**:
```
recebido (marked=false) → marcado (marked=true, marked_at set)
                        ↘ pendente/falhou (marked=false, sem marked_at)
```

**Session**:
```
emitida (login OK, exp futuro) → válida (exp > now) → expirada (exp <= now → 401 → redirect login)
                                                     ↘ encerrada (logout → cookie Max-Age=0)
```
