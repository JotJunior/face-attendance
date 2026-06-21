# Quickstart — Configuração Completa do Dispositivo HikVision

**Feature**: `device-config` | **Date**: 2026-06-21

Cenários de validação por fluxo crítico (happy path + error case). O cenário
**Roundtrip E2E** é obrigatório (borda backend↔frontend) e faz chamada REAL ao
backend, comparando o shape do payload contra os contratos — não usa mock.

Pré-requisitos comuns: binário `presenca-facial` rodando (`make build`),
PostgreSQL com migrations aplicadas (`make migrate` incl. 000007), sessão admin
válida (cookie `admin_session`), `ISAPI_CRED_KEY` exportada (32 bytes hex/base64).

---

## Cenário 1 — Overview com device online (US1, FR-001/002/003)

1. Dispositivo `id=42` acessível na rede com credenciais ISAPI configuradas.
2. `GET /admin/api/devices/42` com cookie de sessão.
3. **Expected**: 200 JSON com `serial_number`, `model`, `firmware_version`
   preenchidos (do banco), `status: "active"`, `isapi_credentials_set: true`,
   `max_users`/`max_faces` (`null` se ainda não lidos), e **sem** nenhum campo
   de senha.

### 1b — device offline (FR-001 graceful)
1. Dispositivo `id=42` fora da rede.
2. `GET /admin/api/devices/42`.
3. **Expected**: 200 (não erro fatal); campos estáticos do banco presentes;
   `status: "offline"`; campos ao vivo `null`.

---

## Cenário 2 — Configurar credenciais (US2, FR-004/005/007)

1. `PUT /admin/api/devices/42/credentials` body
   `{"isapi_username":"admin","isapi_password":"hik12345","isapi_port":80}`.
2. **Expected**: 200 `{"isapi_credentials_set": true, "isapi_port": 80}`.
3. `GET /admin/api/devices/42`.
4. **Expected**: `isapi_credentials_set: true`; resposta NÃO contém
   `"hik12345"` nem `isapi_password`.
5. Inspecionar logs do processo durante os passos 1–4.
6. **Expected (SC-003)**: a string `hik12345` NÃO aparece em nenhuma linha de log.

### 2b — ISAPI_CRED_KEY ausente (FR-007)
1. Subir o serviço SEM `ISAPI_CRED_KEY`.
2. `PUT /admin/api/devices/42/credentials` com body válido.
3. **Expected**: `503` com mensagem orientativa; banco inalterado (senha NÃO
   persistida em claro).

---

## Cenário 3 — Reboot com confirmação (US3, FR-008/011)

1. UI: operador clica "Reiniciar", confirma no modal.
2. `POST /admin/api/devices/42/actions/reboot`.
3. **Expected**: 200 `{"result":"rebooting"}`; ISAPI recebeu
   `PUT /ISAPI/System/reboot`; log estruturado com `device_id=42`, `stage`,
   ação e operador (FR-011).
4. **Expected (US3-AS2)**: dispositivo fica `offline` por ~40s e volta.

### 3b — cancelar confirmação (US3-AS4/SC-004)
1. Operador abre modal de reboot e cancela.
2. **Expected**: nenhuma chamada ISAPI; nenhuma ação no dispositivo.

---

## Cenário 4 — Controle de porta "Destravar" (US4, FR-014/015)

1. `POST /admin/api/devices/42/doors/1/control` body `{"command":"open"}`.
2. **Expected**: 200 `{"result":"ok","command":"open"}`; ISAPI recebeu
   `PUT /ISAPI/AccessControl/RemoteControl/door/1` com `<cmd>open</cmd>`
   (mapa SOURCED, research.md Decision 4); porta destrava e re-tranca conforme
   `open_duration` do dispositivo (Decision 5).

### 4b — device offline (FR-015/US4-AS3)
1. Dispositivo offline; `POST .../doors/1/control` `{"command":"open"}`.
2. **Expected**: `504` (conectividade) com mensagem acionável; distinto de
   `502` (lógica do dispositivo). Nenhuma ação silenciosa.

---

## Cenário 5 — Listar usuários no dispositivo (US5, FR-016)

1. Worker provisionou alguns usuários. `GET /admin/api/devices/42/users?page=1&per_page=100`.
2. **Expected**: 200 com `users[]` (cada item `{employeeNo, name, numOfFace,
   valid, ...}` — nomes ISAPI), envelope `{total, page, per_page}` snake_case;
   contagem bate com os usuários reais do dispositivo (Independent Test US5).

---

## Cenário 6 — ROUNDTRIP E2E (obrigatório — borda backend↔frontend)

> Razão (skill plan §5.3): valida que o shape REAL do payload do backend bate
> com o contrato declarado, expondo drift de case (snake vs camel) que testes
> com mock mascarariam.

1. Subir o backend real (`make build && ./bin/presenca-facial`), DB com seed de
   1 device `id=42`.
2. Autenticar e capturar cookie `admin_session` real.
3. Fazer a chamada REAL: `GET /admin/api/devices/42` (curl, não mock).
4. Capturar o JSON de resposta literal.
5. **Expected**: as CHAVES do payload são EXATAMENTE as do contrato
   `contracts/admin-api.md §Overview` em snake_case:
   `id, device_identifier, ip_address, mac_address, last_heartbeat_at, status,
   webhook_configured, created_at, serial_number, model, firmware_version,
   max_users, max_faces, isapi_credentials_set`.
6. **Expected**: NENHUMA chave em camelCase no envelope; NENHUMA chave de senha
   (`isapi_password`, `isapi_password_enc`) presente.
7. Comparar contra o tipo TS consumido pela SPA (`internal/web/dist`): as chaves
   devem casar 1:1. Divergência = drift de borda → corrigir o contrato OU o
   handler ANTES de prosseguir (não acumular).

### 6b — Roundtrip de erro (device inexistente, FR-023)
1. `GET /admin/api/devices/999999` (id inexistente) com sessão válida.
2. **Expected**: `404` JSON com `error` em português; shape de erro idêntico ao
   `adminJSONError` existente (consistência com os 4 endpoints atuais).

---

## Cenário 7 — Segurança de sessão (FR-006/020)

1. `PUT /admin/api/devices/42/credentials` SEM cookie `admin_session`.
2. **Expected**: `401` JSON + header `X-Redirect-To` (padrão SOURCED
   session.go); nenhuma escrita no banco; nenhuma chamada ISAPI.

---

## Matriz cenário → FR/SC

| Cenário | Cobre |
|---------|-------|
| 1, 1b | FR-001, FR-002, FR-003, SC-001, US1 |
| 2, 2b | FR-004, FR-005, FR-007, SC-002, SC-003 |
| 3, 3b | FR-008, FR-011, SC-004, US3 |
| 4, 4b | FR-014, FR-015, FR-021/022, SC-005, SC-006 |
| 5 | FR-016, US5 |
| 6, 6b | FR-001/003, FR-023, **borda backend↔frontend (anti-drift)** |
| 7 | FR-006, FR-020 |
