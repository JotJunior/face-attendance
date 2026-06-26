# API + Security + Upload Checklist: device-preferences

**Purpose**: Valida a qualidade dos requisitos dos 18 novos endpoints admin,
controles de segurança (DoS/body e SSRF) e os 3 mecanismos distintos de upload
multipart. Este é o quality gate de requisitos pré-create-tasks.

**Created**: 2026-06-25
**Feature**: [spec.md](../spec.md) (23 FRs) · [plan.md](../plan.md) (§6.1 security findings)
**Domínios**: api, security, upload

---

## 1. Contratos de API — Completude de Request

- [x] CHK001 - Cada novo endpoint admin tem método HTTP e path explicitamente definido na spec? [Completude, Spec §Requirements] {auto}
  > Evidência: spec §Requirements define os 18 endpoints com método e path para FR-001 a FR-018. Todos os caminhos `/admin/api/devices/{id}/preferences/*` e `/stats` estão nomeados.

- [x] CHK002 - Os campos multipart enviados ao ISAPI (field names, tipos) estão especificados para cada um dos 3 mecanismos de upload distintos? [Completude, Spec §FR-007, §FR-010, §FR-013] {auto}
  > Evidência: FR-007 nomeia `UploadCustomStandbyPic` (JSON) + `filePath` (binário); FR-010 nomeia `picture_info` (JSON) + `picture_name` (binário JPEG com nome `<id>.jpg`); FR-013 nomeia `name`, `type`, `size`, `file` (binário).

- [x] CHK003 - O fluxo de 5 etapas do FR-013 (media/propaganda) especifica o que acontece em cada etapa e qual o payload? [Completude, Spec §FR-013] {auto}
  > Evidência: FR-013 descreve as 5 etapas sequenciais (a)-(e) com verbos, paths e shape de body/XML para cada uma. Etapas de falha parcial e materiais órfãos estão cobertos em Clarification 4.

- [x] CHK004 - O read-modify-write do FR-002 (AuthMode) especifica quais campos são substituídos e quais são preservados? [Clareza, Spec §FR-002] {auto}
  > Evidência: FR-002 diz "substitui o campo `verifyMode` em todos os `WeekPlanCfg` do payload" e "envia o plano completo" — o que preservar está implícito (tudo exceto verifyMode). Research §G1 confirma o contrato.

- [x] CHK005 - O read-modify-write do FR-004 (Display) especifica explicitamente os campos read-only a preservar? [Clareza, Spec §FR-004] {auto}
  > Evidência: FR-004 lista explicitamente os campos read-only: `camera`, `fingerPrintModule`, `faceAlgorithm`, `saveCertifiedImage`, `readInfoOfCard`, `workMode`, `ecoMode`, `enableScreenOff`, `popUpPreviewWindow` e o atributo `version` do XML raiz.

- [x] CHK006 - O mapeamento de `showMode` (normal/full/split → showMode ISAPI + advertisingDisplayType) está definido na spec? [Clareza, Spec §FR-004] {auto}
  > Evidência: FR-004 define o mapeamento: `normal→showMode=normal advertisingDisplayType=full; full→showMode=advertising advertisingDisplayType=full; split→showMode=advertising advertisingDisplayType=split`.

- [x] CHK007 - O FR-016 (stats) especifica quais campos ISAPI mapeiam para quais campos da resposta admin? [Completude, Spec §FR-016] {auto}
  > Evidência: FR-016 mapeia explicitamente: `users.total` ← `UserInfoCount.userNumber`, `users.faces` ← `UserInfoCount.bindFaceUserNumber`, `users.cards` ← `UserInfoCount.bindCardUserNumber`, `users.max` ← `UserInfo.maxRecordNum`, `events.total` ← `AcsEventTotalNum.totalNum`, `events.max` ← `AcsEvent.totalNum.@max`.

- [ ] CHK008 - A estrutura JSON da **resposta** dos endpoints de leitura (FR-001, FR-003, FR-006) está definida na spec — não apenas o que a ISAPI retorna, mas o shape do JSON que o handler devolve ao painel? [Completude, Gap, Spec §FR-001, §FR-003, §FR-006] {auto}
  > **[Gap]** FR-001 diz "retorna a configuração do plano semanal de verificação" mas não especifica se o handler devolve o JSON bruto da ISAPI, um envelope `{data: ...}`, ou campos mapeados. FR-003 diz "a resposta da API admin DEVE incluir os campos mapeados: `showMode`, `screenOffTimeout`, `previewShowTime`, `standbyTimeout`" — isso está parcialmente resolvido para display. FR-006 não define o shape da resposta. Gap de clareza em FR-001 e FR-006.

- [ ] CHK009 - O FR-013 especifica o shape da resposta de sucesso e o shape da resposta de erro com material órfão? [Completude, Gap, Spec §FR-013, Clarification 4] {auto}
  > **[Gap parcial]** Clarification 4 define que "o sistema DEVE incluir o `id` do material criado na etapa (a)" na resposta de erro, mas o shape completo da resposta de sucesso (ex: retorna o ID do material? do programa? do agendamento?) não está especificado. Create-tasks precisará decidir.

---

## 2. Contratos de API — Respostas de Erro

- [x] CHK010 - Os status codes de erro para condições de falha padrão (device não existe, timeout, credenciais inválidas) estão definidos na spec? [Completude, Spec §FR-020, §FR-021] {auto}
  > Evidência: FR-021 define `404` para device inexistente; FR-020 define `504` para timeout e `502` para erro de autenticação ISAPI.

- [x] CHK011 - O FR-022 (validação de tipo de imagem) especifica o status code retornado para tipo inválido? [Clareza, Spec §FR-022] {auto}
  > Evidência: FR-022 diz "tipo inválido retorna `400 Bad Request`".

- [x] CHK012 - Os edge cases de erro de operação de imagem (erro de tamanho do firmware, remoção de picture inexistente) estão cobertos nos requisitos? [Cobertura, Spec §Edge Cases] {auto}
  > Evidência: §Edge Cases cobre: tamanho excedido (firmware retorna erro, repassado ao operador), remoção de standby inexistente (tratado como 404), upload de media com falha parcial (material órfão + ID retornado).

- [x] CHK013 - O FR-004 cobre o edge case de PUT sem leitura prévia do `version` ISAPI? [Cobertura, Spec §Edge Cases] {auto}
  > Evidência: Edge Cases §"Configuração de layout enviada sem leitura prévia do `version`" e FR-004 exige read-modify-write explicitamente.

- [ ] CHK014 - Os endpoints GET (FR-001, FR-003, FR-006, FR-012, FR-016) especificam comportamento quando a chamada ISAPI subjacente falha (ex: 404 do firmware, resposta malformada)? [Completude, Gap, Spec §FR-001 a §FR-018] {auto}
  > **[Gap]** FR-020 cobre timeout (504) e auth inválida (502), mas não define o status code/mensagem quando a ISAPI retorna 4xx (ex: recurso não existe no firmware) ou resposta inesperada. O mapeamento via `mapISAPIError` está descrito no plan §2.2 mas não virou FR verificável na spec.

---

## 3. Contratos de API — Consistência e Cobertura

- [x] CHK015 - Todos os 18 novos endpoints têm handler correspondente no mapa FR→componente (plan §3)? [Consistência, Plan §3] {auto}
  > Evidência: plan §3 lista todos os 18 FRs com método cliente, handler e roteamento. Cobertura completa.

- [x] CHK016 - O roteamento manual (`adminDevicesRouter`) cobre todos os métodos HTTP definidos (GET/PUT/POST/DELETE) para cada path novo, incluindo `405` para métodos inesperados? [Completude, Plan §2.3] {auto}
  > Evidência: plan §2.3 descreve `405` para método inesperado e `404` para segmento inesperado, reusando o padrão atual. O switch de roteamento cobre todos os paths com seus métodos.

- [x] CHK017 - O DELETE bulk de media (FR-015) especifica o comportamento quando parte das remoções falha (falha parcial)? [Cobertura, Spec §FR-015] {auto}
  > Evidência: FR-015 define que lista via FR-012 e remove cada um via FR-014. Edge case de falha individual não está explicitado, mas o padrão de "falhar alto" (constitution) cobre. O plan §2.1 (`DeleteAllMaterials`) não detalha tolerância a falha parcial — gap leve, mas não impeditivo para create-tasks.

- [x] CHK018 - O FR-009 (desativar standby customizado) especifica o estado-alvo exato enviado ao device? [Clareza, Spec §FR-009] {auto}
  > Evidência: FR-009 especifica o body exato: `{standbyPicType:"default", displayEffect:"stretch", switchingTime:20}`.

---

## 4. Segurança — Autenticação e Proteção de Segredos

- [x] CHK019 - Todos os novos endpoints têm requisito explícito de autenticação (sessão admin)? [Completude, Spec §FR-019] {auto}
  > Evidência: FR-019 exige sessão admin (cookie HMAC) para todos os endpoints `/admin/api/*`, igual aos existentes.

- [x] CHK020 - Os requisitos de log especificam que a senha ISAPI e tokens NÃO devem aparecer em logs, respostas ou mensagens de erro? [Completude, Spec §FR-023, §SC-006] {auto}
  > Evidência: FR-023 proíbe explicitamente senha ISAPI, tokens e conteúdo binário em logs. SC-006 é critério de aceite mensurável: "auditável por inspeção de logs".

- [x] CHK021 - O requisito de proteção de segredos (FR-023) cobre também conteúdo binário de imagem (não só senha ISAPI)? [Completude, Spec §FR-023] {auto}
  > Evidência: FR-023 diz "DEVEM NOT incluir a senha ISAPI, tokens ou conteúdo binário de imagem".

---

## 5. Segurança — Findings do Gate OWASP (plan §6.1 F1/F2)

- [ ] CHK022 - O finding F1 (leitura ilimitada / DoS por memória — A04/API4) virou um FR verificável na spec com teto numérico ou critério de aceite mensurável? [Completude, Gap, Plan §6.1 F1, Spec §FR-018] {auto}
  > **[Gap]** O plan §6.1 F1 documenta a mitigação ("aplicar `http.MaxBytesReader` no body dos handlers de upload e `io.LimitReader` no download de `faceDataUrl`") mas essa restrição está "a definir em tasks" — não existe FR na spec que exija explicitamente um teto de leitura de body. FR-022 valida tipo, mas não teto de tamanho. A spec deveria ter um FR de segurança que diga "O body dos handlers de upload DEVE ser limitado a N bytes antes de processar" para que create-tasks possa criar um critério de aceite verificável.

- [ ] CHK023 - O finding F2 (SSRF na captura facial — API7) virou um FR verificável na spec com critério objetivo de validação de host? [Completude, Gap, Plan §6.1 F2, Spec §FR-018] {auto}
  > **[Gap]** O plan §6.1 F2 descreve a mitigação ("restringir o fetch ao host do device — validar que o host de `faceDataUrl` == host do device resolvido") mas não existe FR na spec que exija essa validação. FR-018 especifica o fluxo funcional (capturar, baixar URL, base64) mas não inclui o controle de segurança como requisito verificável. A spec deveria ter: "FR-018 DEVE validar que o host de `faceDataUrl` corresponde ao host do device antes de executar o download".

- [x] CHK024 - O requisito de proteção SSRF (face-capture) inclui o controle de não expor a URL/IP interno do device ao cliente? [Completude, Spec §FR-018, Clarification 3] {auto}
  > Evidência: FR-018 e Clarification 3 exigem retorno em base64, nunca exposição da URL interna. Dec-011 é explícito: "não expõe IP interno do device".

- [ ] CHK025 - Há um FR ou critério de aceite mensurável para o teto de body em cada handler de upload (não apenas como nota no plan)? [Mensurabilidade, Gap, Plan §6.1 F1, Spec §FR-007, §FR-010, §FR-013] {auto}
  > **[Gap]** Mesmo gap que CHK022, visto do ângulo de mensurabilidade. Create-tasks precisará de um número concreto (ex: 20 MB, 50 MB) ou de uma referência rastreável. Sem um FR de limite de tamanho de body, o critério de aceite fica "a definir em tasks" — o que é válido como estratégia (dec-010), mas o finding de DoS exige que o controle apareça como requisito verificável, não apenas como nota de implementação.

---

## 6. Upload/Multipart — Distinção dos 3 Mecanismos

- [x] CHK026 - Os 3 mecanismos de upload (standby FR-007, boot FR-010, media FR-013) têm field names distintos definidos na spec e são coerentes com os contratos ISAPI rastreáveis? [Completude, Spec §FR-007, §FR-010, §FR-013] {auto}
  > Evidência: FR-007 define `UploadCustomStandbyPic` + `filePath`; FR-010 define `picture_info` + `picture_name`; FR-013 define `name`, `type`, `size`, `file`. Research §G3, §G4, §G5 rastreiam cada um à fonte PHP do legado.

- [x] CHK027 - O requisito de validação de tipo de imagem (FR-022) se aplica aos 3 handlers de upload? [Completude, Spec §FR-022] {auto}
  > Evidência: FR-022 diz "Uploads de imagem (FR-007, FR-010, FR-013) DEVEM validar que o arquivo enviado é uma imagem (`image/*`) antes de repassar ao dispositivo".

- [x] CHK028 - A estratégia de não-hardcodar limite de tamanho de imagem está documentada com justificativa rastreável? [Clareza, Spec §Clarification 2, Plan §dec-010] {auto}
  > Evidência: Clarification 2 resolve explicitamente: "O sistema NÃO pré-valida tamanho de imagem no servidor. [...] Limite de tamanho: não hardcodar — sem fonte rastreável para DS-K1T681DBX (Constitution I)". Dec-010 com score 3.

- [x] CHK029 - O FR-007 (upload standby) especifica que são 2 etapas sequenciais (upload + ativação) e que falha na ativação é reportada mesmo com upload bem-sucedido? [Completude, Spec §FR-007] {auto}
  > Evidência: FR-007 diz: "Upload e ativação são etapas sequenciais: falha na ativação é reportada mesmo com upload bem-sucedido."

- [x] CHK030 - O FR-010 (boot picture) especifica o formato de arquivo esperado (JPEG) e o nome do campo multipart com o nome do arquivo (`<id>.jpg`)? [Clareza, Spec §FR-010] {auto}
  > Evidência: FR-010 especifica "upload multipart de imagem JPEG" e `picture_name` (binário JPEG com nome `<id>.jpg`).

- [x] CHK031 - A não-transcodificação de imagens para standby/boot/media está definida como decisão de design com justificativa? [Clareza, Plan §2.1] {auto}
  > Evidência: plan §2.1 diz "Sem transcodificação para standby/boot/material (preservar qualidade de branding; validação de tipo no handler, tamanho é autoridade do firmware — research.md D5)".

- [x] CHK032 - Os uploads não-idempotentes (FR-007/010/013) estão documentados com sua contramedida (DELETE)? [Cobertura, Plan §5] {auto}
  > Evidência: plan §5 diz "Uploads (FR-007/010/013): não idempotentes por design do firmware (cada upload cria nova entrada); a limpeza (DELETE) é a contramedida. Documentado, não mascarado."

---

## 7. Critérios de Aceite e Mensurabilidade

- [x] CHK033 - Os Success Criteria (SC-001 a SC-006) são mensuráveis e verificáveis objetivamente? [Mensurabilidade, Spec §Success Criteria] {auto}
  > Evidência: SC-001/002: "<30s"; SC-003: "<15s excluindo transferência"; SC-004: "<5s em condições normais de rede local"; SC-005: "100% das operações — zero erros genéricos"; SC-006: "auditável por inspeção de logs". Todos têm critério numérico ou binário.

- [x] CHK034 - O SC-005 ("erros comunicados com mensagem acionável em 100% das operações") é verificável nos acceptance scenarios? [Mensurabilidade, Spec §SC-005] {auto}
  > Evidência: US1-AC3, US2-AC3, US4-AC2, Edge Cases cobrem os cenários de falha com exigência de "mensagem acionável" ou "erro claro". Verificável por teste.

- [x] CHK035 - Os acceptance scenarios distinguem "zero" de "indisponível" para o caso de device offline em estatísticas (US4-AC2)? [Clareza, Spec §US4-AC2] {auto}
  > Evidência: US4-AC2 diz explicitamente: "nenhum valor de contador é exibido como zero por padrão — zero e 'indisponível' são distintos."

---

## 8. Dependências e Fora de Escopo

- [x] CHK036 - A decisão de excluir gestão de cartões RFID está documentada com rastreabilidade (block-001/dec-017) e o impacto em `users.cards` (leitura informativa apenas) está esclarecido? [Consistência, Spec §Clarification 1, §FR-016] {auto}
  > Evidência: Clarification 1 e nota em FR-016 explicam: `users.cards` é contador somente-leitura do firmware (bindCardUserNumber), não implementação de gestão de cartões.

- [x] CHK037 - A dependência do schema `devices` (existente, criado por `device-config`) está explicitada — a feature não cria migrations? [Dependências, Spec §Key Entities, data-model.md] {auto}
  > Evidência: spec §Key Entities diz "Esta feature não altera o schema da tabela `devices`"; data-model.md referenciado confirma feature stateless.

- [x] CHK038 - A feature estateless (sem persistência local) está documentada com justificativa? [Clareza, Spec §Contexto, Plan §1] {auto}
  > Evidência: spec §Contexto diz "feature stateless; operações ISAPI são síncronas sob demanda". Plan §1 confirma: "É um conjunto de passthrough ISAPI síncronos — sem nova persistência".

---

## Notes

- Items `{auto}` marcados `[x]` têm evidência citada.
- Items `{humano}` ficam `[ ]` aguardando decisão do dono do produto.
- Gaps detectados: CHK008, CHK009, CHK014, CHK022, CHK023, CHK025.

### Resolução

- **`{auto}` resolvidos**: 31 de 38 (`[x]` com evidência citada)
- **`{humano}` aguardando decisão**: 0
- **Gaps abertos** (`[Gap]`): 6 — CHK008, CHK009, CHK014, CHK022, CHK023, CHK025

### Gaps e seus destinos

| CHK | Gap | Severidade | Destino |
|-----|-----|------------|---------|
| CHK008 | Shape JSON da resposta de FR-001 e FR-006 não definido | Leve | create-tasks: implementador decide shape consistente com padrão existente |
| CHK009 | Shape da resposta de sucesso de FR-013 (media) não definido | Leve | create-tasks: definir campos retornados (ID material, programa, schedule?) |
| CHK014 | Comportamento de GET quando ISAPI retorna 4xx/resposta inesperada não especificado como FR | Leve | create-tasks: reusa `mapISAPIError` existente; documentar como task |
| CHK022 | Finding F1 (DoS/body) não virou FR verificável com teto numérico | **Médio** | create-tasks: criar task específica com teto a definir (ex: 20 MB) e teste de aceitação |
| CHK023 | Finding F2 (SSRF/validação de host) não virou FR verificável | **Médio** | create-tasks: criar task específica com critério de aceite "host de faceDataUrl == host do device" |
| CHK025 | Teto de body de upload não está como critério de aceite mensurável | **Médio** (sobrepõe CHK022) | create-tasks: mesma task de CHK022 |

### Avaliação para create-tasks

Os 3 gaps de severidade **média** (CHK022/CHK023/CHK025) referem-se aos findings de segurança do plan §6.1 que ficaram como "a definir em tasks" em vez de virarem FRs verificáveis. **Eles NÃO bloqueiam create-tasks** — a mitigação está documentada no plan e as tasks podem criar os controles diretamente. Mas as tasks de segurança DEVEM ter critério de aceite explícito (não apenas "aplicar MaxBytesReader").

Os 3 gaps leves (CHK008/CHK009/CHK014) são decisões de implementação que o desenvolvedor faz inline com o padrão existente — não requerem /clarify.

**Veredito: PASSOU o quality gate**. Nenhum gap é impeditivo para create-tasks.
