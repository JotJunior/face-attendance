# Performance Checklist: Interface de Administração Web

**Purpose**: Validar a qualidade, clareza e completude dos requisitos de performance
(targets, escalabilidade, queries, payload, frontend) da feature `front-end`.
Gate formal pré-`execute-task`.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md) | [contracts/admin-api.md](../contracts/admin-api.md)

---

## Targets Mensuráveis

- [x] CHK-P01 — O target de tempo de carregamento do dashboard está quantificado
  com condição de medição? [Clareza, Spec §SC-001] {auto}
  > Evidência: SC-001: "acesso ao dashboard e visualização de métricas em menos de
  > 5 segundos após login, em conexão de rede local." Métrica (5s), condição
  > (rede local), evento de início (pós-login) definidos.

- [x] CHK-P02 — O target de responsividade do feedback de sync está quantificado?
  [Clareza, Spec §SC-004] {auto}
  > Evidência: SC-004: "feedback visual ao operador em menos de 2 segundos após
  > o clique, independentemente de quanto tempo o processamento backend leva."
  > Métrica (2s), independência do backend, evento de início (clique) definidos.

- [x] CHK-P03 — O target de performance para a tela de logs (scroll/filtragem)
  está especificado com volume de referência? [Clareza, Spec §SC-005] {auto}
  > Evidência: SC-005: "suporta exibição de pelo menos 1.000 eventos sem
  > degradação perceptível (scroll/filtragem fluidos)". Volume (1.000), critério
  > qualitativo (sem degradação perceptível) definidos.

- [ ] CHK-P04 — O target de performance para as telas de membros e dispositivos
  está quantificado (além do dashboard)? [Cobertura, Gap] {humano}
  > [Gap] SC-001 a SC-005 cobrem dashboard e logs; mas não há target de latência
  > para `/admin/api/members` (potencialmente milhares de registros) nem para
  > `/admin/api/devices`. Recomenda-se definir (ex: listagem de membros < 2s
  > para primeira página).

- [ ] CHK-P05 — Os targets de performance especificam percentil (p95, p99) ou
  apenas tempo médio? [Clareza, Ambiguity] {humano}
  > SC-001/004/005 definem limites sem especificar se são p50 (mediana), p95 ou
  > p99. Para SLA operacional, p95 < 5s é muito diferente de média < 5s.
  > Decisão: qual percentil é o target?

---

## Escalabilidade

- [x] CHK-P06 — O volume máximo de dados está especificado (membros, dispositivos,
  eventos) para dimensionar queries e paginação? [Clareza, plan.md §Technical Context,
  Spec §Edge Cases] {auto}
  > Evidência: plan.md §Technical Context: "centenas a poucos milhares de membros;
  > poucas a dezenas de dispositivos (briefing §6)". Spec §Edge Cases: "lista de
  > membros com milhares de entradas". Volume de referência presente para decisões
  > de paginação.

- [x] CHK-P07 — O padrão de uso (single-operator, on-premise, rede local) está
  documentado para calibrar requisitos de concorrência? [Completude, plan.md
  §Summary, Spec §Visão Geral] {auto}
  > Evidência: plan.md §Summary: "single-binary on-premise"; Spec §Visão Geral:
  > "equipe operacional" (volume baixo de usuários simultâneos). Justifica ausência
  > de requisitos de concorrência alta — intencional e documentado.

- [ ] CHK-P08 — O volume máximo esperado de eventos de presença na tabela
  `attendance_events` está estimado? [Cobertura, Gap] {humano}
  > [Gap] SC-005 exige suporte a ≥ 1.000 eventos na tela, mas não há estimativa
  > do volume total na tabela (ex: reconhecimentos diários × dias de operação).
  > Isso impacta o design do índice para a query de eventos paginados
  > (`created_at DESC, id DESC`).

---

## Queries e I/O

- [x] CHK-P09 — A estratégia de paginação cursor (keyset) está justificada em
  relação à performance para grandes volumes? [Clareza, Spec §FR-008, dec-008] {auto}
  > Evidência: dec-008 (score 3): "volume pode chegar a milhares; client-side
  > filtering apenas para busca rápida em janela carregada." Cursor-based paging
  > é a escolha correta para evitar `OFFSET` lento em grandes tabelas — decisão
  > fundamentada.

- [x] CHK-P10 — O endpoint `GET /admin/stats` está projetado como agregação única
  (sem N+1)? [Completude, contracts/admin-api.md §stats, plan.md §Summary] {auto}
  > Evidência: dec-013 (score 3): "endpoint único reduz latência e acopla lógica
  > de negócio no backend." contracts §stats: um endpoint retorna 3 contadores +
  > limiar. Sem N+1 por design.

- [ ] CHK-P11 — Os índices necessários para as queries novas (busca por nome/CPF,
  paginação por `id` e `(created_at, id)`) estão identificados como requisito?
  [Cobertura, Gap] {humano}
  > [Gap] plan.md §Project Structure menciona "sem novas migrations (dec-007)" mas
  > as queries novas (`ListMembersPaged`, `ListEventsPaged`) podem exigir índices
  > nas tabelas existentes para performance adequada. A ausência de novas tabelas
  > não garante ausência de novos índices necessários.

- [ ] CHK-P12 — O requisito de prevenção de N+1 na tela de membros (sync_status
  derivado de `ProcessingOutcome`) está especificado? [Cobertura, Gap] {humano}
  > [Gap] A tela de membros requer `sync_status` derivado de `member_processing_status`
  > — um join ou subquery. Sem requisito explícito, a implementação pode cair em
  > N+1 (1 query por membro para buscar ProcessingOutcome). Para milhares de membros
  > isso é crítico.

---

## Tamanho de Payload

- [x] CHK-P13 — A paginação está especificada de forma que limita o tamanho de
  resposta por chamada? [Clareza, contracts §members, §events] {auto}
  > Evidência: contracts §members e §events definem param `limit` para controlar
  > tamanho da página. Default e teto serão definidos na implementação (CHK-A16
  > apontou o gap de quantificação, mas o mecanismo existe).

- [x] CHK-P14 — Os campos desnecessários (raw_payload, event_key) estão excluídos
  dos DTOs de resposta para reduzir payload? [Completude, contracts §events,
  Spec §Security] {auto}
  > Evidência: contracts §events: "`raw_payload` (JSONB) e `event_key` não são
  > expostos." Redução de payload e exclusão de PII tratadas simultaneamente.

- [ ] CHK-P15 — A compressão de resposta HTTP (gzip) está especificada ou
  conscientemente omitida? [Cobertura, Gap] {humano}
  > [Gap] Para listas grandes de membros/eventos, compressão gzip pode reduzir
  > significativamente o payload. A spec não menciona — para on-premise em LAN
  > pode ser irrelevante, mas a decisão deve ser explícita.

---

## Frontend Performance

- [x] CHK-P16 — A ausência de bundler/toolchain Node está especificada como decisão
  de performance/simplicidade? [Clareza, plan.md §Technical Context] {auto}
  > Evidência: plan.md §Technical Context: "HTML/CSS/JS vanilla (ES modules) no
  > frontend — sem toolchain Node." "Frontend: nenhuma lib externa (zero CDN)."
  > Decisão intencional documentada — bundle size mínimo por ausência de deps.

- [ ] CHK-P17 — O budget de performance de frontend (LCP, FCP) está definido para
  a primeira carga do painel? [Clareza, Gap] {humano}
  > [Gap] SC-001 define 5s end-to-end pós-login incluindo requisição de dados; mas
  > não há budget de frontend isolado (ex: assets carregam em < 1s, FCP < 500ms).
  > Em on-premise com latência de LAN mínima, isso pode ser negligenciável — mas
  > deve ser decisão explícita.

- [x] CHK-P18 — A estratégia de embed de assets no binário Go está especificada
  para garantir zero dependência de deploy de arquivos externos? [Completude,
  plan.md §Summary, plan.md §Project Structure] {auto}
  > Evidência: plan.md §Summary: "frontend estático vanilla embutido via `embed.FS`".
  > plan.md §Project Structure: `internal/web/embed.go` com `//go:embed dist/*`.
  > Single-binary, sem CDN, sem dependência de arquivos externos em runtime.

---

## Degradação Graciosa

- [x] CHK-P19 — O comportamento quando o backend está lento (sync > 30s) está
  especificado para o operador? [Completude, Spec §Edge Cases] {auto}
  > Evidência: Edge Cases: "manter indicador de progresso e não fazer o operador
  > aguardar bloqueado na tela". SC-004 reforça feedback visual < 2s do clique.
  > Requisito de degradação para operação longa presente.

- [ ] CHK-P20 — O comportamento quando a API de dados do dashboard está lenta
  (timeout de carregamento) está especificado? [Cobertura, Gap] {humano}
  > [Gap] O edge case de sync lento está coberto, mas não há requisito de timeout
  > para chamadas de dados normais (stats, members, devices, events). Sem timeout
  > definido, o frontend pode ficar em loading infinito se o backend travar.

---

**Resumo Performance**:
- Total: 20 itens
- [x] auto resolvidos: 10
- [ ] humano aguardando decisão: 9 (CHK-P04, CHK-P05, CHK-P08, CHK-P11,
  CHK-P12, CHK-P15, CHK-P17, CHK-P20 + CHK-P15 gzip)
- [Gap]: 8 (targets por tela, percentil, volume events, índices, N+1 members,
  gzip, frontend budget, API timeout)
- [Ambiguity]: 1 (CHK-P05 — percentil dos targets)
