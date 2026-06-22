# Research: Configuração Completa do Dispositivo HikVision

**Feature**: `device-config` | **Date**: 2026-06-21 | **Phase**: 0 (Research)
**Spec**: [spec.md](./spec.md) | **Constitution**: [../../constitution.md](../../constitution.md)

Este documento resolve os unknowns técnicos do Technical Context e ancora
cada contrato ISAPI/admin em fonte rastreável. **Nenhuma assinatura de
request/response foi inventada** (Constitution I — NON-NEGOTIABLE): cada
endpoint ISAPI abaixo é citado com arquivo e linha do legacy `hik-api` ou do
client Go atual. Endpoints ainda inexistentes (`/admin/api/devices/{id}/...`)
são marcados `[PROPOSTA]`.

---

## Decision 1 — Reuso do Legacy vs. extensão do client Go (Constitution IV)

**Decision**: Estender o client Go `internal/hikvision/client.go` com novos
métodos ISAPI (reboot, factoryReset, time get/set, door capabilities/status/
control, user list/clear, webhook list/delete, faces clear), em vez de invocar
o serviço PHP legacy em runtime. O legacy `hik-api` é usado **exclusivamente
como fonte de contrato verificado** (paths, verbos, shape de payload) — não é
chamado em produção.

**Rationale**:
- O legacy `hik-api` é um microsserviço Hyperf/PHP/Swoole separado
  (`legacy/hik-api/CLAUDE.md`: porta 9501, Swoole >= 5.0). O sistema de
  presença é um binário Go on-premise. Acoplar runtime PHP↔Go reintroduziria
  cache, autenticação e controllers do legacy — exatamente o que
  Constitution IV proíbe ("MUST NOT reutilizar lógica de cache, camada de
  autenticação, ou quaisquer outros controllers/serviços do legacy").
- O client Go atual já segue este padrão: o comentário de cabeçalho de
  `internal/hikvision/client.go:1-9` declara "All contracts verified from
  legacy/hik-api (contracts/hikvision-isapi.md)". A feature continua essa
  prática — extrai o contrato do legacy, implementa em Go.
- Os paths ISAPI são idênticos (mesmo dispositivo-alvo DS-K1T673DWX); só a
  camada de transporte difere. Reimplementar em Go é cópia de contrato, não
  reimplementação de lógica de negócio.

**Tensão com Constitution IV (registrada para o Constitution Check do plan.md)**:
O texto literal de Principle IV limita o reuso a TRÊS operações ISAPI:
(1) criar/atualizar usuário, (2) upload de face, (3) atualizar URL do webhook.
Esta feature adiciona operações ISAPI fora desse trio. **Resolução**: o
Principle IV restringe o que se REUSA do CÓDIGO legacy (cache/auth/controllers),
não proíbe o sistema Go de falar ISAPI para outras operações administrativas.
O escopo desta feature é uma tela de administração de dispositivo já desenhada
na SPA — não o caminho crítico de enrollment (que permanece nas 3 ops). Ainda
assim, por ser uma EXPANSÃO material da superfície ISAPI do Go, é documentada
no Constitution Check como item que exige justificativa explícita (Governance
§Exceções) e re-validada no re-check pós-design. Não é violação de
NON-NEGOTIABLE (I/II); é expansão de escopo administrativo sobre o Principle IV
(que não é marcado NON-NEGOTIABLE na constituição).

**Alternatives considered**:
- *Chamar o serviço PHP legacy via HTTP em runtime*: rejeitado — arrasta
  cache/auth/controllers do legacy (viola Constitution IV diretamente), exige
  deploy de um segundo runtime (Swoole) no Raspberry Pi, e o legacy usa Redis
  para cache (dependência extra). O legacy não está no docker-compose de
  produção (MEMORY.md confirma só `app`/`migrate`/Postgres/RabbitMQ).
- *Reimplementar do zero sem consultar o legacy*: rejeitado — violaria
  Constitution I (inventar shape de payload ISAPI sem fonte). O legacy É a
  fonte rastreável.

---

## Decision 2 — Camada de serviço ISAPI: estender `Client` vs. novo pacote

**Decision**: Adicionar os novos métodos ao mesmo `hikvision.Client`
(`internal/hikvision/client.go`), reusando o helper `doRequest`
(client.go:191-214), o transporte digest (client.go:161-173) e o tipo de erro
`NonRetriableError` (client.go:461-472). Agrupar os novos métodos por domínio
em arquivos separados do mesmo pacote (`client_system.go`, `client_doors.go`,
`client_users.go`, `client_webhooks.go`) para legibilidade, sem novo struct.

**Rationale**:
- `doRequest` já encapsula baseURL + digest auth + leitura de body + status
  code (verificado em client.go:191-214). Todos os novos métodos seguem o
  mesmo formato `(body []byte, status int, err error)`.
- O `Client` já carrega `DeviceConfig{Host, Username, Password}`
  (client.go:148-152) — exatamente o que cada operação ISAPI precisa.
- Manter um único `Client` preserva a fronteira de Constitution IV: um único
  ponto onde ISAPI é falado, auditável.

**Alternatives considered**:
- *Novo pacote `internal/deviceadmin`*: rejeitado — duplicaria o transporte
  digest e a lógica de baseURL; fragmentaria a superfície ISAPI.

---

## Decision 3 — Roteamento dos subpaths `/admin/api/devices/{id}/...`

**Decision**: Estender o `adminDevicesRouter` existente
(`internal/http/server.go`, registrado em `mux.Handle("/admin/api/devices/", ...)`)
para despachar os subpaths por método+sufixo, usando o parsing manual de path
já presente (`extractLastPathSegment`, admin_api_handlers.go:377-384) e um
parser de segmentos para extrair `{id}`, `{door_id}`, `{webhook_id}`.

**Rationale**:
- O projeto usa `net/http.ServeMux` puro (server.go:55, confirmado) — sem chi/
  gorilla. O padrão atual extrai params manualmente (`extractLastPathSegment`).
  Go 1.22+ `ServeMux` suporta wildcards (`{id}`) nativamente, mas o código atual
  ainda usa o parsing manual; a feature mantém consistência local com o padrão
  existente (CLAUDE.md global: "mantenha a consistência local").
- Toda a árvore `/admin/api/devices/` já passa por `sessionMW` (server.go:54),
  satisfazendo FR-006/FR-020 (sessão admin) automaticamente para os novos
  subpaths.

**Alternatives considered**:
- *Migrar para `ServeMux` Go 1.22 com pattern `POST /admin/api/devices/{id}/actions/reboot`*:
  considerado — é mais limpo, mas trocar o esquema de roteamento de toda a
  camada admin está fora do escopo desta feature. Pode ser ADR futuro.
- *Adicionar chi/gorilla*: rejeitado — nova dependência para um problema já
  resolvido pelo padrão local.

---

## Decision 4 — Mapeamento de comandos de porta (API → ISAPI cmd)

**Decision**: O campo `command` da API admin (FR-014) aceita o conjunto
**snake_case** `{open, close, always_open, always_closed, normal}` e é mapeado
para os valores ISAPI `cmd` reais antes de montar o XML `<RemoteControlDoor>`.

**Mapeamento (SOURCED — `legacy/hik-api/.../Door/DoorService.php:39-47`)**:

| API `command` (FR-014) | ISAPI `<cmd>` (DoorService const) | Fonte (linha) |
|------------------------|-----------------------------------|---------------|
| `open`                 | `open`                            | `CMD_OPEN` (L39) |
| `close`                | `close`                           | `CMD_CLOSE` (L41) |
| `always_open`          | `alwaysOpen`                      | `CMD_REMAIN_OPEN` (L43) |
| `always_closed`        | `alwaysClosed`                    | `CMD_REMAIN_CLOSED` (L45) |
| `normal`               | `normalOpen`                      | `CMD_NORMAL` (L47) |

**Rationale**: a spec (FR-014) e a UI usam snake_case (Constitution VII: chaves
de payload em inglês; convenção do projeto é snake_case nos payloads admin —
ver Decision 6). Os valores ISAPI são camelCase definidos pelo firmware. O
mapper é a única fonte da verdade desta tradução e mora no client Go
(`internal/hikvision/client_doors.go`). **Nenhum valor ISAPI inventado**:
todos os 5 `cmd` vêm das constantes do DoorService legacy.

> ⚠️ **Caveat de veracidade (registrado)**: o legacy expõe `open`/`close` via
> `sendCommand` (DoorService.php:148-156), mas NÃO testa empiricamente os 5
> comandos contra o firmware DS-K1T673DWX nesta base. Os valores `cmd` são
> SOURCED do legacy (que os define como constantes e os usa), porém o
> comportamento real de `alwaysOpen`/`alwaysClosed`/`normalOpen` no firmware
> específico deve ser confirmado na implementação (quickstart §Door control).
> A spec US4 "Destravar 5s" requer um comando temporizado — ver Decision 5.

---

## Decision 5 — "Destravar 5s" (US4): comando ISAPI vs. lógica de duração

**Decision**: O firmware DS-K1T673DWX, ao receber `<cmd>open</cmd>` em
`PUT /ISAPI/AccessControl/RemoteControl/door/{N}`, destrava a porta pelo
`openDuration`/`lockTime` configurado no próprio dispositivo (campo
`open_duration` em `AccessControlDoor`, ver DoorService `parseDoorConfig`,
DoorService.php:424-433). O backend Go envia `open` e NÃO implementa um timer
próprio de 5s.

**Rationale**:
- A duração de destravamento é uma propriedade do dispositivo (lockTime), não
  do backend. Implementar um timer no Go que re-tranca após 5s duplicaria
  estado e introduziria condição de corrida (o dispositivo já re-tranca
  sozinho).
- O legacy `sendCommand(open)` apenas envia o comando único; não há lógica de
  timer no legacy (verificado: DoorService.php:304-346 não tem sleep/timer).

> ⚠️ **Caveat de veracidade (NEEDS CONFIRMATION na implementação)**: a spec
> US4-AS1 afirma "porta retorna ao modo normal após 5 segundos". O valor "5s"
> é um requisito de produto; o valor REAL de `openDuration` no dispositivo
> precisa ser lido (`GET /ISAPI/AccessControl/Door/{N}`) e, se necessário,
> ajustado. Esta feature NÃO inventa que o default é 5s — a UI deve refletir o
> `open_duration` real lido do dispositivo. Documentado no contrato de doors.

---

## Decision 6 — Convenção de case do payload admin (snake_case)

**Decision**: Os payloads JSON da API admin usam **snake_case**, consistente
com os endpoints admin existentes.

**Rationale (SOURCED)**: as structs de resposta admin atuais usam json tags
snake_case — `deviceResponse` em `admin_api_handlers.go:110-124`:
`"device_identifier"`, `"ip_address"`, `"mac_address"`, `"last_heartbeat_at"`,
`"webhook_configured"`, `"serial_number"`, `"firmware_version"`. O `statsResponse`
(L56-62) idem: `"members_with_selfie"`, `"devices_active"`. Manter snake_case nos
novos endpoints é consistência local mandatória (CLAUDE.md global). Isso é
distinto do shape ISAPI interno (camelCase do firmware: `employeeNo`, `doorID`),
que NÃO vaza para a API admin — o handler traduz.

**Alternatives considered**:
- *camelCase nos novos endpoints*: rejeitado — quebraria consistência com os 4
  endpoints admin existentes e com a SPA que já os consome. (Este é exatamente
  o tipo de drift que a seção Convenções de Borda do plan.md previne.)

---

## Decision 7 — Persistência de capacidades de hardware (FR-002)

**Decision**: As capacidades de hardware (máximo de usuários, máximo de faces)
NÃO têm coluna no banco atualmente. FR-002 pede "obtidas via ISAPI e
armazenadas em cache no banco". Como NÃO há fonte verificada do shape exato de
`maxUsers`/`maxFaces` no firmware (o legacy `parseCapabilities`,
DeviceService.php:375-390, parseia flags booleanas `isSupportFR` etc., NÃO
contadores de capacidade máxima), esta feature:

1. Busca capacidades via `GET /ISAPI/AccessControl/AcsCfg/capabilities` OU
   `GET /ISAPI/System/capabilities` **[PROPOSTA — shape a validar contra o
   dispositivo real na implementação]**;
2. Persiste em colunas novas `max_users INTEGER NULL`, `max_faces INTEGER NULL`
   (migration nova) — nullable, retornadas como `null` quando indisponíveis
   (FR-002: "campos de capacidade indisponíveis são retornados como nulos,
   nunca estimados").

**Rationale**: FR-002 exige cache no banco. O shape exato dos contadores não
está no legacy verificado (só flags de suporte). Por isso o **endpoint de
capacidades de contagem é PROPOSTA** e o plano marca explicitamente que a
implementação deve descobrir o shape real (via chamada observada ao
dispositivo) antes de codificar o parser — ou registrar bloqueio humano se o
firmware não expuser. **Nunca estimar maxUsers/maxFaces** (Constitution I).

**Alternatives considered**:
- *Hardcode das capacidades do DS-K1T673DWX a partir de datasheet*: rejeitado —
  datasheet não está no repositório (não é fonte rastreável no sentido de
  Constitution I — "código legacy / t.txt / briefing / resposta real"); e
  hardcode por modelo quebra se o parque tiver variantes. Nullable até leitura
  real é a opção honesta.

---

## Decision 8 — Diferenciação de erros ISAPI (FR-021/FR-022)

**Decision**: O client Go distingue três classes de falha e o handler as mapeia
para HTTP:

| Classe de falha (origem)                          | HTTP admin | FR |
|---------------------------------------------------|------------|-----|
| Timeout/conexão recusada (dispositivo offline)    | `504`      | FR-021 |
| ISAPI 401 (digest auth falhou — credencial errada)| `502`      | FR-022 |
| ISAPI 4xx de lógica (ex: porta em alarme)          | `502` + detalhe | FR-015 |
| Dispositivo `{id}` inexistente no banco            | `404`      | FR-023 |
| `ISAPI_CRED_KEY` ausente                            | `503`      | FR-007 |

**Rationale (SOURCED)**: `doRequest` (client.go:191-214) retorna `error` em
falha de transporte (timeout/conn refused) e `(body, statusCode, nil)` quando
há resposta HTTP. O handler já distingue isso no padrão atual. O digest
transport (`github.com/icholy/digest`, client.go:28) propaga 401 como status
code quando a credencial é inválida. O mapeamento de `error de transporte → 504`
e `status 401 → 502` é determinístico a partir do retorno de `doRequest`.
Nenhuma mensagem expõe a senha (FR-005): o erro do client nunca inclui
`DeviceConfig.Password` (comentário client.go:151 "sensitive — never log").

**Alternatives considered**:
- *Retornar 500 genérico*: rejeitado — viola FR-021/FR-022/SC-006 ("zero erros
  genéricos de servidor sem contexto").

---

## Decision 9 — Clear de faces (FR-017) sem endpoint verificado

**Decision**: FR-017 (`DELETE /admin/api/devices/{id}/faces`) NÃO tem endpoint
ISAPI verificado no legacy (a exploração confirmou: UserService tem
`UserInfo/Clear` mas NÃO há endpoint dedicado de "clear all faces"; faces são
atreladas a usuários). Resolução: o backend implementa o clear de faces via o
endpoint de faceDataRecord library já conhecido — **[PROPOSTA — endpoint de
clear da FDLib a validar]** `PUT /ISAPI/Intelligent/FDLib/FDSearch` +
delete, OU documentar que "limpar faces" no DS-K1T673DWX se faz limpando a
FDLib `FDID=1` (a mesma usada em `UploadFace`, client.go:289-293).

**Rationale**: `UploadFace` usa `faceLibType: "blackFD"`, `FDID: "1"`
(client.go:289-293) — fonte verificada de QUE biblioteca de faces o sistema usa.
O endpoint de DELETE/clear dessa biblioteca NÃO está no legacy nem no Go atual.
Por Constitution I, marca-se PROPOSTA e a implementação DEVE descobrir o
endpoint real (doc ISAPI/chamada observada) ou registrar bloqueio humano —
nunca inventar o path de clear.

**Alternatives considered**:
- *Assumir `DELETE /ISAPI/Intelligent/FDLib/FDSearch?FDID=1`*: rejeitado como
  afirmação — é hipótese plausível, não verificada. Fica como PROPOSTA a
  validar, distinta de contrato real.

---

## Resumo de unknowns resolvidos

| Unknown | Status | Resolução |
|---------|--------|-----------|
| Reuso legacy vs. client Go (Const. IV) | RESOLVIDO | Decision 1 — estender client Go, legacy = fonte de contrato |
| Onde colocar novos métodos ISAPI | RESOLVIDO | Decision 2 — mesmo `Client`, arquivos por domínio |
| Roteamento dos subpaths | RESOLVIDO | Decision 3 — estender `adminDevicesRouter` |
| Mapa de comandos de porta | RESOLVIDO (SOURCED) | Decision 4 — tabela API↔ISAPI do DoorService |
| Lógica do "destravar 5s" | RESOLVIDO + caveat | Decision 5 — dispositivo controla duração |
| Case do payload admin | RESOLVIDO (SOURCED) | Decision 6 — snake_case |
| Capacidades maxUsers/maxFaces | PARCIAL — PROPOSTA | Decision 7 — colunas nullable, shape a validar |
| Diferenciação de erros ISAPI | RESOLVIDO (SOURCED) | Decision 8 — tabela 504/502/404/503 |
| Clear de faces (FR-017) | PROPOSTA | Decision 9 — endpoint de clear FDLib a validar |

**NEEDS CLARIFICATION restantes que bloqueiam Phase 1**: 0. Os dois itens
PROPOSTA (capacidades de contagem, clear de faces) NÃO bloqueiam o design —
são marcados como contratos `[PROPOSTA]` a validar na implementação, conforme
permitido por Constitution I (projetar contrato inexistente é legítimo se
rotulado). A implementação deve confirmá-los empiricamente ou registrar
bloqueio humano antes de codificar o parser/path.
