# Implementation Plan: MVP de Controle de Presenca por Reconhecimento Facial

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)

## Summary

Servico Go on-premise que automatiza a marcacao de presenca por reconhecimento
facial, integrando a GOB (fonte de membros + destino da presenca) e dispositivos
HikVision (ISAPI, rede local). Pipeline de 4 etapas: (0) registro de dispositivos
por heartbeat; (1) carga periodica/sob-demanda de membros da GOB com filtro por
`url_selfie` e enfileiramento RabbitMQ; (2) worker que faz upsert de usuario + upload
de face + configura webhook no dispositivo (3 operacoes ISAPI reescritas em Go a
partir do contrato verificado no legacy `hik-api`); (3) webhook que recebe o evento
de reconhecimento, extrai o CPF de `employeeNoString` e marca presenca na GOB.
Resiliencia por filas (retry + DLQ) e idempotencia chaveada por CPF em todas as
escritas externas. Todos os contratos externos foram extraidos de fontes reais
(`t.txt`, `legacy/hik-api`) — nenhum endpoint/campo inventado.

## Technical Context

**Language/Version**: Go (>= 1.22; versao exata fixada no `go.mod` em execute-task — Constitution Stack)
**Primary Dependencies**: driver PostgreSQL (`pgx` ou `database/sql`), cliente RabbitMQ (`amqp091-go`), HTTP Digest (`github.com/icholy/digest` ou equivalente) `[PROPOSTA — fixar em go.mod]`, lib de migration `[PROPOSTA]`
**Storage**: PostgreSQL (members, devices, attendance_events, member_processing_status)
**Messaging**: RabbitMQ (fila `member.processing` + DLX/DLQ; retry queue pattern)
**Testing**: `go test` (unit + integration com stubs HTTP/AMQP; roundtrip E2E no quickstart)
**Target Platform**: on-premise / local (container ou processo unico; sem dependencia de cloud)
**Project Type**: web-service (HTTP server + scheduler + workers no mesmo binario, componentes ativaveis por env)
**Performance Goals**: presenca marcada < 5s apos webhook (SC-002); ciclo de carga de poucos milhares de membros sem intervencao (SC-006)
**Constraints**: segredos so via env de runtime (Principio V); reuso legacy restrito a 3 operacoes ISAPI (Principio IV); idempotencia por CPF (Principio II)
**Scale/Scope**: centenas a poucos milhares de membros/ciclo; poucas a dezenas de dispositivos; workers escalaveis horizontalmente

## Constitution Check

*GATE: Deve passar antes do Phase 0. Re-checado apos Phase 1 (secao final).*

| Principio | Status | Notas |
|-----------|--------|-------|
| I. Veracidade de Dados (NON-NEGOTIABLE) | PASS | Todos os contratos externos extraidos de `t.txt` + `legacy/hik-api` com citacao arquivo:linha (research §4, contracts/). Itens nao-verificaveis marcados `[PROPOSTA]`, nao afirmados como reais. |
| II. Idempotencia por CPF (NON-NEGOTIABLE) | PASS | CPF = chave unica (FR-008/022). `members.federal_document` UNIQUE; upserts ISAPI por `employeeNo`/`FPID`; dedup de evento por `event_key`; marcacao GOB por `{cpf}`. |
| III. Resiliencia por Filas (retry + DLQ) | PASS | RabbitMQ + DLX/DLQ; retry configuravel com backoff (FR-023); SC-004 (nenhuma msg perdida). research §2. |
| IV. Reuso Restrito do Legacy | PASS | Apenas 3 operacoes ISAPI reescritas em Go; cache/auth/demais controllers EXCLUIDOS explicitamente (contracts/hikvision-isapi.md §Reuso PROIBIDO). |
| V. Segredos como Runtime | PASS | `GOB_STATE_URL`, `GOB_STATE_TOKEN`, credenciais ISAPI por env (FR-020); logs sem segredos; `/admin/*` protegido por token de admin. |
| VI. Observabilidade | PASS | Log JSON estruturado (`device_id`, `cpf`, `stage`, `error`) (FR-018); `GET /health` (FR-019); heartbeat = registro + liveness (FR-001/002). |
| VII. Idioma da Sintaxe | PASS | Identifiers (tabelas/colunas/campos/rotas/filas) em ingles; mensagens/comentarios em portugues. |

**Gate: PASS** — sem violacoes de principio MUST. Prosseguiu para Phase 0/1.

## Project Structure

### Documentation (this feature)

```
docs/specs/presenca-facial-mvp/
├── spec.md
├── plan.md          # This file
├── research.md      # Phase 0 output
├── data-model.md    # Phase 1 output
├── quickstart.md    # Phase 1 output
└── contracts/       # Phase 1 output
    ├── gob-api.md
    ├── hikvision-isapi.md
    └── inbound-http.md
```

### Source Code (repository root)

> Estrutura PROPOSTA (o repo so tem `docs/` e `legacy/` hoje; o codigo Go sera
> criado na fase execute-task). Layout idiomatico Go com `cmd/` + `internal/`.

```
face-attendance/
├── cmd/
│   └── presenca-facial/        # main: sobe HTTP + scheduler + workers (por env)
├── internal/
│   ├── config/                 # leitura de env (Principio V)
│   ├── http/                   # handlers: webhook, heartbeat, health, admin/sync
│   ├── gob/                    # cliente GOB (members + attendance) — contracts/gob-api.md
│   ├── hikvision/              # cliente ISAPI Digest: user, face, webhook (3 ops, Principio IV)
│   ├── scheduler/              # ticker de carga + trigger manual (FR-021)
│   ├── worker/                 # consumidor RabbitMQ (FR-009..FR-013)
│   ├── queue/                  # setup RabbitMQ: exchange, fila, DLX/DLQ, retry (FR-023)
│   ├── repository/             # acesso PostgreSQL + mapper snake_case↔camelCase
│   ├── domain/                 # entidades: Member, Device, AttendanceEvent, ProcessingMessage
│   └── logging/                # log JSON estruturado (FR-018)
├── migrations/                 # SQL versionado (members, devices, ...)
├── legacy/hik-api/             # REFERENCIA do contrato ISAPI (NAO linkado)
├── docs/
├── go.mod                      # (a criar)
└── docker-compose.yml          # PostgreSQL + RabbitMQ local (a criar)
```

**Structure Decision**: monolito modular Go, binario unico com componentes
ativaveis por env (`RUN_HTTP`/`RUN_SCHEDULER`/`RUN_WORKERS`), workers escalaveis
rodando N replicas (research §0). Compativel com deploy on-premise simples e SC-006.

## Convencoes de Borda

A feature atravessa fronteiras (PostgreSQL ↔ Go ↔ GOB/HikVision ↔ RabbitMQ).
Fontes da verdade de cada convencao:

| Camada | Case style | Validacao | Fonte da verdade |
|--------|------------|-----------|------------------|
| DB columns (PostgreSQL) | snake_case | migration + constraints (UNIQUE, FK) | `migrations/*.sql` |
| Backend domain/DTO (Go) | CamelCase (exportado) / json tag conforme borda | tipos Go + tags | `internal/domain/*.go` |
| Mensagem RabbitMQ (`member.processing`) | camelCase nas chaves JSON | encode/decode no producer/consumer | `internal/queue/*.go` (ver data-model §ProcessingMessage) |
| GOB request/response (JSON) | snake_case (como a GOB envia: `federal_document`, `url_selfie`) | parse contra `contracts/gob-api.md` | `contracts/gob-api.md` (de `t.txt`) |
| HikVision ISAPI (XML/multipart) | nomes ISAPI exatos (`employeeNo`, `FPID`, `HttpHostNotification`) | shape verificado do legacy | `contracts/hikvision-isapi.md` (de `legacy/hik-api`) |
| Webhook recebido (HikVision) | nomes do dispositivo (`employeeNoString`, `AccessControllerEvent`) | extrair tal-qual, normalizar so o CPF | `contracts/inbound-http.md` (de `legacy/hik-api`) |
| Rotas internas (URL path) | kebab-case (`/admin/sync`, `/health`) | router | `internal/http/*.go` |

**Mapper layer (DB ↔ DTO)**:
- Localizacao: `internal/repository/` (mapeia snake_case do Postgres ↔ structs Go).
- ORM auto-mapping: NAO no MVP (preferir `pgx`/`database/sql` explicito ou `sqlc`
  `[PROPOSTA]`); mapeamento explicito reduz drift silencioso.

**Mapper de CPF (fronteira critica — Principio II)**:
- A GOB entrega CPF em digits (`federal_document`, `t.txt:21`); a marcacao GOB
  espera mascara `00.000.000-00` (`t.txt:48`); o HikVision usa o CPF como
  `employeeNo`/`FPID` (digits, devolvido em `employeeNoString`). Um unico helper em
  `internal/domain/` converte digits↔mascara; a correlacao webhook↔membro normaliza
  ambos para digits antes de comparar (research §7). Esta e a unica conversao de
  formato de dado factual e esta isolada — NAO espalhada pelo codigo.

**Validacao de payload**:
- Bordas de entrada (webhook, resposta GOB): validar presenca dos campos
  verificados; campo faltante → log + tratamento defensivo (FR-017, Edge Cases),
  nunca crash.

## Security Considerations (gate owasp-security)

Findings do gate de seguranca sobre o DESENHO (nenhum `critical`; os `high`/`medium`
viram requisitos firmes de implementacao, nao bloqueio — sao resolviveis no design,
todos `[PROPOSTA — a validar na implementacao]`). Constitution faz seguranca MUST.

| # | Sev | OWASP/ASVS | Finding | Mitigacao firme (requisito de implementacao) |
|---|-----|------------|---------|----------------------------------------------|
| S1 | HIGH | A06/A04 | Webhook recebe payload bruto do dispositivo com `httpAuthenticationMethod=none` (verificado do legacy); qualquer host na LAN pode forjar reconhecimento e marcar presenca de qualquer CPF. | Restringir a rota de webhook por **allowlist de IP** dos dispositivos registrados (`devices.ip_address`) E usar um **path-secret** dificil de adivinhar no `HttpHostNotification.path`. So aceitar eventos correlacionaveis a um `device` conhecido. |
| S2 | HIGH | A05 | Payload externo (`employeeNoString`, `raw_payload`) e nao confiavel; risco de injection ao persistir/correlacionar. | **Prepared statements/parametrizado** em todo acesso PostgreSQL (sem concatenacao). **Validar CPF por regex** (11 digitos) antes de usar em `employeeNo`/`{cpf}`. `raw_payload` so como JSONB parametrizado, nunca interpolado. |
| S3 | MEDIUM | A02/A09 | FR-018 loga `cpf` (PII) em todo evento critico — vetor de vazamento. | **Mascarar CPF nos logs** (ex. `***.***.***-NN`); estende Principio V (segredos) a PII. O campo `cpf` do log estruturado carrega forma mascarada; valor cheio so em colunas de DB com acesso controlado. |
| S4 | MEDIUM | API4/A06 | Webhook publico e `/admin/sync` sem rate limiting → DoS / flood de eventos forjados / martelar a GOB. | **Rate limiting** no webhook (por IP de dispositivo) e em `/admin/sync` (serializacao ja prevista; somar limite de frequencia). |
| S5 | LOW | A01 | `/admin/*` precisa de auth (ja `[PROPOSTA]` com token admin). | Tornar **requisito firme**: `/admin/*` exige token de admin via env (Principio V); deny-by-default. NAO reutilizar auth do legacy (Principio IV). |
| S6 | INFO (risco residual) | A04 | ISAPI usa HTTP + Digest na LAN (verificado do legacy); Digest sobre HTTP permite replay. | Aceito para MVP on-premise (rede local confiavel). Registrado como risco residual; HTTPS no dispositivo e melhoria pos-MVP se o firmware suportar. |

Estes requisitos sao incorporados ao backlog (`/create-tasks`) como tarefas de
seguranca explicitas. Nenhum finding bloqueia o plano (todos resolviveis no
desenho/implementacao); decisao auditada registrada (gate owasp-security).

## Complexity Tracking

> Sem violacoes de constitution que exijam justificativa. Nada a registrar.

| Violacao | Por Que Necessario | Alternativa Simples Rejeitada Porque |
|----------|-------------------|--------------------------------------|
| — | — | — |

## Re-check de Constitution (pos-Phase 1)

Revalidado apos design de data-model + contratos + quickstart:

- **Principio I**: contracts/ citam arquivo:linha do legacy / linha de `t.txt`;
  `[PROPOSTA]` aplicado a tudo que nao e verificavel (paginacao GOB, codigos de
  evento, libs Go, identificador de heartbeat). PASS.
- **Principio II**: data-model fixa UNIQUE por CPF e `event_key` para dedup. PASS.
- **Principio III**: queue design com DLX/DLQ + retry pattern. PASS.
- **Principio IV**: design isola os 3 clientes ISAPI; secao "Reuso PROIBIDO"
  enumera o que NAO portar. PASS.
- **Principio V**: nenhum segredo no design; tudo via env; `/admin/*` com token. PASS.
- **Principio VI**: log estruturado por estagio + health + heartbeat. PASS.
- **Principio VII**: todos os identifiers do data-model/contracts em ingles. PASS.

Design NAO introduziu complexidade nao justificada (binario unico, 4 tabelas, 1
fila + DLQ). **Re-check: PASS.**

## NEEDS CLARIFICATION restantes

0 bloqueantes. Itens em aberto sao config de runtime (Principio V) ou
comportamento tolerante ja verificado, todos marcados `[PROPOSTA]` em research §
"Itens GENUINAMENTE pendentes" — nenhum exige bloqueio humano para o desenho.

## Proximos Passos

1. `/checklist` — quality gate dos requisitos antes de implementar.
2. `/create-tasks` — decompor este plano em backlog executavel (fases + criticidade).
3. `/analyze` — validar consistencia spec ↔ plan ↔ tasks (apos tasks).
