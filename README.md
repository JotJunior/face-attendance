# Presença Facial

Serviço em Go que automatiza o registro de presença de membros via reconhecimento
facial. Integra a plataforma **GOB** (fonte de membros e destino da marcação de
presença) com dispositivos **HikVision** (terminais de acesso ISAPI na rede local)
que fazem o reconhecimento e disparam eventos.

Stack: **Go + PostgreSQL + RabbitMQ**, deploy on-premise via Docker.

## Arquitetura

```
        GOB State API                         Dispositivo HikVision (ISAPI)
              │  (membros + selfies)                 ▲            │ (evento de
              ▼                                       │ enroll     │  reconhecimento)
   ┌──────────────────────────────────────────────────────────────────────┐
   │  presença-facial (Go)                                                  │
   │                                                                        │
   │  scheduler ──► RabbitMQ (member.processing) ──► worker ──► ISAPI       │
   │   (pull periódico,                              (cadastra usuário +    │
   │    publica por membro)                           face no dispositivo)  │
   │                                                                        │
   │  HTTP :8080                                                            │
   │   POST /webhook/{secret} ◄──────────────────── dispositivo (evento)   │
   │      └─► valida CPF ─► marca presença no GOB                           │
   │   GET  /health                                                         │
   │   POST /admin/sync  (dispara um ciclo de carga sob demanda)           │
   └──────────────────────────────────────────────────────────────────────┘
              │                              │
              ▼                              ▼
        PostgreSQL                       RabbitMQ
   (members, devices,              (member.processing
    attendance_events,              + DLQ)
    member_processing_status)
```

Fluxo resumido:

1. **Scheduler** busca membros no GOB periodicamente, faz upsert local e publica
   uma mensagem na fila para cada membro **com selfie**.
2. **Worker** consome a fila e provisiona o membro no dispositivo via ISAPI:
   cria/atualiza o usuário e envia a face.
3. **Dispositivo** reconhece o rosto e faz `POST` no webhook do serviço.
4. O serviço valida o CPF, registra o evento e **marca a presença no GOB**.

## Pré-requisitos

- Docker + Docker Compose v2.
- Acesso de rede do host ao dispositivo HikVision e à API do GOB.
- O dispositivo deve estar na mesma LAN e conseguir alcançar o host na porta `8080`.

## Configuração

Copie o exemplo e preencha os valores reais:

```bash
cp .env.example .env
```

> `.env` contém segredos e **não** deve ser commitado (já está no `.gitignore`).

### Variáveis

Ao usar o `docker-compose.yml` deste repositório, `DATABASE_URL` e `RABBITMQ_URL`
são injetadas automaticamente apontando para os serviços internos — você só
preenche as demais no `.env`.

| Variável | Obrigatória | Default | Descrição |
|---|---|---|---|
| `GOB_STATE_URL` | sim | — | Endpoint da GOB State API (fonte de membros / marcação) |
| `GOB_STATE_TOKEN` | sim | — | Token de acesso ao GOB |
| `ADMIN_TOKEN` | sim | — | Bearer token para `POST /admin/sync` |
| `WEBHOOK_PATH_SECRET` | sim | — | Segredo no path do webhook (`/webhook/{secret}`) |
| `POSTGRES_PASSWORD` | — | `presenca_dev` | Senha do Postgres do compose |
| `RABBITMQ_USER` / `RABBITMQ_PASS` | — | `guest` / `guest` | Credenciais do RabbitMQ do compose |
| `MEMBER_SYNC_INTERVAL_MINUTES` | — | `60` | Intervalo do ciclo de carga de membros |
| `RETRY_MAX_ATTEMPTS` | — | `3` | Tentativas antes de mandar à DLQ |
| `RETRY_INITIAL_BACKOFF_MS` | — | `1000` | Backoff inicial (exponencial) do retry |
| `WEBHOOK_RATE_LIMIT_PER_IP_PER_MIN` | — | `60` | Rate limit do webhook por IP |
| `ADMIN_SYNC_MIN_INTERVAL_SECONDS` | — | `60` | Intervalo mínimo entre syncs manuais |
| `RUN_HTTP` / `RUN_SCHEDULER` / `RUN_WORKERS` | — | `true` | Liga/desliga cada subsistema |
| `ISAPI_DEVICE_{N}_HOST` | — | — | Host/IP do dispositivo `N` (1, 2, …) |
| `ISAPI_DEVICE_{N}_USER` | — | `admin` | Usuário ISAPI do dispositivo `N` |
| `ISAPI_DEVICE_{N}_PASSWORD` | — | — | Senha ISAPI do dispositivo `N` |

Os dispositivos são lidos incrementando o índice (`ISAPI_DEVICE_1_*`,
`ISAPI_DEVICE_2_*`, …) até não encontrar mais `_HOST`.

## Subindo o serviço

```bash
docker compose up -d
```

Isso sobe `postgres`, `rabbitmq`, aplica as migrations (serviço `migrate`,
one-shot) e inicia o `app` em `:8080`.

Valide:

```bash
curl http://localhost:8080/health
# {"db":"ok","rabbitmq":"ok","status":"ok"}
```

> **Nota (Docker Compose v5 / buildx < 0.17):** se aparecer
> `compose build requires buildx 0.17 or later`, pré-construa a imagem e suba sem
> build (o serviço `app` referencia `image: presenca-facial:local`):
>
> ```bash
> docker build -t presenca-facial:local .
> docker compose up -d --no-build
> ```

## Endpoints HTTP

| Método | Rota | Auth | Descrição |
|---|---|---|---|
| `POST` | `/webhook/{WEBHOOK_PATH_SECRET}` | IP allowlist + rate limit | Recebe eventos do dispositivo |
| `GET` | `/health` | nenhuma | Status de Postgres e RabbitMQ |
| `POST` | `/admin/sync` | `Authorization: Bearer {ADMIN_TOKEN}` | Dispara um ciclo de carga de membros |

Disparar um sync manual:

```bash
curl -X POST http://localhost:8080/admin/sync \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

## Configuração do dispositivo HikVision

Requisitos verificados para o fluxo funcionar de ponta a ponta:

1. **Webhook (HTTP Listening / Notificação):** aponte o dispositivo para
   `http://<IP_DO_HOST>:8080/webhook/<WEBHOOK_PATH_SECRET>`. O dispositivo envia
   o evento como `multipart/form-data` (parte `event_log`, JSON) — o serviço já
   trata esse formato.
2. **Allowlist:** o webhook só aceita eventos de IPs presentes e ativos na tabela
   `devices`. O dispositivo se auto-registra no primeiro heartbeat **desde que seu
   IP já esteja liberado** — para o bootstrap de um dispositivo novo, insira uma
   vez a linha em `devices` (use o IP como `device_identifier` e `ip_address`,
   `is_active = true`).
3. **NTP / relógio:** configure a hora correta no dispositivo. Relógio errado
   invalida a janela de validade do usuário e o timestamp dos eventos.
4. **Provisionamento (feito pelo serviço):** ao cadastrar o usuário, o worker
   envia `userType=normal`, janela `Valid` ampla e `RightPlan` vinculando o
   template de horário 24/7 (porta 1, `planTemplateNo="1"`). O dispositivo deve
   ter o `UserRightPlanTemplate/1` e o week plan correspondente configurados
   (padrão de fábrica: liberado todos os dias).
5. **Selfies:** as imagens do GOB são transcodificadas para JPEG antes do upload
   (o dispositivo tem limite de tamanho ~200 KB). Fotos sem rosto detectável são
   rejeitadas pelo próprio dispositivo e não cadastram.

A presença é marcada no GOB quando o evento é uma **face autenticada com sucesso**
(`AccessControllerEvent` com `majorEventType=5`, `subEventType=75`) ou quando o
dispositivo opera em attendance mode (`attendanceStatus=authorized`).

## Resiliência

- Falhas transitórias (timeout, 5xx, conexão recusada) são re-tentadas com backoff
  exponencial; após `RETRY_MAX_ATTEMPTS`, a mensagem vai para a **DLQ**
  (`member.processing.dlq`).
- Erros não-retentáveis (CPF inválido, imagem sem rosto, payload malformado) vão
  direto para a DLQ.
- As operações são **idempotentes por CPF**: reprocessar uma mensagem não duplica
  usuário, face nem presença.

## Desenvolvimento

Requer Go (ver versão em `go.mod`).

```bash
make build            # compila o binário em bin/
make test             # testes unitários (sem dependências externas)
make docker-up        # sobe apenas Postgres + RabbitMQ
make migrate-up       # aplica migrations (requer golang-migrate CLI)
make test-integration # testes contra Postgres/RabbitMQ reais
make lint             # go vet (+ staticcheck se instalado)
```

Estrutura:

```
cmd/presenca-facial/   entrypoint
internal/
  config/        carga de configuração (env)
  scheduler/     ciclo de carga de membros (GOB → fila)
  worker/        consumer da fila → provisionamento ISAPI
  hikvision/     cliente ISAPI (UpsertUser, UploadFace, ConfigureWebhook)
  http/          servidor, handlers e middlewares (webhook/health/admin)
  gob/           cliente da GOB State API
  repository/    persistência (members, devices, events, processing status)
  queue/         topologia e publisher/consumer do RabbitMQ
  domain/        modelos e regras (CPF, member, device, event)
migrations/      migrations SQL (golang-migrate)
```

## Solução de problemas

| Sintoma | Causa provável | Ação |
|---|---|---|
| Webhook responde **403** | IP do dispositivo fora da allowlist (`devices`) | Registrar/ativar o dispositivo em `devices` |
| `payload not JSON` nos logs | Formato inesperado | O serviço espera `multipart/form-data` (`event_log`) ou JSON puro |
| Membro reconhecido mas **presença não marcada** | Evento não é acesso concedido | Conferir `majorEventType`/`subEventType` no `raw_payload` do evento |
| Dispositivo: **"Duração inválida"** | Usuário sem `RightPlan.planTemplateNo` válido | Garantir template de horário no dispositivo; reprocessar o membro |
| Face vai para DLQ (`SubpicAnalysisModelingError`) | Foto sem rosto detectável | Qualidade da imagem de origem (GOB) |
| Face vai para DLQ (`badJsonContent`) | Imagem acima do limite do dispositivo | Já mitigado por transcodificação JPEG |
