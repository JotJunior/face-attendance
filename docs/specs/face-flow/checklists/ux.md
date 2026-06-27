# UX Checklist: face-flow (Editor de Fluxograma Visual)

**Purpose**: Validar a QUALIDADE dos requisitos de UX do editor visual de fluxos
(canvas, paleta de nós, configuração, publicação) — não a implementação.
**Created**: 2026-06-26
**Feature**: [spec.md](../spec.md)

## Editor / Canvas

- [x] CHK001 - Os requisitos de interação do canvas (adicionar, remover, conectar, configurar nós) estão definidos? [Completude, Spec §FR-002] {auto} — Satisfeito: FR-002 enumera "adiciona, remove, conecta e configura nós visualmente".
- [x] CHK002 - Os requisitos para nós dependentes de contrato externo (BLOCKED) definem como o editor os sinaliza ao usuário? [Clareza, Spec §FR-003] {auto} — Satisfeito: FR-003 exige render "sinalizados como 'aguardando contrato' e não executáveis".
- [x] CHK003 - O requisito de bifurcação visual do nó de decisão (duas saídas rotuladas) está especificado? [Clareza, Spec §FR-016/US1-AS3] {auto} — Satisfeito: FR-016 ("exatamente duas saídas") + US1 Acceptance #3 (saídas "válido"/"inválido").
- [ ] CHK004 - Os requisitos definem feedback de erro de validação na publicação (como/onde os erros de FR-005 são exibidos ao admin)? [Gap] {auto} — Gap: FR-005 exige "mensagem de erro" mas a spec não especifica forma de exibição (inline, lista, por-nó). Plan §5/§7 detalha exibição inline, mas é decisão de implementação não ancorada em requisito.
- [ ] CHK005 - Há requisito de acessibilidade (navegação por teclado / leitor de tela) para o canvas? [Gap] {auto} — Gap: nenhum requisito de a11y para o editor. Avaliar se está em escopo (painel admin interno).
- [ ] CHK006 - O comportamento esperado de edição concorrente (dois admins editando o mesmo fluxo) está definido na perspectiva da UX? [Gap] {auto} — Gap: CL-003 cobre edição vs execução em andamento, mas não dois editores simultâneos.

## Gerenciamento de Fluxos

- [x] CHK007 - Os requisitos da tela de listagem de fluxos (status, dispositivo vinculado) estão definidos? [Completude, Spec §FR-001] {auto} — Satisfeito: FR-001 ("lista de fluxos existentes, status, e dispositivo vinculado").
- [x] CHK008 - O requisito de desativar sem excluir um fluxo está especificado? [Completude, Spec §FR-006] {auto} — Satisfeito: FR-006.
- [x] CHK009 - A mensagem ao usuário na rejeição de dupla vinculação 1:1 está exigida como requisito? [Clareza, Spec §FR-004/US1-AS2] {auto} — Satisfeito: FR-004 exige "rejeitada com mensagem explicativa".

## Configuração de Nós

- [x] CHK010 - O requisito de persistência/exibição do valor configurado (ex.: nó de espera) ao reabrir o editor está coberto? [Cobertura, Spec §US1-AS4] {auto} — Satisfeito: US1 Acceptance #4.
- [ ] CHK011 - O vocabulário de variáveis interpoláveis é exposto ao admin no editor como requisito (não só implícito)? [Ambiguity, Spec §FR-020] {auto} — Ambiguidade: FR-020 define o vocabulário, mas nenhum FR exige que o editor o apresente ao usuário (descoberta de variáveis). Plan §7 menciona "hint de variáveis" como detalhe de implementação.

## Priorização (decisão do dono do produto)

- [ ] CHK012 - O escopo de UX excluído nesta release (a11y, undo/redo, edição concorrente) reflete o apetite do produto? [Risco] {humano}
- [ ] CHK013 - A profundidade de feedback de erro de validação (inline por nó vs lista agregada) atende à expectativa de usabilidade do admin? [Risco] {humano}

## Notes

- `{auto}` resolvidos com citação; `{humano}` aguardam o dono do produto.
- Gaps de UX (CHK004, CHK005, CHK006, CHK011) → candidatos a `/create-tasks` ("definir requisito de X") ou `/clarify`.
