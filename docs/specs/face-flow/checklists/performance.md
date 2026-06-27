# Performance Checklist: face-flow (Motor Disparado a Cada Reconhecimento)

**Purpose**: Validar a QUALIDADE dos requisitos de performance/escalabilidade do
motor de execução, acionado a cada evento de reconhecimento facial, sem bloquear o
webhook. Valida requisito, não implementação.
**Created**: 2026-06-26
**Feature**: [spec.md](../spec.md)

## Latência / Não-bloqueio

- [x] CHK001 - O alvo de latência para INICIAR a execução após o evento está quantificado e mensurável? [Mensurabilidade, Spec §SC-002] {auto} — Satisfeito: SC-002 ("menos de 500 ms após o recebimento do evento").
- [x] CHK002 - O requisito de não-bloqueio do handler do webhook (resposta imediata) está definido? [Clareza, Spec §FR-021/nota infra] {auto} — Satisfeito: webhook retorna 200 imediatamente; motor executa em goroutine separada (nota de infraestrutura + FR-021).
- [ ] CHK003 - O alvo de latência distingue "iniciar" de "concluir" e cobre o caso de nós de espera longos? [Ambiguity, Spec §SC-002/FR-012] {auto} — Ambiguidade: SC-002 mede só o INÍCIO; um fluxo com nó de espera de até 3600s tem duração de execução muito maior — não há requisito de duração total nem de que isso seja esperado/aceitável.

## Concorrência / Escalabilidade

- [x] CHK004 - O requisito de independência entre execuções concorrentes do mesmo fluxo está definido e é verificável? [Clareza, Spec §FR-023/SC-005] {auto} — Satisfeito: FR-023 + SC-005 (≥2 execuções concorrentes sem interferência de estado).
- [ ] CHK005 - Há requisito limitando o número de execuções/goroutines concorrentes (proteção de recursos)? [Gap, Spec §FR-012/FR-023] {auto} — Gap: nó de espera segura uma goroutine por até 1h (FR-012, máx 3600s); com alta taxa de reconhecimentos + esperas longas, o número de goroutines vivas é ilimitado. Nenhum requisito de bound/backpressure/limite de execuções simultâneas.
- [ ] CHK006 - Há requisito de throughput (taxa máxima de reconhecimentos por dispositivo/tempo) que o motor deve suportar? [Gap] {auto} — Gap: nenhum alvo de throughput; só latência de início (SC-002) e prova de 2 concorrentes (SC-005).

## Degradação / Timeouts

- [x] CHK007 - O timeout do nó HTTPS (limite de tempo de chamada externa) está quantificado? [Clareza, Spec §CL-005] {auto} — Satisfeito após resolução: `timeout_seconds` por-nó, default 30s, cap 300s (plan §3.3 L216-218).
- [x] CHK008 - O comportamento de degradação sob falha de nó (circuit-break + reset, sem travar reconhecimentos seguintes) está definido? [Cobertura, Spec §FR-021/US4/SC-004] {auto} — Satisfeito: FR-021, US4, SC-004.
- [ ] CHK009 - Há requisito de limite superior para a duração total de uma execução (timeout global do fluxo), evitando goroutines penduradas indefinidamente? [Gap, Spec §FR-021] {auto} — Gap: o circuit-break cobre erro de nó, mas não há requisito de timeout global do fluxo. Plan §3.1 propõe `context.WithTimeout(30min)` como implementação, sem requisito-fonte.

## Recursos / Persistência

- [x] CHK010 - A natureza stateless entre execuções (estado descartado ao fim/circuit-break) está especificada? [Clareza, Spec §nota infra/FR-021] {auto} — Satisfeito: "fluxos são stateless entre execuções; estado de execução é descartado".
- [ ] CHK011 - O custo de I/O por execução (ler fluxo + member lookup + persistir log a cada reconhecimento) tem requisito de performance considerado no alvo de SC-002? [Ambiguity, Spec §FR-019/SC-002] {auto} — Ambiguidade: SC-002 mede o início (dispatch da goroutine), o que naturalmente exclui o lookup; mas não está explícito se a busca de fluxo/membro entra ou não no orçamento de 500ms.

## Priorização (decisão do dono do produto)

- [ ] CHK012 - A ausência de bound de execuções concorrentes (CHK005) é aceitável no perfil de carga real (taxa de reconhecimentos × esperas longas) do deploy on-premise? [Risco] {humano}
- [ ] CHK013 - É necessário um alvo de throughput/duração total (CHK006/CHK003/CHK009), ou só a latência de início (SC-002) basta para o MVP? [Risco] {humano}

## Notes

- Achado central: o nó de espera (até 3600s) combinado com ausência de bound de concorrência (CHK005/CHK009) é o principal risco de recurso → `/clarify` ou `/create-tasks` para definir limite/timeout global como requisito.
- SC-002, SC-004, SC-005 são critérios mensuráveis e cobertos; lacunas estão em throughput/duração total/bound de recursos.
