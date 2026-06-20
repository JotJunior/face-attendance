# Research: Interface de Administração Web (`front-end`)

Documento do Phase 0 do `/plan`. Resolve as decisões técnicas (stack do
frontend, mecanismo de sessão, serving, mascaramento de CPF) antes do design.

> **Aterramento (Constitution Princípio I — Zero Fabricação)**: todas as
> referências a rotas, structs, colunas e helpers do backend foram verificadas
> contra o código Go real em `internal/`, `migrations/` e `go.mod`. Itens
> marcados **[NOVO]** ainda não existem e são criados por esta feature; itens
> marcados **[EXISTENTE]** foram confirmados por leitura de fonte (file:line).

---

## Decision 1: Stack do frontend

**Decision**: Frontend estático em **HTML + CSS + JavaScript vanilla (ES
modules)**, sem framework SPA e sem build step obrigatório, embutido no binário
Go via `embed.FS` e servido same-origin sob `/admin/*`.

**Rationale**:
- **Single-binary on-premise** (Constitution §Stack — deploy local, sem cloud):
  `go.mod` declara apenas `pgx/v5`, `amqp091-go`, `icholy/digest` — nenhum
  toolchain JS. Adicionar React/Vite traria um pipeline de build Node + node_modules
  ao deploy on-premise, contrariando o princípio de operação sem dependência de
  cloud/CDN e o modelo de um único artefato (o binário Go).
- **Escopo modesto**: 4 telas (dashboard, dispositivos, membros, logs) + login.
  Não há estado client-side complexo que justifique um framework reativo. Fetch +
  templates de DOM cobrem o caso.
- **Zero dependência de CDN**: todo CSS/JS é servido pelo próprio binário (embed),
  funcionando em rede local isolada (sem internet).
- **Dark mode único** (FR-002): um único arquivo de design tokens CSS (custom
  properties) — sem necessidade de theming runtime nem toggle.
- **Qualidade visual** (FR-010): a skill `frontend-design` será aplicada na fase
  `execute-task` para produzir o design system dark (tipografia, hierarquia,
  componentes). Vanilla não impede qualidade — o design system mora no CSS.

**Alternatives considered**:
- **React + Vite (build estático embutido)**: bom DX, mas adiciona Node ao build
  on-premise e `node_modules` ao repo; overhead desproporcional para 4 telas.
  Rejeitado pelo custo de toolchain vs. ganho marginal.
- **HTMX + templates Go server-side**: viável e idiomático em Go, mas mistura
  renderização no backend e exige `html/template` por tela; o requisito de busca
  server-side com paginação cursor (dec-008) e a interatividade (estado de loading
  do botão de sync, máscaras) já pedem JS no cliente — vanilla JS + JSON API é mais
  direto e mantém o backend como API pura. Mantido como alternativa de baixo custo
  caso a complexidade de JS cresça.
- **Svelte/SolidJS compilados**: menor runtime que React, mas ainda exigem
  toolchain Node. Mesmo veto do React.

**Build step**: opcional. Se nenhum bundler for usado, os arquivos `.css`/`.js`
são embutidos diretamente. Decisão final de minificação fica para `execute-task`;
o plano não exige bundler (mantém deploy single-binary).

---

## Decision 2: Serving same-origin via `embed.FS` (FR-013)

**Decision**: Embutir os assets do frontend no binário com a diretiva
`//go:embed` (stdlib `embed`) e servi-los via `http.FileServer(http.FS(...))`
registrado no `net/http.ServeMux` **[EXISTENTE]** em `internal/http/server.go`,
sob o prefixo `/admin/` (UI) — coexistindo com as rotas de API `/admin/*`.

**Rationale**:
- **[EXISTENTE]** O servidor já usa `net/http.ServeMux` (server.go:39) e registra
  rotas com `mux.Handle(...)`. Adicionar um `FileServer` segue o padrão atual.
- **[NOVO]** Nenhum `embed`/`FileServer` existe hoje (grep em `internal/`/`cmd/`
  retornou vazio) — é infraestrutura nova desta feature.
- Same-origin elimina CORS e simplifica o cookie de sessão (SameSite=Strict
  funciona naturalmente quando UI e API compartilham origem).
- **Roteamento de prefixo**: a UI vive em rotas como `/admin/` (index, login) e
  assets em `/admin/assets/*`; as rotas de API ficam sob um sub-prefixo distinto
  (ver Decision 5) para o `ServeMux` discriminar API vs. arquivo estático sem
  ambiguidade (o `ServeMux` casa o padrão mais longo).

**Alternatives considered**:
- **`http.Dir` (servir de disco)**: exige distribuir uma pasta de assets junto do
  binário — quebra o modelo single-binary. Rejeitado.
- **Reverse proxy (nginx) servindo o estático**: adiciona um componente de infra ao
  deploy on-premise. Rejeitado por contrariar simplicidade operacional.

---

## Decision 3: Mecanismo de sessão — cookie httpOnly assinado com HMAC stdlib

**Decision**: Autenticação da UI por **cookie de sessão httpOnly, Secure,
SameSite=Strict**, contendo um token **assinado com HMAC-SHA256** usando um
segredo de runtime (env var **[NOVO]** `ADMIN_SESSION_SECRET`). O payload do
cookie é `base64url(payload).base64url(hmac)`, onde `payload` carrega o subject
(usuário admin) e um `exp` (epoch de expiração). TTL configurável via env
**[NOVO]** `ADMIN_SESSION_TTL_HOURS`. Assinatura/validação via stdlib
`crypto/hmac` + `crypto/sha256` — **sem dependência externa nova** (mantém
`go.mod` enxuto).

**Rationale**:
- **dec-006 (score 3)**: cookie httpOnly, TTL via env, credenciais via env. Nunca
  LocalStorage (vetor XSS — FR-001).
- **Constitution §V (Segredos como runtime)**: `ADMIN_SESSION_SECRET`,
  `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `ADMIN_SESSION_TTL_HOURS` são env vars,
  nunca hardcoded; carregadas pelo padrão **[EXISTENTE]** `require()`/`optionalInt()`
  de `config.go`.
- **Stateless**: o token assinado dispensa store de sessão em memória/DB — não há
  nova tabela (alinha com dec-007 "sem novas tabelas") e sobrevive a restart do
  processo (importante em deploy on-premise de instância única). A expiração é
  carregada no próprio token (`exp`), validada no middleware.
- **`crypto/hmac.Equal`** garante comparação em tempo constante (anti-timing).
- **[EXISTENTE]** Distinto do `AdminAuthMiddleware` Bearer (middleware.go:52, valida
  `Authorization: Bearer {ADMIN_TOKEN}`) — esse continua para acesso de API
  backend/CLI; a sessão de cookie é **[NOVO]** e exclusiva da UI.

**Detalhe do payload do token** (proposta — a validar na implementação):
`payload = {"sub":"<ADMIN_USERNAME>","exp":<unix_seconds>}` serializado e assinado.
O middleware rejeita se: cookie ausente, assinatura inválida, ou `exp < now`.

**Alternatives considered**:
- **Session store em memória (map + mutex)**: simples, mas perde sessões no
  restart e adiciona estado mutável compartilhado. Rejeitado — token assinado é
  stateless e mais robusto para instância única on-premise. (Anotado como fallback
  caso se queira revogação imediata de sessão no futuro.)
- **JWT via lib externa (golang-jwt)**: traz dependência nova ao `go.mod` para algo
  que `crypto/hmac` da stdlib resolve. Rejeitado por Constitution (enxugar reuso) e
  para não inflar deps. O formato é um JWT-like minimalista próprio, não um JWT
  completo (não precisamos de claims padronizados nem de RS256).
- **Validação de senha via comparação direta de string**: substituída por
  `crypto/subtle.ConstantTimeCompare` para evitar timing leak na verificação de
  `ADMIN_PASSWORD`.

> **[PROPOSTA — a validar na implementação]** O formato exato do token e o
> conjunto de flags do cookie serão fixados em `execute-task`; a estratégia
> (HMAC stdlib, httpOnly+Secure+SameSite, TTL via env) é a decisão de arquitetura
> desta fase.

**Notas de segurança (gate owasp-security)**:
- **Revogação/rotação (S1)**: o token é stateless (sem `jti`/store) — não há
  revogação individual antes do `exp`. Logout limpa o cookie (best-effort);
  revogação total = rotacionar `ADMIN_SESSION_SECRET` (invalida todas as sessões).
  Mitigar com TTL curto (default recomendado ≤ 8h via `ADMIN_SESSION_TTL_HOURS`).
- **Rate-limit no login (S4, HIGH)**: `POST /admin/api/login` **deve** reusar o
  `RateLimitMiddleware` **[EXISTENTE]** (middleware.go:105-140) contra brute force.
- **TLS / flag Secure (S8)**: o cookie usa `Secure`, que exige HTTPS. Em deploy
  on-premise sem TLS, o cookie não é armazenado — o painel **espera TLS** (proxy/
  cert local). Documentar na operação.
- **Senha (S3)**: comparação **constant-time** (`crypto/subtle.ConstantTimeCompare`).

---

## Decision 4: Mascaramento de CPF no BACKEND (FR-011, SC-006)

**Decision**: O CPF (`federal_document` **[EXISTENTE]**, `VARCHAR(14)`) é
**mascarado no backend**, na serialização das respostas dos endpoints da UI. O
JSON que trafega ao browser nunca contém o CPF completo — apenas a forma
mascarada (ex: `***.NNN.NNN-**`, expondo só os dígitos do meio).

**Rationale**:
- **Redução de superfície**: mascarar no frontend implicaria trafegar o CPF
  completo até o browser, onde poderia vazar via devtools, logs de proxy, ou
  extensões. Mascarar no backend garante SC-006 ("CPF não aparece completo em
  nenhuma tela") na fonte.
- **Constitution §I e regra global**: dado sensível tratado conservadoramente.
- A busca por CPF (FR-008, dec-008) continua server-side: o cliente envia o termo
  de busca via query param `q`, e o backend normaliza/compara contra
  `federal_document` **[EXISTENTE]** — o cliente nunca precisa do CPF completo de
  volta para filtrar.

**Implementação (proposta)**: os DTOs de resposta da UI **[NOVO]** (ex:
`MemberView`, `AttendanceEventView`) expõem um campo `federal_document_masked`
(string já mascarada) em vez de `federal_document` cru. A máscara é aplicada por
uma função utilitária **[NOVO]** no backend (ex: `maskCPF(s string) string`).

**Alternatives considered**:
- **Mascarar no frontend**: rejeitado (trafega dado sensível ao cliente; viola o
  espírito de SC-006).
- **Reusar os domain structs `Member`/`AttendanceEvent` direto na resposta**:
  rejeitado — eles expõem `federal_document` cru (member.go, attendance_event.go).
  A UI usa DTOs de view dedicados que omitem/mascararam o campo sensível.

---

## Decision 5: Namespace de rotas da API da UI vs. estáticos

**Decision**: As rotas de **API consumidas pela UI** ficam sob um sub-prefixo
distinto do estático para o `ServeMux` discriminar sem ambiguidade. Proposta:
estáticos em `/admin/` e `/admin/assets/*`; API em `/admin/api/*` (ex:
`/admin/api/stats`, `/admin/api/devices`, `/admin/api/members`,
`/admin/api/events`, `/admin/api/login`, `/admin/api/logout`).

**Rationale**:
- **[EXISTENTE]** `net/http.ServeMux` casa o padrão registrado mais longo/específico.
  Separar API (`/admin/api/...`) do estático (`/admin/...`) evita que o `FileServer`
  capture chamadas de API e vice-versa.
- O endpoint de sync manual **[EXISTENTE]** `/admin/sync` (server.go:54-55,
  `AdminAuthMiddleware` + `AdminSyncHandler`, responde 202/409) permanece onde está
  — o botão da UI (FR-007) o aciona. **Decisão a confirmar em execute-task**: se o
  botão da UI chama `/admin/sync` (Bearer) ou um wrapper `/admin/api/sync` protegido
  por cookie. Ver Open Question OQ-1.

**Open Question OQ-1 (resolvida no plan, ver §Decisões pendentes)**: o
`/admin/sync` **[EXISTENTE]** é protegido por **Bearer `ADMIN_TOKEN`**, não por
cookie de sessão. A UI autentica por cookie. Opções: (a) a UI chama um endpoint
novo `/admin/api/sync` **[NOVO]** protegido por cookie, que internamente invoca a
mesma lógica de `AdminSyncHandler`; (b) relaxar o `/admin/sync` para aceitar cookie
OU bearer. **Recomendação**: opção (a) — endpoint novo sob cookie que reusa o
`Scheduler`/`SyncSerializer` **[EXISTENTE]**, preservando o `/admin/sync` Bearer
intacto para CLI. Decidido no plan §Backend novo.

**Alternatives considered**:
- **Tudo sob `/admin/*` sem sub-prefixo**: ambiguidade entre arquivo e endpoint;
  rejeitado.
- **API sob domínio/porta separada**: quebra same-origin (FR-013) e reintroduz
  CORS; rejeitado.

---

## Decision 6: Paginação cursor e filtros server-side (dec-008)

**Decision**: Listagens de **membros** e **logs** usam **paginação server-side**.
Membros: busca por `q` (nome ou CPF) + cursor. Logs: filtro por intervalo de datas
(`from`/`to`) + cursor, ordem cronológica decrescente. Os repositórios ganham
métodos **[NOVO]** de listagem paginada; os existentes de contagem/listagem total
(`ListWithSelfie`, `ListActive` **[EXISTENTE]**) não cobrem paginação.

**Rationale**:
- **dec-008 (score 3)** + briefing ("centenas a poucos milhares de membros") +
  edge case (milhares de entradas não podem travar o browser) + SC-005 (1.000+
  eventos sem degradação).
- **Cursor** (keyset) sobre `id` (BIGSERIAL **[EXISTENTE]**, monotônico) é estável
  sob inserções concorrentes e eficiente em PostgreSQL com índice de PK. Para logs,
  o cursor é sobre `(created_at, id)` para ordenação cronológica determinística.

**Alternatives considered**:
- **OFFSET/LIMIT**: simples mas degrada em offsets altos e é instável sob
  inserção. Keyset é preferível para o volume previsto. (OFFSET aceitável como
  fallback de implementação se keyset complicar — anotado.)
- **Busca client-side**: explicitamente fora de escopo para o dataset completo
  (dec-008); client-side só para filtro rápido dentro da página já carregada.
