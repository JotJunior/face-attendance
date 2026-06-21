# UX Checklist: Interface de Administração Web

**Purpose**: Validar a qualidade, clareza e completude dos requisitos de UX
(hierarquia visual, estados de interação, acessibilidade, estados vazios,
responsividade, feedback) da feature `front-end`. Gate formal pré-`execute-task`.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md)

---

## Hierarquia Visual e Tema

- [x] CHK-U01 — O requisito de dark mode está especificado como único tema suportado
  (sem toggle)? [Clareza, Spec §FR-002] {auto}
  > Evidência: FR-002: "dark mode como tema visual padrão. O sistema não precisa
  > oferecer toggle de tema — dark mode é o único tema suportado nesta versão."
  > Sem ambiguidade.

- [ ] CHK-U02 — São os tokens de design (cores, tipografia, espaçamento) definidos
  como requisitos para garantir consistência entre as 4 telas? [Clareza, Spec §FR-010,
  Ambiguity] {humano}
  > FR-010 delega a produção do design system à skill `frontend-design` em
  > `execute-task`, mas não especifica restrições mínimas (ex: paleta base, família
  > tipográfica, densidade de informação). Decisão de produto: existem constraints
  > visuais fixas (ex: compatibilidade com marca GOB, fonte específica)?

- [x] CHK-U03 — O requisito de qualidade visual está especificado com critério
  concreto de como será produzido? [Completude, Spec §FR-010, plan.md §FR-010] {auto}
  > Evidência: FR-010: "design visual MUST ser produzido com a skill `/frontend-design`".
  > plan.md §Dependências: "design system dark produzido com a skill `frontend-design`
  > na fase `execute-task`." Critério de processo definido.

---

## Estados de Interação

- [x] CHK-U04 — O estado de carregamento (loading) está especificado para a ação
  de sincronização manual? [Completude, Spec §FR-007, §US6-AC1, §SC-004] {auto}
  > Evidência: FR-007: "botão MUST mostrar estado de carregamento durante a
  > operação". US6-AC1: "painel exibe indicador de processamento em andamento".
  > SC-004: "feedback visual em menos de 2 segundos após clique." Requisito presente.

- [ ] CHK-U05 — Os estados de loading estão especificados para operações assíncronas
  das demais telas (carregamento de membros, dispositivos, eventos)? [Completude,
  Gap] {humano}
  > [Gap] FR-007 especifica loading para sync; mas as telas de listagem
  > (members, devices, events) não têm requisito de estado de loading enquanto a
  > API responde. Em redes lentas ou backend lento, tabelas em branco são
  > indistinguíveis de erros. Decisão de produto: loading state genérico ou por tela?

- [x] CHK-U06 — O estado de botão desabilitado durante sync em andamento está
  especificado? [Completude, Spec §US6-AC4] {auto}
  > Evidência: US6-AC4: "botão está desabilitado ou exibe aviso de
  > 'sincronização em progresso'". Requisito presente.

- [x] CHK-U07 — O feedback de sucesso/erro após sync manual está especificado?
  [Completude, Spec §FR-007, §US6-AC2, §US6-AC3] {auto}
  > Evidência: FR-007: "feedback de sucesso/erro ao concluir". US6-AC2: "mensagem
  > de confirmação". US6-AC3: "mensagem de erro descritiva". Requisito completo.

---

## Estados Vazios (Empty States)

- [x] CHK-U08 — O requisito de empty state está especificado para todas as telas
  de listagem? [Completude, Spec §FR-009, §Edge Cases] {auto}
  > Evidência: FR-009: "quando não há dados (nenhum dispositivo, nenhum membro,
  > nenhum evento), exibir mensagem informativa em vez de tabelas vazias ou zeros
  > silenciosos". Edge Cases exemplifica "Nenhum dispositivo registrado ainda".
  > Cobertura explícita.

- [ ] CHK-U09 — O requisito de empty state para resultado de busca sem resultados
  está especificado? [Completude, Gap] {humano}
  > [Gap] FR-009 cobre estado sem dados no banco; mas não especifica o empty state
  > quando uma busca por nome/CPF não encontra correspondências (caso diferente de
  > "nenhum membro no sistema"). Decisão: mesma mensagem genérica ou específica
  > para "nenhum resultado para a busca X"?

---

## Acessibilidade

- [ ] CHK-U10 — São os requisitos de acessibilidade (WCAG, nível alvo) especificados
  para o painel? [Completude, Gap] {humano}
  > [Gap] A spec não menciona requisitos de acessibilidade (nível WCAG AA/AAA,
  > navegação por teclado, contraste mínimo, screen reader labels). Para um painel
  > operacional de uso interno, WCAG 2.1 AA é o mínimo razoável — mas a omissão
  > deve ser decisão explícita.

- [ ] CHK-U11 — A navegação por teclado está especificada para fluxos críticos
  (login, navegação entre telas)? [Cobertura, Gap] {humano}
  > [Gap] Nenhum requisito de navegação por teclado na spec. O formulário de login
  > e a navegação pelo painel devem ser acessíveis via Tab/Enter como mínimo.
  > Decisão de produto: in-scope para MVP?

- [ ] CHK-U12 — O contraste de cores está especificado para atender o requisito
  de legibilidade no tema dark? [Clareza, Gap] {humano}
  > [Gap] FR-002 define dark mode mas não especifica relação de contraste mínima
  > (4.5:1 para texto normal, 3:1 para texto grande — WCAG 2.1 AA). A skill
  > `frontend-design` produzirá o design, mas sem restrição de contraste o
  > resultado pode falhar nos testes de acessibilidade.

---

## Responsividade

- [ ] CHK-U13 — Os requisitos de responsividade (breakpoints, dispositivos alvo)
  estão definidos para o painel? [Clareza, Ambiguity] {humano}
  > A spec não menciona responsividade. O briefing descreve uso "em conexão de
  > rede local" — provavelmente uso em desktop/monitor de sala de servidores.
  > Decisão de produto: o painel precisa funcionar em tablet/mobile ou é exclusivo
  > desktop? Se exclusivo desktop, deve ser explícito para a skill `frontend-design`
  > não gastar esforço em mobile layout.

---

## Feedback e Mensagens de Erro

- [x] CHK-U14 — O requisito de mensagem de erro de login está especificado sem
  vazar qual campo (usuário vs. senha) falhou? [Clareza, Spec §US1-AC3,
  contracts §login] {auto}
  > Evidência: US1-AC3: "vê mensagem de erro e permanece na tela de login".
  > contracts §login: `{"error":"credenciais inválidas"}` — mensagem genérica
  > que não distingue usuário vs. senha. Boas práticas de segurança preservadas.

- [x] CHK-U15 — O comportamento pós-expiração de sessão preserva o contexto do
  operador (URL atual)? [Completude, Spec §FR-012, §Edge Cases] {auto}
  > Evidência: FR-012: "redirecionar para login sem perder a URL atual (para
  > redirecionar de volta após re-autenticação)". Edge Cases confirma. contracts
  > §Sessão expirada define `?redirect=<path>` como mecanismo. Requisito completo.

- [x] CHK-U16 — O comportamento quando o sync demora mais de 30 segundos está
  especificado? [Cobertura, Spec §Edge Cases] {auto}
  > Evidência: Edge Cases: "se a chamada de sync demora mais de 30 segundos,
  > o painel deve manter o indicador de progresso e não fazer o operador aguardar
  > bloqueado na tela." Requisito presente. SC-004 reforça feedback < 2s após
  > clique (feedback visual imediato, independente do tempo de processamento).

---

## Paginação e Performance de UI

- [x] CHK-U17 — O requisito de paginação/virtualização para listas grandes está
  especificado? [Completude, Spec §Edge Cases, §SC-005] {auto}
  > Evidência: Edge Cases: "se lista de membros tem milhares de entradas: paginar
  > ou virtualizar". SC-005: "tela de logs suporta ≥ 1.000 eventos sem degradação
  > (scroll/filtragem fluidos)". Requisito de performance de UI presente.

- [x] CHK-U18 — A distinção entre busca client-side (dentro da página) e server-side
  (dataset completo) está especificada? [Clareza, Spec §FR-008] {auto}
  > Evidência: FR-008: "Busca client-side apenas para filtro rápido dentro da
  > página já carregada." Limitação explícita — UX da busca em tempo real é
  > na página carregada; nova busca dispara request ao servidor.

---

## Navegação e Layout Geral

- [ ] CHK-U19 — A estrutura de navegação entre as 4 telas (dashboard, dispositivos,
  membros, logs) e o login está especificada (menu lateral, abas, breadcrumb)? [Clareza,
  Gap] {humano}
  > [Gap] A spec define o conteúdo de cada tela mas não especifica o padrão de
  > navegação global (sidebar, top nav, breadcrumbs). A skill `frontend-design`
  > decidirá — mas constraints de navegação (ex: acesso a todas as telas sempre
  > visível vs. progressivo) deveriam ser requisito.

- [x] CHK-U20 — O requisito de CPF mascarado cobre todas as telas onde o campo
  aparece, incluindo a tela de busca? [Completude, Spec §FR-011, §SC-006] {auto}
  > Evidência: FR-011: "CPF MUST ser exibido mascarado em todas as telas do
  > painel". SC-006: verificável por inspeção de todas as telas. contracts especifica
  > `federal_document_masked` nos DTOs de members e events — mascaramento no
  > backend elimina risco client-side.

---

**Resumo UX**:
- Total: 20 itens
- [x] auto resolvidos: 10
- [ ] humano aguardando decisão: 8 (CHK-U02, CHK-U05, CHK-U09, CHK-U10,
  CHK-U11, CHK-U12, CHK-U13, CHK-U19)
- [Gap]: 7 (loading states listagens, empty state busca, WCAG, teclado,
  contraste, responsividade, padrão de navegação)
- [Ambiguity]: 1 (CHK-U13 — escopo de dispositivos alvo)
