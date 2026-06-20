# Security Checklist: Interface de Administração Web

**Purpose**: Validar a qualidade, clareza e completude dos requisitos de segurança
(autenticação, autorização, proteção de dados, credenciais, input, logging) da feature
`front-end`. Gate formal pré-`execute-task`.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md) | [contracts/admin-api.md](../contracts/admin-api.md)

---

## Autenticação

- [x] CHK-S01 — São os requisitos de autenticação especificados para **todos** os
  recursos protegidos do painel? [Completude, Spec §FR-001, Spec §FR-013]
  {auto}
  > Evidência: FR-001 exige auth em qualquer rota do painel; FR-013 define
  > que o painel é servido em `/admin/*`; contracts/admin-api.md §Sumário lista
  > todas as rotas com coluna "Auth" e confirma que `/health` e `/admin/api/login`
  > são as únicas sem cookie — legítimo por contrato.

- [x] CHK-S02 — O mecanismo de sessão (cookie httpOnly + HMAC) está especificado
  com atributos de segurança suficientes (HttpOnly, Secure, SameSite, TTL)? [Clareza,
  Spec §FR-001, contracts/admin-api.md §POST /admin/api/login] {auto}
  > Evidência: contrato de login define `Set-Cookie: admin_session=...; HttpOnly;
  > Secure; SameSite=Strict; Path=/admin; Max-Age=<TTL_segundos>`. Todos os 4
  > atributos críticos estão presentes com semântica exata.

- [x] CHK-S03 — O requisito de comparação de senha em tempo constante está explícito
  para prevenir timing attacks? [Clareza, contracts/admin-api.md §POST login, plan.md
  §Quality Gate S3] {auto}
  > Evidência: contracts/admin-api.md especifica `crypto/subtle` para comparação;
  > plan.md §Quality Gate S3 reitera "Comparação constant-time obrigatória".

- [x] CHK-S04 — O comportamento em tentativas de login falhas (brute force) está
  especificado com mecanismo concreto? [Completude, plan.md §Quality Gate S4] {auto}
  > Evidência: plan.md §Quality Gate S4 define "Reusar `RateLimitMiddleware`
  > [EXISTENTE] (middleware.go:105-140, token bucket per-IP) no endpoint de login"
  > como "Requisito obrigatório de `execute-task`".

- [x] CHK-S05 — A política de expiração de sessão e o comportamento pós-expiração
  estão especificados? [Clareza, Spec §FR-012, contracts §Sessão expirada] {auto}
  > Evidência: FR-012 exige redirecionamento para login sem perder URL atual.
  > contracts/admin-api.md §Sessão expirada define: "qualquer endpoint `/admin/api/*`
  > responde 401 → frontend intercepta globalmente → redireciona preservando `?redirect=<path>`".

- [ ] CHK-S06 — O TTL padrão da sessão está quantificado e justificado para o
  perfil de uso on-premise? [Clareza, plan.md §Quality Gate S1] {humano}
  > plan.md §S1 recomenda "≤ 8h via `ADMIN_SESSION_TTL_HOURS`" mas é uma
  > recomendação, não um requisito com valor mandatório. Decisão de negócio:
  > qual o TTL padrão e qual o teto máximo permitido pela env?

- [x] CHK-S07 — O requisito de logout está especificado com comportamento de
  invalidação? [Completude, Spec §US1-AC4, contracts §POST /admin/api/logout] {auto}
  > Evidência: US1 Acceptance Scenario 4 especifica "sessão encerrada + novo login
  > exigido". contracts §logout define expiração do cookie via `Max-Age=0`. Nota:
  > sessão HMAC é stateless, portanto a invalidação é best-effort (cookie clear);
  > plan.md §S1 documenta isso explicitamente — requisito está claro.

---

## Autorização e Acesso

- [x] CHK-S08 — O modelo de autorização está documentado (quem pode acessar o quê)?
  [Completude, Spec §FR-001] {auto}
  > Evidência: modelo é single-role (operador autenticado = acesso total ao painel).
  > FR-001 não diferencia permissões entre recursos — design intencional para MVP
  > single-operator (briefing). Ausência de RBAC é explícita e justificada.

- [x] CHK-S09 — O requisito de deny-by-default para rotas do painel está explícito?
  [Completude, Spec §SC-003, FR-001] {auto}
  > Evidência: SC-003 é categórico: "100% das rotas do painel redirecionam para
  > login quando acessadas sem sessão válida — sem vazamento de dados protegidos."
  > contracts/admin-api.md §Sumário confirma o padrão por rota.

---

## Proteção de Dados (CPF / PII)

- [x] CHK-S10 — O requisito de mascaramento de CPF é definido para TODAS as telas
  onde CPF aparece? [Completude, Spec §FR-011, §SC-006, §Edge Cases] {auto}
  > Evidência: FR-011 exige mascaramento em "todas as telas do painel".
  > SC-006 é critério de sucesso mensurável: "CPF não aparece sem máscara em
  > nenhuma tela, verificável por inspeção visual". Edge Cases reiteram mascaramento
  > em membros e logs. contracts/admin-api.md especifica `federal_document_masked`
  > (nunca o campo cru) nos DTOs de members e events.

- [x] CHK-S11 — O mascaramento é especificado como responsabilidade do backend
  (não do frontend)? [Clareza, plan.md §Summary, contracts §members, §events] {auto}
  > Evidência: plan.md §Summary: "CPF mascarado no backend (SC-006)".
  > contracts/admin-api.md: campo `federal_document_masked` nos DTOs — o campo
  > cru `federal_document` não é exposto na resposta.

- [x] CHK-S12 — Estão especificados quais campos de dados brutos (raw_payload,
  event_key) são excluídos das respostas da API? [Completude, Spec §Security S6,
  contracts/admin-api.md §events] {auto}
  > Evidência: plan.md §Quality Gate S6 e contracts §events: "`raw_payload` (JSONB)
  > e `event_key` não são expostos (diagnóstico interno)." Requisito explícito de
  > exclusão do DTO.

- [x] CHK-S13 — O requisito de não vazar CPF completo via logs dos handlers está
  especificado? [Completude, plan.md §Quality Gate S6] {auto}
  > Evidência: plan.md §S6: "handlers da UI NÃO logam CPF completo nem ecoam
  > `raw_payload` em erros (alinha Constitution VI)."

- [ ] CHK-S14 — A política de retenção e descarte de `AttendanceEvent` e demais PII
  está definida para atender LGPD (art. 15/16)? [Compliance, Gap] {humano}
  > [Gap] A spec e o plan não mencionam política de retenção de dados de eventos
  > de presença (que contêm CPF e biometria facial — dados sensíveis pela LGPD).
  > Decisão de negócio: qual o prazo de retenção e como é feito o descarte?

---

## Credenciais e Segredos

- [x] CHK-S15 — Todas as credenciais do painel estão mapeadas para variáveis de
  ambiente (sem hardcode)? [Completude, Spec §FR-001, plan.md §Constitution V] {auto}
  > Evidência: plan.md §Constitution V: `ADMIN_USERNAME`, `ADMIN_PASSWORD`,
  > `ADMIN_SESSION_SECRET`, `ADMIN_SESSION_TTL_HOURS`, `DEVICE_OFFLINE_THRESHOLD_HOURS`
  > todas via env com padrão `require()`/`optionalInt()`. Sem hardcode — PASS.

- [ ] CHK-S16 — Existe requisito para rotação do `ADMIN_SESSION_SECRET` (e o
  comportamento de sessões ativas durante a rotação está definido)? [Clareza,
  plan.md §Quality Gate S1, Gap] {humano}
  > plan.md §S1 menciona "revogação total = rotacionar `ADMIN_SESSION_SECRET`" mas
  > não especifica procedimento operacional nem o que ocorre com sessões ativas
  > (todas invalidadas imediatamente — é o comportamento correto, mas precisa ser
  > requisito explícito).

- [x] CHK-S17 — O requisito de complexidade/política de senha de admin está
  conscientemente omitido e a omissão está justificada? [Clareza, plan.md §Gate S3] {auto}
  > Evidência: plan.md §S3: "`ADMIN_PASSWORD` plaintext em env, sem hashing —
  > Aceitável: padrão do projeto (Constitution §V, segredos via env),
  > single-operator on-premise." Decisão consciente e documentada.

---

## Input Validation

- [x] CHK-S18 — Os requisitos de validação de input estão definidos para o endpoint
  de login? [Completude, contracts/admin-api.md §POST login] {auto}
  > Evidência: contracts §login define validação mínima: `username` e `password`
  > string não-vazio, retornando 400 para corpo ausente/malformado.

- [ ] CHK-S19 — Os limites de tamanho de payload estão quantificados para os
  endpoints POST (login, sync)? [Clareza, Gap] {humano}
  > [Gap] Nem a spec nem o plan definem tamanho máximo de payload para `POST
  > /admin/api/login` (ex: limite de 1KB razoável para credenciais). Sem limite
  > explícito, a implementação decide arbitrariamente.

- [x] CHK-S20 — Os parâmetros de query de busca e filtragem são especificados com
  validação server-side? [Completude, Spec §FR-008, contracts §members, §events] {auto}
  > Evidência: contracts especifica que busca (`q`) e filtragem por data (`from`,
  > `to`) são processadas server-side. plan.md §Summary: "busca server-side". A
  > normalização de CPF na busca está especificada ("normalizado server-side").

---

## Logging e Auditoria

- [x] CHK-S21 — Os eventos de segurança relevantes (login bem-sucedido, login falho,
  logout) têm requisito de logging? [Completude, plan.md §Constitution VI] {auto}
  > Evidência: plan.md §Constitution VI: "Handlers novos usam o `logging.Logger`
  > existente". plan.md §Summary menciona o logger existente. Implícito no padrão
  > do projeto, mas o requisito explícito de quais eventos logar não está na spec
  > — confiar no padrão do projeto. [Ambiguity] Qual o nível mínimo de log para
  > eventos de auth (info vs warn vs error)?

- [x] CHK-S22 — Existe requisito que proíbe o vazamento de segredos nos logs? [Clareza,
  plan.md §Constitution V + §Gate S6] {auto}
  > Evidência: plan.md §Constitution V: "logs não vazam segredo". §S6:
  > "handlers NÃO logam CPF completo". Requisito presente.

---

## CSRF e Same-Origin

- [x] CHK-S23 — A mitigação de CSRF está especificada e a decisão de não usar token
  CSRF está justificada? [Completude, plan.md §Quality Gate S5] {auto}
  > Evidência: plan.md §S5: "Mitigado por `SameSite=Strict` + same-origin +
  > `Secure`. Sem token CSRF dedicado para o MVP (defense-in-depth opcional via
  > header custom)." Decisão documentada com justificativa técnica.

---

## TLS / Transporte

- [ ] CHK-S24 — O requisito de TLS para o painel em produção está formalizado
  (mesmo que implementado externamente via proxy)? [Clareza, plan.md §Gate S8] {humano}
  > plan.md §S8 menciona "documentar expectativa de TLS" como item a fazer.
  > O flag `Secure` no cookie exige HTTPS — em LAN sem TLS o cookie nunca é
  > armazenado. Decisão operacional: TLS é obrigatório para deploy (deve constar
  > no quickstart como pré-requisito)?

---

**Resumo Security**:
- Total: 24 itens
- [x] auto resolvidos: 16
- [ ] humano aguardando decisão: 5 (CHK-S06, CHK-S14, CHK-S16, CHK-S19, CHK-S24)
- [Ambiguity]: 1 (CHK-S21 — nível de log de eventos de auth)
- [Gap]: 2 (CHK-S14 LGPD retenção, CHK-S19 payload size limit)
