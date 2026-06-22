# Data Model: Configuração Completa do Dispositivo HikVision

**Feature**: `device-config` | **Date**: 2026-06-21 | **Phase**: 1 (Design)
**Spec**: [spec.md](./spec.md)

A maior parte desta feature é **stateless do ponto de vista do banco**: doors,
webhooks e usuários-no-dispositivo são lidos da ISAPI em tempo real e NÃO são
persistidos (spec §Key Entities). A única mudança de schema é a adição de duas
colunas nullable para cache de capacidades de hardware (FR-002).

Convenção: colunas em `snake_case` (Constitution VII; consistente com
`migrations/000002`, `000006`). Fonte da verdade de schema: `migrations/*.sql`.

---

## Entity: Device (existente — sem nova coluna obrigatória)

Terminal HikVision. **Já existe** (tabela `devices`, `migrations/000002_create_devices.up.sql`
+ `migrations/000006_device_credentials_serial.up.sql`). Esta feature CONSOME os
campos existentes e ADICIONA dois opcionais (Decision 7).

### Campos existentes relevantes (SOURCED — migrations 002 + 006)

| Campo (coluna) | Tipo SQL | Origem | Uso nesta feature |
|----------------|----------|--------|-------------------|
| `id` | `BIGSERIAL PK` | 002:5 | Identificador do device nos paths `/admin/api/devices/{id}` |
| `device_identifier` | `VARCHAR(64) NOT NULL UNIQUE` | 002:6,14 | MAC; identidade lógica |
| `ip_address` | `INET` | 002:7 | Endereço para conexão ISAPI |
| `mac_address` | `VARCHAR(17)` | 002:8 | Exibido em overview (US1) |
| `last_heartbeat_at` | `TIMESTAMPTZ` | 002:9 | Deriva status online/offline (US1) |
| `is_active` | `BOOLEAN NOT NULL DEFAULT true` | 002:10 | — |
| `webhook_configured` | `BOOLEAN NOT NULL DEFAULT false` | 002:11 | Marcado `false` em factory-reset (FR-009) e remoção do webhook principal (FR-019) |
| `serial_number` | `VARCHAR(64)` (unique idx parcial) | 006:8,15-17 | Overview (US1); preenchido por `FetchDeviceInfo` |
| `model` | `VARCHAR(64)` | 006:9 | Overview (US1) |
| `firmware_version` | `VARCHAR(32)` | 006:10 | Overview (US1) |
| `isapi_username` | `VARCHAR(64)` | 006:11 | Credencial ISAPI (escrita por FR-004) |
| `isapi_password_enc` | `BYTEA` | 006:12 | Senha cifrada AES-GCM (nonce‖ciphertext); **nunca retornada** (FR-005) |
| `isapi_port` | `INTEGER NOT NULL DEFAULT 80` | 006:13 | Porta ISAPI |

### Campo DERIVADO (não persistido)

| Campo | Tipo | Regra |
|-------|------|-------|
| `isapi_credentials_set` | bool | `true` ⟺ `isapi_username` não-vazio E `isapi_password_enc` não-nulo (FR-003). Calculado no handler; NUNCA expõe a senha. |
| `status` | string `active`\|`offline` | Derivado de `last_heartbeat_at` vs threshold (já existe: `deriveDeviceStatus`, admin_api_handlers.go:133-142) |

### Campos NOVOS (migration nova — Decision 7, FR-002)

| Campo (coluna) | Tipo SQL | Nullable | Justificativa |
|----------------|----------|----------|---------------|
| `max_users` | `INTEGER` | SIM | Capacidade máx. de usuários do hardware. Nullable até 1ª leitura ISAPI bem-sucedida. **Nunca estimado** (FR-002). |
| `max_faces` | `INTEGER` | SIM | Capacidade máx. de faces. Mesma regra. |

> ⚠️ **PROPOSTA parcial**: o SHAPE ISAPI de onde `max_users`/`max_faces` vêm
> não está verificado no legacy (Decision 7). As COLUNAS são decisão firme
> (nullable cache); o PARSER que as popula é PROPOSTA a validar na
> implementação. Migration:
> `migrations/000007_device_capabilities.up.sql` (nova).

### Validações (FR)

- `isapi_port`: 1–65535 (validar no handler de FR-004 antes de persistir).
- `isapi_username`: não-vazio para considerar credencial "configurada".
- Escrita de credencial (FR-004) exige `ISAPI_CRED_KEY` presente, senão `503`
  (FR-007) — a senha nunca é persistida em claro.

### State transitions (campos afetados por ações)

```
factory-reset bem-sucedido (FR-009)  → webhook_configured = false
remoção do webhook PRINCIPAL (FR-019) → webhook_configured = false
FetchDeviceInfo bem-sucedido          → serial_number, model, firmware_version (já existe SetDeviceInfo)
PUT credentials (FR-004)              → isapi_username, isapi_password_enc, isapi_port (já existe SetCredentials)
leitura de capacidades (FR-002)       → max_users, max_faces (nova; SetCapabilities a criar)
```

---

## Entity: Door (do dispositivo — NÃO persistida)

Porta física do terminal. Lida da ISAPI em tempo real (spec §Key Entities:
"Não é persistida no banco"). Shape SOURCED do legacy `DoorService` parsers.

| Campo | Tipo | Fonte ISAPI (SOURCED) |
|-------|------|------------------------|
| `door_id` | int (1-based) | `doorID` — `parseDoorList` L364 / `parseDoorStatus` L404 (DoorService.php) |
| `door_name` | string | `doorName` — `parseDoorList` L365 / `parseDoorStatus` L405 (DoorService.php) |
| `door_state` | string | `doorState` (`parseDoorStatus`, DoorService.php:406) |
| `lock_state` | string | `lockState` (DoorService.php:407) |
| `contact_state` | string | `contactState` (DoorService.php:408) |
| `open_duration` | int (segundos) | `openDuration` — `parseDoorConfig` (DoorService.php:430). Relevante para US4 "destravar Ns" (Decision 5). |

Comando de controle: enum `command` (API) → `cmd` (ISAPI), ver
[research.md Decision 4](./research.md) e contrato de doors.

---

## Entity: WebhookDestination (do dispositivo — NÃO persistida)

Destino de notificação configurado no terminal via ISAPI. Lido em tempo real
(spec §Key Entities). Shape SOURCED do legacy `NotificationService.parseWebhookConfig`.

| Campo | Tipo | Fonte ISAPI (SOURCED) |
|-------|------|------------------------|
| `id` | string | `id` (hash do host; `parseWebhookConfig` single-host L406 / list L418, NotificationService.php) — mesmo `id` determinístico usado em `ConfigureWebhook` (`deterministicHostID`, client.go:408-411) |
| `url` | string | `url` (NotificationService.php single-host L407 / list L419) |
| `protocol` | string | `protocolType` → `protocol` (NotificationService.php single-host L408 / list L420) |

Distinção "webhook principal do sistema": é o destino cujo `id` ==
`deterministicHostID(device.Host)` (client.go:341). Remover esse `id` específico
zera `webhook_configured` (FR-019).

---

## Entity: DeviceUser (do dispositivo — NÃO persistida)

Usuário cadastrado no terminal. Listado da ISAPI (FR-016). Shape SOURCED do
legacy `UserService.parseUserData` (UserService.php:434-447).

| Campo | Tipo | Fonte ISAPI (SOURCED) |
|-------|------|------------------------|
| `employeeNo` | string (CPF) | `employeeNo` (UserService.php:435) |
| `name` | string | `name` (UserService.php:436) |
| `userType` | string | `userType` (UserService.php:437) |
| `numOfFace` | int | `numOfFace` (UserService.php:442) |
| `valid` | bool | `Valid.enable` (UserService.php:444) |
| `beginTime` | string | `Valid.beginTime` (UserService.php:445) |
| `endTime` | string | `Valid.endTime` (UserService.php:446) |

> Nota de borda: a ISAPI usa camelCase (`employeeNo`, `numOfFace`). Estes são
> campos do shape ISAPI interno. A API admin pode preservar camelCase nestes
> sub-objetos (são dados do dispositivo, não do nosso domínio) OU normalizar
> para snake_case. **Decisão**: preservar os nomes ISAPI no array `users[]`
> (FR-016) porque são dados externos do dispositivo, mas os campos de envelope
> da resposta admin (`total`, `page`, `per_page`) usam snake_case. Documentado
> no contrato de users e na tabela de Convenções de Borda do plan.md.

---

## Resumo de mudanças de persistência

| Mudança | Tipo | Arquivo |
|---------|------|---------|
| `devices.max_users INTEGER NULL` | nova coluna | `migrations/000007_device_capabilities.up.sql` |
| `devices.max_faces INTEGER NULL` | nova coluna | mesma migration |
| `DeviceRepository.SetCapabilities(ctx, id, maxUsers, maxFaces *int)` | novo método repo | `internal/repository/device_repository.go` |

Tudo o mais (doors, webhooks, device-users) é **read-through ISAPI sem
persistência** — consistente com spec §Key Entities. Migration down:
`migrations/000007_device_capabilities.down.sql` dropa as 2 colunas.
