# API Checklist: Interface de Administração Web

**Purpose**: Validar a qualidade, clareza e completude dos requisitos dos contratos
de API (endpoints, error handling, autenticação, paginação, rate limiting, observabilidade)
da feature `front-end`. Gate formal pré-`execute-task`.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md) | [contracts/admin-api.md](../contracts/admin-api.md)

---

## Contratos e Schemas

- [x] CHK-A01 — Os formatos de request/response estão definidos para todos os
  endpoints novos da feature? [Completude, contracts/admin-api.md §Sumário] {auto}
  > Evidência: contracts/admin-api.md cobre todos os 9 endpoints novos ([NOVO]) com
  > request bodies, response shapes, tipos de campo e exemplos JSON. Rastreabilidade
  > por endpoint completa.

- [x] CHK-A02 — A convenção de case dos campos JSON está definida e é consistente
  com o projeto? [Consistência, plan.md §Convenções de Borda, contracts §Nota] {auto}
  > Evidência: plan.md §Convenções de Borda: "TODOS os campos JSON em snake_case —
  > verificado nos domain structs `internal/domain/*.go`". contracts/admin-api.md §Nota
  > de abertura confirma snake_case ponta-a-ponta. Consistência verificada entre as
  > 4 camadas (DB → structs → DTOs → payload).

- [x] CHK-A03 — Os endpoints novos são claramente marcados como [NOVO] e
  distinguidos dos existentes [EXISTENTE]? [Clareza, contracts/admin-api.md §Sumário] {auto}
  > Evidência: contracts/admin-api.md usa explicitamente `[NOVO — PROPOSTA]` e
  > `[EXISTENTE]` com file:line para os existentes. Sem ambiguidade sobre o que
  > precisa ser construído vs. o que já existe.

- [ ] CHK-A04 — Existe estratégia de versionamento de API documentada (mesmo que
  "sem versão para MVP")? [Clareza, Gap] {humano}
  > [Gap] A spec e o plan não mencionam versionamento de API (ex: prefixo `/v1/` ou
  > ausência deliberada com justificativa). Para MVP on-premise single-binary a omissão
  > pode ser intencional — mas deve ser decisão explícita para evitar dívida futura.

- [x] CHK-A05 — Os campos derivados (calculados) nas respostas são distinguidos dos
  campos de banco? [Clareza, contracts §devices, §members, §events] {auto}
  > Evidência: contracts marca `status` em devices como "derivado (limiar)",
  > `sync_status` em members como "derivado de ProcessingOutcome", `marking_status`
  > em events como "derivado de `marked`/`marked_at`/`attendance_status`". Distinção
  > explícita no contrato.

---

## Error Handling

- [x] CHK-A06 — Os formatos de resposta de erro estão especificados para todos os
  cenários de falha relevantes de cada endpoint? [Completude, contracts/admin-api.md] {auto}
  > Evidência: cada endpoint no contracts tem tabela "Error Responses" com Status,
  > Body e Quando. Login: 401, 400. Stats: 401. Devices: 404, 401. Members: (implícito
  > 401 via SessionMiddleware). Events: (implícito 401). Sync: 409, 401. Cobertura
  > satisfatória para MVP.

- [x] CHK-A07 — Os códigos HTTP seguem convenção consistente entre endpoints
  equivalentes? [Consistência, contracts/admin-api.md] {auto}
  > Evidência: sessão inválida/expirada = 401 em todos os endpoints protegidos.
  > Sync em andamento = 409 (consistente entre `/admin/sync` existente e
  > `/admin/api/sync` novo). Login bem-sucedido = 204 (sem body). Consistência
  > verificada.

- [ ] CHK-A08 — O comportamento quando o banco de dados está inacessível (erro de
  query) está definido como requisito? [Cobertura, Edge Case, Gap] {humano}
  > [Gap] A spec e o plan não definem o que os endpoints retornam quando há falha
  > de conexão com o PostgreSQL (ex: 503 com mensagem genérica vs. 500 sem detalhes).
  > É um edge case operacional relevante para um painel de monitoramento.

- [x] CHK-A09 — As mensagens de erro estão especificadas como PT-BR (mensagens de
  UI) versus inglês interno? [Clareza, plan.md §Constraints, Constitution VII] {auto}
  > Evidência: plan.md §Constraints: "sintaxe em inglês (Constitution §VII)".
  > contracts/admin-api.md: erros voltados à UI estão em PT-BR (`"credenciais
  > inválidas"`, `"sessão expirada"`, `"dispositivo não encontrado"`). Convenção
  > aplicada e verificada.

---

## Autenticação e Autorização de API

- [x] CHK-A10 — O mecanismo de autenticação (cookie vs. Bearer) está diferenciado
  entre endpoints de UI e endpoints de CLI? [Clareza, contracts §OQ-1 resolvida] {auto}
  > Evidência: contracts §OQ-1: "`/admin/sync` Bearer permanece intacto para CLI.
  > `/admin/api/sync` novo usa cookie — evita relaxar a auth do endpoint existente."
  > Separação de responsabilidades documentada.

- [x] CHK-A11 — O TTL da sessão e o comportamento de refresh estão definidos com
  precisão suficiente para a implementação? [Clareza, contracts §login, plan.md §S1] {auto}
  > Evidência: cookie inclui `Max-Age=<TTL_segundos>` derivado de
  > `ADMIN_SESSION_TTL_HOURS`. Não há refresh token — sessão expira e exige novo
  > login. plan.md §S1 documenta TTL recomendado ≤ 8h. Sem ambiguidade funcional.

---

## Rate Limiting

- [x] CHK-A12 — O requisito de rate limiting no endpoint de login está especificado
  com referência ao mecanismo concreto? [Clareza, plan.md §Quality Gate S4] {auto}
  > Evidência: plan.md §S4: "Reusar `RateLimitMiddleware` [EXISTENTE]
  > (middleware.go:105-140, token bucket per-IP)" no login. Mecanismo concreto
  > identificado com file:line.

- [ ] CHK-A13 — Os thresholds do rate limit (requisições/janela) estão quantificados
  para o endpoint de login? [Clareza, Gap] {humano}
  > [Gap] O plan especifica QUAL middleware reusar mas não define os parâmetros
  > (ex: 5 tentativas/minuto por IP). O `RateLimitMiddleware` existente tem
  > configuração própria — confirmar se os valores atuais são adequados para
  > o login ou se precisam ser ajustados.

- [x] CHK-A14 — O comportamento quando o rate limit é atingido está implicitamente
  coberto pelo middleware existente? [Completude] {auto}
  > Evidência: o `RateLimitMiddleware` existente (middleware.go:105-140) já
  > implementa o comportamento de resposta (429 ou bloqueio). Ao reusar, o
  > comportamento é herdado — não requer especificação nova.

---

## Paginação e Filtros

- [x] CHK-A15 — A estratégia de paginação (cursor vs. offset) está definida e é
  consistente entre os endpoints que precisam dela? [Clareza, Spec §FR-008,
  contracts §members, §events] {auto}
  > Evidência: FR-008 e dec-008 especificam cursor-based paging server-side.
  > contracts §members: `cursor` sobre `id`. contracts §events: `cursor` keyset
  > sobre `(created_at, id)`. Consistência de estratégia; diferença no cursor
  > composto de events é justificada pela ordenação cronológica decrescente.

- [ ] CHK-A16 — O tamanho padrão e o teto da página (limit) estão quantificados
  para membros e eventos? [Clareza, Gap] {humano}
  > [Gap] contracts §members e §events definem o param `limit` como "tamanho de
  > página (default e teto definidos na implementação)". O requisito não especifica
  > os valores — a implementação decide arbitrariamente. Recomenda-se definir
  > (ex: default=50, teto=200 para membros; default=100 para eventos).

- [x] CHK-A17 — O endpoint de dispositivos está explicitamente dispensado de
  paginação com justificativa? [Clareza, contracts §devices] {auto}
  > Evidência: contracts §devices: "Sem paginação obrigatória (volume = dezenas de
  > dispositivos, briefing)." Decisão documentada com justificativa de escala.

- [x] CHK-A18 — A semântica de `next_cursor=null` + `has_more=false` (última página)
  está especificada? [Clareza, contracts §members, §events] {auto}
  > Evidência: contracts §members e §events ambos documentam: "`next_cursor=null` +
  > `has_more=false` = última página." Sem ambiguidade para o frontend consumir.

---

## Idempotência

- [x] CHK-A19 — O endpoint de sync manual tem comportamento de idempotência
  definido? [Completude, Spec §FR-007, contracts §sync, §Edge Cases spec] {auto}
  > Evidência: contracts §sync: "409 `sync já em andamento` — serializer bloqueia
  > (mesma semântica do existente)". US6-AC4: "botão desabilitado ou aviso quando
  > sincronização em andamento." Comportamento definido.

---

## Estado Vazio (Empty State) via API

- [x] CHK-A20 — O contrato de resposta para estado vazio (zero registros) está
  especificado para todos os endpoints de listagem? [Completude, Spec §FR-009,
  contracts] {auto}
  > Evidência: contracts §devices: `"devices: []"` → "UI mostra mensagem amigável
  > (FR-009)". contracts §members: "`members: []`" + "`has_more: false`".
  > contracts §events: "`events: []`". FR-009 exige mensagem informativa — contrato
  > define a resposta API; requisito da UI está em FR-009.

---

## Observabilidade

- [x] CHK-A21 — O requisito de logging dos handlers novos referencia o logger
  existente do projeto? [Completude, plan.md §Constitution VI] {auto}
  > Evidência: plan.md §Constitution VI: "Handlers novos usam o `logging.Logger`
  > existente; `/health` permanece." Integração ao sistema de logging existente.

- [ ] CHK-A22 — Métricas por endpoint (latência, taxa de erro) são requeridas para
  os endpoints novos do painel? [Cobertura, Gap] {humano}
  > [Gap] A spec e o plan não definem requisitos de métricas instrumentadas por
  > endpoint (ex: Prometheus, counters de req/err). O briefing menciona observabilidade
  > como princípio (Constitution VI) mas não especifica granularidade de métricas
  > para o painel admin. Decisão de produto: necessário para MVP?

---

## Deprecação

- [x] CHK-A23 — A ausência de política de deprecação está justificada pelo contexto
  (MVP on-premise single-binary)? [Completude] {auto}
  > Evidência: o plano não menciona deprecação de API — contexto on-premise
  > single-binary sem clientes externos, sem SLA de compatibilidade. N/A para MVP.
  > Aceitável e consistente com o escopo declarado (spec §Visão Geral).

---

**Resumo API**:
- Total: 23 itens
- [x] auto resolvidos: 17
- [ ] humano aguardando decisão: 5 (CHK-A04, CHK-A08, CHK-A13, CHK-A16, CHK-A22)
- [Gap]: 5 (versionamento API, erro de DB, rate limit thresholds, page size, métricas)
