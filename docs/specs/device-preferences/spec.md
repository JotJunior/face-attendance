# Feature Specification: Preferências e Branding do Terminal HikVision

**Feature**: `device-preferences`
**Created**: 2026-06-25
**Status**: Draft

## Contexto

A feature `device-config` entregou as seções operacionais do painel admin (identidade,
credenciais, sistema, portas, usuários, faces, webhooks). Esta feature entrega as seções
de **personalização e configuração de comportamento** do terminal — display, imagens de
branding, estatísticas de capacidade, gestão de cartões e recursos avançados de face —
todas verificadas na biblioteca PHP de referência `legacy/hik2go`.

Todos os contratos ISAPI nesta spec têm origem rastreável em `legacy/hik2go/src/Hik2go/`
(Preferences/AuthMode.php, Preferences/IdentityTerminal.php, Preferences/StandbyPicture.php,
Preferences/Media.php, Preferences/Presentation.php, Preferences/InitializationScreen.php,
Card.php, Stats.php, Face.php). Nenhum endpoint foi inventado.

> **Decisões de infraestrutura**: N/A — feature stateless; operações ISAPI são síncronas
> sob demanda (o administrador aciona via painel). Credenciais ISAPI já são cifradas
> AES-GCM na tabela `devices` (migration 006, feature `device-config`).

---

## User Scenarios & Testing

### User Story 1 — Configurar modo de verificação do terminal (Priority: P1)

O operador seleciona o modo de autenticação do terminal HikVision pelo painel admin —
face, cartão, PIN, combinações — e o dispositivo passa a aplicar esse modo sem acesso
à interface web nativa do terminal.

**Why this priority**: É a configuração comportamental mais frequente de um terminal
biométrico. O modo de verificação determina como os membros se autenticam; sem controle
remoto, o operador precisa acessar a interface web de cada dispositivo individualmente.

**Independent Test**: Via painel, selecionar "face only" para um dispositivo; testar
que o terminal passa a reconhecer apenas por face (outros métodos rejeitados); via
painel, ler o modo configurado e confirmar que coincide com o valor enviado.

**Acceptance Scenarios**:

1. **Given** dispositivo online com credenciais ISAPI configuradas,
   **When** o operador seleciona um modo de verificação e confirma,
   **Then** o sistema envia o modo ao dispositivo via ISAPI e confirma sucesso; o
   terminal passa a aplicar o novo modo imediatamente.

2. **Given** dispositivo com modo de verificação configurado,
   **When** o operador abre a seção de display/branding,
   **Then** o painel exibe o modo atual do terminal, proveniente da ISAPI, sem valor
   inventado.

3. **Given** dispositivo offline,
   **When** o operador tenta atualizar o modo de verificação,
   **Then** o painel exibe erro claro de conectividade; o modo anterior permanece no
   dispositivo sem alteração.

---

### User Story 2 — Configurar layout de tela do terminal (Priority: P1)

O operador configura o layout de exibição do terminal — normal (padrão HikVision),
tela cheia (publicidade fullscreen) ou dividida (publicidade split-screen) — e os
tempos de timeout de tela, preview e standby, pelo painel admin.

**Why this priority**: Branding visual do terminal é requisito de implantação em
ambientes institucionais (logomarca, cores corporativas via layout publicitário).
A leitura do estado atual é pré-requisito para o write seguro (read-modify-write).

**Independent Test**: Via painel, alterar layout de "normal" para "full"; verificar
fisicamente que o terminal passa a exibir tela cheia; via painel, ler o layout atual
e confirmar que retorna "full".

**Acceptance Scenarios**:

1. **Given** dispositivo online,
   **When** o operador seleciona layout "split" e configura tempo de standby,
   **Then** o sistema lê o estado atual do terminal via ISAPI, monta o XML com os
   novos valores preservando os campos read-only, e envia o `PUT`; o terminal
   reflete o novo layout.

2. **Given** o PUT de layout foi enviado com sucesso,
   **When** o operador lê o layout atual pelo painel,
   **Then** o painel exibe o layout corrente conforme retornado pela ISAPI (normal,
   full ou split).

3. **Given** operador configura tempo de standby com valor fora do intervalo aceito
   pelo firmware,
   **When** o dispositivo rejeita o valor,
   **Then** o painel exibe a mensagem de erro do dispositivo; o estado anterior é
   preservado.

---

### User Story 3 — Gerenciar imagens de branding no terminal (Priority: P2)

O operador carrega, ativa e remove imagens de standby (proteção de tela personalizada),
imagem de inicialização (power-up logo) e material de propaganda (slideshow) nos
terminais — pelo painel admin, sem acesso à interface web do dispositivo.

**Why this priority**: Imagens de branding são customização visual frequente em
organizações que implantam os terminais com identidade visual própria. Três mecanismos
distintos de upload de imagem precisam ser suportados.

**Independent Test**: Via painel, enviar imagem PNG como standby picture; confirmar
que o terminal passa a exibir a imagem na proteção de tela; remover a imagem e
confirmar que o terminal volta ao padrão.

**Acceptance Scenarios**:

1. **Given** dispositivo online,
   **When** o operador envia uma imagem para standby picture,
   **Then** o sistema faz upload multipart para `/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic`,
   ativa o modo custom via PUT em `StandbyPicDisplayParams` e confirma sucesso; o
   terminal exibe a imagem na proteção de tela.

2. **Given** standby pictures cadastradas no terminal,
   **When** o operador lista as imagens de standby,
   **Then** o painel exibe as imagens disponíveis via `/ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList`.

3. **Given** standby picture cadastrada,
   **When** o operador remove uma imagem,
   **Then** o sistema envia o UUID da imagem para `DeleteCustomStandbyPic` e confirma
   remoção; se era a última imagem, o standby volta ao padrão.

4. **Given** dispositivo online,
   **When** o operador envia imagem de power-up (boot logo),
   **Then** o sistema faz upload multipart para `/ISAPI/System/powerUpPicture` e o
   terminal passa a exibir a imagem na inicialização.

5. **Given** dispositivo com material de propaganda configurado,
   **When** o operador remove a propaganda,
   **Then** o sistema remove o material de `MaterialMgr`, o programa de `ProgramMgr`
   e o agendamento de `ScheduleMgr`; o terminal para de exibir propaganda.

> **Risco documentado**: limites de tamanho de imagem por modelo (DS-K1T673*, DS-K1T681DBX)
> não estão confirmados em fonte rastreável. O sistema DEVE retornar o erro do firmware ao
> operador sem silenciá-lo; o plan DEVE documentar estratégia de validação de tamanho.

---

### User Story 4 — Visualizar estatísticas de capacidade do terminal (Priority: P2)

O operador visualiza, em um painel de estatísticas, o número de usuários cadastrados
no terminal, quantos têm face biométrica vinculada, quantos têm cartão vinculado,
quantos eventos estão armazenados — e os limites máximos de cada categoria.

**Why this priority**: Diagnóstico de capacidade é necessário para identificar terminais
próximos do limite e planejar limpeza de usuários antigos. Hoje essa informação só está
disponível na interface web do dispositivo.

**Independent Test**: Com dispositivo com usuários cadastrados via worker, abrir a
seção de estatísticas; confirmar que os contadores (total users, faces, cards) batem
com os valores reais do dispositivo; conferir o campo `max` contra a capacidade do
modelo.

**Acceptance Scenarios**:

1. **Given** dispositivo online com usuários e faces cadastrados,
   **When** o operador abre a seção de estatísticas,
   **Then** o painel exibe contadores atualizados: total de usuários, usuários com
   face, usuários com cartão, capacidade máxima, total de eventos, capacidade máxima
   de eventos — todos provenientes da ISAPI.

2. **Given** dispositivo offline,
   **When** o operador acessa a seção de estatísticas,
   **Then** o painel exibe erro de conectividade claro; nenhum valor de contador é
   exibido como zero por padrão — zero e "indisponível" são distintos.

---

### User Story 5 — Configurar parâmetros avançados de reconhecimento facial (Priority: P4)

O operador configura a distância máxima de reconhecimento facial do terminal pelo painel
admin, permitindo ajuste para diferentes ambientes físicos (corredor estreito vs. espaço
amplo).

**Why this priority**: Ajuste de distância máxima é calibração de ambiente. Terminais
mal calibrados rejeitam membros válidos (muito longe) ou reconhecem pessoas não
autorizadas por proximidade excessiva. A captura facial ao vivo (para diagnóstico) é
funcionalidade de suporte à configuração.

**Independent Test**: Via painel, configurar distância máxima para 1.5m; verificar
que membros além de 1.5m não são reconhecidos; retornar ao valor padrão e confirmar
normalização.

**Acceptance Scenarios**:

1. **Given** dispositivo online,
   **When** o operador configura a distância máxima de reconhecimento,
   **Then** o sistema envia `PUT /ISAPI/AccessControl/FaceCompareCond` com XML
   `<FaceCompareCond>` contendo o campo `maxDistance` e os demais parâmetros de
   pitch/yaw/borders fixos; o dispositivo aplica a nova configuração.

2. **Given** dispositivo com câmera funcional,
   **When** o operador solicita captura facial ao vivo (para diagnóstico),
   **Then** o sistema envia `POST /ISAPI/AccessControl/CaptureFaceData` com XML
   `<CaptureFaceDataCond captureInfrared="false" dataType="url">`, recebe a URL da
   imagem capturada, faz o download e retorna a imagem ao painel.

---

### Edge Cases

- Operação de imagem de standby enviada com arquivo maior que o aceito pelo firmware
  do modelo → o dispositivo retorna erro; o sistema repassa o erro ao operador com
  mensagem acionável; o upload anterior não é corrompido.
- Upload de material de propaganda (Media) falha após criação do material mas antes do
  upload binário → o material órfão fica no dispositivo; o operador pode tentar novamente
  (idempotência por `id` do material não se aplica — o plan DEVE tratar limpeza de
  material órfão ou expor endpoint de limpeza).
- Configuração de layout (IdentityTerminal PUT) enviada sem leitura prévia do `version`
  → o firmware pode rejeitar com erro de versão; a implementação DEVE sempre fazer
  read-modify-write antes do PUT.
- Dispositivo não suporta o modo de verificação selecionado (ex: biometria habilitada
  mas firmware mais antigo) → o ISAPI retorna erro; o sistema repassa ao operador sem
  silenciar.
- Remoção de standby picture que não existe mais no dispositivo → `DeleteCustomStandbyPic`
  retorna erro; o sistema trata como 404 e remove da listagem local se aplicável.

---

## Requirements

### Functional Requirements

**Grupo 1: Modo de Verificação (AuthMode)**

- **FR-001**: O sistema DEVE expor `GET /admin/api/devices/{id}/preferences/auth-mode`
  que retorna a configuração do plano semanal de verificação via
  `GET /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json`.

- **FR-002**: O sistema DEVE expor `PUT /admin/api/devices/{id}/preferences/auth-mode`
  que, antes de enviar, lê o estado atual via FR-001, substitui o campo `verifyMode`
  em todos os `WeekPlanCfg` do payload, e envia o plano completo via
  `PUT /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json`. A leitura antes do
  PUT é obrigatória — o firmware rejeita payloads incompletos.

**Grupo 2: Layout de Tela (IdentityTerminal)**

- **FR-003**: O sistema DEVE expor `GET /admin/api/devices/{id}/preferences/display`
  que retorna a configuração de exibição do terminal via
  `GET /ISAPI/AccessControl/IdentityTerminal` (XML sem `?format=json`); a resposta
  da API admin DEVE incluir os campos mapeados: `showMode` (normal/full/split),
  `screenOffTimeout`, `previewShowTime`, `standbyTimeout`.

- **FR-004**: O sistema DEVE expor `PUT /admin/api/devices/{id}/preferences/display`
  que executa read-modify-write: lê o estado atual via FR-003 (preservando os campos
  read-only: camera, fingerPrintModule, faceAlgorithm, saveCertifiedImage,
  readInfoOfCard, workMode, ecoMode, enableScreenOff, popUpPreviewWindow e o atributo
  `version` do XML raiz), aplica os valores configuráveis (screenOffTimeout,
  previewShowTime, standbyTimeout) e o mapeamento de showMode (normal→showMode=normal
  advertisingDisplayType=full; full→showMode=advertising advertisingDisplayType=full;
  split→showMode=advertising advertisingDisplayType=split), e envia o XML completo
  via `PUT /ISAPI/AccessControl/IdentityTerminal` com `Content-Type: application/xml`.

- **FR-005**: O sistema DEVE expor `GET /admin/api/devices/{id}/preferences/display/thumbnails`
  que retorna a lista de thumbnails de modo de exibição via
  `GET /ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json`.

**Grupo 3: Imagem de Standby (StandbyPicture)**

- **FR-006**: O sistema DEVE expor `GET /admin/api/devices/{id}/preferences/standby-pictures`
  que lista as imagens de standby customizadas via
  `GET /ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json`.

- **FR-007**: O sistema DEVE expor `POST /admin/api/devices/{id}/preferences/standby-pictures`
  (upload multipart de imagem) que: (a) envia a imagem via multipart `POST` para
  `/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json` com campos
  `UploadCustomStandbyPic` (JSON: `{filePathType:"multipart", filePath:<nome>}`) e
  `filePath` (binário); (b) ativa o modo custom via
  `PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` com
  body `{standbyPicType:"custom", displayEffect:"stretch", switchingTime:20}`.
  Upload e ativação são etapas sequenciais: falha na ativação é reportada mesmo
  com upload bem-sucedido.

- **FR-008**: O sistema DEVE expor `DELETE /admin/api/devices/{id}/preferences/standby-pictures/{uuid}`
  que remove a imagem via `POST /ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json`
  com body `{customStandbyPicUUIDList:[{customStandbyPicUUID:"<uuid>"}]}`.

- **FR-009**: O sistema DEVE expor `PUT /admin/api/devices/{id}/preferences/standby-pictures/disable`
  que desativa o standby customizado via
  `PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json`
  com body `{standbyPicType:"default", displayEffect:"stretch", switchingTime:20}`.

**Grupo 4: Imagem de Boot (InitializationScreen)**

- **FR-010**: O sistema DEVE expor `POST /admin/api/devices/{id}/preferences/boot-picture`
  (upload multipart de imagem JPEG) que envia o arquivo via `POST /ISAPI/System/powerUpPicture?format=json`
  com campos multipart: `picture_info` (JSON: `{type:"filePathType", faceLibType:"binay"}`) e
  `picture_name` (binário JPEG com nome `<id>.jpg`).

- **FR-011**: O sistema DEVE expor `DELETE /admin/api/devices/{id}/preferences/boot-picture`
  que remove a imagem de boot via `DELETE /ISAPI/System/powerUpPicture?format=json`.

**Grupo 5: Material de Propaganda (Media + Presentation)**

- **FR-012**: O sistema DEVE expor `GET /admin/api/devices/{id}/preferences/media`
  que lista os materiais de mídia via `GET /ISAPI/Publish/MaterialMgr/material`.

- **FR-013**: O sistema DEVE expor `POST /admin/api/devices/{id}/preferences/media`
  que executa o fluxo completo de criação de material de propaganda, em 5 etapas
  sequenciais:
  (a) `POST /ISAPI/Publish/MaterialMgr/material` com XML `<Material>` (materialType=static,
      staticMaterialType=picture);
  (b) `POST /ISAPI/Publish/MaterialMgr/material/{ID}/upload` multipart com campos
      `name`, `type`, `size` e `file` (binário);
  (c) `POST /ISAPI/Publish/ProgramMgr/program` com XML `<Program version="2.0">`;
  (d) `PUT /ISAPI/Publish/ProgramMgr/program/1/page/1` com XML `<Page version="2.0">`
      referenciando o material pelo ID;
  (e) `PUT /ISAPI/Publish/ScheduleMgr/playSchedule/1` com XML `<PlaySchedule version="2.0">`
      no modo `screensaver` com cobertura diária (00:00:00–24:00:00).
  Falha em qualquer etapa DEVE ser reportada ao operador com a etapa que falhou;
  materiais órfãos criados antes da falha DEVEM ser documentados no plano como
  risco de limpeza manual.

- **FR-014**: O sistema DEVE expor `DELETE /admin/api/devices/{id}/preferences/media/{id}`
  que remove um material via `DELETE /ISAPI/Publish/MaterialMgr/material/{id}`.

- **FR-015**: O sistema DEVE expor `DELETE /admin/api/devices/{id}/preferences/media`
  (bulk) que lista todos os materiais via FR-012 e remove cada um via FR-014.

**Grupo 6: Estatísticas do Dispositivo (Stats)**

- **FR-016**: O sistema DEVE expor `GET /admin/api/devices/{id}/stats` que agrega
  em uma única resposta:
  - `users.total` e `users.faces` e `users.cards`: de `GET /ISAPI/AccessControl/UserInfo/Count?format=json`
    (campos: `UserInfoCount.userNumber`, `UserInfoCount.bindFaceUserNumber`,
    `UserInfoCount.bindCardUserNumber`);
  - `users.max`: de `GET /ISAPI/AccessControl/UserInfo/capabilities?format=json`
    (campo: `UserInfo.maxRecordNum`);
  - `events.total`: de `POST /ISAPI/AccessControl/AcsEventTotalNum?format=json`
    com body `{AcsEventTotalNumCond:{major:0, minor:0}}` (campo: `AcsEventTotalNum.totalNum`);
  - `events.max`: de `GET /ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json`
    (campo: `AcsEvent.totalNum.@max`).
  Todas as 4 chamadas ISAPI são executadas para montar a resposta agregada.

**Grupo 7: Face Avançado**

- **FR-017**: O sistema DEVE expor `PUT /admin/api/devices/{id}/preferences/face-config`
  que configura os parâmetros de comparação facial via
  `PUT /ISAPI/AccessControl/FaceCompareCond` com XML `<FaceCompareCond version="2.0">`;
  o campo configurável pelo operador é `maxDistance` (float); os demais campos têm
  valores fixos conforme `legacy/hik2go/src/Hik2go/Face.php`: pitch=45, yaw=45,
  leftBorder=0, rightBorder=0, upBorder=0, bottomBorder=0, faceScore=0,
  faceScoreThreshold1=0, ROIRegionMode=manual.

- **FR-018**: O sistema DEVE expor `POST /admin/api/devices/{id}/preferences/face-capture`
  que solicita captura facial ao vivo via `POST /ISAPI/AccessControl/CaptureFaceData`
  com XML body `<CaptureFaceDataCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema"><captureInfrared>false</captureInfrared><dataType>url</dataType></CaptureFaceDataCond>`;
  o sistema recebe a URL da imagem capturada, faz download e retorna a imagem ao
  painel como **base64 em JSON** (Clarification 3 resolvida — dec-011; não expõe IP
  interno do device).

**Grupo 8: Segurança e Validação**

- **FR-019**: Todos os novos endpoints DEVEM exigir sessão admin válida (cookie HMAC,
  igual aos endpoints `/admin/api/*` existentes) — Constitution V.

- **FR-020**: Operações ISAPI que falham por timeout DEVEM retornar `504 Gateway Timeout`
  com mensagem acionável; erros de autenticação ISAPI retornam `502 Bad Gateway`
  sem expor senha ou credenciais.

- **FR-021**: O backend DEVE validar que `{id}` corresponde a dispositivo existente
  antes de qualquer conexão ISAPI; dispositivo inexistente retorna `404`.

- **FR-022**: Uploads de imagem (FR-007, FR-010, FR-013) DEVEM validar que o
  arquivo enviado pelo operador é uma imagem (`image/*`) antes de repassar ao
  dispositivo; tipo inválido retorna `400 Bad Request`.

- **FR-023**: Logs de todas as operações DEVEM incluir `device_id` e `stage` e
  DEVEM NOT incluir a senha ISAPI, tokens ou conteúdo binário de imagem —
  Constitution V, Constitution VI.

### Key Entities

- **Device** (já existente): terminal HikVision identificado na tabela `devices`;
  campos de credenciais (`isapi_username`, `isapi_password_enc`, `isapi_port`) já
  gerenciados pela feature `device-config`. Esta feature não altera o schema da
  tabela `devices`.
- **StandbyPicture** (no dispositivo, não persistida localmente): imagem de proteção
  de tela no firmware do terminal, identificada por UUID retornado pelo dispositivo.
- **Material / Program / Schedule** (no dispositivo, não persistidos localmente):
  material de propaganda, programa de exibição e agendamento — entidades do
  `Publish` namespace do firmware.
---

## Success Criteria

### Measurable Outcomes

- **SC-001**: O operador configura o modo de verificação (face/cartão/PIN) de um terminal
  em menos de 30 segundos pelo painel admin, sem acessar a interface web do dispositivo.

- **SC-002**: O operador configura o layout de tela (normal/full/split) e tempos de
  timeout de um terminal pelo painel em menos de 30 segundos; o terminal reflete a
  mudança sem reinicialização.

- **SC-003**: Upload de imagem de standby ou boot logo é concluído com feedback de
  sucesso ou erro claro em menos de 15 segundos em rede local (excluindo tempo de
  transferência de arquivo); o operador nunca recebe silêncio ou erro genérico.

- **SC-004**: O painel de estatísticas exibe contadores atualizados (usuários, faces,
  cartões, eventos, capacidades máximas) de um terminal em menos de 5 segundos em
  condições normais de rede local.

- **SC-005**: Erros de conectividade com o terminal (offline, credenciais inválidas,
  timeout, rejeição de valor pelo firmware) são comunicados ao operador com mensagem
  acionável em 100% das operações — zero erros genéricos sem contexto.

- **SC-006**: A senha ISAPI não aparece em nenhum log, resposta JSON ou mensagem de
  erro em nenhuma operação desta feature — auditável por inspeção de logs.

---

## Clarifications

### Clarification 1 — Escopo de Cartões [RESOLVIDA — block-001 / dec-017]

A feature `device-config` explicitamente deixou gestão de cartões fora de escopo
("Gestão de cartões RFID/NFC — o fluxo principal é por face"). Os contratos ISAPI
de cartões estão verificados em `legacy/hik2go/src/Hik2go/Card.php`.

**Resolução (operador, opção B — score 3)**: Gestão de cartões RFID está **excluída
permanentemente** desta feature. Cartões não fazem parte do produto — a presença é
por reconhecimento facial. O operador usa a interface web do próprio dispositivo para
operações de `CardInfo`, se necessário. FR-017/018/019 (condicionais) e User Story 5
foram removidos da spec (dec-017, block-001 respondido em 2026-06-25).

> Nota: o campo `users.cards` continua presente em FR-016 (estatísticas) porque
> a ISAPI `UserInfo/Count` retorna `bindCardUserNumber` como contador somente-leitura
> do dispositivo — exibi-lo no painel é leitura informativa, não implementação de
> gestão de cartões.

### Clarification 2 — Limites de Tamanho de Imagem por Modelo [RESOLVIDA — dec-010]

**Resolução (score 3)**: O sistema NÃO pré-valida tamanho de imagem no servidor.
Estratégia adotada:
- FR-022 (já na spec) exige validação de tipo `image/*` antes de repassar ao device.
- Limite de tamanho: **não hardcodar** — sem fonte rastreável para DS-K1T681DBX
  (Constitution I). O erro do firmware é repassado ao operador com mensagem acionável
  (FR-020, spec edge case US3).
- O plan DEVE documentar: (a) exemplo de mensagem de erro acionável para tamanho
  excedido; (b) orientação ao operador sobre limites observados por modelo
  (DS-K1T673*: ~200 KB conforme CLAUDE.md; DS-K1T681DBX: a descobrir em runtime).

### Clarification 3 — Formato de Retorno da Captura Facial ao Vivo [RESOLVIDA — dec-011]

**Resolução (score 2)**: FR-018 retorna imagem como **base64 em JSON**.
- Consistente com padrão de todos os endpoints `/admin/api/*` (JSON).
- Não expõe IP interno nem credenciais do device (Constitution V).
- O backend faz download da URL capturada pelo device e encoda em base64 antes
  de retornar ao painel.

### Clarification 4 — Material de Propaganda Órfão Após Falha Parcial [RESOLVIDA — dec-012]

**Resolução (score 2)**: Estratégia "documentar risco + retornar ID do material órfão".
- FR-013 prescreve: "materiais órfãos DEVEM ser documentados no plano como risco de
  limpeza manual".
- Na resposta de erro de FR-013, o sistema DEVE incluir o `id` do material criado
  na etapa (a) para que o operador use FR-014 (`DELETE /media/{id}`) para limpeza.
- O plan DEVE documentar este fluxo de limpeza manual.

---

## Fora de Escopo

- Fluxo de presença (scheduler, worker, webhook de reconhecimento facial) — esta
  feature não toca nenhum dos três subsistemas do fluxo principal.
- Provisionamento de webhook — já implementado em `ProvisionWebhook` + handler
  `POST /admin/api/devices/{id}/webhooks`.
- Gestão de usuários/faces no dispositivo (listagem, bulk clear) — coberto pela
  feature `device-config`.
- Controle de portas — coberto pela feature `device-config`.
- Stream de eventos em tempo real (SSE/WebSocket via alertStream) — fora de escopo
  desta e da feature `device-config`.
- Gestão de cartões RFID/NFC (`CardInfo`: vincular, consultar, remover cartão) —
  excluída permanentemente por decisão do operador (block-001 / dec-017). Presença
  é por reconhecimento facial; operador usa interface web do dispositivo para CartInfo.
