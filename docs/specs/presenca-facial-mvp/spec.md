# Feature Specification: MVP de Controle de Presenca por Reconhecimento Facial

**Feature**: `presenca-facial-mvp`
**Created**: 2026-06-20
**Status**: Draft

> Fontes autoritativas: `docs/01-briefing-discovery/briefing.md`, `docs/constitution.md`,
> `t.txt`, `legacy/hik-api`. Todo contrato externo (GOB, HikVision ISAPI) citado aqui
> e rastreavel a essas fontes (Constitution Principio I — Veracidade, NON-NEGOTIABLE).
> Nenhum nome de campo, rota ou valor foi inventado.

## User Scenarios & Testing

### User Story 1 - Marcacao automatica de presenca via reconhecimento facial (Priority: P1)

A equipe operacional configura um dispositivo HikVision apontando o webhook para a API
local. Quando um membro previamente cadastrado e reconhecido pelo dispositivo, o sistema
recebe o evento, identifica o membro pelo CPF e registra a presenca de volta na GOB —
sem qualquer acao manual.

**Why this priority**: e o nucleo do produto — a automacao da marcacao de presenca. Sem
ela o sistema nao entrega valor; todas as demais etapas existem para habilita-la.

**Independent Test**: com um membro ja carregado no dispositivo (via US2/US3), simular o
recebimento de um evento de reconhecimento no webhook contendo `employeeNoString` = CPF do
membro (campo bruto do payload HikVision — dec-022), e verificar que uma chamada de
marcacao de presenca e enviada a GOB com o CPF correto.

**Acceptance Scenarios**:

1. **Given** um membro cadastrado no dispositivo com `employeeNo` = CPF (campo ISAPI
   interno), **When** o dispositivo envia ao webhook um evento `AccessControl` de
   reconhecimento positivo contendo `employeeNoString` = CPF no payload
   `AccessControllerEvent`/`EventNotificationAlert`, **Then** o sistema extrai o CPF de
   `employeeNoString`, envia `POST {GOB_STATE_URL}/attendance/3ff4708cb695ad1a6e9f87cb714e1f22`
   com header `Authorization: <GOB_STATE_TOKEN>` e body `{ "cpf": <cpf-do-membro> }`.
2. **Given** o mesmo evento entregue duas vezes (redelivery), **When** ambos sao
   processados, **Then** a presenca resultante e idempotente (re-execucao nao corrompe o
   estado na GOB).
3. **Given** um evento de webhook cujo `employeeNoString` nao corresponde a nenhum membro
   conhecido, **When** o sistema processa o evento, **Then** registra log estruturado de
   evento desconhecido e nao envia marcacao.

---

### User Story 2 - Carga de membros da GOB para a fila de processamento (Priority: P2)

O sistema busca a lista de membros na GOB, descarta os que nao possuem selfie e enfileira
os membros validos para processamento assincrono, um a um.

**Why this priority**: alimenta o pipeline. Sem a carga, nenhum membro chega aos
dispositivos. Depende apenas da GOB, sendo testavel isoladamente.

**Independent Test**: executar a carga contra uma resposta da GOB contendo membros com e
sem `url_selfie`, e verificar que apenas os membros com `url_selfie` sao publicados na
fila local, um por mensagem.

**Acceptance Scenarios**:

1. **Given** a GOB retorna `{ "success": true, "data": [ ... ] }` com membros, **When** a
   carga executa via `GET {GOB_STATE_URL}/api/face-detection/members` com header
   `Authorization: Bearer <GOB_STATE_TOKEN>`, **Then** cada membro de `data[]` que possui
   `url_selfie` nao-vazio e publicado individualmente na fila local.
2. **Given** membros em `data[]` sem o campo `url_selfie` (ausente ou vazio), **When** a
   carga executa, **Then** esses membros sao descartados e nao publicados na fila.
3. **Given** a GOB esta indisponivel ou retorna erro, **When** a carga executa, **Then** o
   sistema registra log estruturado do erro e nao publica mensagens parciais corrompidas.

---

### User Story 3 - Registro de usuario e face no dispositivo HikVision (Priority: P2)

Um worker consome a fila de membros e, para cada um, faz upsert do usuario no dispositivo,
envia a imagem da face e garante que o webhook do dispositivo aponta para a API local —
reutilizando do legacy `hik-api` exclusivamente essas tres operacoes.

**Why this priority**: prepara os dispositivos para reconhecer membros. Depende de US2
(mensagens na fila) e habilita US1 (reconhecimento). Testavel contra um dispositivo (ou
stub ISAPI) isoladamente.

**Independent Test**: publicar uma mensagem de membro valido na fila e verificar que o
worker executa as tres operacoes ISAPI documentadas com o CPF como identificador, e que
re-processar a mesma mensagem nao duplica usuario/face no dispositivo.

**Acceptance Scenarios**:

1. **Given** uma mensagem de membro valido na fila, **When** o worker a consome, **Then**
   faz upsert do usuario via `POST/PUT /ISAPI/AccessControl/UserInfo/Modify` (XML
   `UserInfo` com `employeeNo` = CPF e `name`) no dispositivo.
2. **Given** o usuario foi criado/atualizado, **When** o worker prossegue, **Then** baixa a
   imagem de `url_selfie` e a envia via `POST /ISAPI/Intelligent/FDLib/faceDataRecord`
   (multipart: parte `FaceDataRecord` JSON `{ "type": "concurrent", "faceLibType": "blackFD", "FDID": "1", "FPID": <CPF> }` + parte `FaceImage` binaria).
3. **Given** o dispositivo precisa notificar reconhecimentos, **When** o worker configura o
   dispositivo, **Then** atualiza a URL do webhook via `POST /ISAPI/Event/notification/httpHosts`
   (XML `HttpHostNotification`) apontando para a API local.
4. **Given** a mesma mensagem re-entregue, **When** o worker reprocessa, **Then** o
   resultado e idempotente (chaveado por CPF — sem usuario/face duplicado).
5. **Given** uma chamada ISAPI falha de forma transitoria, **When** o worker trata o erro,
   **Then** a mensagem e re-tentada; falha persistente roteia para a dead-letter queue
   (DLQ) sem perder a mensagem.

---

### User Story 4 - Registro de dispositivos via heartbeat (Priority: P3)

Cada dispositivo HikVision e configurado com webhook para a API local e envia heartbeats
frequentes. A API identifica o dispositivo pelo heartbeat, registra-o na base local na
primeira vez e mantem seu estado de liveness. Dispositivos registrados sao os destinos da
carga de usuarios/faces.

**Why this priority**: necessario para distribuir faces aos dispositivos corretos, mas o
fluxo de US1-US3 pode ser exercitado contra um dispositivo conhecido enquanto o registro
automatico amadurece.

**Independent Test**: enviar um heartbeat de um dispositivo nao registrado e verificar que
ele passa a constar na base local; enviar novo heartbeat do mesmo dispositivo e verificar
que nao ha duplicacao e que o liveness e atualizado.

**Acceptance Scenarios**:

1. **Given** um dispositivo nao registrado, **When** ele envia um heartbeat para a API,
   **Then** o sistema o registra na base local e o marca como ativo.
2. **Given** um dispositivo ja registrado, **When** ele envia novo heartbeat, **Then** o
   sistema atualiza o liveness sem criar registro duplicado.
3. **Given** a carga de usuarios/faces (US3), **When** o worker distribui membros, **Then**
   o faz para os dispositivos registrados.

---

### Edge Cases

- O que acontece quando `url_selfie` aponta para imagem inexistente ou o download falha?
  O membro nao pode ter face registrada — a mensagem deve ser re-tentada e, persistindo,
  roteada para DLQ com log estruturado.
- Como o sistema lida com um evento de webhook malformado ou de um dispositivo nao
  registrado? Deve registrar o evento e nao marcar presenca, sem derrubar o handler.
- O que acontece se a GOB rejeita a marcacao de presenca (erro 4xx/5xx)? Deve aplicar a
  politica de retry/DLQ sem perder o evento de reconhecimento.
- Como o sistema trata um membro cujo CPF (`federal_document`) e invalido ou ausente? Sem
  CPF nao ha identificador para o dispositivo — o membro deve ser descartado com log.
- O que acontece se o mesmo membro for reconhecido multiplas vezes em curto intervalo?
  A marcacao de presenca deve permanecer idempotente.

## Requirements

### Functional Requirements

**Etapa 0 — Registro de dispositivos**

- **FR-001**: System MUST registrar um dispositivo HikVision na base local na primeira vez
  que receber um heartbeat dele, identificando-o de forma estavel entre heartbeats.
- **FR-002**: System MUST atualizar o estado de liveness de um dispositivo ja registrado a
  cada heartbeat subsequente, sem criar registros duplicados.
- **FR-003**: System MUST direcionar a carga de usuarios e faces (FR-009..FR-012) apenas
  para dispositivos registrados.

**Etapa 1 — Carga de membros (GOB)**

- **FR-004**: System MUST buscar a lista de membros via `GET {GOB_STATE_URL}/api/face-detection/members`
  com header `Authorization: Bearer <GOB_STATE_TOKEN>`.
- **FR-005**: System MUST interpretar a resposta como `{ "success": <bool>, "data": [ <member> ] }`,
  onde cada `member` possui os campos verificados `id`, `status`, `created_at`,
  `updated_at`, `federal_document`, `name`, `mobile_number`, `url_selfie`.
- **FR-006**: System MUST descartar membros cujo `url_selfie` esteja ausente ou vazio, sem
  enfileira-los.
- **FR-007**: System MUST publicar cada membro valido (com `url_selfie`) individualmente em
  uma fila local de processamento (uma mensagem por membro).
- **FR-008**: System MUST usar `federal_document` (CPF) do membro como identificador de
  correlacao entre GOB, fila, dispositivo e marcacao de presenca.

**Etapa 2 — Registro de usuario/face no HikVision (worker)**

- **FR-009**: System MUST consumir a fila de membros com um worker que processa cada
  mensagem de forma idempotente, chaveada por CPF.
- **FR-010**: System MUST fazer upsert do usuario no dispositivo via
  `POST/PUT /ISAPI/AccessControl/UserInfo/Modify`, enviando XML `UserInfo` com
  `employeeNo` = CPF e `name` do membro.
- **FR-011**: System MUST baixar a imagem de `url_selfie` e envia-la ao dispositivo via
  `POST /ISAPI/Intelligent/FDLib/faceDataRecord` (multipart), com a parte `FaceDataRecord`
  contendo JSON `{ "type": "concurrent", "faceLibType": "blackFD", "FDID": "1", "FPID": <CPF> }`
  e a parte `FaceImage` com a imagem binaria.
- **FR-012**: System MUST configurar/atualizar a URL do webhook do dispositivo via
  `POST /ISAPI/Event/notification/httpHosts` (XML `HttpHostNotification`) apontando para a
  API local.
- **FR-013**: System MUST reutilizar do legacy `hik-api` EXCLUSIVAMENTE as tres operacoes
  acima (criar/atualizar usuario, upload de face, atualizar URL do webhook), sem reutilizar
  cache, autenticacao ou outros controllers (Constitution Principio IV).

**Etapa 3 — Marcacao de presenca (GOB)**

- **FR-014**: System MUST expor um endpoint de webhook que receba o evento `AccessControl`
  de reconhecimento enviado pelo dispositivo HikVision; o CPF do membro reconhecido vem no
  campo `employeeNoString` dentro de `AccessControllerEvent`/`EventNotificationAlert` do
  payload bruto (conforme codigo legacy `WebhookController.php:212` e
  `WebhookEventProcessor.php:154,232` — dec-022).
- **FR-015**: System MUST, ao receber um evento de reconhecimento positivo, extrair o CPF
  de `employeeNoString` e enviar `POST {GOB_STATE_URL}/attendance/3ff4708cb695ad1a6e9f87cb714e1f22`
  com header `Authorization: <GOB_STATE_TOKEN>` e body `{ "cpf": <cpf-do-membro> }`.
- **FR-016**: System MUST tornar a marcacao de presenca idempotente em re-entrega do mesmo
  evento de reconhecimento.
- **FR-017**: System MUST ignorar (com log estruturado) eventos de webhook cujo
  `employeeNoString` nao corresponda a um membro conhecido, sem marcar presenca.

**Requisitos transversais**

- **FR-018**: System MUST emitir log estruturado em JSON com os campos `device_id`, `cpf`,
  `stage` e `error` (quando aplicavel) nas operacoes criticas (Constitution Principio VI).
- **FR-019**: System MUST expor um endpoint de health check para monitoramento operacional.
- **FR-020**: System MUST obter `GOB_STATE_URL`, `GOB_STATE_TOKEN` e as credenciais ISAPI
  dos dispositivos via configuracao de runtime (variaveis de ambiente), nunca hardcoded
  (Constitution Principio V).

**Decisoes de infraestrutura auditaveis**

- **FR-021-INFRA-SCHED**: A frequencia do ciclo de carga de membros (FR-004) e configuravel
  via env var `MEMBER_SYNC_INTERVAL_MINUTES` (default: 60); o sistema MUST executar o
  ciclo periodicamente por ticker interno E expor endpoint HTTP de trigger manual sob
  demanda (dec-023 — briefing secao 3.1: "chamada periodica (ou sob demanda)").
- **FR-022-INFRA-IDEMP**: A chave de idempotencia de todas as escritas externas e o CPF
  (`employeeNo`/`FPID` no HikVision; `cpf` na GOB), com escopo por membro.
- **FR-023-INFRA-RETRY**: As chamadas ISAPI (FR-010..FR-012) e a marcacao GOB (FR-015) MUST
  ter politica de retry configuravel via env vars `RETRY_MAX_ATTEMPTS` (default: 3) e
  `RETRY_INITIAL_BACKOFF_MS` (default: 1000ms), com backoff exponencial; apos esgotar as
  tentativas, rotear para DLQ (dec-024 — Constitution Principio III + Principio V).

### Key Entities

- **Member (Membro)**: pessoa fornecida pela GOB. Atributos verificados: `id`, `status`,
  `created_at`, `updated_at`, `federal_document` (CPF — identificador de correlacao),
  `name`, `mobile_number`, `url_selfie`. Membro sem `url_selfie` nao e processado.
- **Device (Dispositivo)**: aparelho HikVision na rede local. Identificado pelo heartbeat;
  guarda estado de registro e liveness. Destino da carga de usuarios/faces.
- **AttendanceEvent (Evento de Presenca)**: derivado de um evento de reconhecimento
  (`AccessControl` com `employeeNoString` = CPF no payload bruto HikVision) recebido no
  webhook; o CPF e extraido de `employeeNoString` e resulta na marcacao de presenca na GOB.
- **ProcessingMessage (Mensagem de Processamento)**: unidade enfileirada representando um
  membro valido a registrar no dispositivo (chaveada por CPF).

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% dos membros com `url_selfie` retornados pela GOB em um ciclo de carga
  sao enfileirados; 100% dos membros sem `url_selfie` sao descartados.
- **SC-002**: Um membro reconhecido por um dispositivo tem sua presenca marcada na GOB em
  menos de 5 segundos apos o recebimento do evento de webhook, em condicoes normais de rede.
- **SC-003**: Reprocessar a mesma mensagem de carga ou o mesmo evento de reconhecimento
  nao cria usuarios, faces ou presencas duplicados (idempotencia verificavel em 100% das
  re-execucoes de teste).
- **SC-004**: Nenhuma mensagem de membro valido e perdida em falha de dispositivo ou de
  rede — falhas persistentes terminam na DLQ, recuperaveis para reprocessamento.
- **SC-005**: 100% das operacoes criticas produzem log estruturado que permite rastrear o
  estagio (`stage`) e o membro (`cpf`) envolvido em uma falha sem acesso ao codigo.
- **SC-006**: O sistema processa um ciclo de carga de poucos milhares de membros sem
  intervencao manual, com workers escalaveis horizontalmente.

## Out of Scope (MVP)

- Interface web de administracao.
- Gerenciamento de jornadas/horarios (ponto eletronico completo).
- Integracao com sistemas de RH alem da GOB.
- Suporte a multiplas organizacoes.
- Cadastro manual de membros (GOB e a fonte de verdade).
- Abstracao multi-fornecedor de biometria (HikVision e o unico fornecedor no MVP).

## Resolved Ambiguities

| # | Item (briefing sec.9) | Tratamento | Status | Decisao |
|---|------------------------|------------|--------|---------|
| 1 | Frequencia do ciclo de carga | FR-021-INFRA-SCHED: intervalo configuravel via `MEMBER_SYNC_INTERVAL_MINUTES` (default 60min) + endpoint de trigger manual; ticker Go. | resolvido-clarify | dec-023 |
| 2 | Estrategia de retry ISAPI | FR-023-INFRA-RETRY: retry configuravel via `RETRY_MAX_ATTEMPTS` (default 3) e `RETRY_INITIAL_BACKOFF_MS` (default 1000ms); backoff exponencial; DLQ apos esgotar tentativas. | resolvido-clarify | dec-024 |
| 3 | Campo bruto `employeeNoString` vs `employeeNo` | FR-014/FR-017/US1: o campo no payload bruto do webhook HikVision e `employeeNoString` (verificado em `WebhookController.php:212` e `WebhookEventProcessor.php:154,232`). `employeeNo` e o nome interno apos normalizacao. Spec corrigida. | resolvido-clarify | dec-022 |
| 4 | Formato do CPF enviado ao HikVision (mascara vs digits) | Default documentado: usar o formato de `federal_document` como recebido da GOB para `employeeNo`/`FPID`; o body de marcacao GOB usa o formato mascarado verificado `00.000.000-00`. A consistencia exata e detalhe do plan. | default-documentado | — |
| 5 | Provisionamento de credenciais ISAPI | FR-020 fixa env vars de runtime como mecanismo; o meio de provisionamento operacional (arquivo de config vs banco) e detalhe do plan/ops. | default-documentado | — |
