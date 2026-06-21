# Design Brief — Front-end "Presença Facial"

> Documento para orientar o redesign/expansão do front-end. É auto-contido:
> descreve o produto, o estado atual, o modelo de dados e o que queremos
> desenhado. Idioma da interface: **Português (PT-BR)**. Idioma da sintaxe
> (campos, rotas, código): **Inglês**.

---

## 1. O que é o produto

**Presença Facial** é um serviço *on-premise* que automatiza o registro de
presença de membros por **reconhecimento facial**. Ele faz a ponte entre:

- a **plataforma GOB** — sistema do cliente que é a fonte dos membros (nome,
  CPF, selfie, status) e o destino onde a presença é efetivamente marcada; e
- **terminais HikVision** (dispositivos de controle de acesso na rede local,
  via protocolo ISAPI) que fazem o reconhecimento do rosto e disparam eventos.

O backend é Go + PostgreSQL + RabbitMQ, empacotado em Docker e rodando dentro da
rede do cliente. O front-end é um **painel administrativo** embutido no próprio
binário, servido em `/admin`.

### Fluxo end-to-end (em uma frase cada)

1. **Scheduler**: periodicamente puxa os membros do GOB, salva localmente e
   enfileira cada membro que tem selfie.
2. **Worker**: consome a fila e provisiona o membro no dispositivo (cria o
   usuário + envia a face via ISAPI).
3. **Dispositivo**: reconhece o rosto e faz um `POST` no webhook do serviço.
4. **Serviço**: valida o CPF, registra o evento e **marca a presença no GOB**.

---

## 2. Objetivos do produto

- Tirar o trabalho manual do registro de presença: o membro chega, o rosto é
  reconhecido, a presença é marcada — sem operador no meio.
- Dar ao **operador/administrador** visibilidade e controle total da operação:
  saúde dos dispositivos, status de cada membro, histórico de acessos e
  configuração dos terminais.
- Funcionar **on-premise**, em rede local, muitas vezes sem TLS e em hardware
  modesto (ex.: Raspberry Pi). Robustez e clareza valem mais que efeitos.

### Objetivos específicos deste redesign

1. **Elevar a identidade visual e a usabilidade** do painel atual.
2. **Criar a tela de configuração total do dispositivo HikVision** (hoje
   inexistente — a configuração é feita por variáveis de ambiente e direto no
   terminal).
3. **Criar a tela de perfil do membro + histórico de acessos** (drill-down a
   partir da lista de Membros, dentro do painel admin).

---

## 3. Público / personas

- **Operador / Administrador (usuário primário do painel).** Pessoa técnica ou
  semi-técnica responsável por instalar e manter a operação no local do cliente.
  Cadastra dispositivos, acompanha sincronizações, resolve falhas de
  provisionamento, audita acessos e configura os terminais. Usa
  predominantemente desktop, mas pode precisar de tablet/celular em campo
  (em pé, perto do equipamento).
- **Membro (sujeito, não usuário).** A pessoa cujo rosto é reconhecido. **Não
  faz login** neste momento — seus dados e histórico são consultados *pelo
  operador* através da tela de perfil do membro no admin.

---

## 4. Estado atual do front-end (ponto de partida)

> O redesign tem **liberdade para repensar a identidade visual** (cores,
> tipografia, modo claro/escuro, layout). O que está abaixo é o que existe hoje,
> para contexto — não é uma restrição estética.

- **Tecnologia atual:** HTML/CSS/JS *vanilla* (sem framework, sem build step),
  ~1k linhas, embutido no binário Go via `//go:embed` e servido em `/admin`.
- **Identidade atual ("Obsidian Terminal"):** dark-mode único; base
  `#0f1117`/`#1a1d27`; acento indigo `#6366f1`; tipografia `Syne` (UI) +
  `JetBrains Mono` (timestamps/subtítulos); estética "terminal" com acentos
  geométricos. Tokens em CSS custom properties, foco em contraste alto.
- **Telas existentes (5 + login):**
  1. **Login** — usuário + senha.
  2. **Dashboard** — 4 cards de métricas (membros com selfie, dispositivos
     ativos/inativos, presenças nas últimas 24h), botão de sync manual, alerta
     de dispositivos offline.
  3. **Dispositivos** — tabela (identificador, IP, status, webhook, último
     heartbeat) com drill-down de detalhe.
  4. **Membros** — tabela com busca (nome/CPF mascarado), status de
     sincronização, ação "Reenviar" por membro, paginação.
  5. **Eventos** — log cronológico de acessos com filtro por período.
- **Responsivo:** sidebar fixa no desktop, drawer no mobile.
- **Acessibilidade:** já mira WCAG AA (foco visível, alvos ≥44px, ARIA).

---

## 5. Restrições e contratos técnicos (importante para o entregável)

O front-end **roda como assets estáticos**, sem runtime de servidor:

- O entregável precisa ser **embutível** (servido por um file server estático em
  `/admin`). Pode ser *vanilla* ou uma SPA com build step (ex.: Vite), **desde
  que o resultado final seja HTML/CSS/JS estático** — sem SSR, sem Node em
  tempo de execução.
- **Autenticação:** cookie de sessão `admin_session` (HttpOnly, SameSite=Strict,
  escopo `/admin`). Login em `POST /admin/api/login`; logout em
  `POST /admin/api/logout`. Qualquer `401` deve redirecionar para o login
  preservando a rota de origem.
- **API:** JSON em `/admin/api/*`, campos em `snake_case`. Paginação por
  *keyset cursor* (sem offset). Datas em RFC3339.
- **Privacidade:** o **CPF nunca trafega completo** para o browser — vem
  mascarado do servidor (`federal_document_masked`). Não há segredos no client.
- **Idioma:** toda a UI em **PT-BR**.

### Endpoints já existentes (telas atuais)

| Método | Rota | Para quê |
|---|---|---|
| `POST` | `/admin/api/login` | autentica, emite cookie |
| `POST` | `/admin/api/logout` | encerra sessão |
| `GET`  | `/admin/api/stats` | métricas do dashboard |
| `GET`  | `/admin/api/devices` | lista de dispositivos |
| `GET`  | `/admin/api/devices/{id}` | detalhe de um dispositivo |
| `GET`  | `/admin/api/members` | lista de membros (busca + paginação) |
| `POST` | `/admin/api/members/{id}/resync` | reenfileira um membro |
| `GET`  | `/admin/api/events` | log de eventos (filtro por data) |
| `POST` | `/admin/api/sync` | dispara sync manual |

> As **duas telas novas** (configuração do dispositivo e parte do perfil do
> membro) exigirão **novos endpoints de backend**, que serão implementados
> separadamente. Para o design, projete contra os dados/capacidades descritos
> nas seções 7 e 8 — o backend acompanha.

---

## 6. Modelo de dados disponível (referência)

Os campos abaixo são o que o sistema realmente armazena — use-os como verdade
para o que cada tela pode exibir.

### Membro (`member`)
`name`, `federal_document` (CPF — exibir **mascarado**), `status` (vindo do GOB,
ex.: ativo/inativo), `mobile_number` (telefone, opcional), `url_selfie` (URL
externa da foto, no GOB), `gob_id`, `gob_created_at`, `gob_updated_at`,
`created_at`, `updated_at`.

### Dispositivo (`device`)
`device_identifier` (MAC, identidade estável), `ip_address`, `mac_address`,
`last_heartbeat_at`, `is_active`, `webhook_configured`, `created_at`,
`updated_at`. O status online/offline é **derivado** de `last_heartbeat_at` vs.
um limiar (`device_offline_threshold_hours`).

### Status de provisionamento (`member_processing_status`) — por membro × dispositivo
`user_synced` (usuário criado no device), `face_uploaded` (selfie enviada),
`webhook_set` (webhook configurado), `last_stage`
(`user_sync`|`face_upload`|`webhook`|`done`), `last_error`, `attempts`,
`updated_at`. Estados derivados sugeridos: **Pendente / Provisionado /
Reprocessando / Falhou**.

### Evento de acesso (`attendance_event`)
`event_datetime` (quando o rosto foi reconhecido), `device_identifier`,
`member_name`, `federal_document_masked`, `attendance_status`
(`authorized` = match positivo / outros = negado), `marked` + `marked_at`
(se a presença foi marcada no GOB), `created_at`. Status de marcação sugerido:
**Marcado / Pendente / Falhou / Não-autorizado**. (`raw_payload` existe para
auditoria mas **não** deve ser exposto na UI comum.)

---

## 7. Tela NOVA — Configuração total do dispositivo HikVision

**Objetivo:** dar ao operador um painel para configurar **um dispositivo por
vez** de ponta a ponta, sem precisar acessar a interface nativa do terminal nem
editar variáveis de ambiente.

**Contexto de navegação:** acessível a partir da lista/detalhe de Dispositivos.
Deve deixar claro **qual dispositivo** está sendo configurado (identificador, IP,
status online) e permitir **trocar de dispositivo** facilmente.

> **Sobre o escopo:** a configuração deve cobrir todo o leque que o terminal
> expõe, **incluindo ações destrutivas/sensíveis** (reboot, reset de fábrica,
> abertura/travamento remoto de porta), **com guard-rails e confirmação** (ver
> "Padrões de segurança" ao final desta seção). O catálogo abaixo está
> organizado em seções de UI; cada item corresponde a uma capacidade real do
> dispositivo (comprovada no código de referência da integração).

### 7.1 Visão geral & status (somente leitura)
Modelo, número de série, versão de firmware, IP, MAC, uptime, **capacidades**
(máx. de usuários, máx. de faces, nº de portas), contadores de uso, status
online/offline e último heartbeat.

### 7.2 Sistema & manutenção
- **Data/hora** — sincronização NTP ou ajuste manual (relógio errado quebra a
  validade dos usuários e o timestamp dos eventos — é crítico).
- **Tela de inicialização** — upload da imagem de boot.
- ⚠️ **Reiniciar (reboot)** — ação destrutiva → confirmação.
- ⚠️ **Reset de fábrica** — ação destrutiva e irreversível → confirmação forte
  (digitar o identificador do device para confirmar).

### 7.3 Modo de autenticação & terminal
- **Modo de autenticação** — face / cartão / PIN / combinações.
- **Terminal/identidade** — preferências de UI do device (idioma, timeout de
  tela, etc.).
- **Apresentação / tela de espera (standby)** — imagem e mensagem de boas-vindas;
  com **upload de mídia**.

### 7.4 Controle de acesso / portas
- **Status das portas** (aberta/fechada/normal).
- **Configuração por porta** — delays de destravamento, modos de alarme.
- ⚠️ **Comandos remotos** — abrir / fechar / manter aberta / manter travada →
  ações sensíveis → confirmação (e registro de quem acionou, se possível).

### 7.5 Usuários no dispositivo
Listar/buscar, ver detalhe, criar/editar, excluir um usuário, ⚠️ **limpar todos**
(bulk → confirmação forte) e **captura ao vivo** de face pela câmera do
terminal.

### 7.6 Cartões (RFID/NFC)
CRUD de cartões e ⚠️ **limpar todos** (confirmação forte). (Mesmo que o fluxo
principal seja por face, o terminal suporta cartão.)

### 7.7 Biblioteca de faces
Listar faces cadastradas, visualizar/enviar/excluir uma face, capacidades da
biblioteca e **comparação 1:1** (verificar se uma imagem corresponde a um
usuário).

### 7.8 Eventos & notificações
- **Webhooks** — listar/criar/editar/remover destinos de notificação (CRUD).
- **Assinatura de eventos (triggers)** — escolher quais eventos disparam
  notificação.
- **Stream ao vivo** — monitor de eventos em tempo real do device.
- **Log de eventos / histórico de presença no device** e **estatísticas**
  (contagem de usuários, tentativas, taxa de reconhecimento).

### 7.9 Mídia
Gerenciar imagens enviadas ao dispositivo (listar, enviar, limpar).

### Padrões de segurança para esta tela (obrigatórios)
- Toda ação ⚠️ destrutiva/sensível exige **modal de confirmação** que **mostra a
  identidade do dispositivo-alvo** (evita agir no device errado).
- Ações irreversíveis ou em massa (reset de fábrica, limpar usuários/cartões/
  faces) exigem **confirmação forte** (ex.: digitar o identificador/nome para
  habilitar o botão).
- Botões destrutivos visualmente distintos (cor de perigo), nunca como ação
  primária acidental.
- Feedback claro de sucesso/erro (a operação no device pode demorar ou falhar) —
  prever estados de *loading* por ação e mensagens de erro úteis.

---

## 8. Tela NOVA — Perfil do membro + histórico de acessos

**Objetivo:** o operador clica em um membro (na lista de Membros) e vê tudo
sobre ele: dados, situação de provisionamento nos dispositivos e o histórico de
acessos. **Não há login do membro** — é uma tela do admin.

### 8.1 Cabeçalho do perfil
- **Foto** (selfie via `url_selfie`), com fallback elegante quando não houver.
- **Nome**, **CPF mascarado**, **status** (badge ativo/inativo, vindo do GOB).
- **Telefone** (se houver), `gob_id`, datas de sincronização (sincronizado em /
  atualizado em).

### 8.2 Situação de provisionamento (por dispositivo)
Para cada dispositivo, mostrar o progresso das 3 etapas de provisionamento
(usuário criado → face enviada → webhook configurado) e o **estado derivado**:

- **Provisionado** (tudo ok)
- **Pendente** (ainda não iniciado)
- **Reprocessando** (`attempts > 0`, em retry)
- **Falhou** — exibir `last_stage` e `last_error` de forma legível.

Incluir a ação **"Reenviar"** (resync) por dispositivo/membro — já existe no
backend. Pensar em como comunicar o motivo de uma falha (ex.: "foto sem rosto
detectável", "imagem acima do limite") de forma acionável.

### 8.3 Histórico de acessos
- **Tabela paginada** (mais recentes primeiro; paginação por *keyset*, carregar
  mais sob demanda).
- Colunas: **data/hora** do reconhecimento, **dispositivo**, **resultado**
  (autorizado/negado), **marcação no GOB** (Marcado / Pendente / Falhou /
  Não-autorizado) e quando foi marcado.
- **Filtro por período**.
- Desejável: um **resumo no topo** — último acesso, nº de acessos nos últimos
  7/30 dias — e, se o designer quiser propor, uma visualização leve de
  frequência (ex.: heatmap/linha do tempo). Opcional, mas bem-vindo.

---

## 9. Requisitos transversais (todas as telas)

- **Idioma:** PT-BR em toda a UI. (Hoje as strings são fixas; se introduzir
  i18n, o idioma-alvo continua PT-BR.)
- **Estados sempre previstos:** *loading* (skeletons/spinners), **vazio**
  (mensagens amigáveis), **erro** (toasts/inline), e **401 → login**.
- **Responsividade:** desktop-first (operador), mas precisa funcionar bem em
  **tablet e celular** (uso em campo, em pé, perto do equipamento).
- **Acessibilidade:** WCAG AA — foco visível, alvos de toque ≥44px, ARIA,
  navegação por teclado, contraste adequado.
- **Privacidade:** CPF sempre mascarado; nenhum dado sensível ou segredo no
  client; ações destrutivas protegidas.
- **Performance:** paginação por keyset, busca com debounce, timeouts em
  chamadas (o ambiente pode ser de rede local instável / hardware modesto).
- **Consistência:** um **design system** com tokens e componentes reutilizáveis
  — cards, tabelas com scroll, badges de status, toasts, **modais de
  confirmação** (incl. variante "confirmação forte"), formulários, abas/seções
  para a tela de configuração do device.

---

## 10. Entregáveis esperados do design

1. **Identidade visual / design system:** paleta, tipografia, tokens, modo
   claro/escuro (livre repropor), e biblioteca de componentes.
2. **Telas em alta fidelidade** para: Login, Dashboard, Dispositivos (lista +
   detalhe), **Configuração total do dispositivo** (todas as seções da §7,
   incluindo os modais de confirmação), Membros (lista), **Perfil do membro +
   histórico** (§8) e Eventos.
3. **Cobertura de estados** (loading/vazio/erro) e **layouts responsivos**
   (desktop / tablet / mobile).
4. **Código front-end** (vanilla **ou** SPA com build), entregue como **assets
   estáticos embutíveis**, consumindo os contratos `/admin/api/*` (snake_case,
   keyset, cookie de sessão). Para as telas novas, projetar contra os dados das
   §6–§8 — os endpoints de backend serão criados em paralelo.

---

## 11. Prioridade sugerida

1. **Configuração total do dispositivo** (§7) — maior lacuna funcional hoje.
2. **Perfil do membro + histórico** (§8) — maior valor de visibilidade.
3. **Refino das telas existentes** (§4) sob a nova identidade.
