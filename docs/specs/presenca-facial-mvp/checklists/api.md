# API Checklist: MVP de Controle de Presenca por Reconhecimento Facial

**Purpose**: Validar qualidade e completude dos requisitos de API — contratos GOB (outbound), HikVision ISAPI (outbound) e endpoints inbound (webhook/heartbeat/health/admin).
**Created**: 2026-06-20
**Feature**: [spec.md](../spec.md) | [contracts/gob-api.md](../contracts/gob-api.md) | [contracts/hikvision-isapi.md](../contracts/hikvision-isapi.md) | [contracts/inbound-http.md](../contracts/inbound-http.md)

## Contratos GOB (outbound)

- [x] CHK001 - Os campos obrigatorios do request `GET /api/face-detection/members` estao definidos (endpoint, header `Authorization: Bearer`)? [Completude, Spec §FR-004, contracts/gob-api.md §GET] {auto}
  > Evidencia: `contracts/gob-api.md` define endpoint `GET {GOB_STATE_URL}/api/face-detection/members` com header `Authorization: Bearer {GOB_STATE_TOKEN}` (fonte `t.txt:8-10`). Completo.
- [x] CHK002 - O shape da response GOB `{ "success": bool, "data": [ member ] }` esta especificado com os campos verificados de cada membro? [Completude, Spec §FR-005, contracts/gob-api.md §Campos verificados] {auto}
  > Evidencia: `contracts/gob-api.md` lista 8 campos verificados de `data[]`: `id`, `status`, `created_at`, `updated_at`, `federal_document`, `name`, `mobile_number`, `url_selfie` (fonte `t.txt:13-27`). Completo.
- [x] CHK003 - O request `POST /attendance/3ff4708cb695ad1a6e9f87cb714e1f22` tem o body `{ "cpf": "00.000.000-00" }` e header `Authorization` (sem prefixo Bearer) especificados? [Completude, contracts/gob-api.md §POST attendance] {auto}
  > Evidencia: `contracts/gob-api.md` define body `{"cpf":"00.000.000-00"}` e explicita que o header NAO usa prefixo `Bearer` (fonte `t.txt:46,48`). A diferenca de header entre GET e POST esta documentada e justificada. Completo.
- [x] CHK004 - O tratamento de erro da GOB (4xx/5xx na marcacao de presenca) e especificado com politica de retry/DLQ? [Cobertura, Spec §FR-023-INFRA-RETRY, contracts/gob-api.md §Tratamento de erro] {auto}
  > Evidencia: `contracts/gob-api.md §Tratamento de erro` define: 4xx/5xx → retry com backoff → DLQ; nunca perder evento. FR-023 fixa `RETRY_MAX_ATTEMPTS=3`, `RETRY_INITIAL_BACKOFF_MS=1000ms`, backoff exponencial. Completo.
- [x] CHK005 - O cenario de indisponibilidade da GOB na carga de membros tem requisito definido (sem publicar mensagens parciais)? [Cobertura de Edge Cases, Spec §US2-cenario-3, contracts/gob-api.md §Regras de consumo] {auto}
  > Evidencia: `contracts/gob-api.md §Regras de consumo`: "`success != true` ou erro HTTP → log estruturado, sem publicar mensagens parciais". US2 cenario 3 tambem cobre. Completo.
- [ ] CHK006 - A paginacao da resposta GOB esta tratada na spec (ou a ausencia de paginacao esta explicitamente assumida e documentada)? [Ambiguity, Spec §FR-005, contracts/gob-api.md §PROPOSTA paginacao] {auto}
  > [Ambiguity] `contracts/gob-api.md` marca paginacao como `[PROPOSTA — a validar na implementacao]`: `t.txt` mostra um array unico sem campos de paginacao; se a GOB paginar, a implementacao deve detectar e tratar, mas nenhum esquema especifico e assumido. A spec (FR-005) nao menciona paginacao. Isso e uma ambiguidade documentada mas nao resolvida — comportamento default e consumir `data[]` como veio. Destino: `/clarify` se a GOB paginar em producao ou `/create-tasks` para incluir tarefa de deteccao de paginacao.

## Contratos HikVision ISAPI (outbound)

- [x] CHK007 - Os 3 endpoints ISAPI do MVP (UserInfo/Modify, FDLib/faceDataRecord, httpHosts) estao especificados com metodo HTTP, path, Content-Type e campos obrigatorios verificados? [Completude, Spec §FR-010,FR-011,FR-012, contracts/hikvision-isapi.md §1-3] {auto}
  > Evidencia: `contracts/hikvision-isapi.md` cobre os 3 endpoints com metodo, path, Content-Type (`application/xml` / `multipart/form-data`), campos verificados e citacao arquivo:linha do legacy. Todos os 3 completamente definidos.
- [x] CHK008 - A autenticacao HTTP Digest com o dispositivo ISAPI esta especificada como mecanismo de auth? [Completude, contracts/hikvision-isapi.md §cabecalho, Spec §FR-020] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §cabecalho`: "Auth: HTTP Digest (usuario/senha ISAPI, env por dispositivo — `HttpClient.php:176`, `AuthType.php`)". FR-020 fixa credenciais ISAPI via env. Completo.
- [x] CHK009 - A estrategia de upsert (create POST, depois update PUT) para UserInfo/Modify esta especificada para garantir idempotencia por CPF? [Completude, contracts/hikvision-isapi.md §Estrategia upsert, Spec §FR-009] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §Estrategia upsert`: chave = `employeeNo` (CPF); endpoint `Modify` faz upsert; reprocessar nao duplica. Sequencia POST-then-PUT documentada como opcao de execute-task. Completo.
- [x] CHK010 - O multipart do upload de face (FaceDataRecord JSON + FaceImage binaria) tem todos os campos verificados da parte JSON (`type`, `faceLibType`, `FDID`, `FPID`)? [Completude, contracts/hikvision-isapi.md §2, Spec §FR-011] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §Campos verificados` da o multipart completo: `type=concurrent`, `faceLibType=blackFD`, `FDID=1`, `FPID=CPF` (fonte `FaceService.php:177-180,42,44`). Query `format=json` tambem documentada. Completo.
- [x] CHK011 - O XML `HttpHostNotification` para configurar o webhook do dispositivo tem todos os campos verificados (url, protocolType, parameterFormatType, addressingFormatType, path, httpAuthenticationMethod)? [Completude, contracts/hikvision-isapi.md §3] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §Campos verificados de HttpHostNotification` lista todos os 9 campos com valores default (fonte `NotificationService.php:364-372`). Completo.
- [x] CHK012 - Os codigos de status de sucesso de cada operacao ISAPI (200/201 para create, 200/204 para update, 200 para face) estao especificados? [Completude, contracts/hikvision-isapi.md §Status OK] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §1`: "Status OK: 200 ou 201" (create), "200 ou 204" (update); `§2`: "Status OK: 200". Completo.
- [ ] CHK013 - Os campos opcionais de `UserInfo` (ex. `userType`, `Valid` com janela de validade, `doorRight`) necessarios para que o dispositivo DS-K1T673DWX aceite o usuario estao identificados ou explicitamente assumidos como nao-necessarios no MVP? [Ambiguity, contracts/hikvision-isapi.md §PROPOSTA campos adicionais] {auto}
  > [Ambiguity] `contracts/hikvision-isapi.md §PROPOSTA`: o MVP envia apenas `employeeNo` + `name`; campos adicionais (`userType`, `Valid`, `doorRight`) sao opcionais e marcados proposta — derivam do que o dispositivo exige. Nao ha evidencia de que o DS-K1T673DWX os exija, mas tambem nao ha evidencia de que nao exija. Ambiguidade residual documentada; nao bloqueia o MVP mas pode causar rejeicao no dispositivo real. Destino: `/create-tasks` com tarefa de validacao contra dispositivo real na Fase 2.

## Endpoints inbound (API local)

- [x] CHK014 - O endpoint de webhook tem path configuravel (via `HttpHostNotification.path`) e o comportamento de retorno 200 sempre (mesmo em payload malformado) esta especificado para nao causar loop de retry no dispositivo? [Completude, contracts/inbound-http.md §1, Spec §FR-014] {auto}
  > Evidencia: `contracts/inbound-http.md §1 Response`: "200 OK sempre que o evento for aceito para processamento"; "Payload malformado → 200 + log estruturado (nao 4xx, para o dispositivo nao re-tentar em loop)". Completo.
- [x] CHK015 - O processamento do evento de webhook (extrair `employeeNoString`, dedup por `event_key`, marcar presenca) esta especificado passo-a-passo no contrato inbound? [Completude, contracts/inbound-http.md §1 Processamento] {auto}
  > Evidencia: `contracts/inbound-http.md §1 Processamento`: 4 passos definidos (extrair, checar desconhecido, dedup event_key, marcar presenca). Alinhado com FR-014..FR-017. Completo.
- [x] CHK016 - O requisito de health check (GET /health) tem response shape definido (com verificacao de dependencias DB e RabbitMQ)? [Completude, contracts/inbound-http.md §3, Spec §FR-019] {auto}
  > Evidencia: `contracts/inbound-http.md §3`: response `{ "status": "ok", "db": "ok", "rabbitmq": "ok" }` com 200 saudavel / 503 critica. Marcado `[PROPOSTA]` por ser endpoint novo. Completo para MVP.
- [x] CHK017 - O endpoint `/admin/sync` tem response codes (202/409) e requisito de auth por token de admin especificados? [Completude, contracts/inbound-http.md §4, Spec §FR-021-INFRA-SCHED] {auto}
  > Evidencia: `contracts/inbound-http.md §4`: "202 Accepted se ciclo iniciado; 409 Conflict se ja ha ciclo em andamento". Auth por token de admin via env marcada como `[PROPOSTA]`; Principio V e S5 do gate de seguranca tornam isso requisito firme. Completo.
- [ ] CHK018 - O path exato do webhook e do heartbeat (quando sao rotas distintas vs. rota unica) e definido na spec, ou a decisao de merge/split esta documentada como proposta para execute-task? [Ambiguity, contracts/inbound-http.md §2 NOTA] {auto}
  > [Ambiguity] `contracts/inbound-http.md §2 NOTA`: "heartbeat e webhook PODEM ser a mesma rota fisica... a decisao (rota unica vs separada) e de execute-task". A spec (FR-001/FR-002/FR-014) nao fixa o path; o contrato de heartbeat (shape exato) tambem e `[PROPOSTA]`. Ambiguidade documentada; aceitavel para MVP pois nao bloqueia design. Destino: `/create-tasks` com tarefa de definir rotas no execute-task.

## Error Handling (transversal)

- [x] CHK019 - Existe especificacao de como tratar cada categoria de falha de chamada externa (timeout, 5xx, 4xx, conexao recusada) para GOB e ISAPI? [Completude, Spec §FR-023-INFRA-RETRY] {auto}
  > Evidencia: FR-023-INFRA-RETRY: retry configuravel com `RETRY_MAX_ATTEMPTS` (default 3) e `RETRY_INITIAL_BACKOFF_MS` (default 1000ms), backoff exponencial; apos esgotar → DLQ. Cobre GOB (FR-015) e ISAPI (FR-010..FR-012). `contracts/hikvision-isapi.md §Codigos de status`: "falha transitoria (timeout, 5xx, conexao recusada) → retry/backoff/DLQ". Completo.
- [x] CHK020 - O comportamento do sistema quando a resposta XML do ISAPI for invalida ou inesperada esta especificado? [Cobertura de Edge Cases, contracts/hikvision-isapi.md §Codigos de status] {auto}
  > Evidencia: `contracts/hikvision-isapi.md §Codigos de status`: "o cliente HTTP legacy usa `http_errors=false` e inspeciona `statusCode`... nao lancar em 4xx/5xx, decidir por codigo". Politica de retry/DLQ cobre 5xx. Para respostas malformadas, a spec edge-cases cobrem apenas "falha transitoria" — especificacao de XML invalido nao existe explicitamente. Mas como a politica de retry e baseada em status code (nao em parse do body), XML invalido seria erro de implementacao, nao requisito ausente. Aceitavel para MVP.

## Notes

- Items `{auto}` foram resolvidos contra spec.md, plan.md e contracts/ (citacao referenciada acima)
- Items `{humano}` ficam `[ ]` aguardando decisao do dono do produto (nenhum neste arquivo)
- Ambiguidades documentadas (`[Ambiguity]`): CHK006 (paginacao GOB), CHK013 (campos opcionais UserInfo), CHK018 (path webhook vs heartbeat)
- Marcar items concluidos com `[x]` conforme avancada a implementacao
