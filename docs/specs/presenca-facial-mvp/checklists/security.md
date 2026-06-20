# Security Checklist: MVP de Controle de Presenca por Reconhecimento Facial

**Purpose**: Validar que os requisitos de seguranca estao completos, especificos e rastreavies — cobrindo os findings S1-S6 do gate owasp-security e os principios V (Segredos como Runtime) e VI (Observabilidade) da constitution.
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md §Security Considerations)

## Autenticacao e Autorizacao

- [x] CHK021 - O requisito de auth para `/admin/*` (token de admin via env, deny-by-default) esta definido na spec como requisito firme (nao apenas como proposta)? [Completude, Spec §FR-020, plan.md §S5] {auto}
  > Evidencia: `plan.md §S5`: "Tornar requisito firme: /admin/* exige token de admin via env (Principio V); deny-by-default. NAO reutilizar auth do legacy (Principio IV)". FR-020 cobre variaveis de ambiente. O finding S5 eleva a proposicao para requisito firme explicitamente. Completo.
- [x] CHK022 - O mecanismo de auth do webhook (allowlist de IP + path-secret) esta especificado para mitigar forjamento de presenca por host LAN nao autorizado? [Completude, plan.md §S1] {auto}
  > Evidencia: `plan.md §S1`: "Restringir a rota de webhook por allowlist de IP dos dispositivos registrados (`devices.ip_address`) E usar um path-secret dificil de adivinhar no `HttpHostNotification.path`". Dois controles complementares (IP + path-secret). Completo.
- [ ] CHK023 - O requisito de allowlist de IP do webhook (S1) esta referenciado na spec.md como FR ou como criterio de aceite de US1, ou existe apenas no plan.md? [Consistencia, Spec §FR-014 vs plan.md §S1] {auto}
  > [Gap] O finding S1 (allowlist de IP + path-secret) esta documentado no `plan.md §Security Considerations` mas NAO esta formalizado em nenhum FR na spec.md. FR-014 define o endpoint de webhook mas nao menciona restricao de IP ou path-secret. Isso cria risco de que a implementacao omita os controles de seguranca por nao encontra-los como requisitos formais. Destino: `/create-tasks` — criar tarefa de seguranca explicita para implementar allowlist de IP + path-secret no handler de webhook.
- [ ] CHK024 - O requisito de rate limiting para o webhook (por IP de dispositivo) e para `/admin/sync` (por frequencia) esta definido com limites quantitativos ou ao menos como requisito firme de implementacao? [Clareza, plan.md §S4] {auto}
  > [Gap] `plan.md §S4`: "Rate limiting no webhook (por IP de dispositivo) e em `/admin/sync`". O requisito existe no plan mas sem limites quantitativos (ex. N req/s por IP) e sem formalizacao como FR na spec. Limites quantitativos sao configuracao de implementacao — aceitavel para MVP. Mas o requisito firme (rate limiting DEVE existir) nao esta em nenhum FR. Destino: `/create-tasks` — tarefa de implementar rate limiting com limite configuravel via env.

## Protecao de Dados (PII)

- [x] CHK025 - O requisito de mascaramento de CPF nos logs estruturados (mascarar como `***.***.***-NN`) esta especificado para evitar vazamento de PII? [Completude, plan.md §S3, Spec §FR-018] {auto}
  > Evidencia: `plan.md §S3`: "Mascarar CPF nos logs (ex. `***.***.***-NN`); estende Principio V (segredos) a PII. O campo `cpf` do log estruturado carrega forma mascarada; valor cheio so em colunas de DB com acesso controlado". FR-018 define o log estruturado. Completo.
- [x] CHK026 - O campo `raw_payload` da tabela `attendance_events` (JSONB com payload bruto HikVision) tem requisito de acesso controlado documentado? [Completude, data-model.md §AttendanceEvent, plan.md §S2] {auto}
  > Evidencia: `plan.md §S2`: "`raw_payload` so como JSONB parametrizado, nunca interpolado". `data-model.md §AttendanceEvent`: campo `raw_payload JSONB NOT NULL`. O controle de acesso ao DB e topico de ops/deploy, fora do escopo do MVP. O requisito de nao interpolacao (prepared statements) esta definido. Completo para MVP.
- [x] CHK027 - O requisito de que segredos (`GOB_STATE_TOKEN`, credenciais ISAPI) nunca aparecem em logs esta definido? [Completude, Spec §FR-020, Constitution Principio V] {auto}
  > Evidencia: FR-020: "System MUST obter [...] credenciais ISAPI dos dispositivos via configuracao de runtime (variaveis de ambiente), nunca hardcoded". Constitution Principio V: "segredos so via env de runtime; logs sem segredos". Plan.md: "`last_error` sem segredos — Principio V" (campo da tabela `member_processing_status`). Completo.

## Validacao de Input

- [x] CHK028 - O requisito de validacao de CPF por regex (11 digitos) antes de usar `employeeNoString` como `employeeNo`/`{cpf}` esta especificado para prevenir injection? [Completude, plan.md §S2, Spec §FR-017] {auto}
  > Evidencia: `plan.md §S2`: "Validar CPF por regex (11 digitos) antes de usar em `employeeNo`/`{cpf}`". FR-017: ignorar eventos cujo `employeeNoString` nao corresponde a membro conhecido (validacao implicita). A validacao por regex e requisito firme explicito no plan. Completo.
- [x] CHK029 - O requisito de uso de prepared statements/parametrizado em todo acesso PostgreSQL (sem concatenacao de strings com dados externos) esta especificado? [Completude, plan.md §S2] {auto}
  > Evidencia: `plan.md §S2`: "Prepared statements/parametrizado em todo acesso PostgreSQL (sem concatenacao)". Cobre `employeeNoString`, `raw_payload` e todos os campos derivados de payload externo. Completo.
- [x] CHK030 - O tratamento de payload de webhook malformado (sem crash do handler) esta especificado como requisito? [Completude, Spec §Edge Cases, contracts/inbound-http.md §Response] {auto}
  > Evidencia: `contracts/inbound-http.md §Response`: "nunca derrubar o handler por payload malformado (Edge Case spec)"; payload malformado → 200 + log. Spec §Edge Cases: "registro o evento e nao marcar presenca, sem derrubar o handler". Completo.
- [ ] CHK031 - O requisito de validacao de formato de imagem (JPEG esperado, verificado pelo mime type) antes de enviar ao ISAPI esta especificado para evitar falhas silenciosas no upload de face? [Cobertura de Edge Cases, contracts/hikvision-isapi.md §Pipeline da imagem] {auto}
  > [Gap] `contracts/hikvision-isapi.md §Pipeline da imagem`: "mime detectado da imagem (FaceService.php:189) — esperado `image/jpeg`". A spec nao define o que fazer se a `url_selfie` retornar uma imagem nao-JPEG ou corrompida (alem de "falha de download → retry/DLQ"). Formato invalido pode causar rejeicao silenciosa pelo dispositivo. Destino: `/create-tasks` — tarefa para validar mime type da imagem antes do upload e tratar com log estruturado especifico.

## Riscos Residuais

- [x] CHK032 - O risco residual de ISAPI HTTP + Digest (replay attack na LAN) esta documentado e aceito explicitamente para MVP? [Consistencia, plan.md §S6] {auto}
  > Evidencia: `plan.md §S6`: "Aceito para MVP on-premise (rede local confiavel). Registrado como risco residual; HTTPS no dispositivo e melhoria pos-MVP se o firmware suportar". Risco INFO, explicito, aceito. Completo.
- [x] CHK033 - Os findings de seguranca HIGH (S1, S2) estao convertidos em requisitos de implementacao formais (nao apenas em condicoes de aceite do gate)? [Consistencia, plan.md §Security] {auto}
  > Evidencia: `plan.md §Security Considerations`: "Estes requisitos sao incorporados ao backlog (/create-tasks) como tarefas de seguranca explicitas. Nenhum finding bloqueia o plano (todos resolviveis no desenho/implementacao)". O plan lista os requisitos firmemente como condicoes de mitigacao. A formalizacao via tasks e prevista. Completo como intencao; o gap e que ainda nao existem tarefas (tasks.md nao criado). Destino: `/create-tasks` — incluir S1/S2/S3/S4/S5 como tarefas de seguranca marcadas `[C]` (criticas).
- [ ] CHK034 - Existe requisito de auditoria de acesso ao `/admin/sync` (log de quem disparou o ciclo e quando) alem do rate limiting? [Completude, Spec §FR-018, plan.md §S4] {auto}
  > [Gap] FR-018 define log estruturado para operacoes criticas; `/admin/sync` e operacao critica (dispara carga de todos os membros). Mas nao ha FR especifico exigindo log de auditoria do trigger manual (quem disparou, timestamp). Sem isso, nao ha rastreabilidade de disparo intencional vs. automatico. Destino: `/create-tasks` — tarefa para incluir log estruturado em `/admin/sync` com `stage`, `trigger_type=manual`, e IP do chamador.

## Notes

- Items `{auto}` foram resolvidos contra spec.md, plan.md §Security Considerations, constitution.md
- Items `{humano}` ficam `[ ]` aguardando decisao do dono do produto (nenhum neste arquivo — todos os itens de apetite de risco ja estao decididos no plan)
- Gaps abertos (`[Gap]`): CHK023 (allowlist IP nao formalizada como FR), CHK024 (rate limiting sem limites quantitativos), CHK031 (validacao de mime type da imagem), CHK034 (log de auditoria do /admin/sync)
- Destino de todos os gaps: `/create-tasks` como tarefas de seguranca `[C]` criticas
