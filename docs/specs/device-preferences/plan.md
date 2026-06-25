# Implementation Plan — device-preferences

**Feature**: `device-preferences`
**Status**: Ready for create-tasks
**Spec**: [spec.md](./spec.md) (23 FRs) · **Research**: [research.md](./research.md) · **Data**: [data-model.md](./data-model.md)

## 1. Visão geral

`device-preferences` entrega as seções de **personalização e comportamento** do
terminal HikVision ao painel admin: modo de verificação, layout de tela, imagens de
branding (standby/boot/propaganda), estatísticas de capacidade e config avançada de
face. É um conjunto de **passthrough ISAPI síncronos** — sem nova persistência
(ver data-model.md). Estende três camadas já existentes seguindo seus padrões:

1. **Cliente ISAPI** (`internal/hikvision`) — novos métodos, split por área seguindo
   `client_users/faces/doors/system/verify/webhooks.go`.
2. **Handlers admin** (`internal/http`) — novos `*Handler(cfg DeviceConfigConfig) http.Handler`
   + roteamento manual em `adminDevicesRouter` / `server.go`.
3. **SPA** (`internal/web/dist`) — nova seção de preferências no painel de device,
   estendendo o padrão "aguardando backend" já presente.

**Princípio condutor**: reaproveitamento máximo. `Client.doRequest` (digest auth,
timeout), `loadDeviceAndISAPIClient` (resolução device+credenciais), `mapISAPIError`
(mapa de status), o template multipart de `UploadFace`, `deviceConfigPathSegments`
(roteamento) — todos reusados, nada duplicado.

## 2. Arquitetura por camada

### 2.1 Camada cliente ISAPI — `internal/hikvision`

Novos arquivos, split por área (mesma convenção dos existentes). Cada método segue o
contrato de `client_verify.go`: assinatura `func (c *Client) X(ctx, ...) (..., error)`,
chama `doRequest`, status != 2xx → `retriableOrNot`/`NonRetriableError`, comentário
**SOURCED** com `arquivo:linha` do legado.

| Arquivo novo | Métodos | Grupo / FR |
|--------------|---------|------------|
| `client_authmode.go` | `GetVerifyMode`, `SetVerifyMode` (read-modify-write genérico do verifyMode; generaliza `EnsureFaceVerifyMode`) | G1 / FR-001, FR-002 |
| `client_display.go` | `GetIdentityTerminal` (XML), `PutIdentityTerminal` (read-modify-write XML), `GetShowModeThumbnails` | G2 / FR-003, FR-004, FR-005 |
| `client_standby.go` | `ListStandbyPictures`, `UploadStandbyPicture` (multipart), `EnableCustomStandby`, `DisableCustomStandby`, `DeleteStandbyPicture` | G3 / FR-006–FR-009 |
| `client_bootpic.go` | `UploadBootPicture` (multipart), `DeleteBootPicture` | G4 / FR-010, FR-011 |
| `client_media.go` | `ListMaterials`, `CreateAdvertisingMedia` (fluxo 5 etapas), `DeleteMaterial`, `DeleteAllMaterials` (bulk) | G5 / FR-012–FR-015 |
| `client_stats.go` | `GetDeviceStats` (agrega 4 chamadas) | G6 / FR-016 |
| `client_faceconfig.go` | `SetFaceCompareCond` (XML, só `maxDistance` configurável), `CaptureFaceData` (XML→URL→download→bytes) | G7 / FR-017, FR-018 |

Notas:
- `client_authmode.go` deve **generalizar** `EnsureFaceVerifyMode` (já em
  `client_verify.go`) em vez de duplicá-lo — extrair o read-modify-write para aceitar
  qualquer `verifyMode`. `GetVerifyWeekPlan` já existe e é reusado por `GetVerifyMode`.
- `CaptureFaceData` reusa o helper interno `downloadImage` (em `client.go`) para baixar
  `faceDataUrl` antes de devolver os bytes ao handler (handler encoda base64 — dec-011).
- Uploads multipart usam o template de `UploadFace` (multipart.NewWriter + CreateFormField
  para JSON + CreatePart com Content-Disposition/Content-Type para o binário). **Sem
  transcodificação** para standby/boot/material (preservar qualidade de branding;
  validação de tipo no handler, tamanho é autoridade do firmware — research.md D5).

### 2.2 Camada handler admin — `internal/http`

Novo arquivo `admin_device_preferences_handlers.go` com construtores
`func XHandler(cfg DeviceConfigConfig) http.Handler`, reusando **a mesma
`DeviceConfigConfig`** (struct já existente, `admin_device_config_handlers.go:49`) — sem
nova config struct. Cada handler:
1. extrai `{id}` via `deviceConfigPathSegments`;
2. `loadDeviceAndISAPIClient(ctx, cfg, id)` (404 se device inexiste — FR-021);
3. valida sessão (já garantida pelo `sessionMW` no roteamento — FR-019);
4. chama o método do cliente; mapeia erro via `mapISAPIError` (504 timeout / 502 auth —
   FR-020); responde JSON;
5. loga via `cfg.logInfo/logError` (CPF mascarado, sem segredos/binário — FR-023).

| Handler | Método HTTP + path admin | FR |
|---------|--------------------------|----|
| `GetDeviceAuthModeHandler` | GET `/preferences/auth-mode` | FR-001 |
| `PutDeviceAuthModeHandler` | PUT `/preferences/auth-mode` | FR-002 |
| `GetDeviceDisplayHandler` | GET `/preferences/display` | FR-003 |
| `PutDeviceDisplayHandler` | PUT `/preferences/display` | FR-004 |
| `GetDeviceDisplayThumbnailsHandler` | GET `/preferences/display/thumbnails` | FR-005 |
| `GetDeviceStandbyPicturesHandler` | GET `/preferences/standby-pictures` | FR-006 |
| `PostDeviceStandbyPictureHandler` | POST `/preferences/standby-pictures` (multipart) | FR-007 |
| `DeleteDeviceStandbyPictureHandler` | DELETE `/preferences/standby-pictures/{uuid}` | FR-008 |
| `PutDeviceStandbyDisableHandler` | PUT `/preferences/standby-pictures/disable` | FR-009 |
| `PostDeviceBootPictureHandler` | POST `/preferences/boot-picture` (multipart) | FR-010 |
| `DeleteDeviceBootPictureHandler` | DELETE `/preferences/boot-picture` | FR-011 |
| `GetDeviceMediaHandler` | GET `/preferences/media` | FR-012 |
| `PostDeviceMediaHandler` | POST `/preferences/media` (multipart) | FR-013 |
| `DeleteDeviceMediaItemHandler` | DELETE `/preferences/media/{id}` | FR-014 |
| `DeleteDeviceMediaAllHandler` | DELETE `/preferences/media` (bulk) | FR-015 |
| `GetDeviceStatsHandler` | GET `/stats` | FR-016 |
| `PutDeviceFaceConfigHandler` | PUT `/preferences/face-config` | FR-017 |
| `PostDeviceFaceCaptureHandler` | POST `/preferences/face-capture` | FR-018 |

**Upload (FR-007/010/013, FR-022)**: handlers de upload parseiam `multipart/form-data`,
validam `Content-Type` do part `image/*` (FR-022 → 400 se inválido) **antes** de repassar
ao device; o tamanho é deixado para o firmware (research.md D5). Mensagem de erro de
rejeição de tamanho é acionável (research.md D5).

### 2.3 Roteamento — `server.go`

Estende `adminDevicesRouter` (`server.go:147`). O subtree `/admin/api/devices/{id}/`
ganha o segmento `preferences` (e o já-roteável `stats` no nível do device):

```
switch segs[0] {
  ...
  case "preferences":   // novo subtree, dispatch por segs[1]
    switch segs[1] {
      case "auth-mode":         GET→getAuthMode  PUT→putAuthMode
      case "display":           len==1: GET→getDisplay PUT→putDisplay
                                segs[2]=="thumbnails": GET→getThumbnails
      case "standby-pictures":  len==1: GET→list POST→upload
                                segs[2]=="disable": PUT→disable
                                else (uuid): DELETE→delete
      case "boot-picture":      POST→upload  DELETE→delete
      case "media":             len==1: GET→list POST→create DELETE→deleteAll
                                else (id): DELETE→deleteItem
      case "face-config":       PUT→setFaceConfig
      case "face-capture":      POST→capture
    }
  case "stats":          GET→getStats     // FR-016 (nível device, não sob /preferences)
}
```

Os handlers são construídos uma vez em `adminDevicesRouter` (como os de device-config) e
dispatchados por segmento + método. Método inesperado → `405`; segmento inesperado → `404`
(mesmo padrão atual). Todos sob `sessionMW` (FR-019, herdado de `server.go:102`).

### 2.4 SPA — `internal/web/dist`

Nova seção "Preferências" na tela de device do painel (`app.js`/`app.css`/`index.html`,
vanilla, embed). Estende o padrão da seção §7 "Configuração do dispositivo" (memória
`admin-spa-design-impl`): cards para verify-mode, layout, branding (upload/listar/remover),
stats, face-config. Onde uma operação ainda não tiver UI, degrada com "aguardando backend".
Identidade visual "Amber Terminal" preservada. (Detalhamento de UI fica para create-tasks /
front-end; o backend é a prioridade desta feature.)

## 3. Mapa FR → componente (cobertura completa)

| FR | Método cliente | Handler | Roteamento |
|----|----------------|---------|------------|
| FR-001 | `GetVerifyMode` | `GetDeviceAuthModeHandler` | `preferences/auth-mode` GET |
| FR-002 | `SetVerifyMode` (RMW) | `PutDeviceAuthModeHandler` | `preferences/auth-mode` PUT |
| FR-003 | `GetIdentityTerminal` | `GetDeviceDisplayHandler` | `preferences/display` GET |
| FR-004 | `PutIdentityTerminal` (RMW) | `PutDeviceDisplayHandler` | `preferences/display` PUT |
| FR-005 | `GetShowModeThumbnails` | `GetDeviceDisplayThumbnailsHandler` | `preferences/display/thumbnails` GET |
| FR-006 | `ListStandbyPictures` | `GetDeviceStandbyPicturesHandler` | `preferences/standby-pictures` GET |
| FR-007 | `UploadStandbyPicture` | `PostDeviceStandbyPictureHandler` | `preferences/standby-pictures` POST |
| FR-008 | `DeleteStandbyPicture` | `DeleteDeviceStandbyPictureHandler` | `preferences/standby-pictures/{uuid}` DELETE |
| FR-009 | `DisableCustomStandby` | `PutDeviceStandbyDisableHandler` | `preferences/standby-pictures/disable` PUT |
| FR-010 | `UploadBootPicture` | `PostDeviceBootPictureHandler` | `preferences/boot-picture` POST |
| FR-011 | `DeleteBootPicture` | `DeleteDeviceBootPictureHandler` | `preferences/boot-picture` DELETE |
| FR-012 | `ListMaterials` | `GetDeviceMediaHandler` | `preferences/media` GET |
| FR-013 | `CreateAdvertisingMedia` (5 etapas) | `PostDeviceMediaHandler` | `preferences/media` POST |
| FR-014 | `DeleteMaterial` | `DeleteDeviceMediaItemHandler` | `preferences/media/{id}` DELETE |
| FR-015 | `DeleteAllMaterials` | `DeleteDeviceMediaAllHandler` | `preferences/media` DELETE |
| FR-016 | `GetDeviceStats` (4 chamadas) | `GetDeviceStatsHandler` | `stats` GET |
| FR-017 | `SetFaceCompareCond` | `PutDeviceFaceConfigHandler` | `preferences/face-config` PUT |
| FR-018 | `CaptureFaceData` + download | `PostDeviceFaceCaptureHandler` | `preferences/face-capture` POST |
| FR-019 | — (sessão) | `sessionMW` no roteamento | todos os paths |
| FR-020 | `retriableOrNot`/`NonRetriableError` | `mapISAPIError` (504/502) | — |
| FR-021 | `loadDeviceAndISAPIClient` (pgx.ErrNoRows→404) | todos os handlers | — |
| FR-022 | validação `image/*` no parse multipart | handlers de upload | — |
| FR-023 | `cfg.logInfo/logError` (CPF mascarado, sem segredo) | todos | — |

## 4. Decisões de design herdadas (clarifications)

- **dec-010**: limite de imagem **não hardcodado**; firmware é autoridade de tamanho;
  mensagem de erro acionável (research.md D5).
- **dec-011**: `face-capture` retorna **base64 em JSON**, nunca a URL interna (research.md D7).
- **dec-012**: material órfão → resposta inclui `id` do material; DELETE individual +
  bulk para limpeza (research.md D6).
- **dec-017** (block-001): gestão de cartões **excluída**; `users.cards` em FR-016 é
  apenas leitura informativa do contador `bindCardUserNumber`.

## 5. Idempotência (Constitution II)

- AuthMode (FR-002), Display (FR-004), FaceCompareCond (FR-017): **read-modify-write** —
  reenviar a mesma config = mesmo estado, sem duplicar.
- Standby enable/disable (FR-009): estado-alvo fixo, idempotente por natureza.
- Uploads (FR-007/010/013): não idempotentes por design do firmware (cada upload cria
  nova entrada); a limpeza (DELETE) é a contramedida. Documentado, não mascarado.

## 6. Segurança (Constitution V — base para gate OWASP)

- **Auth**: todos os endpoints sob `sessionMW` (cookie HMAC) — FR-019.
- **Segredos**: senha ISAPI nunca em log/resposta/erro (`mapISAPIError` não ecoa
  credencial; logger mascara) — FR-023, SC-006.
- **SSRF / IP interno**: `face-capture` baixa a URL do device server-side e devolve
  base64 — nunca expõe IP/URL interno ao cliente (dec-011).
- **Upload**: validação `image/*` antes de repassar (FR-022); tamanho delegado ao
  firmware (sem buffer ilimitado — limitar leitura do body a um teto razoável no parse
  multipart, a definir em tasks, sem pré-validar limite por modelo).
- **404 antes de conectar**: device inexistente → 404 sem tentar ISAPI (FR-021).

### 6.1 Findings do gate owasp-security (dec-025 — medium, mitigar em tasks)

Gate `owasp-security` não reportou nenhum finding critical/high (auth e secret-leak já
cobertos por middleware existente). Dois findings **medium** viram controles obrigatórios
no create-tasks:

- **F1 — leitura ilimitada (A04 / API4, DoS por memória)**: o helper `downloadImage`
  (`client.go:384`) usa `io.ReadAll` sem limite, e o parse multipart dos uploads não tem
  teto de body. Mitigação em tasks: aplicar `http.MaxBytesReader` no body dos handlers de
  upload e `io.LimitReader` no download de `faceDataUrl` (FR-018). Teto generoso (não
  pré-valida limite por modelo — research.md D5), apenas barra exaustão.
- **F2 — SSRF na captura facial (API7)**: FR-018 baixa server-side a `faceDataUrl`
  **fornecida pelo device**; `downloadImage` não valida o host. Risco residual: device
  spoofado/comprometido devolve URL apontando para serviço interno. Mitigação em tasks:
  **restringir o fetch ao host do device** (validar que o host de `faceDataUrl` == host do
  device resolvido) antes de baixar. Já mitigado parcialmente: a URL nunca é exposta ao
  cliente (retorno base64 — dec-011) e o device está atrás de digest auth na LAN.

## 7. Testes (orientação para create-tasks)

- **Unit** (`*_test.go` por arquivo de cliente, sem firmware): serialização XML/JSON,
  mapeamento de showMode, read-modify-write preservando read-only, agregação de stats,
  validação de tipo de imagem, mascaramento de log. Mesma disciplina de
  `client_verify_test.go` / `admin_device_config_handlers_test.go`.
- **Idempotência** (Constitution II — exige teste): RMW de AuthMode/Display/Face reenviado
  duas vezes produz mesmo payload; enable/disable standby idempotente.
- **Integração** (`//go:build integration`, opcional): só se houver device de teste;
  não obrigatório para os contratos não exercitados (falha alta cobre o resto).

## 8. Fora de escopo

Idêntico à spec §Fora de Escopo: fluxo de presença, webhook provisioning, gestão de
usuários/faces (device-config), portas, SSE/alertStream, **gestão de cartões RFID/NFC**
(excluída permanentemente — dec-017).
