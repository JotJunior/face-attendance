# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## O que é

**Presença Facial** — serviço Go que automatiza o registro de presença de membros
via reconhecimento facial. Integra a plataforma **GOB** (fonte de membros e destino
da marcação de presença) com terminais **HikVision** (ISAPI, na LAN). Stack:
Go 1.26 + PostgreSQL (pgx/v5) + RabbitMQ (amqp091), deploy on-premise via Docker.
Sem framework web — `net/http` puro. Módulo: `github.com/jotjunior/face-attendance`.

## Comandos

```bash
make build            # compila bin/presenca-facial (CGO desligado, puro Go)
make test             # testes unitários: go test ./... -v -count=1
make lint             # go vet (+ staticcheck se instalado)
make docker-up        # sobe só Postgres + RabbitMQ (--wait)
make migrate-up       # aplica migrations (precisa do CLI golang-migrate)
make migrate-down     # rollback de 1 migration
make quickstart       # docker-up + migrate-up + build
make dev              # quickstart + roda o binário (requer env vars exportadas)

# Teste unitário único
go test ./internal/domain -run TestNormalizeCPF -v -count=1

# Testes de integração (build tag `integration`; exigem Postgres/RabbitMQ reais)
make test-integration                       # injeta TEST_DATABASE_URL do Makefile
go test ./internal/repository -tags integration -run TestX -v   # arquivo único
```

Integração precisa de `TEST_DATABASE_URL` (e `TEST_RABBITMQ_URL` para os testes de
fila). Rode `make docker-up` antes. Os testes de integração ficam atrás de
`//go:build integration` — `make test` (sem a tag) **não** os executa.

> Compose: Postgres e RabbitMQ **não publicam porta no host** (rede interna do
> compose). O `app` os acessa por service name. Migrations rodam no serviço
> one-shot `migrate` antes do `app` subir. `docker compose up -d` sobe tudo.

## Arquitetura

Um único binário (`cmd/presenca-facial/main.go` faz todo o wiring) com **três
subsistemas independentes**, ligados/desligados por `RUN_HTTP` / `RUN_SCHEDULER`
/ `RUN_WORKERS`:

1. **Scheduler** (`internal/scheduler`) — a cada `MEMBER_SYNC_INTERVAL_MINUTES`
   busca membros no GOB, faz upsert local e publica **uma mensagem por membro com
   selfie** na fila `member.processing`. Membros sem `url_selfie` são descartados,
   nunca enfileirados.
2. **Worker** (`internal/worker`) — consome `member.processing` e provisiona o
   membro em **TODOS os devices ativos** (multi-device) via ISAPI: `UpsertUser`
   (employeeNo = CPF) + `UploadFace` (FPID = CPF). Falha não-retriável num device
   é registrada por device e não bloqueia os demais; falha retriável re-enfileira
   com backoff exponencial → após `RETRY_MAX_ATTEMPTS`, DLQ.
3. **HTTP server** (`internal/http`, `:8080`) — webhook dos devices, health, sync
   admin e o painel de administração web.

**Fluxo de 4 etapas** (critério de aceite do MVP): registro de dispositivo → carga
de membros → enroll de usuário/face → marcação de presença. O device reconhece o
rosto e faz `POST /webhook/{secret}`; o handler valida o CPF e chama
`gob.Client.MarkAttendance(ctx, cpf)`.

```
GOB ──pull──► Scheduler ──► RabbitMQ (member.processing) ──► Worker ──ISAPI──► Devices
                                                                                  │ evento
GOB ◄──MarkAttendance── HTTP /webhook/{secret} ◄──────────────────────────────────┘
```

Pacotes em `internal/`: `config` (env), `domain` (modelos + CPF), `gob` (cliente
GOB), `hikvision` (cliente ISAPI, split por área: users/faces/doors/system/webhooks),
`http` (servidor/handlers/middleware), `logging`, `queue` (topologia + pub/consume),
`repository` (pgx), `scheduler`, `secrets` (AES-256-GCM), `web` (SPA embarcada),
`worker`. Cada pacote tem um `doc.go`.

## Governança (SDD) — leia antes de implementar

Este projeto é **spec-driven**. `docs/constitution.md` é a fonte de governança e
**prevalece sobre conveniência/prazo**. Features vivem em `docs/specs/{feature}/`
com `spec.md` / `plan.md` / `tasks.md` / `data-model.md` / `research.md`. O código
referencia esses artefatos (ex.: `spec.md §FR-013`, `tasks.md §4.2`) — ao mexer
numa área, consulte o spec correspondente. Features atuais:
`presenca-facial-mvp`, `device-config`, `front-end`.

Princípios **NON-NEGOTIABLE** da constituição que moldam o código:

- **I. Zero fabricação de dados** — assinaturas de request/response, URLs/endpoints
  e valores concretos só entram se vierem de fonte rastreável (`legacy/hik-api`,
  `t.txt`, briefing, doc oficial ISAPI/GOB, ou chamada real). Sem fonte →
  **bloqueio humano**, nunca suposição plausível.
- **II. Idempotência chaveada por CPF** — toda escrita externa usa o CPF como
  chave (employeeNo, FPID, `{cpf}` no GOB). Reprocessar a mesma mensagem deve
  produzir o mesmo estado, sem duplicar usuário/face/presença. Todo recurso
  externo escrito **exige teste de idempotência**.

Demais princípios: resiliência por filas (retry+DLQ), reuso de `legacy/hik-api`
restrito **só às 3 operações ISAPI** (upsert user, upload face, set webhook —
nada de cache/auth/controllers do legacy), segredos como config de runtime,
observabilidade (log JSON estruturado), e idioma da sintaxe (identifiers em inglês,
mensagens/comentários em português).

## Convenções e invariantes não-óbvios

- **CPF é a chave de correlação de todo o produto.** Use os helpers de
  `internal/domain/cpf.go`: `NormalizeCPF`, `ValidateCPF`, `MaskCPFForLog`,
  `FormatCPF`/`ParseCPF`. O que é enviado ao device (employeeNo) tem que bater com
  o que volta no webhook e vai ao GOB — um formato divergente quebra a presença em
  silêncio.

- **Logging estruturado obrigatório.** Use o wrapper `internal/logging`, não
  `slog` direto. Assinatura: `logger.Info(stage, deviceID, cpfRaw, msg, extra...)`.
  O `cpfRaw` é **sempre mascarado** automaticamente — nunca passe CPF cru noutro
  campo. **Nunca** passe segredos (tokens, senhas ISAPI) como args de log.

- **A tabela `devices` é a fonte de verdade da conexão ISAPI**, não o `.env`. O IP
  acompanha o heartbeat do device (troca de IP não derruba a conexão); as
  credenciais ficam cifradas (AES-256-GCM, `internal/secrets`, chave em
  `ISAPI_CRED_KEY`). As vars `ISAPI_DEVICE_{N}_*` são apenas **bootstrap/seed** na
  1ª inicialização. O `dbConnResolver` (em `main.go`) resolve os alvos por mensagem.
  Sem `ISAPI_CRED_KEY` → comportamento legado (credenciais lidas do `.env`).

- **Webhook = IP allowlist dinâmica.** Só aceita IPs de linhas ativas em `devices`
  (consultadas a cada request). Um device novo só se auto-registra no heartbeat
  **se o IP já estiver liberado** — bootstrap exige inserir a linha em `devices`
  manualmente uma vez.

- **Painel admin** é ligado quando `ADMIN_USERNAME` + `ADMIN_SESSION_SECRET` estão
  presentes. Sessão via cookie HMAC (`internal/http/session.go`). A SPA é servida
  de `internal/web/dist` via `embed.FS` (precisa de `fs.Sub` — ver `web/embed.go`).
  Quando o painel está ligado, o `Publisher` do RabbitMQ também é necessário (o
  resync individual de membro publica na fila). `ADMIN_COOKIE_SECURE=false` em
  deploy HTTP puro sem TLS, senão o login entra em loop de redirect.

- **Roteamento HTTP é stdlib.** `ServeMux` + handlers como funções-construtoras que
  retornam `http.Handler`. O subtree `/admin/api/devices/*` tem roteamento manual
  por segmento de path (`adminDevicesRouter` em `server.go`). Middlewares:
  `IPAllowlist`, `RateLimit`, `AdminAuth` (Bearer), `Session` (cookie HMAC).

- **Topologia RabbitMQ** (`internal/queue/setup.go`, idempotente): exchange
  `member.processing.exchange` (direct) → fila `member.processing`; DLX
  `member.processing.dlx` (fanout) → `member.processing.dlq`. Retentativas contadas
  no header `x-retry-count`. Erros não-retriáveis vão direto à DLQ — a
  classificação é `hikvision.IsNonRetriable` / `hikvision.NonRetriableError`.

- **Cliente ISAPI** (`internal/hikvision`) usa **digest auth** (`icholy/digest`) e
  fala XML + multipart. Selfies do GOB são **transcodificadas para JPEG** antes do
  upload (limite ~200 KB do device); fotos sem rosto são rejeitadas pelo device.

- **Migrations** em `migrations/`, formato golang-migrate (`NNNNNN_nome.{up,down}.sql`).
  Toda mudança de schema é uma migration nova numerada — nunca edite uma existente.

- `t.txt` (raiz) e `legacy/hik-api` são fontes de contrato verificadas — consulte-os
  para nomes de campo/rota ISAPI antes de inventar (ver Princípio I).
