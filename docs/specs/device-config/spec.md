# Feature Specification: Configuração Completa do Dispositivo HikVision

**Feature**: `device-config`
**Created**: 2026-06-21
**Status**: Draft

## Contexto

O painel admin já possui a tela "Configuração do dispositivo" (`/admin/#device-config?id=N`)
com 9 seções construídas em estado "aguardando backend". Esta feature entrega os
endpoints de backend Go e a camada de integração ISAPI que ativam cada seção.

Contratos ISAPI de referência verificados em `internal/hikvision/client.go` e
`legacy/hik-api/app/Service/HikVision/` (DeviceService.php, DoorService.php,
NotificationService.php). Nenhum contrato foi inventado.

> **Decisões de infraestrutura**: N/A — feature stateless do ponto de vista do
> scheduler; operações ISAPI são síncronas sob demanda (o administrador aciona).
> Credenciais ISAPI já são cifradas AES-GCM na tabela `devices` (migration 006).

---

## User Scenarios & Testing

### User Story 1 — Visualizar status e identidade do dispositivo (Priority: P1)

O operador abre a tela de configuração de um terminal HikVision e vê imediatamente
informações atualizadas: modelo, número de série, versão de firmware, IP, MAC,
status online/offline, capacidades (máximo de usuários e faces suportados) e
contadores de uso. Sem precisar acessar a interface web nativa do dispositivo.

**Why this priority**: É a seção de abertura da tela (`overview`). Sem dados reais
de hardware a tela fica vazia com placeholders `—`. Todas as demais seções dependem
saber com qual dispositivo estão interagindo. Dados de identidade (serial/model/
firmware) já são persitidos no banco via `FetchDeviceInfo`; esta story só expõe
capacidades e status ao vivo.

**Independent Test**: Com o dispositivo acessível em rede, abrir a tela de
configuração e confirmar que modelo, serial, firmware, capacidades (max users,
max faces) e status online são exibidos corretamente, sem usar a interface web do
terminal.

**Acceptance Scenarios**:

1. **Given** dispositivo com credenciais ISAPI configuradas e acessível na rede,
   **When** o operador abre a tela de configuração,
   **Then** o painel exibe modelo, serial, firmware, IP corrente, MAC, capacidades
   de hardware e status de conectividade derivado do último heartbeat — todos
   provenientes do banco ou da ISAPI, nunca inventados.

2. **Given** dispositivo fora da rede (offline),
   **When** o operador abre a tela de configuração,
   **Then** os dados estáticos (serial, modelo, firmware, IP último registrado)
   são exibidos a partir do banco; campos de status ao vivo (uptime, contadores)
   são marcados como indisponíveis — sem erro fatal na tela.

3. **Given** dispositivo online mas sem credenciais ISAPI configuradas,
   **When** o operador abre a tela,
   **Then** o painel exibe aviso claro de que as credenciais ISAPI ainda não foram
   configuradas e as seções que dependem de ISAPI ficam desabilitadas com mensagem
   orientativa.

---

### User Story 2 — Configurar credenciais ISAPI do dispositivo (Priority: P1)

O operador informa o usuário, senha e porta ISAPI de um dispositivo via painel admin,
sem precisar editar variáveis de ambiente nem reiniciar o serviço. O sistema persiste
essas credenciais de forma segura (cifradas) e passa a usá-las para todas as operações
ISAPI subsequentes naquele terminal.

**Why this priority**: Todas as demais seções de configuração (sistema, portas,
usuários, webhook) dependem de credenciais ISAPI válidas. Sem esta story, as seções
P2–P5 não têm credenciais para operar. Credenciais já são suportadas no banco
(`isapi_username`, `isapi_password_enc`, `isapi_port`); a story entrega o endpoint
de escrita e o formulário correspondente.

**Independent Test**: Via painel, informar credenciais de um dispositivo de teste;
verificar que operações ISAPI subsequentes (ex: buscar informações do hardware)
funcionam com as novas credenciais; confirmar que a senha nunca aparece em logs nem
na resposta JSON da API.

**Acceptance Scenarios**:

1. **Given** dispositivo registrado sem credenciais ISAPI,
   **When** o operador submete usuário, senha e porta pelo painel,
   **Then** o sistema persiste as credenciais cifradas, confirma sucesso e o
   dispositivo passa a ser acessível para operações ISAPI.

2. **Given** credenciais ISAPI já configuradas,
   **When** o operador submete novas credenciais,
   **Then** o sistema substitui as credenciais anteriores; a senha antiga é
   descartada; todas as chamadas ISAPI futuras usam as novas credenciais.

3. **Given** quaisquer credenciais persistidas,
   **When** o painel exibe os dados do dispositivo,
   **Then** a senha ISAPI nunca aparece — nem na resposta JSON, nem em logs,
   nem na UI (apenas indica "configurada" ou "não configurada").

---

### User Story 3 — Controlar data/hora e manutenção do dispositivo (Priority: P2)

O operador pode sincronizar o relógio do terminal (NTP ou manual), iniciar uma
reinicialização (reboot) controlada ou executar um reset de fábrica — tudo pelo
painel admin, com confirmação obrigatória para ações destrutivas.

**Why this priority**: Data/hora incorreta quebra silenciosamente a validade dos
usuários (campo `endTime` no ISAPI) e corrompe timestamps de eventos. Reboot e
factory reset são menos frequentes mas necessários para manutenção. Seção
`system` da tela já está desenhada.

**Independent Test**: Com dispositivo online, usar o painel para reboot e confirmar
que o terminal reinicia (fica offline por ~40s) e volta. Para data/hora, ajustar
para um valor incorreto via painel e confirmar que o dispositivo reflete a mudança.

**Acceptance Scenarios**:

1. **Given** dispositivo online com relógio desincronizado,
   **When** o operador aciona "Sincronizar relógio" com servidor NTP informado,
   **Then** o dispositivo sincroniza com o NTP configurado e confirma sucesso.

2. **Given** operador aciona "Reiniciar dispositivo" com confirmação no modal,
   **When** confirmação é submetida,
   **Then** o dispositivo inicia reboot; o painel reflete status offline durante
   ~40s; a ação é registrada nos logs com identificação do operador.

3. **Given** operador aciona "Reset de fábrica" e digita o identificador para confirmar,
   **When** confirmação forte é submetida,
   **Then** o dispositivo executa factory reset; o sistema registra a ação como
   irreversível nos logs; o campo `webhook_configured` do banco é marcado como falso.

4. **Given** operador tenta ação destrutiva (reboot/factory reset),
   **When** cancela o modal de confirmação,
   **Then** nenhuma ação é executada no dispositivo.

---

### User Story 4 — Controlar portas remotamente (Priority: P2)

O operador pode visualizar o estado das portas do terminal, acionar abertura remota
temporária (ex: destravar por 5 segundos), manter travada ou retornar ao modo normal
— pelo painel admin, com confirmação para ações sensíveis.

**Why this priority**: Controle remoto de porta é operação frequente em manutenção
e situações de emergência (visitante no portão, bloqueio de acesso). Seção `doors`
já desenhada na UI.

**Independent Test**: Com dispositivo online, acionar "Destravar 5s" pelo painel;
confirmar fisicamente (ou por log do dispositivo) que a porta destravou e travou
novamente após o período.

**Acceptance Scenarios**:

1. **Given** dispositivo online com porta no modo normal,
   **When** o operador aciona "Destravar 5s" com confirmação no modal,
   **Then** a porta é destravada temporariamente; o painel confirma a ação com
   feedback visual; a porta retorna ao modo normal após 5 segundos.

2. **Given** dispositivo online,
   **When** o operador consulta o status das portas,
   **Then** o painel exibe estado atual (aberta/fechada/normal/sempre-aberta/
   sempre-fechada) de cada porta do terminal, proveniente da ISAPI.

3. **Given** operador aciona comando de porta,
   **When** o dispositivo está offline,
   **Then** o painel exibe erro claro de conectividade; nenhuma ação silenciosa
   é executada.

---

### User Story 5 — Gerenciar usuários e faces no dispositivo (Priority: P3)

O operador pode listar os usuários cadastrados diretamente no terminal, verificar
quantas faces existem na biblioteca, limpar todos os usuários ou todas as faces
com confirmação forte — pelo painel admin.

**Why this priority**: Ferramenta de diagnóstico e manutenção para casos onde o
banco de dados e o dispositivo ficaram dessincronizados (ex: após factory reset).
Seções `users` e `faces` já desenhadas. O provisionamento normal é via worker/fila —
esta story cobre apenas gestão administrativa direta.

**Independent Test**: Com alguns usuários cadastrados via worker, abrir a seção
"Usuários no terminal" e confirmar que a lista bate com os usuários reais do dispositivo.
Depois usar "Limpar todos" e confirmar que o terminal fica sem usuários.

**Acceptance Scenarios**:

1. **Given** dispositivo com usuários cadastrados,
   **When** o operador abre a seção "Usuários no terminal",
   **Then** o painel lista os usuários presentes no dispositivo (identificador/
   nome), provenientes da ISAPI, com contagem total.

2. **Given** operador aciona "Limpar todos os usuários" e digita confirmação,
   **When** confirmação forte é submetida,
   **Then** todos os usuários (e suas faces associadas) são removidos do terminal;
   a lista fica vazia; o painel confirma com feedback.

3. **Given** dispositivo com faces cadastradas na biblioteca,
   **When** o operador aciona "Limpar biblioteca de faces" com confirmação forte,
   **Then** todas as faces são removidas; os usuários permanecem no dispositivo
   sem capacidade de reconhecimento facial até reenvio.

---

### User Story 6 — Gerenciar webhooks e notificações do dispositivo (Priority: P3)

O operador pode visualizar os destinos de notificação (webhooks) configurados no
terminal, adicionar, editar ou remover destinos — pelo painel admin. O webhook
principal do sistema já é configurado pelo worker; esta story permite gerenciar
destinos adicionais ou verificar o estado atual da configuração.

**Why this priority**: Visibilidade da configuração de webhook é necessária para
diagnóstico ("por que os eventos não estão chegando?"). Seção `events` já desenhada.

**Independent Test**: Após o worker configurar o webhook principal, abrir a seção
"Eventos & Webhooks" e confirmar que o destino configurado aparece na lista. Adicionar
um destino secundário via painel e confirmar que o dispositivo o registra.

**Acceptance Scenarios**:

1. **Given** dispositivo com webhook principal configurado pelo worker,
   **When** o operador abre a seção "Eventos & Webhooks",
   **Then** o painel lista os destinos de notificação ativos no terminal,
   provenientes da ISAPI.

2. **Given** operador remove um destino de webhook pelo painel,
   **When** confirmação é dada,
   **Then** o destino é removido do terminal; o campo `webhook_configured` no
   banco é atualizado se o webhook principal do sistema for removido.

---

### Edge Cases

- O que acontece quando a ISAPI do dispositivo responde com erro de autenticação
  (credenciais erradas)? → Retornar erro claro ao operador sem expor a senha;
  não fazer retry automático.
- O que acontece quando uma operação destrutiva (reboot, factory reset) é acionada
  e o dispositivo fica inacessível antes de responder? → Timeout com erro explicativo;
  a ação pode ou não ter sido executada — avisar ao operador para verificar fisicamente.
- O que acontece quando o dispositivo não tem a ISAPI_CRED_KEY configurada (chave
  mestra ausente)? → As seções que dependem de ISAPI ficam desabilitadas com mensagem
  orientativa; a seção de credenciais ainda deve funcionar para configurar usuário/
  senha (que serão cifrados assim que a chave estiver disponível).
- O que acontece quando a operação "Limpar todos os usuários" falha no meio (timeout)?
  → O dispositivo pode ter ficado em estado parcial; o painel exibe erro e orienta
  o operador a verificar manualmente ou tentar novamente.
- Operador tenta configurar credenciais para um dispositivo desconhecido (ID inválido)?
  → 404 com mensagem clara; nenhuma escrita no banco.

---

## Requirements

### Functional Requirements

**Grupo 1: Status e Identidade (seção overview)**

- **FR-001**: O sistema DEVE expor via API admin os dados de identidade do dispositivo
  (serial, modelo, firmware, IP, MAC, último heartbeat, status) a partir do banco de
  dados, sem requisitar a ISAPI a cada carregamento de tela.
- **FR-002**: O sistema DEVE expor as capacidades de hardware do dispositivo (máximo
  de usuários, máximo de faces) obtidas via ISAPI e armazenadas em cache no banco;
  campos de capacidade indisponíveis são retornados como nulos, nunca estimados.
- **FR-003**: O sistema DEVE indicar na resposta da API se as credenciais ISAPI estão
  configuradas para o dispositivo (`isapi_credentials_set: true|false`), sem retornar
  a senha.

**Grupo 2: Credenciais ISAPI (seção credentials — nova)**

- **FR-004**: O sistema DEVE aceitar via endpoint `PUT /admin/api/devices/{id}/credentials`
  um conjunto de credenciais ISAPI (usuário, senha, porta), cifrar a senha com AES-GCM
  usando a chave `ISAPI_CRED_KEY` e persisti-las na tabela `devices`.
- **FR-005**: A senha ISAPI MUST NOT ser retornada em nenhuma resposta JSON, log
  estruturado, ou corpo de resposta de erro — em nenhuma circunstância (Constitution V).
- **FR-006**: O endpoint de credenciais DEVE exigir autenticação de sessão admin
  (mesmo mecanismo dos demais endpoints `/admin/api/*`).
- **FR-007**: Se `ISAPI_CRED_KEY` não estiver configurada, o endpoint de credenciais
  DEVE retornar erro `503` com mensagem orientativa — nunca persistir senha em claro.

**Grupo 3: Sistema e Manutenção (seção system)**

- **FR-008**: O sistema DEVE expor endpoint `POST /admin/api/devices/{id}/actions/reboot`
  que envia o comando de reboot ao dispositivo via ISAPI (`PUT /ISAPI/System/reboot`)
  e retorna confirmação ou erro.
- **FR-009**: O sistema DEVE expor endpoint `POST /admin/api/devices/{id}/actions/factory-reset`
  que executa factory reset via ISAPI (`PUT /ISAPI/System/factoryReset`) e, após
  execução bem-sucedida, marca `webhook_configured = false` no banco para o dispositivo.
- **FR-010**: O sistema DEVE expor endpoint `GET /admin/api/devices/{id}/time` e
  `PUT /admin/api/devices/{id}/time` para leitura e ajuste de data/hora via ISAPI
  (`GET|PUT /ISAPI/System/time`).
- **FR-011**: Todas as ações destrutivas (reboot, factory reset) DEVEM ser registradas
  em log estruturado com `device_id`, `stage`, ação executada e identificação do
  operador (via sessão), conforme Constitution VI.

**Grupo 4: Controle de Portas (seção doors)**

- **FR-012**: O sistema DEVE expor endpoint `GET /admin/api/devices/{id}/doors` que
  retorna a lista de portas e capacidades via ISAPI (`GET /ISAPI/AccessControl/Door/capabilities`).
- **FR-013**: O sistema DEVE expor endpoint `GET /admin/api/devices/{id}/doors/{door_id}/status`
  que retorna o estado atual da porta via ISAPI (`POST /ISAPI/AccessControl/Door/Status`).
- **FR-014**: O sistema DEVE expor endpoint `POST /admin/api/devices/{id}/doors/{door_id}/control`
  com campo `command` ∈ `{open, close, always_open, always_closed, normal}` que envia
  o comando à porta via ISAPI (`PUT /ISAPI/AccessControl/RemoteControl/door/{N}`).
- **FR-015**: Respostas de controle de porta DEVEM incluir o resultado da operação;
  erros de conectividade com o dispositivo DEVEM ser distinguidos de erros de lógica
  do dispositivo (ex: porta travada por alarme).

**Grupo 5: Usuários no Dispositivo (seção users)**

- **FR-016**: O sistema DEVE expor endpoint `DELETE /admin/api/devices/{id}/users`
  (bulk delete) que limpa todos os usuários do dispositivo via ISAPI e retorna
  confirmação ou erro detalhado.
- **FR-017**: O sistema DEVE expor endpoint `DELETE /admin/api/devices/{id}/faces`
  (bulk delete) que limpa a biblioteca de faces do dispositivo via ISAPI e retorna
  confirmação ou erro detalhado.
  [NEEDS CLARIFICATION: O endpoint de listagem de usuários do dispositivo via ISAPI
  `GET /ISAPI/AccessControl/UserInfo/Search` existe no legacy? Verificar se
  `UserService.php` expõe uma chamada de listagem paginada — se sim, implementar
  `GET /admin/api/devices/{id}/users`; se não, a seção fica somente com as ações bulk.]

**Grupo 6: Webhooks e Notificações (seção events)**

- **FR-018**: O sistema DEVE expor endpoint `GET /admin/api/devices/{id}/webhooks`
  que lista os destinos de notificação do dispositivo via ISAPI (`GET /ISAPI/Event/notification/httpHosts`).
- **FR-019**: O sistema DEVE expor endpoint `DELETE /admin/api/devices/{id}/webhooks/{webhook_id}`
  que remove um destino de notificação via ISAPI; se o webhook removido for o webhook
  principal do sistema, atualizar `webhook_configured = false` no banco.

**Grupo 7: Validação e Segurança**

- **FR-020**: Todos os novos endpoints de configuração DEVEM exigir sessão admin válida
  (cookie HMAC, igual aos endpoints `/admin/api/*` existentes).
- **FR-021**: Operações ISAPI que falham por timeout (dispositivo offline) DEVEM
  retornar `504 Gateway Timeout` com mensagem explicativa; o operador não deve
  receber um erro genérico de servidor.
- **FR-022**: Operações ISAPI que falham por credenciais inválidas DEVEM retornar
  `502 Bad Gateway` com mensagem indicando problema de autenticação com o dispositivo,
  sem expor a senha ou detalhes internos.
- **FR-023**: O backend DEVE validar que `{id}` corresponde a um dispositivo existente
  antes de tentar qualquer conexão ISAPI; dispositivo inexistente retorna `404`.

### Key Entities

- **Device** (já existente): terminal HikVision identificado por `device_identifier`
  (MAC). Campos relevantes desta feature: `id`, `isapi_username`, `isapi_password_enc`,
  `isapi_port`, `serial_number`, `model`, `firmware_version`, `webhook_configured`.
  A senha ISAPI nunca é retornada pela API; o campo `isapi_credentials_set` (bool
  derivado) indica presença.
- **Door** (do dispositivo, não persistida): porta física do terminal; identificada
  por `door_id` (inteiro 1-based, conforme ISAPI). Não é persistida no banco —
  proveniente da ISAPI em tempo real.
- **WebhookDestination** (do dispositivo, não persistida): destino de notificação
  configurado no terminal via ISAPI; identificado por `id` (hash do host, já
  usado em `ConfigureWebhook`). Não é persistida no banco.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: O operador visualiza dados completos de identidade e status do dispositivo
  (modelo, serial, firmware, capacidades) sem acessar a interface web nativa do terminal.
- **SC-002**: Credenciais ISAPI são configuradas pelo operador em menos de 30 segundos
  pelo painel admin, sem edição de arquivos de configuração ou reinicialização do serviço.
- **SC-003**: A senha ISAPI não aparece em nenhum log, resposta JSON ou mensagem de
  erro em 100% das operações — auditável por inspeção de logs do sistema.
- **SC-004**: Ações destrutivas (reboot, factory reset, limpar usuários, limpar faces)
  exigem confirmação explícita do operador; nenhuma ação destrutiva é executada por
  acidente (zero falsos acionamentos em testes de usabilidade).
- **SC-005**: Operações de controle de porta (destravar 5s) são executadas e confirmadas
  em menos de 5 segundos em condições de rede local normal.
- **SC-006**: Erros de conectividade com o dispositivo (offline, credenciais inválidas,
  timeout) são comunicados ao operador com mensagem acionável em 100% dos casos —
  zero erros genéricos de servidor sem contexto.

---

## Clarifications

### [NEEDS CLARIFICATION: FR-016]
O endpoint de listagem de usuários no dispositivo via ISAPI existe no legacy
(`UserService.php`)? Se a operação `GET /ISAPI/AccessControl/UserInfo/Search`
foi verificada como funcional no firmware DS-K1T671, implementar listagem paginada
em `GET /admin/api/devices/{id}/users`. Caso contrário, a seção "Usuários no terminal"
fica somente com as ações bulk (limpar todos).

---

## Fora de Escopo

- Gestão de cartões RFID/NFC (seção `cards`) — o terminal suporta mas o fluxo
  principal é por face; a UI já tem placeholder; não está no escopo desta feature.
- Mídia de boot/standby (seção `media`) — upload de imagem ao dispositivo; requer
  endpoint ISAPI específico não verificado no legacy; não está no escopo desta feature.
- Stream de eventos em tempo real (seção `events` — "Stream ao vivo") — SSE/WebSocket
  via `alertStream`; complexidade de infraestrutura separada; não está no escopo.
- Modo de autenticação (seção `auth`) — configuração de face/cartão/PIN; requer
  endpoints ISAPI de preferências não verificados no código de referência;
  não está no escopo desta feature.
- Provisionamento de membros (UpsertUser, UploadFace) — já implementado via worker/fila;
  esta feature não altera o fluxo de enrollment.
