# API Checklist: device-config

**Purpose**: Validar qualidade dos requisitos de API — contratos, error handling, auth, versionamento, observabilidade.
**Created**: 2026-06-21
**Feature**: [spec.md](../spec.md) | [admin-api.md](../contracts/admin-api.md)
**Domínio**: API

> Items `{auto}` resolvidos pelo agente com citação de evidência.
> Items `{humano}` aguardam decisão do dono do produto.
> `[Gap]` = requisito ausente; `[Ambiguity]` = requisito ambíguo.

---

## Contratos de Endpoint

- [x] CHK036 - Cada endpoint tem verbo HTTP, path e response shape especificados? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: admin-api.md define verbo/path/response para todos os 13 endpoints: GET/PUT/POST/DELETE com shapes JSON. Todos os grupos (Overview, Credentials, System, Doors, Users, Webhooks) cobertos.

- [x] CHK037 - Os endpoints que modificam estado usam verbos não-GET (POST/PUT/DELETE) de forma consistente? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: Ações destrutivas: POST para reboot/factory-reset/door-control, PUT para credentials/time, DELETE para users/faces/webhooks. Leituras: GET para overview/doors/time/users/webhooks. Consistente com o padrão REST.

- [x] CHK038 - Está definido o formato do payload de request para cada endpoint de escrita (PUT/POST/DELETE com body)? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: admin-api.md define request body para: PUT credentials (`{isapi_username, isapi_password, isapi_port}`), PUT time (`{time_mode, local_time, time_zone, ntp_server?}`), POST door control (`{command}`). Endpoints sem body (reboot, factory-reset, DELETE users/faces/webhooks) documentados como sem body.

- [x] CHK039 - O endpoint `GET /devices/{id}/users` tem parâmetros de paginação especificados? [Completude, Spec §FR-016, Contracts §admin-api.md] {auto}
  > SOURCED: admin-api.md §Grupo Users: `GET /admin/api/devices/{id}/users?page=1&per_page=100`. Response inclui `total`, `page`, `per_page`. Paginação via query params documentada.

- [x] CHK040 - Os endpoints que retornam listas têm campo `total` na response? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: admin-api.md define `total` em: GET users (`"total": 1`), GET doors (`"total": 1`), GET webhooks (`"total": 1`). Consistente entre todos os endpoints de listagem.

- [x] CHK041 - Está definido o response shape para todos os endpoints de ação (reboot, factory-reset, door control)? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: reboot → `{"result": "rebooting", "device_id": 42}`, factory-reset → `{"result": "factory_reset_initiated", "webhook_configured": false}`, door control → `{"result": "ok", "command": "open"}`, DELETE users → `{"result": "cleared", "device_id": 42}`, DELETE faces → `{"result": "faces_cleared"}`.

- [ ] CHK042 - O endpoint `PUT /devices/{id}/time` com modo NTP tem o shape de request documentado como `[PROPOSTA]` de forma que a implementação saiba o que confirmar? [Completude, Contracts §admin-api.md §PUT time, §hikvision-isapi.md §Set time] {auto}
  > SOURCED: admin-api.md §PUT time: `"ntp_server": "<host>"?` marcado como opcional; hikvision-isapi.md §Set time: NTP marcado `[PROPOSTA — shape do bloco NTP a validar]`. O requisito reconhece explicitamente o que não está verificado. [Gap] — não há requisito de fallback para quando NTP não é suportado pelo firmware (ex: rejeitar `time_mode: "ntp"` com 501 Not Implemented até confirmação). A implementação pode optar por silenciar o erro ou retornar 502.

- [x] CHK043 - Está definido o response para o endpoint de credenciais (PUT) sem ecoar a senha? [Completude, Spec §FR-005, Contracts §admin-api.md §PUT credentials] {auto}
  > SOURCED: admin-api.md §PUT credentials response: `{"isapi_credentials_set": true, "isapi_port": 80}` — sem `isapi_password`. FR-005 como MUST NOT. Contrato explícito e alinhado.

---

## Error Handling

- [x] CHK044 - Todos os status codes de erro relevantes estão especificados por grupo de causa? [Completude, Spec §FR-021/022/023, Contracts §hikvision-isapi.md §status codes] {auto}
  > SOURCED: Tabela hikvision-isapi.md: 504 (timeout/offline), 502 (401 digest / 4xx lógica), 503 (key ausente). FR-023: 404 para `{id}` inexistente. FR-006/020: 401 sessão inválida. admin-api.md §PUT credentials: 400 para `isapi_port` fora de 1-65535.

- [x] CHK045 - Erros de conectividade com o dispositivo são distinguíveis de erros de lógica do dispositivo nos requisitos? [Clareza, Spec §FR-015] {auto}
  > SOURCED: FR-015 especifica explicitamente: "erros de conectividade com o dispositivo DEVEM ser distinguidos de erros de lógica do dispositivo (ex: porta travada por alarme)". Tabela hikvision-isapi.md usa 504 vs 502 respectivamente.

- [x] CHK046 - Os requisitos proíbem respostas de erro genéricas sem contexto (500 sem mensagem)? [Clareza, Spec §SC-006] {auto}
  > SOURCED: SC-006: "zero erros genéricos de servidor sem contexto" em 100% dos casos. Plan §Constitution Check VI: "Erros ISAPI mapeados com contexto (504/502/503), não genéricos (SC-006)".

- [x] CHK047 - O requisito FR-022 especifica que mensagens de erro de autenticação ISAPI não expõem a senha? [Completude, Spec §FR-022/005] {auto}
  > SOURCED: FR-022 especifica "sem expor a senha ou detalhes internos". FR-005 é transversal. hikvision-isapi.md: "`DeviceConfig.Password` é 'sensitive — never log', client.go:151".

- [ ] CHK048 - Estão especificados os requisitos de erro para o caso de `isapi_port` com valor inválido (fora de 1-65535) em endpoints além de PUT credentials? [Completude, Contracts §admin-api.md §PUT credentials] {auto}
  > SOURCED: admin-api.md §PUT credentials: `400 se isapi_port fora de 1-65535`. Os demais endpoints que recebem `{door_id}` ou `{webhook_id}` como path param têm apenas FR-023 (404 para device inexistente) mas não especificam validação de `{door_id}` fora de 1-based válido. [Gap] — requisito de validação de path params numéricos além do `{id}` do device está ausente.

- [ ] CHK049 - O response de 401 (sessão inválida) está documentado de forma consistente com o comportamento atual dos endpoints admin? [Completude, Contracts §admin-api.md §Convenções] {auto}
  > SOURCED: admin-api.md §Convenções: "401 JSON + header `X-Redirect-To` em sessão inválida (session.go)". O comportamento está documentado como herdado. Não é gap — o contrato referencia o mecanismo existente.
  > [x] Resolvido: 401 herdado do session.go existente, documentado. {auto}

---

## Autenticação e Autorização

- [x] CHK050 - Todos os novos endpoints exigem a mesma sessão admin HMAC dos endpoints existentes? [Completude, Spec §FR-006/020, Plan §Project Structure] {auto}
  > SOURCED: FR-006 e FR-020 especificam sessão admin para todos os endpoints. Plan §Project Structure: "Roteamento estende `adminDevicesRouter` (sob `sessionMW` já vigente) — zero mudança no esquema de auth". admin-api.md §Convenções: toda a árvore passa por `SessionMiddleware`.

- [x] CHK051 - Está especificado que nenhum endpoint de configuração é acessível sem sessão válida (deny-by-default)? [Completude, Spec §FR-020, Plan §Security Considerations] {auto}
  > SOURCED: Plan §Security Considerations: "deny-by-default, server.go:54". FR-020 é requisito explícito de sessão em todos os endpoints. Cobertura transversal via middleware.

- [x] CHK052 - Está definido que o modelo de autorização é single-admin (sem multitenancy, sem roles)? [Clareza, Plan §Security Considerations §BOLA/IDOR] {auto}
  > SOURCED: Plan §Security Considerations: "Modelo single-admin: devices são globais, sem ownership por tenant (Constitution §Volume MVP; MEMORY single-org)". Esclarecido que não há escalada de privilégio.

---

## Convenções de Payload

- [x] CHK053 - O case style do payload admin (snake_case) está definido e consistente com os endpoints existentes? [Consistência, Plan §Convenções de Borda] {auto}
  > SOURCED: Plan §Convenções de Borda: tabela define snake_case para DB columns, Backend DTO, Frontend DTO, API payload. Contratos em admin-api.md usam snake_case nos campos do envelope. Consistente.

- [x] CHK054 - A distinção entre snake_case (envelope admin) e camelCase (dados do dispositivo ISAPI) está documentada como convenção explícita? [Clareza, Plan §Convenções de Borda, Contracts §admin-api.md §GET users] {auto}
  > SOURCED: Plan §Convenções de Borda: "Sub-objetos de dados do DEVICE (`users[]`, `doors[]`) preservam os nomes ISAPI deliberadamente (são dados externos)". admin-api.md §GET users: "Nota de borda: `users[]` preserva os nomes ISAPI (camelCase `employeeNo`, `numOfFace`) por serem dados externos do dispositivo; envelope (`total`, `page`, `per_page`) em snake_case".

- [ ] CHK055 - O response de `GET /devices/{id}/doors/{door_id}/status` tem os campos `door_state` e `lock_state` com valores possíveis documentados? [Clareza, Contracts §admin-api.md §GET doors status] {auto}
  > SOURCED: admin-api.md §GET doors status: `{"door_id": 1, "door_state": "...", "lock_state": "...", "open_duration": <int>}` com reticências. hikvision-isapi.md §Door status: response inclui `{doorState, lockState, contactState, currentAction}` mas não lista os enum values possíveis do firmware. [Ambiguity] — valores possíveis de `door_state` e `lock_state` não estão documentados nos contratos; a SPA precisa deles para exibir o estado corretamente (ex: "aberta"/"fechada"/"normal"/"sempre-aberta").

- [x] CHK056 - O mapa de `command` (API) → `cmd` (ISAPI) para controle de porta está documentado? [Completude, Contracts §admin-api.md §POST door control, §hikvision-isapi.md §Door control] {auto}
  > SOURCED: admin-api.md §POST door control: `"command": "open"|"close"|"always_open"|"always_closed"|"normal"`. hikvision-isapi.md §Door control: XML `<cmd>{open|close|alwaysOpen|alwaysClosed|normalOpen}</cmd>` com mapa sourced de DoorService.php. Plan §Convenções de Borda: "mapper no client (mapa SOURCED research Decision 4)".

---

## Observabilidade de API

- [x] CHK057 - As operações ISAPI destrutivas têm requisito de log estruturado com campos definidos? [NF-Observabilidade, Spec §FR-011] {auto}
  > SOURCED: FR-011 especifica campos: `device_id`, `stage`, ação executada, identificação do operador (via sessão). Plan §Security Considerations: "Log estruturado de FR-011 NÃO deve incluir corpo de request de credenciais".

- [ ] CHK058 - Está definido se a API retorna o `device_id` no body de respostas de ação para facilitar rastreabilidade client-side? [Completude, Contracts §admin-api.md] {auto}
  > SOURCED: reboot e DELETE users retornam `"device_id": 42`, mas factory-reset, door control, DELETE faces e DELETE webhooks não incluem `device_id` no response. Inconsistência menor de contrato — não é gap funcional mas pode dificultar logging client-side. [Gap] — inconsistência no `device_id` nos responses de ação; não bloqueante.

---

## Resumo de Resolução

| Marcador | Count |
|----------|-------|
| `[x]` auto-resolvidos | 18 |
| `[ ]` humano (aguardando decisão) | 0 |
| `[Gap]` (requisito ausente) | 3 (CHK042, CHK048, CHK058) |
| `[Ambiguity]` (requisito ambíguo) | 1 (CHK055) |

### Gaps — destino obrigatório

- **CHK042 [Gap]** → tarefa "definir comportamento de fallback para `PUT time` com NTP não suportado pelo firmware" em create-tasks (ou verificar durante execute-task).
- **CHK048 [Gap]** → tarefa "adicionar validação de `{door_id}` e `{webhook_id}` como path params (range check)" ao handler de criação.
- **CHK058 [Gap]** → informativo menor; incluir `device_id` nos responses de door control / factory-reset / DELETE faces/webhooks — decidir em execute-task.

### Ambiguidades — destino

- **CHK055 [Ambiguity]** → ao implementar FR-013, documentar os enum values de `door_state`/`lock_state` a partir da resposta ISAPI real observada (não presumir).
