# API Checklist: face-flow (Chamadas HTTPS / Mensagem + API Admin)

**Purpose**: Validar a QUALIDADE dos requisitos de API — nó HTTPS (nó 5), disparo
de mensagem (nó 8) e a API REST de gestão de fluxos. Valida requisito, não código.
**Created**: 2026-06-26
**Feature**: [spec.md](../spec.md)

## Nó HTTPS (nó 5)

- [x] CHK001 - Os componentes da requisição (URL, método, headers, body) estão definidos como configuráveis? [Completude, Spec §FR-014/US3] {auto} — Satisfeito: FR-014 + US3 (URL, cabeçalhos, corpo JSON).
- [x] CHK002 - O comportamento esperado quanto ao código de resposta HTTP está especificado (fire-and-forget)? [Clareza, Spec §FR-014] {auto} — Satisfeito: FR-014 ("qualquer código HTTP de resposta é aceito").
- [x] CHK003 - O timeout do HTTPS (gatilho de circuit-break) está quantificado com valor? [Clareza, Spec §CL-005] {auto} — Satisfeito após resolução: CL-005 firmada — `timeout_seconds` por-nó, default 30s (plan §3.3 L216, cap 300s).
- [ ] CHK004 - Há requisito de retry/backoff para a chamada HTTPS, ou está explicitamente fora de escopo? [Gap, Spec §FR-014] {auto} — Gap: a intenção é "disparar"; ausência de retry é coerente mas não declarada explicitamente como requisito (NA).
- [ ] CHK005 - O formato de variável inválida/ausente no body e nos HEADERS está definido de forma consistente? [Consistency, Spec §FR-020] {auto} — Ambiguidade: FR-020 define ausente→"" e sintaxe inválida→literal para o body; não especifica o mesmo para headers (ex.: `Authorization` interpolado). Plan §3.3 interpola headers, mas o requisito não cobre headers explicitamente.

## Idempotência / Efeitos colaterais

- [x] CHK006 - A idempotência do REGISTRO de execução (FlowExecutionLog) por evento está especificada? [Clareza, Spec §CL-001/Key Entities] {auto} — Satisfeito: `event_key` unique (CL-001).
- [ ] CHK007 - A idempotência do EFEITO EXTERNO (chamada HTTPS, envio de mensagem, troca de background) sob execuções concorrentes do mesmo evento está definida? [Conflict, Spec §FR-023 vs Constitution §II] {auto} — CONFLITO: FR-023 declara execuções concorrentes "independentes" e o edge case admite dois eventos quase-simultâneos; a unicidade só protege o LOG, não os side-effects — dois disparos do mesmo AttendanceEvent podem chamar o HTTPS/mensagem 2×, contrariando o Princípio II (idempotência chaveada por CPF, sem duplicar). Requer requisito de deduplicação de side-effect por evento ou declaração explícita de "at-least-once aceitável".

## Disparo de mensagem (nó 8) — BLOCKED_API

- [x] CHK008 - A spec marca o nó 8 como dependente de contrato externo não fornecido (sem inventar assinatura)? [Completude, Spec §FR-017/Dependências] {auto} — Satisfeito: FR-017 `[BLOCKED_API]` + tabela de Dependências + alerta Princípio I; nenhum endpoint/payload fabricado.
- [x] CHK009 - O comportamento do motor ao encontrar nó BLOCKED em execução está definido? [Clareza, Spec §Dependências] {auto} — Satisfeito: "motor MUST detectar nós BLOCKED ... acionar circuit-break com log descritivo".

## API REST Admin (gestão de fluxos)

- [ ] CHK010 - Os formatos de resposta de erro da API de fluxos (status + corpo) estão especificados como requisito? [Gap] {auto} — Gap: a spec não define contrato de erro da API admin; plan §5 propõe 422/409 como implementação, sem requisito-fonte.
- [x] CHK011 - O requisito de validação antes de publicar/ativar (rejeição de fluxo inválido) está definido? [Cobertura, Spec §FR-005/FR-022/SC-006] {auto} — Satisfeito: FR-005, FR-022, SC-006.
- [ ] CHK012 - Limites de payload da API (ex.: tamanho de upload de imagem de background) estão quantificados como requisito? [Gap, Spec §FR-024] {auto} — Gap: FR-024 exige upload mas não limita tamanho/formato; plan §5 propõe "≤5MB, JPEG/PNG" sem requisito-fonte.

## Priorização (decisão do dono do produto)

- [ ] CHK013 - O modelo at-least-once (side-effects podem duplicar sob concorrência) é aceitável para o negócio, ou exige deduplicação por evento? [Risco, ref CHK007] {humano}
- [ ] CHK014 - A ausência de retry no nó HTTPS é aceitável (perda silenciosa em falha transitória) frente ao propósito de automação? [Risco] {humano}

## Notes

- CHK007 (`[Conflict]`) é o achado de maior risco deste gate → `/clarify` para definir semântica de idempotência de side-effect, ou aceite explícito at-least-once.
- Gaps CHK005/CHK010/CHK012 → `/create-tasks` ("definir requisito de X") ou `/clarify`.
