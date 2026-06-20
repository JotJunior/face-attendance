# Integration & Queues Checklist: MVP de Controle de Presenca por Reconhecimento Facial

**Purpose**: Validar qualidade e completude dos requisitos de integracao com dispositivos HikVision (ISAPI), filas RabbitMQ (DLQ/retry), idempotencia por CPF e performance do pipeline.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [data-model.md](../data-model.md) | [plan.md](../plan.md)

## Integracao com Dispositivos HikVision

- [x] CHK035 - O escopo de reuso do legacy esta definido precisamente (apenas 3 operacoes ISAPI) e o que NAO deve ser portado esta listado explicitamente? [Completude, Spec §FR-013, contracts/hikvision-isapi.md §Reuso PROIBIDO] {auto}
  > Evidencia: FR-013 lista as 3 operacoes permitidas; `contracts/hikvision-isapi.md §Reuso PROIBIDO` enumera explicitamente o que nao pode ser reusado: `UserRepository`/`FaceRepository`, `JwtAuthMiddleware`, `AuthService`, demais controllers, demais endpoints ISAPI. Secao §Reuso PROIBIDO e exclusiva e precisa. Completo.
- [x] CHK036 - A estrategia de distribuicao de membros (para quais dispositivos o worker distribui) esta especificada como "apenas dispositivos registrados"? [Completude, Spec §FR-003, US4] {auto}
  > Evidencia: FR-003: "System MUST direcionar a carga de usuarios e faces (FR-009..FR-012) apenas para dispositivos registrados". US4 cenario 3 confirma. Completo.
- [x] CHK037 - O identificador estavel do dispositivo entre heartbeats (MAC ou serial) esta documentado como proposta pendente de validacao com o heartbeat real? [Clareza, data-model.md §Device NOTA] {auto}
  > Evidencia: `data-model.md §Device NOTA`: "`[PROPOSTA]`: o campo exato que identifica o dispositivo de forma estavel... NAO consta com granularidade no contrato verificado. O legacy loga `payload.macAddress` e `payload.ipAddress`... a implementacao deve preferir o identificador mais estavel disponivel". Esta documentado como proposta/desconhecido, nao como fato inventado. Completo para MVP.
- [x] CHK038 - O requisito de comportamento da API local ao receber heartbeat (insert na primeira vez, upsert nas subsequentes, sem duplicar) esta especificado via FR-001/FR-002 com cenarios de aceite em US4? [Completude, Spec §FR-001,FR-002, US4] {auto}
  > Evidencia: FR-001: "registrar dispositivo na base local na primeira vez"; FR-002: "atualizar liveness sem criar registros duplicados". US4 tem 3 cenarios de aceite cobrindo primeiro heartbeat, heartbeat subsequente, e distribuicao de membros. Completo.
- [ ] CHK039 - O requisito de liveness timeout (marcar dispositivo como inativo apos T sem heartbeat) esta explicitamente fora do escopo do MVP ou tem um default definido? [Clareza, data-model.md §Device state transitions] {auto}
  > [Gap] `data-model.md §Device state transitions`: `active --sem heartbeat por T (futuro)--> inactive [PROPOSTA — liveness timeout pos-MVP]`. A spec nao menciona liveness timeout; nao ha FR sobre inatividade de dispositivo. E razoavel que o MVP nao implemente — mas a exclusao nao e listada em "Out of Scope" do spec.md. Consequencia pratica: um dispositivo que para de funcionar continua na lista de ativos, e o worker pode tentar distribuir membros para ele. Destino: `/create-tasks` — tarefa para definir comportamento default (dispositivo offline visivel mas nao bloqueia pipeline) e adicionar a Out of Scope se nao for MVP.
- [x] CHK040 - O requisito de idempotencia do webhook (mesma face reconhecida multiplas vezes em curto intervalo nao gera multiplas marcacoes) esta especificado via event_key? [Completude, Spec §FR-016, data-model.md §AttendanceEvent, Edge Cases] {auto}
  > Evidencia: FR-016: "System MUST tornar a marcacao de presenca idempotente em re-entrega do mesmo evento"; `data-model.md §AttendanceEvent`: `UNIQUE (event_key)`; spec §Edge Cases: "reconhecimento multiplas vezes em curto intervalo → idempotente". SC-003 confirma. Completo.

## Filas RabbitMQ (resiliencia)

- [x] CHK041 - A estrutura de filas (fila principal `member.processing` + DLQ `member.processing.dlq` + header `x-retry-count`) esta especificada na data-model? [Completude, data-model.md §ProcessingMessage] {auto}
  > Evidencia: `data-model.md §ProcessingMessage`: fila `member.processing`; DLQ `member.processing.dlq`; header AMQP `x-retry-count (int)`. Plan.md confirma: "fila `member.processing` + DLX/DLQ; retry queue pattern". Completo.
- [x] CHK042 - O comportamento do worker ao esgotar tentativas de retry (rotear para DLQ sem perder a mensagem) esta especificado com os parametros configuravies? [Completude, Spec §FR-023-INFRA-RETRY, US3-cenario-5] {auto}
  > Evidencia: FR-023-INFRA-RETRY: "apos esgotar as tentativas, rotear para DLQ". US3 cenario 5: "falha persistente roteia para a dead-letter queue (DLQ) sem perder a mensagem". SC-004: "falhas persistentes terminam na DLQ, recuperaveis para reprocessamento". Parametros: `RETRY_MAX_ATTEMPTS=3`, `RETRY_INITIAL_BACKOFF_MS=1000ms`. Completo.
- [x] CHK043 - O requisito de que mensagens na DLQ sao recuperaveis (para reprocessamento manual) esta especificado? [Completude, Spec §SC-004] {auto}
  > Evidencia: SC-004: "falhas persistentes terminam na DLQ, recuperaveis para reprocessamento". A mecanica de recuperacao da DLQ (mensagem no RabbitMQ permanece ate ser consumida/deletada) e implicita ao design DLQ. Completo para MVP.
- [ ] CHK044 - O requisito de ordering (ordem de processamento das mensagens da fila) e de concorrencia de workers (multiplas replicas do worker consumindo a mesma fila) esta especificado para garantir que dois workers nao processem o mesmo membro simultaneamente? [Completude, plan.md §Structure Decision, Spec §FR-009] {auto}
  > [Gap] `plan.md §Structure Decision`: "workers escalaveis rodando N replicas". FR-009: "worker que processa cada mensagem de forma idempotente, chaveada por CPF". Mas nao ha requisito de exclusao mutua de processamento do mesmo CPF por dois workers simultaneos. A idempotencia garante que o resultado final e correto (upsert), mas nao impede processamento duplo simultaneo que pode causar condicao de corrida no dispositivo ou no DB. Destino: `/create-tasks` — tarefa para avaliar se lock por CPF (pessimista ou otimista via `member_processing_status`) e necessario para o MVP.
- [x] CHK045 - O requisito de que uma publicacao parcial na fila (apenas alguns membros enfileirados antes de falha) nao ocorre — ou seja, a carga e atomica ou descartavel em caso de falha — esta especificado? [Completude, Spec §US2-cenario-3, contracts/gob-api.md] {auto}
  > Evidencia: `contracts/gob-api.md §Regras de consumo`: "sem publicar mensagens parciais corrompidas" em caso de falha da GOB. US2 cenario 3: sistema nao publica mensagens parciais. A atomicidade e no nivel da "carga de um ciclo completo": se a GOB retorna erro, nenhuma mensagem e publicada. Completo para MVP (ciclo inteiro descartado vs. publicacao parcial nao e especificado com granularidade fina, mas o requisito firme de nao-parcial esta presente).

## Idempotencia por CPF (transversal)

- [x] CHK046 - O CPF (`federal_document`) esta definido como a chave unica de correlacao entre GOB, fila, dispositivo ISAPI e marcacao de presenca (4 fronteiras)? [Completude, Spec §FR-008,FR-022-INFRA-IDEMP, data-model.md] {auto}
  > Evidencia: FR-008: "System MUST usar `federal_document` (CPF) como identificador de correlacao entre GOB, fila, dispositivo e marcacao de presenca". FR-022-INFRA-IDEMP: "chave de idempotencia de todas as escritas externas e o CPF". `data-model.md §members`: `UNIQUE (federal_document)`. Completo.
- [x] CHK047 - A conversao de formato de CPF (digits da GOB ↔ mascara na marcacao GOB ↔ digits no HikVision) tem um helper isolado especificado no plan, com o ponto de conversao definido? [Completude, plan.md §Mapper de CPF] {auto}
  > Evidencia: `plan.md §Mapper de CPF`: "Um unico helper em `internal/domain/` converte digits↔mascara; a correlacao webhook↔membro normaliza ambos para digits antes de comparar (research §7). Esta e a unica conversao de formato de dado factual e esta isolada — NAO espalhada pelo codigo". Completo.
- [x] CHK048 - O requisito de idempotencia de upsert de usuario no HikVision (chaveado por `employeeNo`/CPF) esta especificado para cobrir reprocessamento da mesma mensagem da fila? [Completude, Spec §FR-009, contracts/hikvision-isapi.md §Estrategia upsert] {auto}
  > Evidencia: FR-009: "worker processa cada mensagem de forma idempotente, chaveada por CPF". `contracts/hikvision-isapi.md §Estrategia upsert`: "Chave = `employeeNo` (CPF). O endpoint `Modify` faz upsert; reprocessar a mesma mensagem nao duplica usuario". US3 cenario 4 confirma. Completo.
- [ ] CHK049 - O comportamento de idempotencia quando o mesmo membro tem sua selfie atualizada na GOB (nova `url_selfie`) e especificado — o sistema deve re-enviar a nova face ou manter a existente? [Completude, Spec §US3, data-model.md §ProcessingOutcome] {auto}
  > [Gap] A spec define que `url_selfie` e usada para upload de face (FR-011); a idempotencia e chaveada por CPF. Mas se a GOB atualizar a selfie de um membro e um novo ciclo de carga enfileirar o mesmo membro, o comportamento de update de face no dispositivo nao e especificado. O upsert de usuario (UserInfo) e idempotente por `employeeNo`; o upload de face (`faceDataRecord`) nao tem comportamento de update claro — o legacy nao tem evidencia de um caso de "atualizar face existente". Destino: `/clarify` — resolver se o MVP deve detectar mudanca de `url_selfie` e re-enviar face, ou se o ciclo de carga simplesmente re-faz o upload (e o dispositivo sobrescreve ou duplica).

## Performance

- [x] CHK050 - O target de latencia end-to-end (presenca marcada < 5s apos webhook) esta especificado como criterio de aceite mensuravel? [Mensurabilidade, Spec §SC-002, plan.md §Performance Goals] {auto}
  > Evidencia: SC-002: "tem sua presenca marcada na GOB em menos de 5 segundos apos o recebimento do evento de webhook, em condicoes normais de rede". `plan.md §Performance Goals`: "presenca marcada < 5s apos webhook (SC-002)". Mensuravel e com condicoes de contorno (rede normal). Completo.
- [x] CHK051 - O target de throughput da carga (poucos milhares de membros por ciclo sem intervencao) esta especificado? [Mensurabilidade, Spec §SC-006, plan.md §Scale/Scope] {auto}
  > Evidencia: SC-006: "sistema processa um ciclo de carga de poucos milhares de membros sem intervencao manual, com workers escalaveis horizontalmente". `plan.md §Scale/Scope`: "centenas a poucos milhares de membros/ciclo; poucas a dezenas de dispositivos". Mensuravel (escala declarada). Completo.
- [ ] CHK052 - O comportamento do sistema sob carga extrema (mais membros que o esperado, ou muitos dispositivos simultaneamente) esta especificado com degradacao graceful ou limit de carga? [Cobertura de Edge Cases, Spec §SC-006] {humano}
  > SC-006 define a escala esperada ("poucos milhares") mas nao especifica o comportamento fora do envelope (ex: 100k membros, 50 dispositivos). Degradacao graceful ou rejeicao explicita sao decisoes de produto — dependem do apetite de risco operacional do sistema on-premise. Aguardando decisao do dono do produto: o sistema deve ter um limite explicito de carga ou escalar indefinidamente?
- [x] CHK053 - O requisito de acessibilidade do trigger manual de carga (endpoint `/admin/sync`) para evitar esperar o proximo ciclo de 60min esta especificado? [Completude, Spec §FR-021-INFRA-SCHED] {auto}
  > Evidencia: FR-021-INFRA-SCHED: "expor endpoint HTTP de trigger manual sob demanda (dec-023 — briefing secao 3.1: 'chamada periodica (ou sob demanda)')". O trigger manual existe como requisito firme. Completo.

## Notes

- Items `{auto}` foram resolvidos contra spec.md, data-model.md, plan.md e contracts/
- Items `{humano}` aguardando decisao: CHK052 (comportamento sob carga extrema / degradacao graceful)
- Gaps abertos (`[Gap]`): CHK039 (liveness timeout fora de scope), CHK044 (ordering/concorrencia de workers), CHK049 (update de face quando selfie muda)
- Ambiguidades: CHK049 destino `/clarify`; CHK039/CHK044 destino `/create-tasks`
