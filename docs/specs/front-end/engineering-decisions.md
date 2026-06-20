# Engineering Decisions — Interface de Administração Web

**Feature**: `front-end`
**Referência**: tasks.md §1.1 | spec.md | plan.md | checklists/

Este arquivo consolida os defaults técnicos adotados pela FASE 1
para guiar a execução das fases seguintes (FASE 2/3/4).

---

## Defaults Técnicos Adotados

### Autenticação e Sessão

| Variável | Tipo | Default | Observação |
|----------|------|---------|------------|
| `ADMIN_SESSION_TTL_HOURS` | `optionalInt` | `8` | Recomendação de segurança; sem teto de código — operador controla |
| `ADMIN_USERNAME` | `require()` | — | Obrigatória; sem default |
| `ADMIN_PASSWORD` | `require()` | — | Obrigatória; sem default |
| `ADMIN_SESSION_SECRET` | `require()` | — | Obrigatória; sem default |

**Rotação de secret**: rotacionar `ADMIN_SESSION_SECRET` invalida todas as sessões ativas
imediatamente (HMAC stateless — sem estado em banco). Documentado em `quickstart.md`.

**TLS**: cookie `Secure` exige HTTPS. Pré-requisito de deploy documentado em `quickstart.md`.
Flag `Secure` permanece mesmo em LAN on-premise.

### Rate Limiting

- **Login** (CHK-A13): `NewRateLimitMiddleware(10)` — 10 req/minuto por IP.
  - Validado empiricamente: `NewRateLimitMiddleware(maxPerMinute int)` existe em `middleware.go:113`.
  - Retorna `*RateLimitMiddleware` com método `.Handler(next http.Handler) http.Handler`.
  - 10 req/min mitiga brute force sem prejudicar operação normal (single-operator).

### Paginação

| Recurso | `limit` default | `limit` teto | Decisão |
|---------|----------------|-------------|---------|
| Membros | 50 | 200 | CHK-A16 |
| Eventos | 100 | 500 | CHK-A16 (volume maior, ordenação cronológica) |

- Paginação keyset (cursor) — sem offset; sem nova tabela de histórico (dec-007).
- Busca em membros: `q=` case-insensitive em `name` e `federal_document`; índice composto (1.3.2).

### Tratamento de Erros de DB

- `pgx` retornar erro de conexão → HTTP 503 com body `{"error":"serviço temporariamente indisponível"}` (CHK-A08).
- Detectado por `pgconn.ConnectError` ou erro não-nil de query estrutural.

### Índices de Performance

Criados em migration `000005_add_admin_indexes`:

- `members(name, federal_document)` — busca por nome/CPF (`q=`).
- `attendance_events(created_at DESC, id DESC)` — paginação keyset de eventos.
- Migrations condicionais (`CREATE INDEX IF NOT EXISTS`).

### Timeouts de Request no Frontend

- Chamadas de dados normais (stats, devices, members, events): `AbortController` com **10s** (CHK-P20).
- Sync manual: **60s** (operação potencialmente longa).

### Itens Explicitamente Excluídos (N/A)

- **Gzip** (CHK-P15): N/A para on-premise LAN; latência de LAN torna gzip irrelevante.
- **Métricas por endpoint** (Prometheus): N/A para MVP; logger existente cobre (CHK-A22).
- **Versionamento de API** (`/v1/`): `/admin/api/` serve como namespace (CHK-A04).
- **Teto operacional de TTL em código**: operador controla via env (sem validação de teto).

### Responsividade (dec-030)

Layout fluido obrigatório para:
- Desktop: ≥ 1024px
- Tablet: 768–1023px
- Mobile: ≤ 767px

Sidebar fixa em desktop/tablet; menu hamburguer (drawer) em mobile.

### Acessibilidade Básica (dec-031)

- Navegação por teclado: Tab/Enter/Escape
- Foco visível em todos os elementos interativos (`:focus-visible` com outline)
- Contraste mínimo texto/fundo: ≥ 4.5:1 (texto normal), ≥ 3:1 (texto grande/badges) — dark mode
- Targets de toque ≥ 44×44px em mobile
- `autocomplete` e `<label for=...>` no formulário de login

### Retenção de PII (dec-032)

**FORA DESTA FEATURE.** Política de retenção/purga de `AttendanceEvent` (LGPD art. 15/16)
é deferida para feature futura. Gap documentado em `spec.md §Edge Cases` e
`checklists/security.md CHK-S14`.

---

## Validações Empíricas Realizadas (FASE 1)

| Afirmação | Comando | Resultado |
|-----------|---------|-----------|
| `NewRateLimitMiddleware(10)` aceita `int` | `grep -n "NewRateLimitMiddleware" middleware.go` | linha 113: `func NewRateLimitMiddleware(maxPerMinute int) *RateLimitMiddleware` ✓ |
| Índice em `members(name, federal_document)` ausente | `grep "idx_members" 000001_create_members.up.sql` | Só `idx_members_gob_id` — ausente ✓ |
| Índice em `attendance_events(created_at, id)` ausente | `grep "idx_attendance_events" 000004_create_attendance_events.up.sql` | `federal_document` e `member_id` apenas — composto keyset ausente ✓ |
