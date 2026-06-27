# Feature Specification: Editor de Fluxo por Reconhecimento Facial

**Feature**: `face-flow`
**Created**: 2026-06-27
**Status**: Draft

> **ALERTAS DE PRINCÍPIO I (Zero Fabricação — NON-NEGOTIABLE)**
>
> Quatro tipos de nó dependem de operações ISAPI para as quais **não existe
> contrato verificado** em `t.txt`, `legacy/hik-api` ou documentação oficial
> disponível no repositório: nó 1 (ligar câmera), nó 2 (desligar câmera),
> nó 4 (trocar background), nó 6 (QR code como background). Esses nós são
> especificados em termos de **comportamento esperado** (o QUE); os
> contratos ISAPI (endpoints, payloads XML/multipart) só podem ser definidos
> após fornecimento de fonte rastreável — bloqueio humano obrigatório antes
> da implementação ISAPI de cada um deles.
>
> O nó 8 (disparo de mensagem) depende de API cujos parâmetros
> (endpoint, método, headers, schema de payload, auth) **não foram
> fornecidos pelo operador**. Especificado de forma abstrata; bloqueio
> humano obrigatório antes da implementação.
>
> Marcadores `[BLOCKED_ISAPI]` e `[BLOCKED_API]` identificam cada FR afetado.

---

## User Scenarios & Testing

### User Story 1 — Editor de fluxo visual vinculado a dispositivo (Priority: P1)

O administrador acessa o painel admin, cria um novo fluxo e o associa a um
dispositivo específico. No editor visual, adiciona nós dos tipos disponíveis
(3, 5, 6, 7, 8 — e futuramente 1, 2, 4 quando os contratos ISAPI estiverem
disponíveis), conecta-os em sequência e bifurcações, configura os parâmetros
de cada nó, e publica o fluxo. O dispositivo fica com o fluxo ativo.

**Why this priority**: sem o editor, nenhuma das outras histórias é viável.
É o ponto de entrada de toda a feature.

**Independent Test**: criar um fluxo com pelo menos 3 nós (incluindo uma
bifurcação de decisão), vinculá-lo a um dispositivo e verificar que o fluxo
persiste corretamente — sem acionar reconhecimento real.

**Acceptance Scenarios**:

1. **Given** nenhum fluxo cadastrado, **When** o admin cria um fluxo e o
   vincula ao dispositivo D, **Then** o fluxo aparece listado, vinculado a D,
   e o dispositivo D não pode ser vinculado a outro fluxo.
2. **Given** um fluxo vinculado ao dispositivo D, **When** o admin tenta
   vincular outro fluxo ao mesmo dispositivo D, **Then** o sistema rejeita com
   mensagem explicando a restrição 1:1.
3. **Given** um fluxo com nó de decisão (nó 7), **When** o admin conecta saída
   "válido" a um caminho e "inválido" a outro, **Then** a bifurcação é salva
   corretamente e visível no editor.
4. **Given** um nó de espera (nó 3) com valor configurado de 30 segundos,
   **When** o admin salva o fluxo, **Then** o valor é persistido e exibido
   corretamente no nó ao reabrir o editor.

---

### User Story 2 — Execução automática do fluxo ao reconhecimento facial (Priority: P2)

Ao receber um evento de tentativa de reconhecimento facial de um dispositivo
(único gatilho válido), o sistema localiza o fluxo vinculado ao dispositivo
que originou o evento, valida o fluxo, e executa os nós em sequência. Em nós
de espera, o sistema pausa pelo tempo configurado. Em nós de decisão, o sistema
bifurca conforme o resultado do reconhecimento (usuário válido/inválido). Em nós
de chamada HTTPS, o sistema realiza a requisição com as variáveis interpoladas.
Heartbeats e outros tipos de eventos do dispositivo são ignorados.

**Why this priority**: é o propósito central da feature — o fluxo só tem valor
se executar automaticamente a cada reconhecimento.

**Independent Test**: simular um evento `AccessControllerEvent` via requisição
direta ao webhook e verificar que o motor de execução do fluxo é acionado,
passando pelos nós na ordem correta. Verificar que um evento de heartbeat **não**
aciona o fluxo.

**Acceptance Scenarios**:

1. **Given** um dispositivo D com fluxo F vinculado e ativo, **When** o
   dispositivo envia evento de reconhecimento facial (eventType =
   `AccessControllerEvent`), **Then** o motor executa os nós do fluxo F na
   ordem configurada.
2. **Given** um dispositivo D com fluxo F contendo nó de espera de 5 segundos,
   **When** a execução chega ao nó de espera, **Then** o motor aguarda 5
   segundos antes de avançar ao próximo nó.
3. **Given** o fluxo contendo nó de decisão (nó 7) e o reconhecimento retornou
   `authorized`, **When** o motor avalia o nó de decisão, **Then** segue o
   caminho "usuário válido".
4. **Given** o fluxo contendo nó de decisão (nó 7) e o reconhecimento retornou
   qualquer status diferente de `authorized`, **When** o motor avalia o nó de
   decisão, **Then** segue o caminho "usuário inválido".
5. **Given** um evento de heartbeat chegando no webhook, **When** o motor
   processa o evento, **Then** não inicia execução de nenhum fluxo.
6. **Given** um dispositivo D **sem** fluxo vinculado, **When** o dispositivo
   envia evento de reconhecimento, **Then** o webhook conclui normalmente sem
   erro (sem acionar motor).

---

### User Story 3 — Chamada HTTPS com variáveis de contexto (Priority: P3)

Ao configurar um nó de chamada HTTPS (nó 5), o admin informa URL, cabeçalhos e
corpo (JSON). No corpo pode usar variáveis no formato `[user.name]`,
`[user.document]`, `[device.id]`, entre outras disponíveis no vocabulário. No
momento da execução, o sistema interpola as variáveis com os dados reais do
evento (membro reconhecido + dispositivo de origem) antes de realizar a
chamada. O resultado da chamada é registrado para auditoria.

**Why this priority**: é o mecanismo de integração com sistemas externos sem
contratos ISAPI; viabiliza automações antes de ter os nós 1/2/4 disponíveis.

**Independent Test**: configurar nó 5 com URL de teste e corpo contendo
`[user.document]` e `[device.id]`, acionar execução simulada, verificar que a
requisição HTTP saiu com os valores corretos interpolados.

**Acceptance Scenarios**:

1. **Given** nó 5 com body `{"cpf": "[user.document]", "device": "[device.id]"}`,
   **When** o motor executa o nó durante reconhecimento de membro com CPF
   `12345678901` no dispositivo com ID `7`, **Then** a requisição é enviada com
   body `{"cpf": "12345678901", "device": "7"}`.
2. **Given** nó 5 com URL inválida ou serviço externo fora do ar, **When** o
   motor tenta executar o nó, **Then** o circuit-break é ativado e o fluxo é
   resetado ao estado inicial (sem travar a aplicação).
3. **Given** nó 5 com cabeçalho `Authorization: Bearer <token>` configurado,
   **When** o motor executa o nó, **Then** o cabeçalho é enviado integralmente
   na requisição.

---

### User Story 4 — Resiliência e circuit-break do motor de execução (Priority: P3)

Qualquer erro durante a execução de um nó (timeout de HTTPS, erro de
interpolação de variável inválida, nó corrompido, etc.) deve acionar o
circuit-break: o fluxo retorna ao estado inicial (primeiro nó) sem propagar
a falha para o restante do sistema. O erro é registrado em log estruturado.
O fluxo continua disponível para a próxima tentativa de reconhecimento.

**Why this priority**: sem circuit-break, um erro em um nó pode travar a
execução e bloquear reconhecimentos subsequentes do dispositivo.

**Independent Test**: injetar falha proposital em um nó de chamada HTTPS (URL
não-responsiva com timeout curto) e verificar que: (1) o fluxo reseta ao
início, (2) o erro é logado, (3) o próximo evento de reconhecimento aciona
o fluxo do zero normalmente.

**Acceptance Scenarios**:

1. **Given** execução em progresso no nó N (N > 1), **When** o nó N falha com
   qualquer erro, **Then** o motor reseta o estado de execução para o início do
   fluxo, registra o erro em log, e retorna HTTP 200 ao dispositivo
   (comportamento padrão do webhook).
2. **Given** um fluxo com definição corrompida (ex.: nó referencia outro nó
   inexistente), **When** o motor inicia a execução, **Then** detecta a
   invalidade antes de executar qualquer nó e aborta sem side-effects.
3. **Given** falha no circuit-break, **When** chega o próximo evento de
   reconhecimento no mesmo dispositivo, **Then** o motor executa o fluxo do
   início normalmente, sem estado residual da execução anterior.

---

### Edge Cases

- Dispositivo envia dois eventos de reconhecimento quase simultaneamente: o
  motor MUST processar cada execução de forma independente; não há compartilhamento
  de estado entre execuções concorrentes do mesmo fluxo.
- Fluxo é desativado (ou dispositivo desvinculado) enquanto uma execução está em
  progresso: a execução em andamento termina normalmente (ou sofre circuit-break);
  a desvinculação só afeta execuções subsequentes.
- Variável referenciada no corpo de um nó não está disponível no contexto do
  evento (ex.: membro não encontrado no banco → `[user.name]` indefinida): o
  motor MUST substituir por string vazia (não deve falhar a execução pelo fato
  de variável ausente, a menos que o nó exija validação explícita).
- Ciclo de nós detectado na validação do fluxo (nó A → nó B → nó A): o sistema
  MUST rejeitar o fluxo na publicação com mensagem de erro, não durante a execução.
- Fluxo sem nenhum nó ou sem nó de início definido: o sistema MUST rejeitar a
  publicação.
- Nó de decisão (nó 7) sem os dois ramos conectados: o sistema MUST rejeitar
  a publicação.

---

## Requirements

### Functional Requirements

#### Editor Visual (Painel Admin)

- **FR-001**: O painel admin MUST incluir tela de gerenciamento de fluxos com
  lista de fluxos existentes, status (ativo/inativo), e dispositivo vinculado.
- **FR-002**: A tela de edição MUST apresentar canvas interativo onde o
  administrador adiciona, remove, conecta e configura nós visualmente.
- **FR-003**: O editor MUST suportar os 8 tipos de nó descritos nesta spec
  (`camera_on`, `camera_off`, `wait`, `change_background`, `https_call`,
  `qrcode_background`, `decision`, `send_message`). Nós com dependência de
  contrato ISAPI ou API externas [BLOCKED_ISAPI / BLOCKED_API] devem ser
  renderizados no editor mas sinalizados como "aguardando contrato" e não
  executáveis até que o contrato seja definido.
- **FR-004**: A vinculação dispositivo↔fluxo MUST ser 1:1: um dispositivo pode
  ter no máximo um fluxo ativo; um fluxo pode ser vinculado a no máximo um
  dispositivo por vez. Tentativa de dupla vinculação MUST ser rejeitada com
  mensagem explicativa.
- **FR-005**: O editor MUST validar o fluxo antes de publicar: ausência de nó de
  início, ciclos, nó de decisão com ramos incompletos ou nó referenciando nó
  inexistente MUST impedir a publicação com mensagem de erro.
- **FR-006**: O admin MUST poder desativar um fluxo sem excluí-lo; fluxo inativo
  não é executado pelo motor.

#### Tipos de Nó

- **FR-010**: **Nó 1 — Ligar câmera** [BLOCKED_ISAPI]: o sistema MUST enviar
  comando ao dispositivo para habilitar a câmera. Comportamento esperado:
  câmera do dispositivo entra em estado operacional. Contrato ISAPI (endpoint,
  payload, resposta) deve ser fornecido pelo operador antes da implementação.
- **FR-011**: **Nó 2 — Desligar câmera** [BLOCKED_ISAPI]: o sistema MUST enviar
  comando ao dispositivo para desabilitar a câmera. Comportamento esperado:
  câmera do dispositivo entra em estado inativo. Contrato ISAPI deve ser
  fornecido pelo operador antes da implementação.
- **FR-012**: **Nó 3 — Aguardar N segundos**: o motor MUST pausar a execução do
  fluxo pelo número de segundos configurado no nó (inteiro positivo, mínimo 1,
  máximo 3600). O valor é configurável por instância de nó no editor.
- **FR-013**: **Nó 4 — Trocar background** [BLOCKED_ISAPI]: o sistema MUST enviar
  ao dispositivo a imagem selecionada para ser exibida como fundo da tela.
  O editor MUST apresentar lista de imagens disponíveis para seleção. Contrato
  ISAPI para envio de background (endpoint, formato de imagem aceito, payload)
  deve ser fornecido pelo operador antes da implementação.
- **FR-014**: **Nó 5 — Chamada HTTPS**: o motor MUST realizar requisição HTTPS
  com URL, cabeçalhos e corpo configurados no nó. O corpo suporta template de
  variáveis no formato `[<variavel>]` (ver vocabulário em FR-020). Headers são
  pares chave-valor livres. Qualquer código HTTP de resposta é aceito (a
  intenção é "disparar" — não aguardar resultado de negócio).
- **FR-015**: **Nó 6 — QR Code como background** [BLOCKED_ISAPI]: o sistema MUST
  gerar imagem PNG com QR code cujo conteúdo é o template de variáveis
  configurado no nó (mesmo formato do corpo do nó 5), enviar a imagem gerada
  ao dispositivo e torná-la a imagem de fundo. Contrato ISAPI para envio de
  imagem de background deve ser fornecido pelo operador antes da implementação.
- **FR-016**: **Nó 7 — Decisão**: o motor MUST bifurcar a execução com base no
  resultado do reconhecimento: se `attendanceStatus == "authorized"` (fonte:
  `domain.AttendanceEvent.IsAuthorized()`), seguir o ramo "usuário válido";
  caso contrário, seguir o ramo "usuário inválido". O nó possui exatamente
  duas saídas; ambas MUST ser conectadas para publicação ser permitida.
- **FR-017**: **Nó 8 — Disparo de mensagem** [BLOCKED_API]: o motor MUST enviar
  mensagem cujo texto é o template de variáveis configurado no nó (mesmo formato
  das anteriores). A API de envio (endpoint, método, headers, schema de payload,
  autenticação) será fornecida pelo operador — bloqueio humano obrigatório
  antes da implementação. O editor MUST apresentar textarea para o template da
  mensagem e área de configuração da API (a ser preenchida quando o contrato
  estiver disponível).

#### Motor de Execução

- **FR-018**: O motor MUST ser acionado **exclusivamente** por eventos
  `AccessControllerEvent` recebidos no webhook (campo `eventType` do payload
  HikVision). Todos os outros tipos de evento (heartbeat e demais) MUST ser
  ignorados pelo motor.
- **FR-019**: Ao receber evento elegível, o motor MUST identificar o dispositivo
  pela chave `MACAddress` do payload e localizar o fluxo ativo vinculado. Se
  não houver fluxo ativo vinculado, o motor não executa nada (passthrough
  silencioso).
- **FR-020**: O motor MUST suportar interpolação de variáveis nos templates dos
  nós 5, 6 e 8. O vocabulário de variáveis disponíveis no contexto de execução
  é derivado dos dados reais do evento, conforme tabela abaixo.

  | Variável | Fonte (campo no código) | Disponibilidade |
  |----------|------------------------|-----------------|
  | `[user.name]` | `domain.Member.Name` | Quando membro encontrado por CPF |
  | `[user.document]` | `domain.Member.FederalDocument` (CPF, 11 dígitos) | Quando membro encontrado por CPF |
  | `[user.status]` | `domain.Member.Status` (ex.: `"REGULAR"`) | Quando membro encontrado por CPF |
  | `[user.mobile]` | `domain.Member.MobileNumber` (nullable) | Quando membro encontrado e campo preenchido |
  | `[device.id]` | `domain.Device.ID` (ID interno do banco) | Sempre |
  | `[device.identifier]` | `domain.Device.DeviceIdentifier` (MAC address) | Sempre |
  | `[device.ip]` | `domain.Device.IPAddress` (nullable) | Quando disponível |
  | `[device.mac]` | `domain.Device.MACAddress` (nullable) | Quando disponível |
  | `[event.authorized]` | `"true"` se `attendanceStatus == "authorized"`, senão `"false"` | Sempre |
  | `[event.datetime]` | `domain.AttendanceEvent.EventDatetime` (ISO 8601) | Quando presente no payload |

  Variável ausente no contexto (ex.: membro não localizado) MUST ser
  substituída por string vazia. Variável com sintaxe inválida é preservada
  literalmente no template (sem falhar o nó).

- **FR-021**: O motor MUST implementar circuit-break: qualquer erro em qualquer
  nó durante a execução (incluindo timeout de HTTPS, variável inválida,
  falha ISAPI, nó corrompido) MUST: (1) interromper a execução, (2) resetar o
  estado de execução para o início do fluxo, (3) registrar o erro em log
  estruturado com campos `device_id`, `flow_id`, `node_id`, `error`. O webhook
  MUST retornar HTTP 200 ao dispositivo independentemente de erros internos.
- **FR-022**: O motor MUST validar o fluxo antes de iniciar cada execução. Fluxo
  com estrutura inválida (ciclo, nó de início ausente, ramos de decisão
  incompletos) MUST acionar circuit-break imediatamente sem executar nenhum nó.
- **FR-023**: Execuções concorrentes do mesmo fluxo (dois eventos simultâneos no
  mesmo dispositivo) MUST ser independentes, sem compartilhar estado de execução.

#### Gestão de Imagens de Background

- **FR-024**: O sistema MUST manter uma biblioteca de imagens disponíveis para
  seleção no nó 4 (trocar background). O admin MUST poder fazer upload de
  imagens para essa biblioteca pelo painel admin.
- **FR-025**: Imagens enviadas ao dispositivo (nó 4 e nó 6) MUST atender aos
  requisitos de formato e tamanho do dispositivo HikVision (a serem determinados
  pelo contrato ISAPI fornecido pelo operador [BLOCKED_ISAPI]).

> **Decisões de infraestrutura**: esta feature envolve execução assíncrona
> desencadeada por webhook síncronos (os eventos chegam no handler HTTP). A
> execução dos nós (especialmente nó 3 — espera, e nó 5 — HTTPS) MUST ser
> não-bloqueante para o handler HTTP: o webhook responde 200 imediatamente e o
> motor executa o fluxo em goroutine separada. Não há scheduling periódico.
> Fluxos são stateless entre execuções (estado de execução é descartado ao
> final ou no circuit-break). Persistência: fluxos e vinculações são salvos no
> PostgreSQL existente. N/A para: key rotation, refresh de token externo,
> mutex multi-pod (deploy on-premise single instance).

---

### Key Entities

- **Flow**: representa um fluxo configurado. Atributos: identificador único,
  nome, status (ativo/inativo), dispositivo vinculado (nullable), lista de nós,
  lista de arestas (conexões entre nós), timestamps de criação/atualização.
  Restrição: no máximo um fluxo ativo por dispositivo.

- **FlowNode**: representa um nó no fluxo. Atributos: identificador único (dentro
  do fluxo), tipo (`camera_on`, `camera_off`, `wait`, `change_background`,
  `https_call`, `qrcode_background`, `decision`, `send_message`), configuração
  específica do tipo (JSON livre, validado por tipo), posição no canvas (x, y).

- **FlowEdge**: conexão entre dois nós. Atributos: nó de origem, nó de destino,
  rótulo da saída (para o nó de decisão: `"valid"` ou `"invalid"`).

- **BackgroundImage**: imagem disponível para seleção no nó 4. Atributos:
  identificador, nome exibido no editor, path de armazenamento local,
  timestamps.

- **FlowExecutionLog**: registro de execução do motor para auditoria. Atributos:
  flow_id, device_id, event_key (chave de idempotência do AttendanceEvent),
  status (`completed` | `circuit_break`), nó onde ocorreu falha (nullable),
  erro (nullable), started_at, finished_at.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: Administrador cria um fluxo completo com ao menos 3 nós (incluindo
  decisão) e o vincula a um dispositivo em menos de 5 minutos.
- **SC-002**: O motor inicia a execução do fluxo em menos de 500 ms após o
  recebimento do evento de reconhecimento no webhook.
- **SC-003**: 100% dos eventos de heartbeat e de tipos não-`AccessControllerEvent`
  não acionam o motor de execução (auditável via logs).
- **SC-004**: Em caso de falha em qualquer nó, o circuit-break reseta o fluxo ao
  estado inicial em 100% dos casos, sem nenhum nó ser executado parcialmente
  sem registro de erro.
- **SC-005**: O sistema processa ao menos 2 execuções concorrentes do mesmo fluxo
  sem interferência de estado (verificável com eventos simultâneos simulados).
- **SC-006**: Fluxo com ciclo ou ramo de decisão incompleto é rejeitado na
  publicação em 100% dos casos, sem exceção atingindo o motor.

---

## Dependências e Bloqueios

| Item | Status | Ação necessária |
|------|--------|-----------------|
| Contrato ISAPI — ligar câmera (nó 1) | BLOCKED_ISAPI | Operador fornece endpoint/payload verificado |
| Contrato ISAPI — desligar câmera (nó 2) | BLOCKED_ISAPI | Operador fornece endpoint/payload verificado |
| Contrato ISAPI — trocar background (nós 4 e 6) | BLOCKED_ISAPI | Operador fornece endpoint/formato de imagem verificado |
| Parâmetros API de mensagem (nó 8) | BLOCKED_API | Operador fornece endpoint, método, headers, schema, auth |

Implementação dos nós marcados BLOCKED_* prossegue como placeholder visual
no editor. O motor de execução MUST detectar nós BLOCKED em tempo de execução
e acionar circuit-break com log descritivo (`nó do tipo X requer contrato não
disponível — bloqueio humano pendente`).
