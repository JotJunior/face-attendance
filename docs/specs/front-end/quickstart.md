# Quickstart: Interface de Administração Web (`front-end`)

Cenários de teste que validam a implementação end-to-end. Cobrem happy path,
error cases e o roundtrip backend↔frontend obrigatório.

> **Pré-requisitos de deploy (obrigatórios):**
>
> - **TLS**: o cookie `admin_session` tem flag `Secure`; HTTPS é obrigatório mesmo
>   em LAN on-premise. Configurar TLS no proxy reverso (nginx, Caddy) ou no binário
>   diretamente. Sem HTTPS, o browser descarta o cookie e o login não funciona.
>
> - **Env vars obrigatórias** (sem default — serviço recusa inicializar se ausentes):
>   `ADMIN_USERNAME`, `ADMIN_PASSWORD`, `ADMIN_SESSION_SECRET`, além das já
>   existentes (`ADMIN_TOKEN`, `WEBHOOK_PATH_SECRET`, `DATABASE_URL`, `RABBITMQ_URL`,
>   `GOB_STATE_URL`, `GOB_STATE_TOKEN`).
>
> - **Env vars opcionais** com default:
>   | Variável | Default | Descrição |
>   |----------|---------|-----------|
>   | `ADMIN_SESSION_TTL_HOURS` | `8` | TTL da sessão de admin em horas |
>   | `DEVICE_OFFLINE_THRESHOLD_HOURS` | `24` | Horas sem heartbeat para considerar device offline |
>
> - **Rotação de secret**: rotacionar `ADMIN_SESSION_SECRET` invalida **todas** as
>   sessões ativas imediatamente (HMAC stateless — sem estado em banco). Usuários
>   autenticados perdem a sessão e precisam fazer login novamente.
>
> - **Assets embutidos**: binário compilado com `embed.FS` (FASE 3 gera os assets
>   em `internal/web/dist/` antes do `go build`).

---

## Scenario 1: Login e acesso ao dashboard (happy path — US1 + US2)

1. Acessar `GET /admin/` sem cookie de sessão.
2. **Expected**: redireciona para a tela de login (não exibe conteúdo protegido —
   SC-003).
3. Submeter `POST /admin/api/login` com `username`/`password` corretos.
4. **Expected**: resposta 204 com `Set-Cookie: admin_session=...; HttpOnly;
   Secure; SameSite=Strict`.
5. Acessar o dashboard; o frontend chama `GET /admin/api/stats`.
6. **Expected**: 200 com `members_with_selfie`, `devices_active`,
   `devices_inactive`, `attendance_last_24h`, `device_offline_threshold_hours`;
   métricas renderizadas (SC-001 — < 5s em rede local).

## Scenario 2: Credenciais inválidas (error case — US1)

1. Submeter `POST /admin/api/login` com senha incorreta.
2. **Expected**: 401 `{"error":"credenciais inválidas"}`; nenhum cookie emitido;
   UI permanece na tela de login com mensagem de erro (PT-BR).

## Scenario 3: Roundtrip End-to-End (OBRIGATÓRIO — borda backend↔frontend)

Valida que o payload **real** do backend casa com o contrato declarado em
`contracts/admin-api.md` e com o que o frontend consome. **NÃO usar mock/fixture
— chamar o backend de verdade.**

1. Subir o backend localmente (binário com assets embutidos + DB/RabbitMQ).
2. Autenticar e capturar o cookie:
   ```sh
   curl -s -c /tmp/cj.txt -X POST http://localhost:<port>/admin/api/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"'"$ADMIN_USERNAME"'","password":"'"$ADMIN_PASSWORD"'"}' -i
   ```
3. Chamar o endpoint crítico com o cookie e capturar o payload **real**:
   ```sh
   curl -s -b /tmp/cj.txt http://localhost:<port>/admin/api/stats | tee /tmp/stats.json
   ```
4. Comparar o shape contra `contracts/admin-api.md`:
   - **Case style**: todos os campos em **snake_case** (`members_with_selfie`,
     `device_offline_threshold_hours`) — bate com os domain structs Go reais.
   - **Tipos**: contadores são `number` (sem coerção para string); o limiar é
     `number`.
   - Repetir para `GET /admin/api/devices`: confirmar `device_identifier`,
     `last_heartbeat_at`, `is_active`, `webhook_configured` (snake_case real do
     `domain.Device`).
5. O frontend consome esse mesmo payload e renderiza sem erro de parse.
6. **Expected**: zero divergência entre payload real, contrato declarado e o que
   o frontend lê. Como o backend é Go com `json:"snake_case"` em uma única camada
   de domínio (sem mapper camelCase), o risco de drift snake_case/camelCase é
   baixo — este roundtrip o confirma empiricamente na primeira execução.

> **Por que obrigatório**: execuções históricas mascararam drift de case porque
> testes parseavam mocks, não o payload real. Aqui a fonte da verdade é única
> (domain structs snake_case servidos diretamente), mas o roundtrip valida que os
> DTOs de view **[NOVO]** (com CPF mascarado) mantêm a mesma convenção.

## Scenario 4: Sessão expirada durante navegação (error case — FR-012)

1. Autenticar; aguardar expirar a sessão (ou forçar TTL curto via
   `ADMIN_SESSION_TTL_HOURS`).
2. Navegar para uma tela protegida; o frontend chama `GET /admin/api/members`.
3. **Expected**: 401 `{"error":"sessão expirada"}`; o frontend redireciona para
   login **preservando a URL atual** (ex: `?redirect=/admin/members`); após
   re-login, retorna à tela original.

## Scenario 5: Busca e paginação de membros (US4, FR-008, dec-008)

1. Com membros no banco, chamar
   `GET /admin/api/members?q=<termo>&limit=<n>` (autenticado).
2. **Expected**: 200 com `members[]` filtrados server-side por nome/CPF; cada
   item traz `federal_document_masked` (**CPF nunca cru** — SC-006); `next_cursor`
   preenchido se houver mais.
3. Chamar de novo com `cursor=<next_cursor>`.
4. **Expected**: próxima página, sem duplicar itens; `has_more=false` e
   `next_cursor=null` na última página.

## Scenario 6: Estado vazio (FR-009)

1. Com banco sem dispositivos, chamar `GET /admin/api/devices` (autenticado).
2. **Expected**: 200 `{"devices":[], "device_offline_threshold_hours":<n>}`; a UI
   exibe "Nenhum dispositivo registrado ainda" (mensagem amigável), **não** uma
   tabela vazia nem zeros que pareçam falha.

## Scenario 7: Dispositivo offline (US2, US3, dec-012/015)

1. Ter um device com `last_heartbeat_at` mais antigo que
   `DEVICE_OFFLINE_THRESHOLD_HOURS`.
2. Carregar dashboard e tela de dispositivos.
3. **Expected**: o device conta em `devices_inactive` no `/admin/api/stats` e
   aparece com `status:"offline"` + indicador visual de alerta na lista
   (SC-002 — identificável em < 30s).

## Scenario 8: Sync manual (US6, FR-007)

1. Autenticado, acionar o botão "Sincronizar membros agora"
   (`POST /admin/api/sync`).
2. **Expected**: feedback de loading imediato (SC-004 — < 2s); 202
   `{"status":"accepted"}`.
3. Acionar novamente durante o ciclo.
4. **Expected**: 409 `{"error":"sincronização já em andamento"}`; o botão fica
   desabilitado/avisa (mesma semântica do `AdminSyncHandler` existente).

## Scenario 9: CPF mascarado em todas as telas (SC-006, FR-011)

1. Inspecionar visualmente Membros e Logs, e os payloads `GET /admin/api/members`
   e `GET /admin/api/events`.
2. **Expected**: CPF aparece apenas mascarado (`federal_document_masked`); o CPF
   completo **não** trafega ao browser em nenhuma resposta (mascarado no backend).
