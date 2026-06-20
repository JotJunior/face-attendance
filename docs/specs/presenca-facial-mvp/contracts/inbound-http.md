# Contract — Inbound HTTP (API local exposta)

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20
**Direcao**: INBOUND (dispositivos e operador chamam a API local)

> O webhook recebido da HikVision tem seu payload extraido do codigo legacy
> (`WebhookController.php`, `WebhookEventProcessor.php`) — campos reais
> (Constitution Principio I). Os endpoints de health/trigger sao NOVOS (projetados
> deste zero) e marcados `[PROPOSTA]` por serem da nossa API, nao contrato externo.

## 1. POST {webhook_path} — receber evento da HikVision

**Fonte do payload**: `WebhookController.php:199-215`,
`WebhookEventProcessor.php:135-162`. FR-014, FR-015, FR-017.

O `{webhook_path}` e a rota configurada no dispositivo via
`HttpHostNotification.path` (ver contrato ISAPI). O dispositivo envia o evento
neste endpoint quando reconhece uma face.

### Request (payload bruto da HikVision)
O dispositivo pode enviar XML ou JSON; o parser e tolerante (o legacy faz
`json_decode(json_encode(simplexml))` para XML, `HttpClient.php:266-279`, e o
processor aceita `$payload` ou `$payload['EventNotificationAlert']`,
`WebhookEventProcessor.php:149`).

Campos verificados do payload (nivel raiz e dentro de `AccessControllerEvent` /
`EventNotificationAlert`):

| Campo | Local | Tipo | Fonte | Uso |
|-------|-------|------|-------|-----|
| `eventType` | raiz | string | `WebhookController.php:199` | classificacao |
| `ipAddress` | raiz | string | `:200` | correlacao de dispositivo |
| `macAddress` | raiz | string | `:201` | correlacao de dispositivo |
| `dateTime` | raiz / alert | string | `:202`, `WebhookEventProcessor.php:153` | timestamp do evento |
| `majorEventType` | AccessControllerEvent | (int/string) | `WebhookController.php:209` | tipo de evento |
| `subEventType` | AccessControllerEvent | (int/string) | `:210` | subtipo |
| `name` | AccessControllerEvent / alert | string | `:211`, processor:153 | nome do membro |
| `employeeNoString` | AccessControllerEvent / alert | string | `:212`, processor:154,232 | **CPF bruto (dec-022)** |
| `cardReaderNo` | AccessControllerEvent | (int) | `:213` | leitor |
| `doorNo` | AccessControllerEvent / alert | (int) | `:214`, processor:155 | porta |
| `currentVerifyMode` | AccessControllerEvent | string | `:215` | modo de verificacao |
| `attendanceStatus` | alert | string | processor:159 | `authorized` = reconhecimento positivo |

### Processamento (FR-014..FR-017)
1. Extrair `employeeNoString` de `AccessControllerEvent` OU `EventNotificationAlert`.
2. Se vazio ou nao corresponde a membro conhecido → **log estruturado**, sem marcar
   presenca (FR-017, US1 cenario 3). Persistir em `attendance_events` com
   `marked=false`.
3. Se membro conhecido + evento autorizado (`attendanceStatus == "authorized"`) →
   dedup por `event_key`; se novo → marcar presenca na GOB
   (`POST /attendance/...`, ver contrato GOB) e setar `marked=true`.
4. Re-entrega do mesmo evento → no-op (FR-016, idempotencia via `event_key`).

### Response
- **200 OK** sempre que o evento for aceito para processamento (o dispositivo so
  precisa saber que foi recebido). O processamento de marcacao e assincrono/
  resiliente; nunca derrubar o handler por payload malformado (Edge Case spec).
- Payload malformado → **200** + log estruturado (nao 4xx, para o dispositivo nao
  re-tentar em loop). `[PROPOSTA — a validar na implementacao]`: o codigo de status
  exato esperado pelo DS-K1T673DWX nao consta do legacy; default 200.

> `[PROPOSTA — a validar na implementacao]`: os valores numericos exatos de
> `majorEventType`/`subEventType` que identificam "reconhecimento facial positivo"
> NAO constam do codigo legacy. Por isso o filtro de positividade usa
> `employeeNoString` nao-vazio + membro conhecido + `attendanceStatus==authorized`,
> sinais TODOS verificados no legacy — NAO um codigo magico inventado.

---

## 2. POST {heartbeat_path} — heartbeat / registro de dispositivo

**Fonte**: comportamento descrito em `t.txt:3-6`, `briefing.md:36-40,148`.
FR-001, FR-002. (Endpoint NOVO da nossa API — campos de identificacao do
heartbeat real definem o contrato exato.)

### Request `[PROPOSTA — shape a validar com heartbeat real]`
O heartbeat do dispositivo carrega ao menos um identificador estavel (MAC/serial)
+ IP. O legacy observa `macAddress` e `ipAddress` no payload de eventos
(`WebhookController.php:200-201`); o heartbeat real do DS-K1T673DWX define os
campos exatos.

### Processamento
1. Identificar o dispositivo (MAC/serial). Primeira vez → INSERT em `devices`
   (`is_active=true`), registro automatico (FR-001).
2. Ja registrado → UPDATE `last_heartbeat_at` (liveness), sem duplicar (FR-002).

### Response
- **200 OK**.

> NOTA: heartbeat e webhook PODEM ser a mesma rota fisica se o dispositivo enviar
> ambos pela URL configurada; nesse caso o handler distingue por `eventType`. A
> decisao (rota unica vs separada) e de execute-task; o contrato firme e: receber
> heartbeat → registrar/atualizar dispositivo.

---

## 3. GET /health — health check

**Fonte**: FR-019, Constitution Principio VI. (Endpoint NOVO.)

### Response 200 (application/json) `[PROPOSTA]`
```json
{ "status": "ok", "db": "ok", "rabbitmq": "ok" }
```
- 200 = saudavel; 503 = dependencia critica (DB/RabbitMQ) indisponivel.

---

## 4. POST /admin/sync — disparar ciclo de carga manualmente

**Fonte**: FR-021-INFRA-SCHED, dec-023. (Endpoint NOVO.)

### Request
```
POST /admin/sync
```
- Dispara um ciclo de carga de membros (mesmo caminho do ticker). Concorrencia
  serializada (ver research Decision 1).

### Response `[PROPOSTA]`
- **202 Accepted** se o ciclo foi iniciado; **409 Conflict** se ja ha ciclo em
  andamento.

> `[PROPOSTA — a validar na implementacao]`: proteger `/admin/*` por mecanismo de
> auth simples (token de admin via env) — Principio V. Detalhe de execute-task; NAO
> reutilizar a auth do legacy (Principio IV).

---

## Convencao de chaves
- Payload bruto da HikVision: chaves como o dispositivo envia (verificadas acima) —
  NAO normalizamos os nomes de entrada; extraimos `employeeNoString` tal-qual.
- Respostas das nossas rotas novas (health/sync): JSON camelCase (convencao de
  borda do plan).
