# Research — MVP de Controle de Presenca por Reconhecimento Facial

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20
**Spec**: [spec.md](./spec.md)

> Fontes autoritativas: `docs/01-briefing-discovery/briefing.md`, `docs/constitution.md`,
> `t.txt`, codigo real `legacy/hik-api` (PHP/Hyperf). Nenhum endpoint, campo ou
> valor de API externa foi inventado (Constitution Principio I — Veracidade,
> NON-NEGOTIABLE). Cada contrato cita o arquivo:linha da fonte.

## Decision 0 — Estrutura do servico (monolito modular Go com workers internos)

**Decision**: Um unico binario Go (`cmd/presenca-facial`) que sobe um servidor HTTP
(webhook + heartbeat + health + trigger manual), um scheduler (ticker) de carga
de membros, e workers de consumo RabbitMQ no mesmo processo (goroutines). Deploy
on-premise como um container/processo unico; workers escalam horizontalmente
rodando N replicas do mesmo binario com flags/env controlando quais componentes
sobem.

**Rationale**: O briefing (sec.6) fixa Go + PostgreSQL + RabbitMQ e deploy
on-premise/local "sem dependencia de cloud". Volume MVP (centenas a poucos
milhares de membros por ciclo; poucas a dezenas de dispositivos) nao justifica
microservicos. Um binario unico com componentes ativaveis por env (`RUN_HTTP`,
`RUN_SCHEDULER`, `RUN_WORKERS`) preserva a escalabilidade horizontal exigida por
SC-006 (rodar so workers em replicas) sem o custo operacional de multiplos
deploys. A durabilidade vem do RabbitMQ (Constitution Principio III), nao do
numero de processos.

**Alternatives considered**:
- *Microservicos separados (loader, worker, webhook-api)*: rejeitado — overhead de
  deploy/observabilidade incompativel com deploy on-premise simples e volume MVP.
- *Cron externo (systemd timer / k8s CronJob) para a carga*: rejeitado como default —
  acopla a frequencia ao orquestrador de SO/cloud; a spec (FR-021-INFRA-SCHED, dec-023)
  exige ticker interno + trigger manual no proprio servico. Operador ainda PODE
  desativar o ticker (`MEMBER_SYNC_INTERVAL_MINUTES=0`) e acionar via cron externo
  chamando o endpoint de trigger — fica como opcao, nao como mecanismo primario.

## Decision 1 — Frequencia do ciclo de carga (ticker interno + trigger manual)

**Decision**: Ticker Go (`time.Ticker`) com periodo `MEMBER_SYNC_INTERVAL_MINUTES`
(default 60). Endpoint HTTP `POST /admin/sync` dispara um ciclo sob demanda. Os
dois caminhos chamam a mesma funcao de sincronizacao; execucoes concorrentes sao
serializadas por um lock em memoria (mutex `try-lock`) para evitar duas cargas
simultaneas.

**Rationale**: Resolve diretamente o item 9 do briefing ("Frequencia do ciclo de
carga... cron? trigger manual? webhook GOB?") via dec-023 / FR-021-INFRA-SCHED. O
briefing sec.3.1 descreve a carga como "chamada periodica (ou sob demanda)" —
ambos os modos sao requisito, nao escolha. `MEMBER_SYNC_INTERVAL_MINUTES=0`
desativa o ticker (so trigger manual), util quando um cron externo controla a
cadencia.

**Alternatives considered**:
- *Webhook da GOB notificando mudancas*: rejeitado — `t.txt` e o briefing nao
  descrevem nenhum endpoint de notificacao push da GOB; supor um seria fabricar
  contrato (Principio I). Pull periodico e o unico mecanismo verificavel.
- *Cron de SO*: rejeitado como primario (ver Decision 0); fica como uso opcional do
  endpoint de trigger.

## Decision 2 — Estrategia de retry/DLQ (RabbitMQ + backoff exponencial)

**Decision**: Cada fila de trabalho (`member.processing`) tem uma DLX
(dead-letter exchange) associada. Falhas transitorias sao re-tentadas com backoff
exponencial no consumidor ate `RETRY_MAX_ATTEMPTS` (default 3), partindo de
`RETRY_INITIAL_BACKOFF_MS` (default 1000ms). O numero da tentativa viaja no header
AMQP `x-retry-count`. Esgotadas as tentativas, a mensagem e publicada na DLQ
(`member.processing.dlq`) com header `x-death`/`x-failure-reason` para inspecao.
Mesma politica aplica a marcacao GOB (FR-015), encapsulada num cliente HTTP com
retry.

**Rationale**: Constitution Principio III (Resiliencia por Filas) e FR-023-INFRA-RETRY
/ dec-024 fixam retry configuravel + DLQ. Backoff no consumidor (com requeue
atrasado via fila de espera com TTL, ou re-publish com delay) evita tempestade de
retentativas contra um dispositivo indisponivel. SC-004 (nenhuma mensagem de
membro valido perdida) exige DLQ como rede de seguranca.

**Padrao de delay escolhido**: fila de espera dedicada com `x-message-ttl` por
tentativa + `x-dead-letter-exchange` de volta a fila de trabalho ("retry queue
pattern"), em vez de `time.Sleep` no consumidor (que prenderia o canal). Detalhe
de implementacao confirmado na fase execute-task.

**Alternatives considered**:
- *`time.Sleep` no handler*: rejeitado — bloqueia o worker e nao sobrevive a
  restart; perde o estado de backoff.
- *Sem DLQ, apenas descartar apos N tentativas*: rejeitado — viola Principio III e
  SC-004 (mensagem perdida).

## Decision 3 — Idempotencia chaveada por CPF

**Decision**: O CPF (`federal_document`) e a chave de correlacao unica em toda a
pipeline (FR-008, FR-022-INFRA-IDEMP, Constitution Principio II). As escritas
externas sao naturalmente idempotentes pelo proprio contrato HikVision:
`UserInfo/Modify` faz upsert por `employeeNo`; `faceDataRecord` usa `FPID` =
CPF; a marcacao GOB envia `{ "cpf": ... }`. Localmente, a tabela `members`
tem `federal_document` UNIQUE; o processamento de um evento de reconhecimento
e deduplicado por uma chave de evento (ver Decision 6).

**Rationale**: Re-entrega de mensagem RabbitMQ (retry, restart, redelivery) e o
caso normal numa arquitetura de filas (Principio III). Sem idempotencia por CPF,
cada falha de rede duplicaria usuario/face/presenca. As tres operacoes ISAPI
foram verificadas como upsert-por-CPF no codigo legacy (ver Decision 4).

**Alternatives considered**:
- *Hash do CPF como identificador no dispositivo*: rejeitado — briefing sec.4
  (trade-off aceito) documenta CPF direto em `employeeNo`; hash exigiria
  mapeamento extra no webhook handler sem ganho no MVP.

## Decision 4 — Reuso restrito do legacy hik-api (3 operacoes ISAPI) reescritas em Go

**Decision**: As tres operacoes ISAPI sao **reimplementadas em Go**, replicando
fielmente os contratos verificados no PHP legacy (endpoints, metodos, content-types,
shape de payload, codigos de status aceitos), SEM portar a infraestrutura legacy
(cache, auth de aplicacao, demais controllers/servicos). O legacy e fonte de
verdade do CONTRATO, nao biblioteca a ser linkada (e PHP/Hyperf — incompativel com
Go). Auth no dispositivo = **HTTP Digest** (usuario/senha ISAPI).

Contratos verificados (Constitution Principio I):

| Operacao | Metodo + Endpoint | Content-Type | Status OK | Fonte |
|----------|-------------------|--------------|-----------|-------|
| Upsert usuario (create) | `POST /ISAPI/AccessControl/UserInfo/Modify` | `application/xml` | 200/201 | `UserService.php:34,146-184` |
| Upsert usuario (update) | `PUT /ISAPI/AccessControl/UserInfo/Modify` | `application/xml` | 200/204 | `UserService.php:34,186-227` |
| Upload de face | `POST /ISAPI/Intelligent/FDLib/faceDataRecord?format=json` (multipart) | `multipart/form-data` | 200 | `FaceService.php:30,42-44,158-221` |
| Configurar webhook | `POST /ISAPI/Event/notification/httpHosts` | `application/xml` | 200/201 | `NotificationService.php:33,360-375` |

Detalhes verificados de payload:
- `UserInfo`: `employeeNo` = CPF, `name` = nome do membro (`UserService.php:193`,
  `parseUserData:435-447`).
- `faceDataRecord` multipart: parte campo `FaceDataRecord` = JSON
  `{"type":"concurrent","faceLibType":"blackFD","FDID":"1","FPID":"<CPF>"}`; parte
  arquivo `FaceImage` nomeada `<CPF>.jpg`, mime detectado da imagem
  (`FaceService.php:42-44,172-191`). Constantes `FACE_LIB_TYPE='blackFD'`,
  `FACE_LIB_ID='1'`.
- `HttpHostNotification`: `id`, `url`, `protocolType='HTTP'`,
  `parameterFormatType='XML'`, `addressingFormatType='ipaddress'`, `ipAddress`,
  `portNo`, `path`, `httpAuthenticationMethod='none'` (`NotificationService.php:360-375`).
- Auth: `AuthType::DIGEST` (`HttpClient.php:176`, `AuthType.php`).

**Rationale**: Constitution Principio IV (Reuso Restrito) e FR-013 limitam o reuso
EXCLUSIVAMENTE a essas tres operacoes. O legacy e PHP/Hyperf, nao linkavel em Go;
reescrever os clientes em Go mantendo o contrato e a unica forma de cumprir IV sem
arrastar cache/auth/controllers nao-verificados.

**Alternatives considered**:
- *Chamar o servico legacy PHP como sidecar*: rejeitado — arrasta toda a
  infraestrutura Hyperf/Swoole/Redis do legacy para o deploy, violando o espirito do
  Principio IV (superficie minima) e o requisito de deploy simples on-premise.
- *Inventar payloads "padrao ISAPI" de documentacao generica*: rejeitado — Principio I;
  usamos exatamente o que o codigo legacy emite.

## Decision 5 — Cliente HTTP Go com Digest Auth para ISAPI

**Decision**: Usar uma lib Go de HTTP Digest (ex: `github.com/icholy/digest` como
RoundTripper sobre `net/http`) para autenticar nas chamadas ISAPI. A escolha exata
da lib e confirmada na fase execute-task contra `go.mod`; o requisito firme e:
Digest auth, suporte a `multipart/form-data` (face), body XML cru (usuario/webhook)
e timeouts configuraveis.

**Rationale**: O legacy usa Guzzle com `auth => [user, pass, 'digest']`
(`HttpClient.php:168-180`). HikVision DS-K1T673DWX usa Digest (comentario explicito
em `AuthType.php`). `net/http` puro nao implementa Digest (challenge-response);
uma lib dedicada e necessaria.

> NOTA: a versao/import exato da lib e `[PROPOSTA — a validar na implementacao]`.
> O requisito (Digest auth) e fato verificado; a biblioteca concreta e decisao de
> implementacao, marcada como proposta ate fixada em `go.mod`.

**Alternatives considered**:
- *Implementar Digest manualmente*: possivel, porem retrabalho e risco; preferir lib
  testada. Reavaliar so se a dep adicionar peso indevido.
- *Basic auth*: rejeitado — dispositivo usa Digest; Basic falharia o challenge.

## Decision 6 — Deteccao de reconhecimento positivo + idempotencia do evento

**Decision**: O webhook recebe o payload bruto da HikVision. O CPF e extraido de
`AccessControllerEvent.employeeNoString` (ou `EventNotificationAlert.employeeNoString`)
— campo bruto verificado (dec-022). Um evento e tratado como **reconhecimento
positivo** quando ha `employeeNoString` nao-vazio correspondente a um membro
conhecido e o evento e de controle de acesso autorizado (`attendanceStatus ==
"authorized"`, `WebhookEventProcessor.php:159`). A idempotencia da marcacao
(FR-016) usa uma chave de evento derivada de `(employeeNoString, dateTime,
device)` persistida em `attendance_events`; re-entrega do mesmo evento nao
re-marca.

**Rationale**: FR-014/FR-015/FR-017 e dec-022 fixam `employeeNoString` como campo
bruto. O legacy le exatamente esse campo (`WebhookController.php:212`,
`WebhookEventProcessor.php:154,232`) e o sinal de autorizacao em `attendanceStatus`
(`WebhookEventProcessor.php:159`). A chave de dedup local protege contra redelivery
do dispositivo e reprocessamento.

> NOTA: o formato XML/JSON exato e os valores de `majorEventType`/`subEventType`
> de um evento de reconhecimento facial real do DS-K1T673DWX nao constam do legacy
> com granularidade de codigo numerico; o parsing tolerante (extrair
> `employeeNoString` de `AccessControllerEvent` OU `EventNotificationAlert`,
> aceitar XML e JSON) replica o comportamento do legacy
> (`WebhookEventProcessor.php` faz `?? $payload` fallback). Itens marcados
> `[PROPOSTA — a validar na implementacao]` no contrato. Sem inventar codigos
> numericos de evento: filtramos por presenca de `employeeNoString` + autorizacao,
> nao por codigo magico nao-verificado.

**Alternatives considered**:
- *Confiar so em `eventType` string*: rejeitado — o legacy nao garante um valor
  fixo; filtrar por `employeeNoString` presente + membro conhecido e mais robusto
  e nao depende de string nao-verificada.

## Decision 7 — Formato do CPF nas fronteiras

**Decision**: Persistir `federal_document` exatamente como recebido da GOB (digits,
ex. `00000000000`, conforme `t.txt:21`). Para o HikVision (`employeeNo`/`FPID`),
usar o CPF no formato recebido da GOB (digits) — e o que sera devolvido em
`employeeNoString` e usado para correlacao. Para a marcacao GOB, o body usa o
formato mascarado `00.000.000-00` (verificado em `t.txt:48` e
`briefing.md:83,188`). Um helper de formatacao converte digits ↔ mascara numa
unica camada.

**Rationale**: Item 9 do briefing (formato do CPF) e ambiguidade #4 da spec
(`default-documentado`). A unica fonte que mostra mascara e o body de marcacao GOB
(`t.txt:48`); a unica fonte que mostra digits e a resposta de membros
(`t.txt:21`). Respeitar cada fronteira como a fonte mostra evita fabricar formato.
Correlacao webhook↔membro normaliza ambos para digits antes de comparar (defensivo).

**Alternatives considered**:
- *Mascara em todas as fronteiras*: rejeitado — a resposta da GOB entrega digits;
  forcar mascara no `employeeNo` poderia divergir do que o dispositivo devolve.
- *Digits em todas as fronteiras*: rejeitado — o body de marcacao GOB verificado
  usa mascara (`t.txt:48`); alterar seria fabricar contrato.

## Itens GENUINAMENTE pendentes (nao bloqueiam o desenho)

| Item | Natureza | Por que NAO bloqueia |
|------|----------|----------------------|
| Valor real de `GOB_STATE_TOKEN` | Segredo de runtime (config do operador) | Constitution Principio V — injetado em runtime; o desenho usa env var, nao o valor. |
| `GOB_STATE_URL` concreto | Config de runtime | `t.txt` mostra `https://digital.gob-es.org.br`; o desenho parametriza via env. |
| Credenciais ISAPI (IP/user/senha) de cada dispositivo | Config de runtime (item 9 briefing, FR-020) | Provisionadas via env/config por dispositivo; o desenho fixa o mecanismo, nao os valores. |
| Codigos numericos exatos de `majorEventType`/`subEventType` do evento facial | Nao consta do legacy com granularidade | Filtro por `employeeNoString` + autorizacao (Decision 6) nao depende deles; marcado PROPOSTA. |

Nenhum desses e dado factual fabricavel no plan — todos sao config de runtime
(Principio V) ou comportamento tolerante ja verificado no legacy. Nenhum exige
bloqueio humano para o desenho prosseguir.
