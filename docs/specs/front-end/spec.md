# Feature Specification: Interface de Administração Web

**Feature**: `front-end`
**Created**: 2026-06-20
**Status**: Clarify concluído — avançando para plan

## Visão Geral

Interface web de administração para o sistema de controle de presença por
reconhecimento facial. Permite à equipe operacional monitorar o estado dos
membros GOB, acompanhar a saúde dos dispositivos HikVision (heartbeat), visualizar
os logs de sincronização com a API GOB, e acionar manualmente a carga de membros.
A interface é protegida por autenticação simples (login e senha). O tema visual
padrão é dark mode.

> **Escopo em relação ao MVP original**: o briefing marca "Interface web de
> administração" como fora do escopo do MVP; esta feature a adiciona como extensão
> post-MVP controlada, sem alterar nenhuma das quatro etapas do fluxo de
> reconhecimento.

> **Decisões de infraestrutura**: esta feature não introduz schedulers, sessões
> persistentes de longa duração, rotação de chaves externas ou mutex multi-pod.
> N/A explícito — feature não requer os FRs de infraestrutura do checklist.

> **Escopo de backend incluído nesta feature** (dec-012 + dec-013): além da interface
> web, esta feature requer dois itens de backend:
> 1. Nova env var `DEVICE_OFFLINE_THRESHOLD_HOURS` lida pelo servidor Go e exposta
>    via API (campo `device_offline_threshold_hours` em resposta do endpoint de stats).
> 2. Novo endpoint `GET /admin/stats` no backend Go retornando os 3 contadores do
>    dashboard numa única chamada.

---

## User Scenarios & Testing

### User Story 1 — Autenticação no painel (Priority: P1)

Como membro da equipe operacional, quero acessar o painel de administração com
meu login e senha, para que somente pessoas autorizadas vejam dados operacionais
do sistema.

**Why this priority**: sem autenticação não há como publicar o painel sem expor
dados de membros e dispositivos. Bloqueante para todas as demais stories.

**Independent Test**: acessar a URL do painel sem credenciais deve redirecionar
para a tela de login. Com credenciais corretas, deve exibir o dashboard. Com
credenciais incorretas, deve exibir mensagem de erro e não redirecionar.

**Acceptance Scenarios**:

1. **Given** o operador não está autenticado, **When** acessa qualquer rota
   protegida do painel, **Then** é redirecionado para a tela de login sem ver
   conteúdo protegido.
2. **Given** o operador está na tela de login, **When** submete credenciais
   corretas, **Then** é redirecionado para o dashboard e vê as métricas principais.
3. **Given** o operador está na tela de login, **When** submete senha incorreta,
   **Then** vê mensagem de erro e permanece na tela de login.
4. **Given** o operador está autenticado, **When** clica em "sair", **Then** a
   sessão é encerrada e qualquer tentativa de navegar exige novo login.

---

### User Story 2 — Dashboard com métricas principais (Priority: P1)

Como operador, quero ver em uma única tela as métricas mais importantes do
sistema — total de membros carregados, total de dispositivos ativos, quantidade
de presenças marcadas nas últimas 24 horas e alertas de dispositivos offline —
para diagnosticar rapidamente se o sistema está operando normalmente.

**Why this priority**: P1 junto com autenticação porque é a primeira tela
pós-login; sem ela o painel não entrega valor imediato ao operador.

**Independent Test**: com pelo menos um membro carregado, um dispositivo ativo e
um evento de presença registrado, os três contadores devem refletir os valores
reais do banco de dados.

**Acceptance Scenarios**:

1. **Given** o sistema tem membros carregados, **When** o operador acessa o
   dashboard, **Then** vê o total de membros com selfie disponível.
2. **Given** existem dispositivos com heartbeat recente, **When** o operador
   acessa o dashboard, **Then** vê a contagem de dispositivos ativos (heartbeat
   nas últimas N horas) e inativos separadamente.
3. **Given** há eventos de reconhecimento nas últimas 24 horas, **When** o
   operador acessa o dashboard, **Then** vê a contagem de presenças marcadas no
   período.
4. **Given** um dispositivo não enviou heartbeat há mais de X horas, **When** o
   operador acessa o dashboard, **Then** esse dispositivo aparece em destaque como
   "offline" no painel de métricas.

---

### User Story 3 — Lista e detalhe de dispositivos (Priority: P2)

Como operador, quero ver a lista de todos os dispositivos HikVision registrados,
com seu status de heartbeat e se o webhook está configurado, para saber quais
dispositivos estão ativos e prontos para reconhecer presenças.

**Why this priority**: P2 — funcionalidade de monitoramento; o dashboard já
dá o resumo; esta tela dá o drill-down operacional.

**Independent Test**: com dois dispositivos no banco — um com heartbeat recente e
webhook configurado, outro sem — a lista deve mostrar ambos com seus estados
corretos.

**Acceptance Scenarios**:

1. **Given** há dispositivos registrados, **When** o operador acessa a tela de
   dispositivos, **Then** vê uma tabela com identificador, IP, última vez que
   enviou heartbeat, e status do webhook.
2. **Given** um dispositivo enviou heartbeat recentemente, **When** o operador
   visualiza a lista, **Then** aquele dispositivo aparece como "ativo".
3. **Given** um dispositivo não enviou heartbeat há mais do que o limiar de
   inatividade, **When** o operador visualiza a lista, **Then** aquele dispositivo
   aparece como "offline" com indicador visual de alerta.
4. **Given** um dispositivo está na lista, **When** o operador clica nele, **Then**
   vê detalhes: ID interno, MAC address, IP, data de primeiro registro, histórico
   resumido de heartbeats.

---

### User Story 4 — Lista de membros e status de sincronização (Priority: P2)

Como operador, quero visualizar a lista de membros carregados da GOB e o estado
de sincronização de cada um nos dispositivos (usuário criado, face enviada), para
saber quem já está apto a ser reconhecido e quem ainda tem pendências.

**Why this priority**: P2 — diagnóstico operacional; permite ao operador identificar
membros que não foram propagados corretamente para os dispositivos.

**Independent Test**: com um membro totalmente sincronizado em um dispositivo e
outro com falha na etapa de upload de face, a lista deve refletir esses estados
distintos.

**Acceptance Scenarios**:

1. **Given** há membros no banco, **When** o operador acessa a tela de membros,
   **Then** vê uma lista com nome, CPF (mascarado na exibição), status GOB, e
   resumo de sincronização nos dispositivos.
2. **Given** um membro foi totalmente sincronizado (usuário + face), **When** o
   operador visualiza a lista, **Then** aquele membro aparece com indicador
   "sincronizado".
3. **Given** um membro tem erro em alguma etapa de sincronização, **When** o
   operador visualiza a lista, **Then** aquele membro aparece com indicador de
   falha e a última etapa que falhou.
4. **Given** o operador quer encontrar um membro específico, **When** digita nome
   ou CPF no campo de busca, **Then** a lista filtra em tempo real.

---

### User Story 5 — Log de eventos de presença (Priority: P3)

Como operador, quero ver o histórico de eventos de reconhecimento facial recebidos
dos dispositivos, com o status de marcação na GOB, para auditar se as presenças
estão sendo registradas corretamente na plataforma.

**Why this priority**: P3 — auditoria; o sistema funciona sem esta tela, mas ela
é essencial para diagnóstico de falhas na marcação GOB.

**Independent Test**: com eventos de presença no banco — alguns marcados, outros
não — a tela deve listar ambos com seus respectivos estados.

**Acceptance Scenarios**:

1. **Given** há eventos de reconhecimento no banco, **When** o operador acessa a
   tela de logs, **Then** vê lista cronológica com: data/hora, identificador do
   dispositivo, nome do membro, e status de marcação GOB (marcado / não marcado /
   não autorizado).
2. **Given** um evento foi marcado com sucesso na GOB, **When** o operador
   visualiza a lista, **Then** aquele evento aparece como "marcado" com data/hora
   de marcação.
3. **Given** um evento não foi marcado (falha na chamada GOB), **When** o operador
   visualiza a lista, **Then** aquele evento aparece como "pendente" ou "falhou"
   com indicador de alerta.
4. **Given** o operador quer filtrar por período, **When** seleciona intervalo de
   datas, **Then** a lista exibe apenas os eventos dentro daquele intervalo.

---

### User Story 6 — Sincronização manual de membros (Priority: P3)

Como operador, quero acionar manualmente a carga de membros da GOB sem precisar
acessar a API por linha de comando, para forçar uma sincronização quando necessário
(ex: novo membro cadastrado na GOB).

**Why this priority**: P3 — conveniência operacional. A sincronização periódica
(se configurada) já acontece automaticamente; este botão é para casos excepcionais.

**Independent Test**: clicar no botão de sincronização deve disparar uma chamada
ao endpoint `/admin/sync` e exibir feedback de sucesso ou erro para o operador.

**Acceptance Scenarios**:

1. **Given** o operador está autenticado, **When** clica no botão "Sincronizar
   membros agora", **Then** o painel exibe indicador de processamento em andamento.
2. **Given** a sincronização foi acionada, **When** o servidor confirma o início
   do ciclo, **Then** o operador vê mensagem de confirmação.
3. **Given** a sincronização foi acionada, **When** o servidor retorna erro (ex:
   GOB indisponível), **Then** o operador vê mensagem de erro descritiva.
4. **Given** uma sincronização já está em andamento, **When** o operador tenta
   acionar outra, **Then** o botão está desabilitado ou exibe aviso de
   "sincronização em progresso".

---

### Edge Cases

- O que acontece se o backend retorna 401 durante a navegação (token de sessão
  expirado)? O usuário deve ser redirecionado para login sem perder o contexto de
  onde estava.
- O que acontece se a lista de membros tem milhares de entradas? A tela deve
  paginar ou virtualizar para não travar o navegador.
- O que acontece se o dashboard carrega sem nenhum dispositivo registrado? Deve
  exibir estado vazio amigável ("Nenhum dispositivo registrado ainda") em vez de
  zeros que podem ser confundidos com falha.
- O que acontece se a chamada de sincronização manual demora mais de 30 segundos?
  O painel deve manter o indicador de progresso e não fazer o operador aguardar
  bloqueado na tela.
- CPF exibido em telas de membros e logs: deve ser mascarado (`***.000.***-**`)
  para reduzir exposição de dado sensível no painel.

---

## Requirements

### Functional Requirements

- **FR-001**: O sistema MUST exigir autenticação com login e senha antes de
  exibir qualquer tela do painel. O backend valida as credenciais (usuário e senha
  configurados como variáveis de ambiente) e emite um cookie httpOnly com TTL
  configurável via env. Não é permitido armazenar token em LocalStorage (XSS).
  (Clarificado em clarify: dec-006, score 3)
- **FR-002**: O painel MUST usar dark mode como tema visual padrão. O sistema
  não precisa oferecer toggle de tema — dark mode é o único tema suportado nesta
  versão.
- **FR-003**: O dashboard MUST exibir no mínimo: (a) total de membros com selfie
  no banco, (b) contagem de dispositivos ativos vs inativos com base no heartbeat,
  (c) número de eventos de presença marcados nas últimas 24 horas. O frontend
  DEVE obter esses dados via **`GET /admin/stats`** (endpoint novo no backend Go),
  que retorna os 3 contadores e o valor de `device_offline_threshold_hours` numa
  única resposta JSON. (dec-013, score 3 — operador)
- **FR-004**: A tela de dispositivos MUST listar todos os dispositivos registrados
  com: identificador de dispositivo, IP, status de atividade (ativo/offline baseado
  no último heartbeat), e se o webhook está configurado. O limiar de inatividade é
  definido pela env var **`DEVICE_OFFLINE_THRESHOLD_HOURS`** no backend; o frontend
  compara `last_heartbeat_at` com o horário atual usando o valor retornado por
  `GET /admin/stats`. (dec-012, score 3 — operador; dec-007, score 3) A tela de
  detalhe exibe dados atuais do dispositivo (ID, MAC, IP, data de registro, último
  heartbeat, status calculado) — sem histórico de série temporal, conforme schema
  existente.
- **FR-005**: A tela de membros MUST listar membros com: nome, CPF mascarado,
  status na GOB, e estado de sincronização por dispositivo (usuário criado + face
  enviada + tentativas + último erro se houver).
- **FR-006**: A tela de logs MUST listar eventos de reconhecimento em ordem
  cronológica decrescente com: data/hora, dispositivo de origem, membro identificado,
  e status de marcação GOB (marcado, pendente, falhou, não autorizado).
- **FR-007**: O painel MUST oferecer um botão de sincronização manual que aciona
  o endpoint existente de carga de membros. O botão MUST mostrar estado de
  carregamento durante a operação e feedback de sucesso/erro ao concluir.
- **FR-008**: A tela de membros MUST suportar busca server-side via query param
  (ex: `GET /admin/members?q=nome_ou_cpf`) com paginação cursor. A tela de logs
  MUST suportar filtragem por intervalo de datas (server-side). Busca client-side
  apenas para filtro rápido dentro da página já carregada. (dec-008, score 3)
- **FR-009**: O painel MUST responder adequadamente a estados vazios: quando não
  há dados (nenhum dispositivo, nenhum membro, nenhum evento), exibir mensagem
  informativa em vez de tabelas vazias ou zeros silenciosos.
- **FR-010**: O design visual MUST ser produzido com a skill `/frontend-design`
  para garantir qualidade estética acima do padrão genérico. Interface polida,
  tipografia clara, hierarquia visual consistente.
- **FR-011**: CPF MUST ser exibido mascarado em todas as telas do painel
  (`***.NNN.NNN-**` ou equivalente) para reduzir exposição de dado sensível.
- **FR-012**: Sessão de autenticação expirada durante uso MUST redirecionar o
  operador para login sem perder a URL atual (para redirecionar de volta após
  re-autenticação).
- **FR-013**: O painel MUST ser servido na mesma origem do backend (não requer
  domínio separado), integrado como rota `/admin/*` ou como arquivos estáticos
  servidos pelo servidor Go existente.

### Key Entities

- **Device**: dispositivo HikVision registrado (identificador, IP, MAC, último
  heartbeat, status de atividade, webhook configurado). Fonte: banco de dados local.
- **Member**: membro GOB carregado (nome, CPF mascarado, status GOB, estado de
  sincronização por dispositivo). Fonte: banco de dados local.
- **ProcessingOutcome**: estado de sincronização de um membro em um dispositivo
  específico (usuário criado, face enviada, último erro, número de tentativas).
  Fonte: banco de dados local.
- **AttendanceEvent**: evento de reconhecimento facial recebido (data/hora,
  dispositivo, membro, status de marcação GOB). Fonte: banco de dados local.
- **Session**: sessão autenticada do operador (credencial validada, token de
  sessão com TTL). Fonte: backend — não persiste dados de membros/dispositivos.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: Operador consegue acessar o dashboard e visualizar todas as métricas
  principais em menos de 5 segundos após o login, em conexão de rede local.
- **SC-002**: Operador consegue identificar um dispositivo offline ou um membro
  com falha de sincronização em menos de 30 segundos navegando pelo painel, sem
  consultar logs de terminal.
- **SC-003**: 100% das rotas do painel redirecionam para login quando acessadas sem
  sessão válida — sem vazamento de dados protegidos.
- **SC-004**: A ação de sincronização manual fornece feedback visual ao operador
  em menos de 2 segundos após o clique, independentemente de quanto tempo o
  processamento backend leva.
- **SC-005**: A tela de logs suporta exibição de pelo menos 1.000 eventos sem
  degradação perceptível de performance no navegador (scroll/filtragem fluidos).
- **SC-006**: CPF não aparece em formato completo (sem máscara) em nenhuma tela
  do painel, verificável por inspeção visual de todas as telas.

---

## Clarifications

_Etapa clarify concluída em 2026-06-20. Todas as 5 perguntas respondidas —
sem itens pendentes de decisão humana. Decisões abaixo derivadas via heurística
score 0..3 (briefing + constitution + código-fonte) ou confirmadas pelo operador._

| Decisão | Escolha adotada | Score | Justificativa |
|---------|-----------------|-------|---------------|
| Credencial de admin | Variável de ambiente no backend | — | Constitution V: segredos como runtime; o token `AdminToken` já existe no `ServerConfig` |
| Integração frontend | Arquivos estáticos servidos pelo servidor Go (`/admin/*`) | — | Sem overhead de deploy separado; consistente com arquitetura on-premise |
| Limiar de dispositivo "offline" | Env var `DEVICE_OFFLINE_THRESHOLD_HOURS` no backend; valor exposto via `GET /admin/stats`; frontend compara `last_heartbeat_at` com hora atual usando esse valor | 3 | dec-012 — decisão do operador (block-001 resolvido). Backend lê a env e inclui o limiar na resposta de stats para o frontend consumir sem hardcode. |
| Paginação | Lado servidor para membros e logs (cursor ou offset) | — | Volume pode chegar a milhares; client-side filtering apenas para busca rápida em janela carregada |
| Mecanismo de sessão | Cookie httpOnly com TTL configurável via env | — | Sem LocalStorage de token (XSS); alinhado com Constitution V |
| Mecanismo de autenticação (UI) | Formulário usuário + senha → backend valida via env vars → emite cookie httpOnly | 3 | FR-001 especifica "login e senha"; tabela Clarifications já decidiu cookie httpOnly; Constitution V requer config via env. (dec-006) |
| Contrato de API do dashboard | Criar endpoint `GET /admin/stats` no backend Go retornando os 3 contadores e o limiar `device_offline_threshold_hours` em única resposta JSON | 3 | dec-013 — decisão do operador (block-002 resolvido). Endpoint único reduz latência e acopla lógica de negócio no backend onde pertence. Implica escopo de backend nesta feature. |
| Tela de detalhe de dispositivo | Exibe dados atuais: ID, MAC, IP, data de registro, último heartbeat e status calculado — sem nova tabela de histórico | 3 | Migration `000002` tem apenas `last_heartbeat_at` (campo único); briefing não menciona histórico; Constitution proíbe expansão de escopo sem amendment. (dec-007) |
| Busca na tela de membros | Server-side via query param (ex: `GET /admin/members?q=nome`) com paginação cursor | 3 | Briefing: "centenas a poucos milhares de membros"; spec já decidiu paginação server-side; edge case refere volume alto. (dec-008) |
