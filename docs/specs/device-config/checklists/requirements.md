# Requirements Quality Checklist: device-config

**Purpose**: Validar qualidade, clareza e completude dos requisitos escritos — não da implementação.
**Created**: 2026-06-21
**Feature**: [spec.md](../spec.md)
**Domínio**: Requirements (qualidade geral de requisitos)

> Items `{auto}` resolvidos pelo agente com citação de evidência.
> Items `{humano}` aguardam decisão do dono do produto.
> `[Gap]` = requisito ausente; `[Ambiguity]` = requisito ambíguo; `[Conflict]` = requisito conflitante.

---

## Completude de Requisitos

- [x] CHK001 - Cada user story tem critérios de aceite (acceptance scenarios) enumerados e verificáveis? [Completude, Spec §US1-US6] {auto}
  > SOURCED: US1 tem 3 cenários, US2 tem 3, US3 tem 4, US4 tem 3, US5 tem 3, US6 tem 2 — todos com Given/When/Then (spec.md §User Scenarios).

- [x] CHK002 - Todos os functional requirements têm rastreabilidade explícita para uma user story? [Completude, Spec §Requirements] {auto}
  > SOURCED: FRs agrupados em 7 grupos com referência a US (Grupo 1=US1, Grupo 2=US2, ..., Grupo 7 transversal). Rastreabilidade presente no título de cada grupo.

- [x] CHK003 - Os requisitos cobrem o estado "sem credenciais ISAPI configuradas" como caso distinto? [Completude, Spec §US1-AS3, §FR-007] {auto}
  > SOURCED: US1-AS3 define mensagem orientativa; FR-007 especifica retorno 503 se `ISAPI_CRED_KEY` ausente; FR-003 define `isapi_credentials_set` booleano.

- [x] CHK004 - Os requisitos de ações destrutivas (reboot, factory reset, limpar usuários, limpar faces) especificam confirmação obrigatória? [Completude, Spec §SC-004, §US3-AS2/3, §US5-AS2/3] {auto}
  > SOURCED: SC-004 exige confirmação explícita. US3-AS2 "confirmação no modal", US3-AS3 "digita o identificador para confirmar" (forte), US5-AS2 "digita confirmação", US5-AS3 "com confirmação forte".

- [x] CHK005 - Os requisitos incluem o comportamento esperado para dispositivo offline em cada grupo funcional? [Completude, Spec §US1-AS2, §US4-AS3, §Edge Cases] {auto}
  > SOURCED: US1-AS2 (dados estáticos do banco + campos ao vivo indisponíveis), US4-AS3 (erro claro + nenhuma ação silenciosa), Edge Cases (timeout com erro explicativo). US3 e US5 offline cobertos nos Edge Cases.

- [x] CHK006 - Os requisitos especificam quais dados persistem no banco versus o que é read-through da ISAPI? [Completude, Spec §Key Entities, Plan §Summary] {auto}
  > SOURCED: Key Entities documenta explicitamente: Device (banco), Door (não persistida — ISAPI real-time), WebhookDestination (não persistida — ISAPI real-time). Plan §Summary: "Persistência limitada a 2 colunas nullable de cache de capacidades (`max_users`, `max_faces`)".

- [x] CHK007 - Os requisitos cobrem o que acontece quando `ISAPI_CRED_KEY` está ausente para endpoints que dependem de AES-GCM? [Completude, Spec §FR-007, §Edge Cases-3] {auto}
  > SOURCED: FR-007 especifica 503 com mensagem orientativa para endpoint de credenciais. Edge Cases §3 especifica seções que dependem de ISAPI ficam desabilitadas. Mas: apenas FR-007 cobre o endpoint PUT credentials — os demais endpoints ISAPI quando a chave está ausente não têm FR equivalente (a descriptografia da senha também requer a chave). [Gap] — considerar FR adicional para quando a chave está ausente e operações ISAPI (não apenas PUT credentials) são acionadas.

- [x] CHK008 - Os requisitos para `GET /devices/{id}/users` incluem paginação e os campos retornados? [Completude, Spec §FR-016] {auto}
  > SOURCED: FR-016 especifica paginação via `searchResultPosition`/`maxResults`, campos `employeeNo`, `name`, `userType`, `numOfFace`, `valid`, `beginTime`, `endTime` e `total`. Clarification FR-016 adiciona dec-005 score 3 com verificação no legacy (spec.md §Clarifications).

- [ ] CHK009 - Os requisitos definem comportamento quando `DELETE /devices/{id}/users` falha parcialmente (timeout no meio da operação)? [Completude, Spec §Edge Cases] {auto}
  > Edge Cases §4 menciona "pode ter ficado em estado parcial; o painel exibe erro e orienta o operador a verificar manualmente ou tentar novamente". Mas FR-016b não especifica o tratamento de falha parcial no requisito funcional — apenas o Edge Case documenta o comportamento. [Gap] — FR-016b deveria mencionar comportamento de falha parcial para ser completo enquanto requisito.

- [x] CHK010 - Os success criteria são quantificados e mensuráveis (não apenas qualitativos)? [Clareza, Spec §Success Criteria] {auto}
  > SOURCED: SC-002 "<30 segundos", SC-003 "100% das operações — auditável por inspeção de logs", SC-004 "zero falsos acionamentos", SC-005 "<5 segundos em rede local normal", SC-006 "100% dos casos — zero erros genéricos". Todos mensuráveis.

---

## Clareza de Requisitos

- [x] CHK011 - O termo "confirmação forte" (US3-AS3, US5-AS2/AS3) está definido com critério verificável? [Clareza, Spec §US3-AS3, §US5-AS2] {auto}
  > SOURCED: US3-AS3 "digita o identificador para confirmar" — define confirmação forte como digitação de identificador. US5-AS2 "digita confirmação" — idem. Consistente entre as duas ocorrências. Verificável.

- [x] CHK012 - O termo "mensagem orientativa" (FR-007, Edge Cases §3) é suficientemente específico para implementação? [Clareza, Spec §FR-007] {auto}
  > FR-007 especifica retorno 503 com "mensagem orientativa" — o conteúdo da mensagem não é definido na spec, mas mensagens de UI são definidas em português na camada de implementação (plan.md §Constitution Check VII). Nível de especificação adequado para este estágio. Não é ambiguidade bloqueante.

- [x] CHK013 - "Status online/offline" (US1-AS1) tem critério de derivação definido? [Clareza, Spec §US1-AS1, §FR-001] {auto}
  > SOURCED: US1-AS1 especifica "status de conectividade derivado do último heartbeat". FR-001 especifica que dados vêm do banco (sem requisitar ISAPI a cada carregamento). O critério de "online" (ex: heartbeat nos últimos N segundos) não está quantificado na spec. [Ambiguity] — o threshold para considerar "online" vs "offline" baseado em heartbeat não está definido; a implementação precisará escolher um valor arbitrário.

- [x] CHK014 - O campo `open_duration` de US4-AS1 ("destravar 5s") é parâmetro fixo ou configurável pelo operador? [Clareza, Spec §US4-AS1] {auto}
  > US4-AS1 usa "Destravar 5s" como ação nomeada. A spec não define se o período é configurável ou fixo. O contrato ISAPI (hikvision-isapi.md §Door config) menciona `openDuration` lido do dispositivo. [Ambiguity] — não está claro se o "5s" é o valor do `openDuration` do dispositivo ou um parâmetro fixo da API admin. Implementação precisa de decisão.

- [x] CHK015 - Os campos `beginTime`/`endTime` de FR-016 têm formato de data especificado? [Clareza, Spec §FR-016, Contracts §admin-api.md] {auto}
  > FR-016 lista os campos mas não especifica formato. O contrato admin-api.md mostra `"beginTime": "..."` com reticências — não define ISO 8601 vs ISAPI nativo. hikvision-isapi.md lista os campos sem formato explícito. [Ambiguity] — formato de data de `beginTime`/`endTime` não especificado; o contrato admin-api.md §DeviceUser cita que preserva nomes ISAPI mas não o formato.

- [x] CHK016 - O requisito FR-011 ("identificação do operador via sessão") é suficientemente específico sobre o que logar? [Clareza, Spec §FR-011] {auto}
  > SOURCED: FR-011 especifica: `device_id`, `stage`, ação executada e identificação do operador (via sessão). Plan §Security especifica: "Log estruturado de FR-011 NÃO deve incluir corpo de request de credenciais". Suficientemente específico para implementação.

- [ ] CHK017 - O requisito FR-019 ("se o webhook removido for o webhook principal") tem critério de identificação do webhook principal verificável? [Clareza, Spec §FR-019] {auto}
  > SOURCED: FR-019 menciona atualizar `webhook_configured=false` "se o webhook principal do sistema for removido". O contrato admin-api.md §Grupo Webhooks especifica: "se `webhook_id` == `deterministicHostID`, client.go:341". A spec não menciona `deterministicHostID` — o critério está no contrato mas não na spec. Nível de detalhe adequado (spec define o comportamento; o "como" fica no contrato). OK — não é gap bloqueante.
  > [x] Resolvido: o critério está documentado no contrato (admin-api.md §DELETE webhooks). {auto}

---

## Consistência de Requisitos

- [x] CHK018 - Os requisitos de FR-021 (504 para timeout) e FR-015 (erros de porta distintos) são consistentes com o mapa de status codes em hikvision-isapi.md? [Consistência, Spec §FR-021/015, Contracts §hikvision-isapi.md] {auto}
  > SOURCED: Tabela hikvision-isapi.md §status codes: timeout → 504 (alinha FR-021), 401 digest → 502 (alinha FR-022), 4xx lógica → 502 distinto (alinha FR-015). Consistente.

- [x] CHK019 - O FR-009 (factory reset → `webhook_configured=false`) e o FR-019 (remoção de webhook principal → `webhook_configured=false`) são consistentes e não se contradizem? [Consistência, Spec §FR-009/019] {auto}
  > SOURCED: Ambos atualizam `webhook_configured=false` em cenários diferentes (reset do dispositivo vs. remoção explícita do webhook). Não há conflito — são dois caminhos independentes para o mesmo resultado esperado. Consistente.

- [x] CHK020 - O escopo de FR-016b (limpar usuários remove FACES associadas) é consistente com FR-017 (limpar faces separadamente)? [Consistência, Spec §FR-016b/017, Contracts §hikvision-isapi.md §Clear users] {auto}
  > SOURCED: hikvision-isapi.md §Clear users: "remove todos os usuários E suas faces (US5-AS2)". FR-017 limpa apenas a biblioteca de faces (FDLib) sem remover usuários. As duas operações são distintas e consistentes — FR-016b = tudo; FR-017 = só faces da FDLib.

- [x] CHK021 - O requisito de que `max_users`/`max_faces` sejam retornados como `null` quando indisponíveis (FR-002) é consistente com a definição na tabela como nullable (data-model.md)? [Consistência, Spec §FR-002, Plan §Technical Context] {auto}
  > SOURCED: FR-002 "campos de capacidade indisponíveis são retornados como nulos, nunca estimados". Plan §Project Structure menciona "000007 (2 colunas nullable)". admin-api.md §GET devices/{id}: `"max_users": null, "max_faces": null`. Consistente.

---

## Cobertura de Edge Cases

- [x] CHK022 - O Edge Case de credenciais erradas na ISAPI tem tratamento especificado sem expor a senha? [Cobertura, Spec §Edge Cases §1, §FR-022] {auto}
  > SOURCED: Edge Cases §1: "Retornar erro claro ao operador sem expor a senha; não fazer retry automático". FR-022 mapeia para 502 com mensagem sobre problema de autenticação com o dispositivo, sem expor senha.

- [x] CHK023 - O Edge Case de operação destrutiva com dispositivo que fica inacessível antes de responder está coberto? [Cobertura, Spec §Edge Cases §2] {auto}
  > SOURCED: Edge Cases §2: "Timeout com erro explicativo; a ação pode ou não ter sido executada — avisar ao operador para verificar fisicamente". FR-021 (504) cobre o tratamento.

- [x] CHK024 - O Edge Case de `ISAPI_CRED_KEY` ausente está coberto com comportamento definido? [Cobertura, Spec §Edge Cases §3, §FR-007] {auto}
  > SOURCED: Edge Cases §3 e FR-007 cobrem. Ver CHK007 — o Gap para outros endpoints persiste.

- [x] CHK025 - O Edge Case de falha parcial em "Limpar todos os usuários" está documentado? [Cobertura, Spec §Edge Cases §4] {auto}
  > SOURCED: Edge Cases §4 cobre. Ver CHK009 — o Gap (FR-016b não menciona) persiste no FR mas o Edge Case existe.

- [x] CHK026 - O Edge Case de dispositivo desconhecido (ID inválido) está coberto para todos os grupos de endpoints? [Cobertura, Spec §Edge Cases §5, §FR-023] {auto}
  > SOURCED: Edge Cases §5 e FR-023 cobrem: 404 para `{id}` inexistente, validado antes de tentar qualquer conexão ISAPI. FR-023 é transversal a todos os endpoints.

- [ ] CHK027 - Existem requisitos para o comportamento quando o factory reset é acionado mas o dispositivo está offline? [Cobertura, Spec §US3-AS3, §Edge Cases] {humano}
  > Edge Cases §2 cobre ações destrutivas genéricas quando o dispositivo fica inacessível DEPOIS do comando. Mas o cenário de "factory reset acionado com dispositivo já offline antes do envio" (timeout imediato) não tem cenário de aceite próprio em US3. Julgamento do dono do produto sobre se o nível de cobertura é adequado.

- [ ] CHK028 - O comportamento do sistema quando `GET /devices/{id}/users` retorna resultado paginado com `total` maior que a página deve ser especificado? [Cobertura, Spec §FR-016] {humano}
  > FR-016 define paginação mas não especifica se a SPA deve gerenciar paginação automática ("carregar mais") ou se exibe somente a primeira página com indicação do total. Decisão de UX/produto, não técnica.

---

## Requisitos Não-Funcionais

- [x] CHK029 - Os requisitos de performance têm targets mensuráveis? [NF-Performance, Spec §SC-005, Plan §Performance Goals] {auto}
  > SOURCED: SC-005 "<5 segundos em rede local normal" para controle de porta. Plan §Performance Goals: "operações ISAPI têm `defaultTimeout = 30s` (SOURCED — client.go:43)". Targets definidos.

- [x] CHK030 - Os requisitos de segurança (senha ISAPI) têm critério de auditabilidade explícito? [NF-Security, Spec §SC-003] {auto}
  > SOURCED: SC-003 "auditável por inspeção de logs do sistema" — critério explícito. FR-005 como MUST NOT para todos os cenários.

- [ ] CHK031 - Existem requisitos de logging/observabilidade para operações NÃO-destrutivas (ex: leitura de status, listagem de usuários)? [NF-Observabilidade, Spec §FR-011] {humano}
  > FR-011 especifica log estruturado apenas para ações destrutivas (reboot, factory reset). Operações de leitura (GET time, GET doors, GET users, GET webhooks) e PUT credentials não têm requisito de log explícito. Decisão do produto sobre nível de rastreabilidade operacional desejado.

- [x] CHK032 - Os requisitos cobrem o timeout de operações ISAPI (não apenas erros de autenticação e offline)? [NF-Resiliência, Spec §FR-021] {auto}
  > SOURCED: FR-021 especifica `504 Gateway Timeout` para timeout. Plan §Technical Context: `defaultTimeout = 30s` (client.go:43). Coberto.

---

## Dependências e Premissas

- [x] CHK033 - A dependência de `ISAPI_CRED_KEY` como variável de ambiente está documentada como premissa? [Dependência, Spec §FR-007, Plan §Constitution Check V] {auto}
  > SOURCED: FR-007 menciona `ISAPI_CRED_KEY`; Plan §Constitution Check V: "via env (SOURCED — config.go:149-156)". Dependência documentada.

- [x] CHK034 - A premissa de que o legado PHP não é chamado em runtime está documentada? [Dependência, Plan §Summary, §Constitution Check IV] {auto}
  > SOURCED: Plan §Summary: "Todos os contratos ISAPI são SOURCED do legacy `hik-api` (que serve como fonte de contrato, NÃO é chamado em runtime — research Decision 1)". Premissa explícita.

- [x] CHK035 - Os itens `[PROPOSTA]` (NTP, clear faces, maxUsers/maxFaces) estão identificados como riscos de implementação? [Dependência, Contracts §hikvision-isapi.md, §admin-api.md] {auto}
  > SOURCED: hikvision-isapi.md marca `[PROPOSTA]` em NTP, clear faces e maxUsers/maxFaces. admin-api.md §Resumo de proveniência lista explicitamente. plan.md §Re-check Constitution menciona "3 itens sem fonte verificada permanecem rotulados `[PROPOSTA]`". Riscos identificados e rastreados.

---

## Resumo de Resolução

| Marcador | Count |
|----------|-------|
| `[x]` auto-resolvidos | 27 |
| `[ ]` humano (aguardando decisão) | 5 (CHK027, CHK028, CHK031 + implícito CHK007 gap, CHK009 gap) |
| `[Gap]` (requisito ausente) | 2 (CHK007, CHK009) |
| `[Ambiguity]` (requisito ambíguo) | 3 (CHK013, CHK014, CHK015) |
| `[Conflict]` | 0 |

### Gaps — destino obrigatório

- **CHK007 [Gap]** → tarefa "definir FR para operações ISAPI quando `ISAPI_CRED_KEY` está ausente e a chamada exige descriptografia" em create-tasks.
- **CHK009 [Gap]** → tarefa "adicionar ao FR-016b o comportamento de falha parcial do `UserInfo/Clear`" em create-tasks.

### Ambiguidades — destino

- **CHK013 [Ambiguity]** → threshold de heartbeat para "online" vs "offline" a definir na implementação.
- **CHK014 [Ambiguity]** → se `open_duration` é fixo ou do dispositivo: verificar em execute-task ao implementar US4.
- **CHK015 [Ambiguity]** → formato de `beginTime`/`endTime`: confirmar ao implementar FR-016 (usar o formato nativo do ISAPI passthrough).
