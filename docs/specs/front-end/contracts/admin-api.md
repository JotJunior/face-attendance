# Contracts: Admin API (`front-end`)

Contratos dos endpoints que a Interface de Administração consome.

> **Convenção de case (fonte da verdade = código real)**: TODOS os campos JSON
> em **snake_case** — verificado nos domain structs `internal/domain/*.go` (ex:
> `device_identifier`, `last_heartbeat_at`, `federal_document`). Constitution VII:
> sintaxe em inglês.
>
> **Aterramento (Constitution Princípio I)**:
> - **[EXISTENTE]** = rota/handler/shape confirmado no código (file:line citado).
>   Os shapes vêm dos domain structs reais.
> - **[NOVO — PROPOSTA a validar na implementação]** = endpoint que **não existe
>   hoje** e é criado por esta feature. Os campos derivam de domain structs reais,
>   mas o endpoint em si é projetado aqui (legítimo desenhar contrato inexistente;
>   marcado explicitamente como proposta, conforme regra da skill plan §5.2).

---

## Sumário de rotas

| Rota | Método | Status | Auth | Handler |
|------|--------|--------|------|---------|
| `/admin/sync` | POST | **[EXISTENTE]** | Bearer `ADMIN_TOKEN` | `AdminSyncHandler` (server.go:54, handlers.go:315) |
| `/health` | GET | **[EXISTENTE]** | nenhuma | `HealthHandler` (server.go:51) |
| `/admin/api/login` | POST | **[NOVO]** | nenhuma (emite cookie) | `AdminLoginHandler` |
| `/admin/api/logout` | POST | **[NOVO]** | cookie de sessão | `AdminLogoutHandler` |
| `/admin/api/stats` | GET | **[NOVO]** | cookie de sessão | `AdminStatsHandler` |
| `/admin/api/devices` | GET | **[NOVO]** | cookie de sessão | `AdminDevicesHandler` |
| `/admin/api/devices/{id}` | GET | **[NOVO]** | cookie de sessão | `AdminDeviceDetailHandler` |
| `/admin/api/members` | GET | **[NOVO]** | cookie de sessão | `AdminMembersHandler` |
| `/admin/api/events` | GET | **[NOVO]** | cookie de sessão | `AdminEventsHandler` |
| `/admin/api/sync` | POST | **[NOVO]** | cookie de sessão | wrapper reusando `Scheduler`/`SyncSerializer` |
| `/admin/`, `/admin/assets/*` | GET | **[NOVO]** | cookie (exceto login/assets) | `embed.FS` + `http.FileServer` |

> **OQ-1 resolvida**: a UI usa **[NOVO]** `POST /admin/api/sync` (protegido por
> cookie), que internamente invoca a **mesma** lógica de sync (`Scheduler` +
> `SyncSerializer` **[EXISTENTE]**). O `/admin/sync` Bearer permanece intacto para
> CLI. Evita relaxar a auth do endpoint existente.

---

## [EXISTENTE] POST /admin/sync — disparar carga de membros

**Auth**: `Authorization: Bearer {ADMIN_TOKEN}` (`AdminAuthMiddleware`,
middleware.go:52). **Reusado pelo botão de sync via wrapper de cookie (abaixo).**

### Response (202) — confirmado handlers.go:329-330
```json
{ "status": "accepted" }
```

### Error Responses (confirmados)
| Status | Body | Quando |
|--------|------|--------|
| 409 | `{"error":"sync already in progress"}` | sync em andamento (handlers.go:318) |
| 401 | `unauthorized: missing Authorization header` | sem header (middleware.go) |
| 403 | `forbidden: invalid admin token` | token errado (middleware.go) |

---

## [NOVO] POST /admin/api/login — autenticação da UI

**Auth**: nenhuma (esta rota emite a sessão). Valida usuário+senha contra
`ADMIN_USERNAME` + `ADMIN_PASSWORD` **[NOVO]** (env, `require()`). Comparação de
senha em tempo constante (`crypto/subtle`).

### Request
| Field | Type | Required | Validation |
|-------|------|----------|------------|
| `username` | string | yes | não-vazio |
| `password` | string | yes | não-vazio |

```json
{ "username": "<admin user>", "password": "<senha>" }
```

### Response (204) — sucesso
Sem corpo. Emite header:
```
Set-Cookie: admin_session=<base64url(payload).base64url(hmac)>; HttpOnly; Secure; SameSite=Strict; Path=/admin; Max-Age=<TTL_segundos>
```
Token assinado HMAC-SHA256 com `ADMIN_SESSION_SECRET`; payload `{"sub":"<username>","exp":<unix>}`.

### Error Responses
| Status | Body | Quando |
|--------|------|--------|
| 401 | `{"error":"credenciais inválidas"}` | usuário/senha incorretos (mensagem PT-BR — UI) |
| 400 | `{"error":"requisição inválida"}` | corpo ausente/malformado |

---

## [NOVO] POST /admin/api/logout — encerrar sessão

**Auth**: cookie de sessão.

### Response (204)
Sem corpo. Expira o cookie:
```
Set-Cookie: admin_session=; HttpOnly; Secure; SameSite=Strict; Path=/admin; Max-Age=0
```

---

## [NOVO] GET /admin/api/stats — métricas do dashboard (FR-003)

**Auth**: cookie de sessão. Agrega 3 contadores + o limiar offline numa resposta.

### Response (200) — campos snake_case (PROPOSTA, derivada das entidades reais)
| Field | Type | Description |
|-------|------|-------------|
| `members_with_selfie` | int | COUNT members com `url_selfie` não-nulo/não-vazio |
| `devices_active` | int | devices com `last_heartbeat_at >= now() - threshold` |
| `devices_inactive` | int | devices offline (abaixo do limiar) ou `is_active=false` |
| `attendance_last_24h` | int | COUNT attendance_events com `created_at >= now() - 24h` |
| `device_offline_threshold_hours` | int | valor de `DEVICE_OFFLINE_THRESHOLD_HOURS` **[NOVO]** (dec-012/015) |

```json
{
  "members_with_selfie": 0,
  "devices_active": 0,
  "devices_inactive": 0,
  "attendance_last_24h": 0,
  "device_offline_threshold_hours": 24
}
```
> Valores `0`/`24` acima são **placeholders de exemplo do contrato**, não dados
> reais. Os contadores vêm de métodos de repo **[NOVO]**: `CountMembersWithSelfie`,
> `CountDevicesByActivity(thresholdHours)`, `CountAttendanceSince(24h)`.

### Error Responses
| Status | Body | Quando |
|--------|------|--------|
| 401 | `{"error":"sessão expirada"}` | cookie ausente/inválido/expirado (FR-012) |

---

## [NOVO] GET /admin/api/devices — lista de dispositivos (FR-004)

**Auth**: cookie de sessão. Sem paginação obrigatória (volume = dezenas de
dispositivos, briefing).

### Response (200) — array; cada item derivado de `domain.Device` (snake_case real)
| Field | Type | Source |
|-------|------|--------|
| `id` | int | Device.id |
| `device_identifier` | string | Device.device_identifier |
| `ip_address` | string\|null | Device.ip_address |
| `mac_address` | string\|null | Device.mac_address |
| `last_heartbeat_at` | string(RFC3339)\|null | Device.last_heartbeat_at |
| `is_active` | bool | Device.is_active |
| `webhook_configured` | bool | Device.webhook_configured |
| `created_at` | string(RFC3339) | Device.created_at |
| `status` | string (`"active"`\|`"offline"`) | **derivado** (limiar) |

```json
{
  "devices": [],
  "device_offline_threshold_hours": 24
}
```
> Inclui `device_offline_threshold_hours` para a UI calcular/confirmar o status
> client-side (dec-012). Estado vazio: `devices: []` → UI mostra mensagem amigável
> (FR-009).

---

## [NOVO] GET /admin/api/devices/{id} — detalhe de dispositivo (FR-004, dec-007)

**Auth**: cookie de sessão. **Dados atuais, sem histórico** (dec-007 — só
`last_heartbeat_at`).

### Response (200) — mesmo shape do item de lista (campos de `domain.Device` +
`status` derivado). Sem array de heartbeats.

### Error Responses
| Status | Body | Quando |
|--------|------|--------|
| 404 | `{"error":"dispositivo não encontrado"}` | id inexistente |
| 401 | `{"error":"sessão expirada"}` | sessão inválida |

---

## [NOVO] GET /admin/api/members — lista de membros (FR-005, FR-008, dec-008)

**Auth**: cookie de sessão. **Busca + paginação cursor server-side**.

### Query params
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `q` | string | no | termo de busca: nome OU CPF (normalizado server-side) |
| `cursor` | string | no | cursor opaco da página (keyset sobre `id`) |
| `limit` | int | no | tamanho de página (default e teto definidos na implementação) |

### Response (200) — `members[]` com CPF MASCARADO (FR-011, research Decision 4)
Cada item (DTO de view **[NOVO]**, derivado de `domain.Member` + `ProcessingOutcome`):
| Field | Type | Source |
|-------|------|--------|
| `id` | int | Member.id (cursor) |
| `name` | string | Member.name |
| `federal_document_masked` | string | **mascarado** de Member.federal_document (ex `***.NNN.NNN-**`) |
| `status` | string | Member.status (status GOB) |
| `sync_status` | string (`"synced"`\|`"failed"`\|`"pending"`) | **derivado** de ProcessingOutcome |
| `last_failed_stage` | string\|null | ProcessingOutcome.last_stage quando há `last_error` |

```json
{
  "members": [],
  "next_cursor": null,
  "has_more": false
}
```
> `federal_document` **cru nunca aparece** na resposta. `next_cursor=null` +
> `has_more=false` = última página. Estado vazio → mensagem amigável (FR-009).

---

## [NOVO] GET /admin/api/events — log de eventos (FR-006, FR-008, dec-008)

**Auth**: cookie de sessão. **Filtro por intervalo de datas + paginação cursor
server-side**, ordem cronológica **decrescente** (`created_at DESC, id DESC`).

### Query params
| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string (RFC3339 ou date) | no | início do intervalo (server-side) |
| `to` | string (RFC3339 ou date) | no | fim do intervalo |
| `cursor` | string | no | cursor keyset sobre `(created_at, id)` |
| `limit` | int | no | tamanho de página |

### Response (200) — `events[]` (DTO de view **[NOVO]**, derivado de `AttendanceEvent`)
| Field | Type | Source |
|-------|------|--------|
| `id` | int | AttendanceEvent.id |
| `event_datetime` | string(RFC3339)\|null | AttendanceEvent.event_datetime |
| `created_at` | string(RFC3339) | AttendanceEvent.created_at |
| `device_id` | int\|null | AttendanceEvent.device_id |
| `device_identifier` | string\|null | join com Device (rótulo legível) |
| `member_name` | string\|null | join com Member (`null` se não identificado) |
| `federal_document_masked` | string\|null | **mascarado** de AttendanceEvent.federal_document |
| `marking_status` | string (`"marked"`\|`"pending"`\|`"failed"`\|`"unauthorized"`) | **derivado** de `marked`/`marked_at`/`attendance_status` |
| `marked_at` | string(RFC3339)\|null | AttendanceEvent.marked_at |

```json
{
  "events": [],
  "next_cursor": null,
  "has_more": false
}
```
> `raw_payload` (JSONB) e `event_key` **não são expostos** (diagnóstico interno).
> CPF mascarado. Estado vazio → mensagem amigável (FR-009).

---

## [NOVO] POST /admin/api/sync — sync manual via cookie (FR-007)

**Auth**: cookie de sessão. Reusa a **mesma** lógica do `AdminSyncHandler`
**[EXISTENTE]** (`Scheduler` + `SyncSerializer`), apenas com guarda de cookie em
vez de Bearer.

### Response (202)
```json
{ "status": "accepted" }
```
### Error Responses
| Status | Body | Quando |
|--------|------|--------|
| 409 | `{"error":"sincronização já em andamento"}` | serializer bloqueia (mesma semântica do existente) |
| 401 | `{"error":"sessão expirada"}` | sessão inválida |

---

## Sessão expirada durante navegação (FR-012, edge case)

Qualquer endpoint `/admin/api/*` protegido responde **401** quando o cookie está
ausente/inválido/expirado. O frontend intercepta 401 globalmente, redireciona
para a tela de login **preservando a URL atual** (ex: query `?redirect=<path>`)
para retornar após re-autenticação. Cumpre FR-012 e SC-003 (100% das rotas
protegidas exigem sessão).
