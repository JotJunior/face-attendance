# Implementation Plan: Interface de Administração Web

**Feature**: `front-end` | **Date**: 2026-06-20 | **Spec**: [spec.md](./spec.md)

## Summary

Painel web de administração (autenticado, dark mode único) para monitorar membros
GOB, saúde dos dispositivos HikVision, logs de presença e acionar sync manual —
servido **same-origin** pelo binário Go existente (FR-013). A abordagem técnica:
frontend **estático vanilla** (HTML/CSS/JS) embutido via `embed.FS`; backend Go
ganha **3 grupos de adições** decididos no clarify — (1) env var
`DEVICE_OFFLINE_THRESHOLD_HOURS` exposta na API; (2) endpoint agregado
`GET /admin/api/stats`; (3) autenticação de UI por **cookie httpOnly assinado com
HMAC** (stdlib, sem dep nova) + endpoints de listagem/detalhe que a UI consome.
**Reusa** integralmente as 4 tabelas existentes (sem novas — dec-007) e o
endpoint de sync existente (`/admin/sync`). CPF mascarado **no backend** (SC-006).

> **Aterramento (Constitution Princípio I)**: todo contrato neste plano referencia
> código real (`internal/http/server.go`, `internal/config/config.go`,
> `internal/domain/*.go`, `migrations/*`, `go.mod`) verificado file:line, OU está
> marcado **[NOVO — PROPOSTA]**. Nenhum nome de rota/campo foi inventado. Detalhes
> em [research.md](./research.md), [data-model.md](./data-model.md) e
> [contracts/admin-api.md](./contracts/admin-api.md).

## Technical Context

**Language/Version**: Go 1.26.4 (backend, `go.mod`); HTML/CSS/JS vanilla (ES
modules) no frontend — **sem toolchain Node**.
**Primary Dependencies**: backend usa stdlib `net/http`, `embed`, `crypto/hmac`,
`crypto/sha256`, `crypto/subtle`, `encoding/json` (todas **stdlib** — nenhuma dep
nova no `go.mod`); deps existentes: `pgx/v5`, `amqp091-go`, `icholy/digest`.
Frontend: nenhuma lib externa (zero CDN).
**Storage**: PostgreSQL (reuso das tabelas `members`, `devices`,
`member_processing_status`, `attendance_events` — **sem novas tabelas**, dec-007).
**Testing**: `go test` (padrão do projeto — há `*_test.go` em `internal/`); +
roundtrip empírico via `curl` (quickstart Scenario 3).
**Target Platform**: on-premise / rede local (binário único; sem cloud/CDN).
**Project Type**: web-service (Go) + UI estática embutida same-origin.
**Performance Goals**: dashboard < 5s (SC-001); feedback de sync < 2s (SC-004);
1.000+ eventos sem degradação (SC-005) via paginação cursor server-side.
**Constraints**: CPF nunca trafega cru (SC-006); 100% das rotas de painel exigem
sessão (SC-003); segredos via env (Constitution §V); sintaxe em inglês
(Constitution §VII).
**Scale/Scope**: centenas a poucos milhares de membros; poucas a dezenas de
dispositivos (briefing §6). 4 telas + login.

## Constitution Check

*GATE: passou antes do Phase 0; re-checado após Phase 1 (§Re-check no fim).*

| Princípio | Status | Notas |
|-----------|--------|-------|
| I. Veracidade de Dados — Zero Fabricação (NON-NEGOTIABLE) | PASS | Contratos aterrados em código real (file:line) ou marcados `[NOVO — PROPOSTA]`. Shapes derivam de `internal/domain/*.go`. Sem nomes inventados. |
| II. Idempotência Chaveada por CPF (NON-NEGOTIABLE) | N/A (PASS) | Feature é read-only sobre os dados (listagens/contadores) + sync manual que reusa o caminho idempotente existente (`AdminSyncHandler`). Não escreve em recurso externo. |
| III. Resiliência por Filas (retry + DLQ) | N/A (PASS) | UI não introduz processamento assíncrono novo. O sync manual delega ao `Scheduler`/fila existentes. |
| IV. Reuso Restrito do Legacy hik-api | PASS | Feature não toca `legacy/hik-api`; só consome dados já persistidos no DB local. |
| V. Segredos como Configuração de Runtime | PASS | `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `ADMIN_SESSION_SECRET`, `ADMIN_SESSION_TTL_HOURS`, `DEVICE_OFFLINE_THRESHOLD_HOURS` via env (padrão `require()`/`optionalInt()` de `config.go`); nunca hardcoded; logs não vazam segredo. |
| VI. Observabilidade Operacional | PASS | Handlers novos usam o `logging.Logger` existente; `/health` permanece. A UI complementa observabilidade (dashboard) sem removê-la. |
| VII. Idioma da Sintaxe | PASS | Rotas, campos JSON (snake_case), funções, arquivos em inglês; mensagens de UI/erro em PT-BR. |

**Resultado**: PASS em todos os princípios MUST. Sem violação bloqueante.

## Project Structure

### Documentation (this feature)

```
docs/specs/front-end/
├── spec.md            # existente (specify + clarify)
├── plan.md            # este arquivo (Phase 1)
├── research.md        # Phase 0 — stack, sessão, serving, máscara CPF
├── data-model.md      # Phase 1 — entidades reusadas + Session stateless
├── quickstart.md      # Phase 1 — 9 cenários (incl. roundtrip)
└── contracts/
    └── admin-api.md   # Phase 1 — endpoints EXISTENTE vs NOVO
```

### Source Code (repository root — árvore real verificada)

```
face-attendance/
├── go.mod                              # Go 1.26.4; pgx/v5, amqp091-go, icholy/digest
├── cmd/presenca-facial/
│   └── main.go                         # [EDIT] wiring: novos handlers + embed.FS + config
├── internal/
│   ├── config/
│   │   └── config.go                   # [EDIT] add env vars novas (padrão require/optionalInt)
│   ├── http/
│   │   ├── server.go                   # [EDIT] registrar rotas /admin/api/* + FileServer
│   │   ├── handlers.go                 # [EXISTENTE] AdminSyncHandler, HealthHandler
│   │   ├── middleware.go               # [EXISTENTE] AdminAuthMiddleware (Bearer)
│   │   ├── admin_ui_handlers.go        # [NOVO] login/logout/stats/devices/members/events/sync
│   │   ├── session.go                  # [NOVO] HMAC sign/verify + SessionMiddleware (cookie)
│   │   └── *_test.go                   # [NOVO] testes dos handlers e da sessão
│   ├── domain/                         # [EXISTENTE] device.go, member.go, attendance_event.go,
│   │   │                               #   processing_outcome.go (structs snake_case)
│   │   └── view.go                     # [NOVO] DTOs de view (CPF mascarado) + maskCPF()
│   ├── repository/
│   │   ├── member_repository.go        # [EDIT] CountMembersWithSelfie + ListMembersPaged(q,cursor)
│   │   ├── device_repository.go        # [EDIT] CountDevicesByActivity(threshold) + ListAll
│   │   ├── attendance_event_repository.go # [EDIT] CountAttendanceSince + ListEventsPaged(from,to,cursor)
│   │   └── processing_outcome_repository.go # [EXISTENTE] (reuso p/ sync_status)
│   ├── gob/  hikvision/  scheduler/  worker/  queue/  logging/   # [EXISTENTE] intocados
│   └── web/                            # [NOVO] assets embutidos do frontend
│       ├── embed.go                    # //go:embed dist/* — expõe a embed.FS
│       └── dist/
│           ├── index.html              # [NOVO] shell SPA-lite (4 telas + login)
│           ├── assets/app.css          # [NOVO] design system dark (via skill frontend-design)
│           └── assets/app.js           # [NOVO] fetch + render + 401-redirect + máscara já vem do backend
└── migrations/                         # [EXISTENTE] 000001-000004 — SEM novas migrations (dec-007)
```

**Structure Decision**: backend novo confinado a `internal/http` (handlers +
sessão) e métodos adicionais nos repositórios existentes; assets do frontend em
`internal/web` embutidos via `embed.FS` e servidos pelo `ServeMux` atual. **Sem
novas tabelas, sem nova dependência no `go.mod`, sem novo serviço/processo** —
preserva o modelo single-binary on-premise.

## Convenções de Borda

Feature atravessa DB ↔ backend ↔ frontend. Fonte da verdade de cada convenção
(adaptado ao **código real** deste projeto — que usa **snake_case ponta-a-ponta**,
não camelCase):

| Camada | Case style | Validação | Fonte da verdade |
|--------|------------|-----------|------------------|
| DB columns (PostgreSQL) | snake_case | migration | `migrations/000001-000004/*.sql` |
| Backend domain structs (Go) | snake_case (json tags) | `json:"..."` tags | `internal/domain/*.go` (EXISTENTE) |
| Backend DTOs de view (Go) | snake_case (json tags) | `json:"..."` tags | `internal/domain/view.go` (NOVO — mesma convenção) |
| API payload (request/response) | snake_case | shape em `contracts/admin-api.md` | `contracts/admin-api.md` |
| Frontend (JS) consumo | snake_case (lê direto o JSON) | sem transformação de case | `internal/web/dist/assets/app.js` |
| URL query/path params | snake_case/lowercase (`q`, `cursor`, `from`, `to`, `{id}`) | router (`ServeMux`) | `internal/http/server.go` |

**Mapper layer (DB ↔ DTO)**:
- Backend: mapeamento explícito DB-row → struct nos repositórios
  (`internal/repository/*.go`); DTOs de view derivam dos domain structs em
  `internal/domain/view.go`.
- ORM auto-mapping: **NÃO** — o projeto usa `pgx/v5` com SQL explícito e scan
  manual (verificado: `member_repository.go`, `device_repository.go`). Sem gorm/sqlc.

**Validação de schema**:
- Em qual borda? Resposta serializada por `encoding/json` no backend (json tags);
  frontend lê os campos snake_case diretamente.
- Schema compartilhado? **NÃO há** `packages/shared-types` (projeto Go puro +
  JS vanilla). A convenção única (snake_case) elimina o mapper camelCase e o
  risco de drift — **fonte única** é `internal/domain` (validado no quickstart
  Scenario 3 — roundtrip empírico).

> **Nota sobre o template de Convenções de Borda**: o template sugere camelCase
> para DTOs e Zod no frontend. Este projeto **não** usa TS/Zod nem camelCase — a
> fonte da verdade real é **snake_case em Go** (domain structs verificados). A
> tabela acima reflete o código real, não o default do template (Constitution I).

## Quality Gate — Segurança (OWASP Top 10:2025 / ASVS 5.0)

Auditoria de arquitetura (pré-código) da `owasp-security` sobre auth/sessão.
Sem finding `critical`. Recomendações de design incorporadas abaixo (não bloqueiam
— não há código ainda; viram requisitos para `execute-task`).

| # | Severidade | Finding | Decisão de design (incorporada) |
|---|-----------|---------|-------------------------------|
| S4 | **HIGH** | Sem rate-limit/lockout no `POST /admin/api/login` → brute force (A07/API4/CWE-307) | **Reusar `RateLimitMiddleware` [EXISTENTE]** (middleware.go:105-140, token bucket per-IP, já usado no `/webhook`) no endpoint de login. Requisito obrigatório de `execute-task`. |
| S1 | medium | Token HMAC stateless sem `jti`/store → sem revogação individual; logout só limpa cookie (A07/ASVS V16) | TTL curto (default recomendado ≤ 8h via `ADMIN_SESSION_TTL_HOURS`); logout é best-effort (cookie clear); revogação total = rotacionar `ADMIN_SESSION_SECRET`. Documentado em research Decision 3. |
| S6 | medium→ok | PII (CPF): vetor de vazamento via `raw_payload` JSONB / logs | Mitigado: DTOs de view omitem `raw_payload`/`event_key`; CPF mascarado no backend. **Requisito**: handlers da UI NÃO logam CPF completo nem ecoam `raw_payload` em erros (alinha Constitution VI). |
| S8 | low | Flag `Secure` + deploy HTTP plano = cookie nunca armazenado (footgun operacional) | **Documentar expectativa de TLS** para o painel; em LAN sem TLS, operador deve prover TLS (proxy/cert local). Nota em quickstart/research. |
| S3 | low | `ADMIN_PASSWORD` plaintext em env, sem hashing | Aceitável: padrão do projeto (Constitution §V, segredos via env), single-operator on-premise. Comparação **constant-time** (`crypto/subtle.ConstantTimeCompare`) obrigatória. |
| S5 | low | CSRF nos POSTs (login/logout/sync) | Mitigado por `SameSite=Strict` + same-origin + `Secure`. Sem token CSRF dedicado para o MVP (defense-in-depth opcional via header custom). |
| S2/S7 | low→ok | Session fixation; path traversal no static | Não aplicável: token criado fresh no login (sem id pré-existente p/ fixar); `embed.FS` + `http.FileServer` não traversa p/ FS do host. Seguro por construção. |

> Sem MFA/passkeys: aceitável para MVP on-premise single-operator (briefing). Anotado
> como evolução futura, não requisito do MVP.

## Dependências de implementação (execute-task)

- **FR-010 (qualidade visual)**: o design system dark (tokens CSS, tipografia,
  hierarquia, componentes das 4 telas + login) **deve** ser produzido com a skill
  `frontend-design` na fase `execute-task`. O plano fixa a arquitetura (vanilla +
  embed); a skill produz o CSS/markup de alta qualidade.
- **FR-002 (dark mode único)**: um único conjunto de design tokens CSS; sem
  toggle, sem theming runtime.

## Decisões herdadas honradas (clarify)

dec-006 (cookie httpOnly + creds via env), dec-007 (detalhe de device sem
histórico — sem nova tabela), dec-008 (busca/paginação server-side cursor),
dec-012/015 (`DEVICE_OFFLINE_THRESHOLD_HOURS` via env, exposto na API),
dec-013/016 (`GET /admin/stats` agregado). Todas refletidas em research/
data-model/contracts.

## Re-check de Constitution (pós-Phase 1)

Design **não** introduziu: novo serviço, novo processo, nova dependência externa,
nova tabela, nem mecanismo que viole princípio MUST. A sessão HMAC é stdlib
(reforça §V e enxuga reuso). CPF mascarado no backend reforça §I. **PASS mantido**
em todos os princípios. Complexity Tracking vazio (sem violação a justificar).

## Complexity Tracking

> Sem violações de constitution — nenhuma justificativa necessária.

| Violação | Por Que Necessário | Alternativa Simples Rejeitada Porque |
|----------|-------------------|--------------------------------------|
| (nenhuma) | — | — |
