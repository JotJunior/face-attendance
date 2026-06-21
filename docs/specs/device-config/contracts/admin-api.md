# Contrato Admin API — Endpoints de Configuração do Dispositivo

**Feature**: `device-config` | **Date**: 2026-06-21 | **Camada**: backend Go ↔ frontend SPA

Endpoints REST sob `/admin/api/devices/{id}/...`. **Todos os endpoints abaixo
são `[PROPOSTA — a validar na implementação]`**: nenhum existe hoje. O que é
SOURCED são os PADRÕES herdados (auth, formato de erro, derivação de status,
shape de `deviceResponse`) dos 4 endpoints admin já implementados em
`internal/http/admin_api_handlers.go`.

Convenções (SOURCED da camada admin existente):
- **Auth**: toda a árvore `/admin/api/devices/` passa por `SessionMiddleware`
  (cookie HMAC `admin_session`) — `server.go:54`, `session.go:95-114`.
  Satisfaz FR-006/FR-020 sem código novo. 401 JSON + header `X-Redirect-To`
  em sessão inválida (session.go).
- **Case do payload**: `snake_case` (SOURCED — `deviceResponse` json tags,
  admin_api_handlers.go:110-124). Ver Convenções de Borda no plan.md.
- **Erro**: `adminJSONError(w, status, msg)` com mensagens em português
  (SOURCED — usado em todos os handlers, ex: admin_api_handlers.go:206,213).
- **Extração de `{id}`**: `extractLastPathSegment` + `strconv.ParseInt`
  (SOURCED — admin_api_handlers.go:203-208); 404 se device inexistente
  (FR-023, padrão `pgx.ErrNoRows` → 404, admin_api_handlers.go:212-214).
- **Mapa de erro ISAPI**: 504 (offline) / 502 (auth/lógica) / 503 (key ausente)
  — ver hikvision-isapi.md.

---

## Grupo Overview (US1)

### `GET /admin/api/devices/{id}` (ESTENDER existente)
O handler `AdminDeviceDetailHandler` (admin_api_handlers.go:195-224) JÁ retorna
`deviceResponse`. ESTENDER com 3 campos (FR-002/003):

Resposta (200) — campos novos em **negrito**:
```json
{
  "id": 42,
  "device_identifier": "AA:BB:CC:DD:EE:FF",
  "ip_address": "192.168.68.50",
  "mac_address": "AA:BB:CC:DD:EE:FF",
  "last_heartbeat_at": "2026-06-21T18:00:00Z",
  "status": "active",
  "webhook_configured": true,
  "created_at": "2026-06-01T10:00:00Z",
  "serial_number": "<serial do dispositivo — preenchido por FetchDeviceInfo>",
  "model": "DS-K1T673DWX",
  "firmware_version": "<versão de firmware lida do dispositivo>",
  "max_users": null,
  "max_faces": null,
  "isapi_credentials_set": true
}
```
- `isapi_credentials_set` (FR-003): bool derivado; **senha nunca presente** (FR-005).
- `max_users`/`max_faces` (FR-002): `null` se não lidos; nunca estimados.
- SOURCED: shape base = `deviceResponse` (admin_api_handlers.go:110-124).

---

## Grupo Credentials (US2)

### `PUT /admin/api/devices/{id}/credentials` (FR-004)
Request (snake_case):
```json
{ "isapi_username": "admin", "isapi_password": "<secret>", "isapi_port": 80 }
```
Resposta (200):
```json
{ "isapi_credentials_set": true, "isapi_port": 80 }
```
- **Nunca** ecoa `isapi_password` (FR-005). Cifra via `secrets.Cipher.Encrypt`
  (SOURCED — aesgcm.go:62-82) com `ISAPI_CRED_KEY` (config.go:149-156); persiste
  via `DeviceRepository.SetCredentials` (SOURCED — device_repository.go:241-254).
- Erros: `503` se `ISAPI_CRED_KEY` ausente (FR-007); `404` se device inexistente
  (FR-023); `400` se `isapi_port` fora de 1–65535.

---

## Grupo System (US3)

### `POST /admin/api/devices/{id}/actions/reboot` (FR-008)
- ISAPI: `PUT /ISAPI/System/reboot` (SOURCED — hikvision-isapi.md §Reboot).
- Resposta (200): `{ "result": "rebooting", "device_id": 42 }`.
  **`device_id` SEMPRE presente** em responses de ação (CHK058 — implementado).
- Log estruturado obrigatório: `device_id`, `stage`, ação, operador (FR-011, Constitution VI).

### `POST /admin/api/devices/{id}/actions/factory-reset` (FR-009)
- ISAPI: `PUT /ISAPI/System/factoryReset` body `{mode:basic}` (SOURCED).
- Pós-sucesso: `webhook_configured=false` no banco.
- Resposta (200): `{ "result": "factory_reset_initiated", "webhook_configured": false, "device_id": 42 }`.
  **`device_id` SEMPRE presente** em responses de ação (CHK058 — implementado).
- Log estruturado obrigatório (FR-011); ação registrada como irreversível.

### `GET /admin/api/devices/{id}/time` (FR-010)
- ISAPI: `GET /ISAPI/System/time` (SOURCED).
- Resposta (200): `{ "local_time": "...", "time_zone": "...", "time_mode": "manual" }`
  (snake_case do envelope admin; valores do parse SOURCED `parseTimeData`).

### `PUT /admin/api/devices/{id}/time` (FR-010)
- Request: `{ "time_mode": "manual"|"ntp", "local_time": "YYYY-MM-DDThh:mm:ss", "time_zone": "<offset>", "ntp_server": "<host>"? }`
- **`time_mode`**: enum validado no handler; valores permitidos: `"manual"` ou `"ntp"`.
  Qualquer outro valor retorna `400` com mensagem `"time_mode deve ser 'manual' ou 'ntp'"`.
  Implementado em `PutDeviceTimeHandler` (CHK071 — validado, não é mais PROPOSTA).
- ISAPI: `PUT /ISAPI/System/time` (manual SOURCED; **NTP `[PROPOSTA — requer device físico]`** — shape
  do bloco NTP não verificado, research.md Decision 5 / hikvision-isapi.md — ver bloqueio bl-001).

---

## Grupo Doors (US4)

### `GET /admin/api/devices/{id}/doors` (FR-012)
- ISAPI: `GET /ISAPI/AccessControl/Door/capabilities` (SOURCED).
- Resposta (200): `{ "doors": [{ "door_id": 1, "door_name": "...", "reader_count": 1 }], "total": 1 }`.

### `GET /admin/api/devices/{id}/doors/{door_id}/status` (FR-013)
- ISAPI: `POST /ISAPI/AccessControl/Door/Status` (SOURCED).
- Resposta (200): `{ "door_id": 1, "door_state": "...", "lock_state": "...", "open_duration": <int> }`.

### `POST /admin/api/devices/{id}/doors/{door_id}/control` (FR-014)
- Request: `{ "command": "open"|"close"|"always_open"|"always_closed"|"normal" }`
- ISAPI: `PUT /ISAPI/AccessControl/RemoteControl/door/{N}` com `<cmd>` mapeado
  (SOURCED mapa — research.md Decision 4).
- Resposta (200): `{ "result": "ok", "command": "open" }`.
- Erros distintos (FR-015): `504` conectividade vs `502` lógica do dispositivo
  (porta em alarme).

---

## Grupo Users (US5)

### `GET /admin/api/devices/{id}/users?page=1&per_page=100` (FR-016)
- ISAPI: `POST /ISAPI/AccessControl/UserInfo/Search` paginado (SOURCED, dec-005 score 3).
- **Constraints de paginação (CHK073 — validado no handler)**:
  - `per_page`: inteiro `1–1000`; default `100` se ausente. Retorna `400` se fora do range.
  - `page`: inteiro `>= 1`; default `1` se ausente. Retorna `400` se `< 1`.
- Resposta (200):
```json
{
  "users": [
    { "employeeNo": "12345678900", "name": "...", "userType": "normal",
      "numOfFace": 1, "valid": true, "beginTime": "...", "endTime": "..." }
  ],
  "total": 1, "page": 1, "per_page": 100
}
```
- Nota de borda: `users[]` preserva os nomes ISAPI (camelCase `employeeNo`,
  `numOfFace`) por serem dados externos do dispositivo; envelope (`total`,
  `page`, `per_page`) em snake_case. Ver data-model.md §DeviceUser.

### `DELETE /admin/api/devices/{id}/users` (FR-016b)
- ISAPI: `PUT /ISAPI/AccessControl/UserInfo/Clear` (SOURCED).
- Resposta (200): `{ "result": "cleared", "device_id": 42 }`. Confirmação forte na UI.

### `DELETE /admin/api/devices/{id}/faces` (FR-017)
- ISAPI: **`[PROPOSTA]`** — endpoint de clear da FDLib não verificado
  (research.md Decision 9 / hikvision-isapi.md §Clear faces).
- Resposta (200, quando implementado): `{ "result": "faces_cleared" }`.

---

## Grupo Webhooks (US6)

### `GET /admin/api/devices/{id}/webhooks` (FR-018)
- ISAPI: `GET /ISAPI/Event/notification/httpHosts` (SOURCED).
- Resposta (200): `{ "webhooks": [{ "id": "<hash>", "url": "...", "protocol": "HTTP" }], "total": 1 }`.

### `DELETE /admin/api/devices/{id}/webhooks/{webhook_id}` (FR-019)
- ISAPI: `DELETE /ISAPI/Event/notification/httpHosts/{webhook_id}` (SOURCED).
- Pós-sucesso: se `webhook_id` == webhook principal (`deterministicHostID`,
  client.go:341), `webhook_configured=false`.
- Resposta (200): `{ "result": "removed", "webhook_configured": false }`.

---

## Resumo de proveniência

| Aspecto | Proveniência |
|---------|--------------|
| Auth (sessão admin HMAC) | SOURCED — session.go:95-114, server.go:54 |
| Formato de erro JSON | SOURCED — `adminJSONError` (admin_api_handlers.go) |
| Shape base `deviceResponse` | SOURCED — admin_api_handlers.go:110-124 |
| Mapa command→cmd (doors) | SOURCED — DoorService.php:39-47 |
| Paths/shapes ISAPI | SOURCED — ver hikvision-isapi.md (legacy + Go) |
| **Rotas `/admin/api/devices/{id}/...`** | **IMPLEMENTADO** — 13 endpoints em admin_device_config_handlers.go |
| `time_mode` enum validation | **IMPLEMENTADO** — `"manual"\|"ntp"` validado em PutDeviceTimeHandler (CHK071) |
| `per_page` constraint 1–1000 | **IMPLEMENTADO** — validado em GetDeviceUsersHandler (CHK073) |
| `device_id` em responses de ação | **IMPLEMENTADO** — CHK058, via ActionResponse struct |
| NTP set-time body shape | **[PROPOSTA — requer device físico]** — bloqueio bl-001 |
| Clear faces endpoint | **[PROPOSTA — requer device físico]** — stub ErrNotImplemented → 501; bloqueio bl-002 |
| max_users/max_faces parser | **[PROPOSTA]** — shape ISAPI não verificado |
