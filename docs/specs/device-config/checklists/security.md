# Security Checklist: device-config

**Purpose**: Validar qualidade dos requisitos de segurança — autenticação, proteção de segredos, CSRF, input validation, logging de auditoria.
**Created**: 2026-06-21
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md)
**Domínio**: Security (OWASP API Top 10 / ASVS)

> Items `{auto}` resolvidos pelo agente com citação de evidência.
> Items `{humano}` aguardam decisão do dono do produto.
> `[Gap]` = requisito ausente; `[Ambiguity]` = requisito ambíguo.

---

## Proteção de Credenciais ISAPI (Dado Sensível Crítico)

- [x] CHK059 - Está especificado que a senha ISAPI NUNCA aparece em resposta JSON, logs ou mensagens de erro? [Completude, Spec §FR-005, §SC-003] {auto}
  > SOURCED: FR-005 como MUST NOT explícito. SC-003: "A senha ISAPI não aparece em nenhum log, resposta JSON ou mensagem de erro em 100% das operações". Plan §Constitution Check V: `DeviceConfig.Password` "sensitive — never log" (client.go:151). Requisito completo e mensuravelmente auditável.

- [x] CHK060 - O requisito de cifragem da senha (AES-GCM com chave de 32 bytes) está especificado com nível de detalhe suficiente? [Completude, Spec §FR-004, Plan §Constitution Check V] {auto}
  > SOURCED: FR-004 menciona "cifrar a senha com AES-GCM usando a chave `ISAPI_CRED_KEY`". Plan §Security Considerations §A04 Crypto: "AES-256-GCM (nonce‖ciphertext), `ParseKey` exige 32 bytes (aesgcm.go)". Nível de detalhe adequado — referencia implementação existente.

- [x] CHK061 - Está especificado que o endpoint de credenciais retorna 503 quando `ISAPI_CRED_KEY` está ausente (nunca persiste senha em claro)? [Completude, Spec §FR-007] {auto}
  > SOURCED: FR-007: "Se `ISAPI_CRED_KEY` não estiver configurada, o endpoint de credenciais DEVE retornar erro `503` com mensagem orientativa — nunca persistir senha em claro". Requisito explícito com dois comportamentos definidos (503 + bloqueio de persistência em claro).

- [x] CHK062 - O campo de resposta `isapi_credentials_set` (bool derivado) em vez da senha está especificado como única forma de indicar presença de credenciais? [Completude, Spec §FR-003, §US2-AS3] {auto}
  > SOURCED: FR-003 e US2-AS3 especificam que apenas "configurada" ou "não configurada" é indicado — nunca o valor. admin-api.md §GET devices/{id}: `"isapi_credentials_set": true` sem `isapi_password`. Consistente.

- [ ] CHK063 - Está especificado como a senha ISAPI é protegida em memória durante o processamento do request PUT credentials (ex: não logada pelo framework de request)? [Completude, Spec §FR-005, Plan §Security] {humano}
  > FR-005 e Plan §Security Considerations cobrem JSON/log de output. Mas o request body (contendo `isapi_password`) pode ser logado pelo middleware HTTP (ex: debug middleware). A spec não especifica proteção in-memory do campo de request. Decisão de profundidade de segurança para o produto — o risco é baixo em ambiente single-org local, mas relevante se logs de request forem habilitados.

---

## Autenticação de Sessão Admin

- [x] CHK064 - Todos os novos endpoints herdam o mecanismo de sessão admin existente sem exceção? [Completude, Spec §FR-006/020, Plan §Security §BOLA/IDOR] {auto}
  > SOURCED: FR-006 (credentials) e FR-020 (todos os demais) são requisitos explícitos. Plan §Structure: "estende `adminDevicesRouter` (sob `sessionMW` já vigente) — zero mudança no esquema de auth". Cobertura por design arquitetural, não por configuração ad-hoc.

- [x] CHK065 - A proteção CSRF via `SameSite: Strict` + `HttpOnly` + `Secure` está especificada como controle vigente para as novas ações destrutivas? [Completude, Plan §Security §CSRF] {auto}
  > SOURCED: Plan §Security Considerations §CWE-352 CSRF: "Cookie `admin_session` com `SameSite: http.SameSiteStrictMode` + `HttpOnly` + `Secure` (admin_ui_handlers.go:86-88). Requisições cross-site não carregam o cookie → POST/PUT/DELETE destrutivos protegidos por construção. Os novos endpoints herdam o mesmo cookie/middleware". Controle SOURCED e herdado por construção.

- [x] CHK066 - Está documentado que ações destrutivas são restritas a métodos não-GET (protegendo o mecanismo SameSite)? [Completude, Plan §Security §CSRF, Plan §Recomendações] {auto}
  > SOURCED: Plan §Recomendações: "Manter os handlers destrutivos restritos a métodos não-GET (já é o padrão dos handlers atuais, ex: resync exige POST, admin_resync_handler.go:39) para que o SameSite=Strict seja efetivo". Requisito de design documentado como recomendação de implementação.

- [ ] CHK067 - Está definido o comportamento quando a sessão admin expira durante uma operação longa (ex: factory reset aguardando resposta do dispositivo)? [Cobertura, Spec §FR-020] {humano}
  > FR-020 exige sessão válida para iniciar a operação. Mas não especifica o comportamento se a sessão expirar enquanto a operação ISAPI (com timeout de 30s) está em andamento. O token expira mid-flight? A resposta ainda é retornada? Decisão de produto sobre granularidade da sessão.

---

## Input Validation e Injeção

- [x] CHK068 - Está especificada a validação do parâmetro `{id}` do dispositivo antes de qualquer operação ISAPI? [Completude, Spec §FR-023] {auto}
  > SOURCED: FR-023: "O backend DEVE validar que `{id}` corresponde a um dispositivo existente antes de tentar qualquer conexão ISAPI; dispositivo inexistente retorna `404`". Validação de path param como gate obrigatório.

- [x] CHK069 - Está especificada a validação de `isapi_port` (1-65535) no endpoint de credenciais? [Completude, Contracts §admin-api.md §PUT credentials] {auto}
  > SOURCED: admin-api.md §PUT credentials: "`400` se `isapi_port` fora de 1–65535". Validação de range documentada no contrato.

- [x] CHK070 - Está especificado que o `command` no endpoint de controle de porta aceita apenas valores do enum `{open, close, always_open, always_closed, normal}`? [Completude, Spec §FR-014, Contracts §admin-api.md §POST door control] {auto}
  > SOURCED: FR-014 especifica o conjunto `∈ {open, close, always_open, always_closed, normal}`. admin-api.md documenta os mesmos valores. Enum fechado definido — a implementação deve rejeitar valores fora do conjunto.

- [ ] CHK071 - Está definido o comportamento para `time_mode` com valor fora do enum `{manual, ntp}` no endpoint `PUT /time`? [Completude, Contracts §admin-api.md §PUT time] {auto}
  > admin-api.md §PUT time: `"time_mode": "manual"|"ntp"` — enum implícito mas o comportamento para valores inválidos (ex: `"time_mode": "nfs"`) não está especificado como erro. [Gap] — falta requisito de validação de `time_mode` inválido (400 esperado, não especificado).

- [ ] CHK072 - Existem requisitos de validação do campo `searchID` gerado para paginação de usuários (ex: deve ser UUID, não user-controlled)? [Completude, Spec §FR-016, Contracts §hikvision-isapi.md §List users] {auto}
  > hikvision-isapi.md §List users: body inclui `"searchID": "<uid>"`. A spec não define quem gera o `searchID` nem se é user-controlled. [Gap] — se o `searchID` vier do client (query param) e for repassado para o ISAPI, há risco de injection no corpo ISAPI. O requisito deveria especificar que o backend gera o `searchID` internamente (não aceita do client). Verificar em execute-task.

- [ ] CHK073 - O parâmetro `per_page` do endpoint de listagem de usuários tem validação de range (ex: máximo de 100 ou 1000 por página)? [Completude, Contracts §admin-api.md §GET users] {auto}
  > admin-api.md §GET users: `?page=1&per_page=100` como exemplo, mas não define o máximo aceito. ISAPI pode ter limitação própria de `maxResults`. [Gap] — sem validação máxima de `per_page` especificada; o backend poderia repassar um `per_page=99999` ao ISAPI, que pode rejeitar ou retornar erro inesperado.

---

## Proteção de Dados Sensíveis em Logs

- [x] CHK074 - Está especificado que o log de ações destrutivas NÃO inclui o corpo de request de credenciais? [Completude, Plan §Security Considerations §Recomendações] {auto}
  > SOURCED: Plan §Recomendações: "Log estruturado de FR-011 NÃO deve incluir corpo de request de credenciais". Requisito de exclusão documentado.

- [x] CHK075 - Está definido que mensagens de erro de autenticação ISAPI (502) não incluem detalhes internos do dispositivo? [Completude, Spec §FR-022] {auto}
  > SOURCED: FR-022: "sem expor a senha ou detalhes internos". Requisito explícito.

- [ ] CHK076 - Existe requisito que proteja o `isapi_username` de aparecer em logs de erro de autenticação (apenas a senha está explicitamente protegida)? [Completude, Spec §FR-005/022] {humano}
  > FR-005 especifica "senha ISAPI MUST NOT ser retornada em nenhuma resposta JSON, log estruturado". O `isapi_username` não está protegido explicitamente — pode aparecer em logs de erro. Em ambientes com múltiplos dispositivos, o username pode ser sensível. Decisão do produto sobre extensão da proteção.

---

## Operações de Alto Risco

- [x] CHK077 - Os requisitos de log para factory reset incluem identificação do operador E registro como ação irreversível? [Completude, Spec §FR-011, §US3-AS3] {auto}
  > SOURCED: FR-011: log com `device_id`, `stage`, ação, operador. US3-AS3: "o sistema registra a ação como irreversível nos logs". admin-api.md §factory-reset: "Log estruturado obrigatório (FR-011); ação registrada como irreversível". Cobertura dupla (spec + contrato).

- [x] CHK078 - Está especificada a necessidade de confirmação forte (digitação de identificador) para factory reset e clear de usuários? [Completude, Spec §US3-AS3, §US5-AS2/3, §SC-004] {auto}
  > SOURCED: US3-AS3: "digita o identificador para confirmar". US5-AS2: "digita confirmação". SC-004: "zero falsos acionamentos em testes de usabilidade". Confirmação forte especificada e verificável.

- [ ] CHK079 - Está especificado um mecanismo de rate-limiting ou cool-down para ações destrutivas (reboot, factory-reset, door control)? [NF-Security, Plan §Security Considerations §API4] {humano}
  > SOURCED: Plan §Security Considerations §API4 Resource consumption: "Recomendação: rate-limit nas ações destrutivas/de porta (não há hoje). Risco baixo dado SameSite + sessão admin local; registrar como melhoria futura (fora do escopo MVP)". O plan reconhece a ausência como decisão consciente de escopo MVP. Confirmação do produto se é aceitável para V1.

- [ ] CHK080 - Existe requisito para impedir que a mesma ação destrutiva seja submetida em paralelo (ex: dois reboot simultâneos do mesmo dispositivo)? [Completude, Spec §US3/US5] {humano}
  > A spec não define comportamento para submissão paralela de ações destrutivas. Em ambiente multi-aba ou multi-operador (even single-org), dois reboot simultâneos poderiam ser problemáticos. Decisão do produto sobre se serialização é necessária para V1.

---

## Princípios de Constituição Aplicados

- [x] CHK081 - O Princípio V (segredos como config de runtime) está coberto como requisito verificável? [Completude, Plan §Constitution Check V] {auto}
  > SOURCED: Plan §Constitution Check V: "`ISAPI_CRED_KEY` via env (SOURCED — config.go:149-156). Senha cifrada AES-GCM; `BYTEA` no banco; NUNCA em JSON/log/erro (FR-005); `503` se key ausente (FR-007)". Cada aspecto do princípio tem FR correspondente.

- [x] CHK082 - O Princípio VI (observabilidade operacional) está coberto com campos de log definidos para ações destrutivas? [Completude, Plan §Constitution Check VI, Spec §FR-011] {auto}
  > SOURCED: Plan §Constitution Check VI: "FR-011 exige log estruturado (`device_id`, `stage`, ação, operador)". Erros ISAPI mapeados com contexto (504/502/503). Princípio satisfeito com requisitos rastreáveis.

---

## Resumo de Resolução

| Marcador | Count |
|----------|-------|
| `[x]` auto-resolvidos | 17 |
| `[ ]` humano (aguardando decisão) | 5 (CHK063, CHK067, CHK076, CHK079, CHK080) |
| `[Gap]` (requisito ausente) | 3 (CHK071, CHK072, CHK073) |
| `[Ambiguity]` | 0 |

### Gaps — destino obrigatório

- **CHK071 [Gap]** → tarefa "validar `time_mode` no handler PUT time (400 para enum inválido)" em create-tasks.
- **CHK072 [Gap]** → tarefa "gerar `searchID` no backend (não aceitar do client) no handler GET users" — risco de ISAPI injection.
- **CHK073 [Gap]** → tarefa "validar máximo de `per_page` no handler GET users (ex: cap 1000)" em create-tasks.

### Itens humano — decisão de produto

- **CHK063** → proteção do request body em memória (baixo risco MVP).
- **CHK067** → comportamento de sessão expirada mid-request.
- **CHK076** → proteção do `isapi_username` em logs.
- **CHK079** → rate-limiting em ações destrutivas (reconhecido no plan como fora de MVP).
- **CHK080** → serialização de ações destrutivas paralelas.
