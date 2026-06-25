# Backlog de Tarefas: Preferências e Branding do Terminal HikVision

**Feature**: `device-preferences`
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)
**Created**: 2026-06-25
**Pipeline**: specify → clarify → plan → checklist → **create-tasks** → execute-task → review-task

**Legenda de status:**
- `[ ]` Pendente
- `[~]` Em andamento
- `[x]` Concluído
- `[!]` Bloqueado

**Legenda de criticidade:**
- `[C]` Crítico — impacto de segurança (SSRF, DoS por leitura ilimitada, senha ISAPI)
- `[A]` Alto — funcionalidade core sem a qual o sistema não opera (cliente ISAPI, handlers, roteamento)
- `[M]` Médio — necessário mas não bloqueia o caminho principal (UX refinements, SPA avançada)

---

## Matriz de Dependências

| Task | Depende de |
|------|-----------|
| FASE 2 (handlers) | FASE 1 (cliente ISAPI) |
| FASE 3 (roteamento) | FASE 2 (handlers) |
| FASE 4 (SPA) | FASE 3 (roteamento funcionando) |
| 1.7 (client_authmode — SetVerifyMode RMW) | 1.6 (GetVerifyMode) |
| 1.9 (client_standby — UploadStandbyPicture) | 1.8 (ListStandbyPictures) |
| 2.2 (PutDeviceAuthModeHandler) | 2.1 (GetDeviceAuthModeHandler) |
| 2.4 (PutDeviceDisplayHandler — RMW) | 2.3 (GetDeviceDisplayHandler) |
| 2.8 (PostDeviceStandbyPictureHandler) | 2.7 (GetDeviceStandbyPicturesHandler) |
| 2.13 (PostDeviceMediaHandler — 5 etapas) | 2.12 (GetDeviceMediaHandler) |
| 1.10 (client_bootpic) | — independente |
| 1.11 (client_media) | — independente |
| 1.12 (client_stats) | — independente |
| 1.13 (client_faceconfig) | — independente |
| CHK022/025 sec-task (1.14) | 1.8–1.10, 1.13 (multipart helpers prontos) |
| CHK023 sec-task (1.15) | 1.13 (CaptureFaceData pronto) |

---

## Resumo

| Fase | Descrição | Tasks | Criticidade dominante |
|------|-----------|-------|----------------------|
| FASE 1 | Cliente ISAPI — novos métodos Go | 1.1–1.15 (15 tasks) | A + 2C (segurança) |
| FASE 2 | Handlers admin HTTP | 2.1–2.18 (18 tasks) | A |
| FASE 3 | Roteamento `server.go` | 3.1 (1 task) | A |
| FASE 4 | SPA — painel de preferências | 4.1–4.5 (5 tasks) | M |

**Total**: 39 tasks distribuídas em 4 fases.

---

## Escopo Coberto

- Todos os 23 FRs da spec (FR-001 a FR-023)
- 18 novos endpoints admin (`/preferences/*` + `/stats`)
- 7 novos arquivos de cliente ISAPI (`client_authmode`, `_display`, `_standby`, `_bootpic`, `_media`, `_stats`, `_faceconfig`)
- Arquivo novo de handlers (`admin_device_preferences_handlers.go`)
- Extensão do roteamento (`server.go` — `adminDevicesRouter`)
- SPA: nova seção "Preferências" no painel de device
- 2 tasks de segurança obrigatórias (gaps CHK022/023/025 do checklist)

## Escopo Excluído

- Gestão de cartões RFID/NFC (excluída permanentemente — dec-017 / block-001)
- Schema de banco de dados (feature stateless — sem migrations)
- Fluxo de presença (scheduler, worker, webhook de reconhecimento facial)
- Provisionamento de webhook (já implementado em `device-config`)
- Stream de eventos em tempo real (SSE/alertStream)
- Testes de integração com device real (opcional; unit tests cobrem os contratos)

---

## FASE 1 - Cliente ISAPI: novos métodos Go

> Todos os novos métodos Go em `internal/hikvision/`. Reusam `doRequest`, digest auth e
> `NonRetriableError` existentes. Precedem os handlers. Cada método inclui comentário
> SOURCED com `arquivo:linha` do legado `legacy/hik2go/`.

### 1.1 `client_authmode.go`: GetVerifyMode `[A]`

Ref: spec.md §FR-001, plan.md §2.1, SOURCED `Hik2go/Preferences/AuthMode.php`

- [x] 1.1.1 Criar `internal/hikvision/client_authmode.go` com struct `VerifyWeekPlan {WeekPlanCfgs []WeekPlanCfg}` e `WeekPlanCfg {WeekNo int; VerifyMode string}`
- [x] 1.1.2 Implementar `(c *Client) GetVerifyMode(ctx context.Context) (*VerifyWeekPlan, error)` — `GET /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json` (SOURCED AuthMode.php)
- [x] 1.1.3 Escrever testes unitários: `GetVerifyMode` parseia JSON de exemplo com 7 planos semanais; erro 401 → `NonRetriableError`

### 1.2 `client_authmode.go`: SetVerifyMode (read-modify-write) `[A]`

Ref: spec.md §FR-002, plan.md §2.1 nota "generalizar EnsureFaceVerifyMode", SOURCED `AuthMode.php`

Dependência: 1.1 concluída.

- [x] 1.2.1 Implementar `(c *Client) SetVerifyMode(ctx context.Context, mode string) error` — lê o plano atual via `GetVerifyMode`, substitui `verifyMode` em todos os `WeekPlanCfg`, envia `PUT /ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json` com o plano completo
- [x] 1.2.2 **Teste de idempotência obrigatório** (Constitution II): chamar `SetVerifyMode` duas vezes com o mesmo modo produz payload idêntico; reprocessar não corrompe o plano
- [x] 1.2.3 Escrever teste: `SetVerifyMode("face")` monta o body com todos os 7 planos e `verifyMode: "face"` em cada um; teste de payload vs mock

### 1.3 `client_display.go`: GetIdentityTerminal `[A]`

Ref: spec.md §FR-003, plan.md §2.1, SOURCED `Hik2go/Preferences/IdentityTerminal.php`

- [x] 1.3.1 Criar `internal/hikvision/client_display.go` com struct `IdentityTerminalDisplay {ShowMode string; ScreenOffTimeout int; PreviewShowTime int; StandbyTimeout int; ReadOnlyFields map[string]interface{}}` (campos read-only preservados como raw para o RMW)
- [x] 1.3.2 Implementar `(c *Client) GetIdentityTerminal(ctx context.Context) (*IdentityTerminalDisplay, error)` — `GET /ISAPI/AccessControl/IdentityTerminal` (XML sem `?format=json`); parsear os campos mapeados preservando os read-only (SOURCED IdentityTerminal.php)
- [x] 1.3.3 Escrever testes: `GetIdentityTerminal` parseia XML com `showMode=normal` e todos os campos read-only presentes; erro 401 → `NonRetriableError`

### 1.4 `client_display.go`: PutIdentityTerminal (read-modify-write) `[A]`

Ref: spec.md §FR-004, plan.md §2.1, SOURCED `IdentityTerminal.php`

Dependência: 1.3 concluída. Edge case: PUT sem leitura prévia do `version` → firmware rejeita.

- [x] 1.4.1 Implementar `(c *Client) PutIdentityTerminal(ctx context.Context, screenOffTimeout, previewShowTime, standbyTimeout int, showMode string) error` — lê via `GetIdentityTerminal`, aplica os valores configuráveis, mapeia `showMode` (normal→normal/full; full→advertising/full; split→advertising/split), preserva todos os campos read-only e o atributo `version` do XML raiz, envia `PUT /ISAPI/AccessControl/IdentityTerminal` com `Content-Type: application/xml`
- [x] 1.4.2 **Teste de idempotência obrigatório** (Constitution II): `PutIdentityTerminal` com mesmos valores produz XML idêntico; campos read-only não são alterados
- [x] 1.4.3 Escrever testes: mapeamento de `showMode` (`"normal"` → `showMode=normal advertisingDisplayType=full`; `"full"` → `showMode=advertising advertisingDisplayType=full`; `"split"` → `showMode=advertising advertisingDisplayType=split`); campos read-only preservados no XML de saída

### 1.5 `client_display.go`: GetShowModeThumbnails `[M]`

Ref: spec.md §FR-005, plan.md §2.1, SOURCED `IdentityTerminal.php`

- [x] 1.5.1 Implementar `(c *Client) GetShowModeThumbnails(ctx context.Context) (interface{}, error)` — `GET /ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json`; retorna o JSON bruto passado ao handler
- [x] 1.5.2 Escrever teste: `GetShowModeThumbnails` retorna payload não-nulo para 200; erro 404 (não suportado pelo firmware) retorna erro rastreável

### 1.6 `client_standby.go`: ListStandbyPictures `[A]`

Ref: spec.md §FR-006, plan.md §2.1, SOURCED `Hik2go/Preferences/StandbyPicture.php`

- [x] 1.6.1 Criar `internal/hikvision/client_standby.go` com struct `StandbyPicture {UUID string; FileName string}`
- [x] 1.6.2 Implementar `(c *Client) ListStandbyPictures(ctx context.Context) ([]StandbyPicture, error)` — `GET /ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json` (SOURCED StandbyPicture.php)
- [x] 1.6.3 Escrever testes: lista de 2 imagens com UUID e FileName corretos; lista vazia retorna slice vazio (não nil)

### 1.7 `client_standby.go`: UploadStandbyPicture, EnableCustomStandby, DisableCustomStandby, DeleteStandbyPicture `[A]`

Ref: spec.md §FR-007, §FR-008, §FR-009, plan.md §2.1, SOURCED `StandbyPicture.php`

Dependência: 1.6 concluída.

- [x] 1.7.1 Implementar `(c *Client) UploadStandbyPicture(ctx context.Context, filename string, data []byte) error` — multipart `POST /ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json` com campo `UploadCustomStandbyPic` (JSON: `{filePathType:"multipart", filePath:<filename>}`) e campo `filePath` (binário); usa template multipart de `UploadFace` como referência (SOURCED StandbyPicture.php)
- [x] 1.7.2 Implementar `(c *Client) EnableCustomStandby(ctx context.Context) error` — `PUT /ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` body `{standbyPicType:"custom", displayEffect:"stretch", switchingTime:20}`
- [x] 1.7.3 Implementar `(c *Client) DisableCustomStandby(ctx context.Context) error` — mesma rota com body `{standbyPicType:"default", displayEffect:"stretch", switchingTime:20}`
- [x] 1.7.4 Implementar `(c *Client) DeleteStandbyPicture(ctx context.Context, uuid string) error` — `POST /ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json` body `{customStandbyPicUUIDList:[{customStandbyPicUUID:"<uuid>"}]}`
- [x] 1.7.5 **Teste de idempotência obrigatório** (Constitution II): `DisableCustomStandby` chamado duas vezes produz o mesmo estado-alvo no device (body idêntico)
- [x] 1.7.6 Escrever testes unitários: upload multipart monta os campos corretos (UploadCustomStandbyPic JSON + filePath binário); `DeleteStandbyPicture` envia o UUID correto no body; enable/disable enviam os bodies distintos corretos

### 1.8 `client_bootpic.go`: UploadBootPicture, DeleteBootPicture `[A]`

Ref: spec.md §FR-010, §FR-011, plan.md §2.1, SOURCED `Hik2go/Preferences/InitializationScreen.php`

- [x] 1.8.1 Criar `internal/hikvision/client_bootpic.go`
- [x] 1.8.2 Implementar `(c *Client) UploadBootPicture(ctx context.Context, deviceID int64, data []byte) error` — multipart `POST /ISAPI/System/powerUpPicture?format=json` com campo `picture_info` (JSON: `{type:"filePathType", faceLibType:"binay"}`) e campo `picture_name` (binário JPEG com nome `<deviceID>.jpg`); SOURCED `InitializationScreen.php`
- [x] 1.8.3 Implementar `(c *Client) DeleteBootPicture(ctx context.Context) error` — `DELETE /ISAPI/System/powerUpPicture?format=json`
- [x] 1.8.4 Escrever testes: upload monta multipart com campos exatos (`picture_info` JSON + `picture_name` binário com nome correto); delete faz DELETE no path correto; erro 404 firmware (imagem inexistente) → erro rastreável

### 1.9 `client_media.go`: ListMaterials, CreateAdvertisingMedia (5 etapas), DeleteMaterial, DeleteAllMaterials `[A]`

Ref: spec.md §FR-012, §FR-013, §FR-014, §FR-015, plan.md §2.1, SOURCED `Hik2go/Preferences/Media.php` + `Presentation.php`

- [x] 1.9.1 Criar `internal/hikvision/client_media.go` com struct `Material {ID string; Name string}` e `AdvertisingMediaResult {MaterialID string; ProgramID string; ScheduleID string}`
- [x] 1.9.2 Implementar `(c *Client) ListMaterials(ctx context.Context) ([]Material, error)` — `GET /ISAPI/Publish/MaterialMgr/material` (SOURCED Media.php)
- [x] 1.9.3 Implementar `(c *Client) CreateAdvertisingMedia(ctx context.Context, filename string, data []byte) (*AdvertisingMediaResult, error)` — executa as 5 etapas sequenciais (a)-(e) do FR-013: (a) POST Material XML; (b) POST upload binário; (c) POST Program XML; (d) PUT Page XML; (e) PUT Schedule XML; retorna IDs criados ou erro com etapa falha + `OrphanMaterialID` (Clarification 4)
- [x] 1.9.4 Implementar `(c *Client) DeleteMaterial(ctx context.Context, id string) error` — `DELETE /ISAPI/Publish/MaterialMgr/material/{id}` (SOURCED Media.php)
- [x] 1.9.5 Implementar `(c *Client) DeleteAllMaterials(ctx context.Context) error` — lista via `ListMaterials` e remove cada um via `DeleteMaterial`; erro individual não bloqueia os demais (falhar alto no final)
- [x] 1.9.6 Escrever testes: `CreateAdvertisingMedia` simula falha na etapa (c) → retorna erro com `OrphanMaterialID` preenchido; `DeleteAllMaterials` com 3 materiais chama Delete 3 vezes; `ListMaterials` parseia XML de exemplo

### 1.10 `client_stats.go`: GetDeviceStats `[A]`

Ref: spec.md §FR-016, plan.md §2.1, SOURCED `Hik2go/Stats.php`

- [x] 1.10.1 Criar `internal/hikvision/client_stats.go` com struct `DeviceStats {Users UserStats; Events EventStats}`, `UserStats {Total int; Faces int; Cards int; Max int}`, `EventStats {Total int; Max int}`
- [x] 1.10.2 Implementar `(c *Client) GetDeviceStats(ctx context.Context) (*DeviceStats, error)` — executa as 4 chamadas ISAPI em paralelo ou sequencial: (1) `GET /ISAPI/AccessControl/UserInfo/Count?format=json` → `users.total/faces/cards`; (2) `GET /ISAPI/AccessControl/UserInfo/capabilities?format=json` → `users.max`; (3) `POST /ISAPI/AccessControl/AcsEventTotalNum?format=json` body `{AcsEventTotalNumCond:{major:0, minor:0}}` → `events.total`; (4) `GET /ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json` → `events.max`; mapeia campos conforme FR-016 (SOURCED Stats.php)
- [x] 1.10.3 Escrever testes: `GetDeviceStats` agrega os 4 payloads mock em `DeviceStats` com todos os campos corretos; erro numa das 4 chamadas → retorna erro com indicação de qual falhou

### 1.11 `client_faceconfig.go`: SetFaceCompareCond, CaptureFaceData `[A]`

Ref: spec.md §FR-017, §FR-018, plan.md §2.1, SOURCED `Hik2go/Face.php`

- [x] 1.11.1 Criar `internal/hikvision/client_faceconfig.go`
- [x] 1.11.2 Implementar `(c *Client) SetFaceCompareCond(ctx context.Context, maxDistance float64) error` — `PUT /ISAPI/AccessControl/FaceCompareCond` com XML `<FaceCompareCond version="2.0">` contendo `maxDistance` e os demais campos com valores fixos: pitch=45, yaw=45, leftBorder=0, rightBorder=0, upBorder=0, bottomBorder=0, faceScore=0, faceScoreThreshold1=0, ROIRegionMode=manual (SOURCED Face.php)
- [x] 1.11.3 **Teste de idempotência obrigatório** (Constitution II): `SetFaceCompareCond` com mesma `maxDistance` produz XML idêntico; campos fixos não variam entre chamadas
- [x] 1.11.4 Implementar `(c *Client) CaptureFaceData(ctx context.Context) ([]byte, error)` — `POST /ISAPI/AccessControl/CaptureFaceData` com XML body `<CaptureFaceDataCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema"><captureInfrared>false</captureInfrared><dataType>url</dataType></CaptureFaceDataCond>`; extrai `faceDataUrl` da resposta; **valida que o host de `faceDataUrl` == host do device** antes de baixar (mitigação SSRF — CHK023); faz download via `downloadImage` + `io.LimitReader` (limitado a 10 MB — mitigação DoS F1); retorna bytes da imagem (SOURCED Face.php)
- [x] 1.11.5 Escrever testes: `SetFaceCompareCond(1.5)` produz XML com `maxDistance=1.5` e todos os campos fixos corretos; `CaptureFaceData` com `faceDataUrl` para host diferente do device → retorna erro `ErrSSRFHostMismatch`

### 1.12 Controles de segurança: cap de body em uploads e LimitReader em downloads `[C]`

Ref: plan.md §6.1 F1 (DoS/leitura ilimitada A04/API4), checklist CHK022/CHK025

Critério de aceite obrigatório e verificável: (a) body dos handlers de upload limitado a **20 MB** via `http.MaxBytesReader` antes de parse multipart; (b) download de `faceDataUrl` limitado a **10 MB** via `io.LimitReader` em `CaptureFaceData` (já coberto por 1.11.4).

- [x] 1.12.1 Definir constante `maxUploadBodyBytes = 20 * 1024 * 1024` em `admin_device_preferences_handlers.go` (20 MB — teto generoso, sem pré-validar limite por modelo conforme dec-010; apenas barra exaustão de memória)
- [x] 1.12.2 Em cada handler de upload (PostDeviceStandbyPictureHandler, PostDeviceBootPictureHandler, PostDeviceMediaHandler): aplicar `http.MaxBytesReader(w, r.Body, maxUploadBodyBytes)` antes de `r.ParseMultipartForm`; retornar `413 Request Entity Too Large` com mensagem acionável se body exceder o teto
- [x] 1.12.3 Verificar que `downloadImage` em `client.go` já usa `io.LimitReader` ou adicionar limite de 10 MB no call de `CaptureFaceData` (se `downloadImage` não tiver limite próprio — checar `client.go:384`)
- [x] 1.12.4 Escrever testes: POST de standby picture com body > 20 MB → 413 com mensagem acionável; POST de boot picture com body exato de 20 MB → processado normalmente

### 1.13 Controle de segurança: validação de host anti-SSRF em CaptureFaceData `[C]`

Ref: plan.md §6.1 F2 (SSRF API7), checklist CHK023

Critério de aceite obrigatório e verificável: o host de `faceDataUrl` retornada pelo device DEVE corresponder ao host do device (campo `ip` ou `host` em `devices`); host diferente → retornar `ErrSSRFHostMismatch` sem executar o download; teste unitário com URL de host divergente confirma o bloqueio.

- [x] 1.13.1 Implementar helper `validateFaceDataURL(faceDataURL, deviceHost string) error` em `client_faceconfig.go`: parseia `faceDataURL`, compara `host` (sem porta) com `deviceHost`; retorna `ErrSSRFHostMismatch` se diferente; retorna nil se igual
- [x] 1.13.2 Chamar `validateFaceDataURL` em `CaptureFaceData` antes de invocar `downloadImage`; logar a tentativa bloqueada via `log_err` com `device_id` e host inválido (sem URL completa para evitar log de IP interno em contextos auditados)
- [x] 1.13.3 Escrever testes: `validateFaceDataURL("http://192.168.68.107/img.jpg", "192.168.68.107")` → nil; `validateFaceDataURL("http://10.0.0.1/img.jpg", "192.168.68.107")` → `ErrSSRFHostMismatch`; `validateFaceDataURL("http://internal-service/secret", "192.168.68.107")` → `ErrSSRFHostMismatch`

### 1.14 Validação de tipo de imagem `image/*` nos 3 mecanismos de upload `[A]`

Ref: spec.md §FR-022, plan.md §2.2

- [x] 1.14.1 Implementar helper `validateImageContentType(contentType string) error` em `admin_device_preferences_handlers.go`: retorna erro se `contentType` não começa com `"image/"`; retorna nil se válido
- [x] 1.14.2 Aplicar `validateImageContentType` nos handlers PostDeviceStandbyPictureHandler, PostDeviceBootPictureHandler e PostDeviceMediaHandler: validar o `Content-Type` do part binário após parse multipart; retornar `400 Bad Request` com mensagem `"arquivo deve ser imagem (image/*)"` se inválido
- [x] 1.14.3 Escrever testes: upload com `Content-Type: application/octet-stream` → 400; upload com `Content-Type: image/png` → processado; upload com `Content-Type: image/jpeg` → processado

---

## FASE 2 - Handlers HTTP Admin

> Arquivo novo `internal/http/admin_device_preferences_handlers.go`.
> Reutiliza `DeviceConfigConfig` (struct existente), `loadDeviceAndISAPIClient`,
> `mapISAPIError`, `cfg.logInfo/logError`. Todos exigem sessão admin válida (garantida
> pelo `sessionMW` no roteamento — FR-019).

### 2.1 GetDeviceAuthModeHandler + PutDeviceAuthModeHandler `[A]`

Ref: spec.md §FR-001, §FR-002, plan.md §2.2

- [x] 2.1.1 Criar `admin_device_preferences_handlers.go` com `GetDeviceAuthModeHandler(cfg DeviceConfigConfig) http.Handler`: extrai `{id}` via `deviceConfigPathSegments`, chama `loadDeviceAndISAPIClient`, chama `client.GetVerifyMode(ctx)`, responde JSON com o plano semanal completo (shape: `{weekPlans:[{weekNo, verifyMode}]}` — decisão inline consistente com padrão de passthrough)
- [x] 2.1.2 Criar `PutDeviceAuthModeHandler(cfg DeviceConfigConfig) http.Handler`: decode `{verifyMode string}` do body, valida não-vazio, chama `client.SetVerifyMode(ctx, mode)`, responde `{result:"ok", device_id:N}`
- [x] 2.1.3 Log estruturado em ambos: `device_id`, `stage: "auth-mode"`, sem senha ISAPI (FR-023)
- [x] 2.1.4 Escrever testes: GET retorna `{weekPlans:[...]}` com mock; PUT com `verifyMode` vazio → 400; PUT com device inexistente → 404; timeout ISAPI → 504

### 2.2 GetDeviceDisplayHandler + PutDeviceDisplayHandler + GetDeviceDisplayThumbnailsHandler `[A]`

Ref: spec.md §FR-003, §FR-004, §FR-005, plan.md §2.2

- [x] 2.2.1 Criar `GetDeviceDisplayHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.GetIdentityTerminal(ctx)`, responde JSON com campos mapeados: `{showMode, screenOffTimeout, previewShowTime, standbyTimeout}`
- [x] 2.2.2 Criar `PutDeviceDisplayHandler(cfg DeviceConfigConfig) http.Handler`: decode `{showMode string; screenOffTimeout int; previewShowTime int; standbyTimeout int}`, valida `showMode ∈ {normal, full, split}` (400 se inválido), chama `client.PutIdentityTerminal(ctx, ...)`, responde `{result:"ok", device_id:N}`
- [x] 2.2.3 Criar `GetDeviceDisplayThumbnailsHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.GetShowModeThumbnails(ctx)`, responde o JSON da ISAPI diretamente
- [x] 2.2.4 Escrever testes: GET display retorna os 4 campos corretos; PUT com `showMode: "invalid"` → 400; timeout → 504

### 2.3 GetDeviceStandbyPicturesHandler + PostDeviceStandbyPictureHandler + DeleteDeviceStandbyPictureHandler + PutDeviceStandbyDisableHandler `[A]`

Ref: spec.md §FR-006, §FR-007, §FR-008, §FR-009, plan.md §2.2

- [x] 2.3.1 Criar `GetDeviceStandbyPicturesHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.ListStandbyPictures(ctx)`, responde `{pictures:[{uuid, fileName}], total:N}`
- [x] 2.3.2 Criar `PostDeviceStandbyPictureHandler(cfg DeviceConfigConfig) http.Handler`: aplica `http.MaxBytesReader` (constante de 1.12.1), parseia multipart, valida `image/*` (helper de 1.14.1), extrai filename e binário, chama `client.UploadStandbyPicture(ctx, filename, data)` seguido de `client.EnableCustomStandby(ctx)`; responde `{result:"uploaded_and_enabled", device_id:N}`; falha na ativação é reportada mesmo com upload bem-sucedido
- [x] 2.3.3 Criar `DeleteDeviceStandbyPictureHandler(cfg DeviceConfigConfig) http.Handler`: extrai `{uuid}` do path (segmento após `standby-pictures/`; validar não-vazio e sem `/` nem `..`); chama `client.DeleteStandbyPicture(ctx, uuid)`; responde `{result:"deleted", device_id:N}`
- [x] 2.3.4 Criar `PutDeviceStandbyDisableHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.DisableCustomStandby(ctx)`, responde `{result:"disabled", device_id:N}`
- [x] 2.3.5 Escrever testes: POST sem file → 400; POST com tipo inválido → 400; POST com body > 20 MB → 413; DELETE com uuid vazio → 400 ou 404; PUT disable → 200

### 2.4 PostDeviceBootPictureHandler + DeleteDeviceBootPictureHandler `[A]`

Ref: spec.md §FR-010, §FR-011, plan.md §2.2

- [x] 2.4.1 Criar `PostDeviceBootPictureHandler(cfg DeviceConfigConfig) http.Handler`: aplica `http.MaxBytesReader`, parseia multipart JPEG, valida `image/*`, chama `client.UploadBootPicture(ctx, deviceID, data)`, responde `{result:"uploaded", device_id:N}`; mensagem de erro de rejeição de tamanho pelo firmware é repassada ao operador com contexto acionável
- [x] 2.4.2 Criar `DeleteDeviceBootPictureHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.DeleteBootPicture(ctx)`, responde `{result:"deleted", device_id:N}`
- [x] 2.4.3 Escrever testes: POST com arquivo não-JPEG (mas image/png) → aceito pelo handler (validação é `image/*`, não exclusivo JPEG); POST com `Content-Type: text/plain` → 400

### 2.5 GetDeviceMediaHandler + PostDeviceMediaHandler + DeleteDeviceMediaItemHandler + DeleteDeviceMediaAllHandler `[A]`

Ref: spec.md §FR-012, §FR-013, §FR-014, §FR-015, plan.md §2.2

- [x] 2.5.1 Criar `GetDeviceMediaHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.ListMaterials(ctx)`, responde `{materials:[{id, name}], total:N}`
- [x] 2.5.2 Criar `PostDeviceMediaHandler(cfg DeviceConfigConfig) http.Handler`: aplica `http.MaxBytesReader`, parseia multipart, valida `image/*`, chama `client.CreateAdvertisingMedia(ctx, filename, data)`; resposta de sucesso: `{result:"created", materialId, programId, scheduleId, device_id:N}`; resposta de erro com material órfão: `{error:"...", stage:"<etapa falhou>", orphanMaterialId:"<id>", device_id:N}` (Clarification 4 — dec-012)
- [x] 2.5.3 Criar `DeleteDeviceMediaItemHandler(cfg DeviceConfigConfig) http.Handler`: extrai `{id}` do path, chama `client.DeleteMaterial(ctx, id)`, responde `{result:"deleted", device_id:N}`
- [x] 2.5.4 Criar `DeleteDeviceMediaAllHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.DeleteAllMaterials(ctx)`, responde `{result:"all_deleted", device_id:N}`
- [x] 2.5.5 Escrever testes: POST com imagem → resposta com os 3 IDs; POST falha na etapa (c) → resposta com `orphanMaterialId`; DELETE item → 200; DELETE all → 200

### 2.6 GetDeviceStatsHandler `[A]`

Ref: spec.md §FR-016, plan.md §2.2

- [x] 2.6.1 Criar `GetDeviceStatsHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.GetDeviceStats(ctx)`, responde `{users:{total, faces, cards, max}, events:{total, max}}`; device offline → 504 ou 502 (via `mapISAPIError`), NUNCA retorna zeros por padrão (US4-AC2: zero e "indisponível" são distintos)
- [x] 2.6.2 Escrever testes: GET stats agrega os 6 campos corretos de mock; device offline → 504, não retorna zeros; erro de auth → 502

### 2.7 PutDeviceFaceConfigHandler + PostDeviceFaceCaptureHandler `[A]`

Ref: spec.md §FR-017, §FR-018, plan.md §2.2

- [x] 2.7.1 Criar `PutDeviceFaceConfigHandler(cfg DeviceConfigConfig) http.Handler`: decode `{maxDistance float64}` do body, valida `maxDistance > 0`, chama `client.SetFaceCompareCond(ctx, maxDistance)`, responde `{result:"ok", device_id:N}`
- [x] 2.7.2 Criar `PostDeviceFaceCaptureHandler(cfg DeviceConfigConfig) http.Handler`: chama `client.CaptureFaceData(ctx)`, encoda os bytes resultantes em base64, responde `{image:"<base64>", device_id:N}` (dec-011 — nunca expõe URL/IP interno)
- [x] 2.7.3 Escrever testes: PUT com `maxDistance: -1` → 400; PUT com `maxDistance: 1.5` → 200; POST face-capture retorna base64 válido (não URL); erro SSRF (host diferente) → 502 com mensagem de segurança

### 2.8 Infraestrutura transversal dos handlers `[A]`

Ref: spec.md §FR-019, §FR-020, §FR-021, §FR-023, plan.md §2.2

- [x] 2.8.1 Garantir que todos os 18 handlers usam `loadDeviceAndISAPIClient` → `404` para device inexistente (FR-021); o factory dos handlers está em `admin_device_preferences_handlers.go`
- [x] 2.8.2 Garantir que todos os handlers mapeiam erros via `mapISAPIError`: timeout → 504, auth inválida (digest 401) → 502 sem expor senha (FR-020, SC-006)
- [x] 2.8.3 Garantir log estruturado em todos os handlers: campos `device_id` e `stage` presentes; senha ISAPI, tokens e conteúdo binário NUNCA logados (FR-023)
- [x] 2.8.4 Escrever teste transversal: para qualquer handler da feature, device com `id = 9999` (inexistente) → 404 sem tentar ISAPI; timeout → 504; password nunca aparece em log ou response body

---

## FASE 3 - Roteamento server.go

> Extensão de `adminDevicesRouter` em `internal/http/server.go`.
> Nenhum novo middleware — todos os paths herdam `sessionMW` existente (FR-019).

### 3.1 Registrar subtree `preferences` e rota `stats` no adminDevicesRouter `[A]`

Ref: plan.md §2.3, spec.md §FR-001 a §FR-018

- [x] 3.1.1 Em `server.go`, estender o `switch segs[0]` de `adminDevicesRouter` com `case "preferences"` + `case "stats"`
- [x] 3.1.2 Implementar switch de `segs[1]` dentro de `preferences` cobrindo todos os subpaths do plan.md §2.3: `auth-mode`, `display` (com `thumbnails`), `standby-pictures` (com `disable` e `{uuid}`), `boot-picture`, `media` (com `{id}` e bulk), `face-config`, `face-capture`
- [x] 3.1.3 Garantir `405 Method Not Allowed` para métodos inesperados e `404` para segmentos desconhecidos (padrão existente)
- [x] 3.1.4 Construir os handlers no `adminDevicesRouter` uma única vez (variáveis locais `getAuthMode := GetDeviceAuthModeHandler(cfg)`, etc.) seguindo o padrão existente
- [x] 3.1.5 Escrever testes de roteamento: `GET /admin/api/devices/1/preferences/auth-mode` → 200; `PUT /admin/api/devices/1/preferences/display` → 200; `PATCH /admin/api/devices/1/preferences/auth-mode` → 405; `GET /admin/api/devices/1/preferences/nao-existe` → 404; `GET /admin/api/devices/1/stats` → 200

---

## FASE 4 - SPA: painel de preferências

> Extensão do painel admin em `internal/web/dist` (vanilla JS, embed.FS).
> Estende a seção §7 "Configuração do dispositivo" seguindo o padrão "Amber Terminal".
> Backend é prioridade; telas complexas (media/face-capture) podem iniciar com
> "aguardando backend" até FASE 3 estar concluída.

### 4.1 Seção "Modo de Verificação" (Verify Mode) `[M]`

Ref: spec.md §US1, §FR-001, §FR-002, §SC-001

- [ ] 4.1.1 Adicionar card "Modo de Verificação" na tela de device (`app.js`): dropdown com opções de `verifyMode` (face, card, pin, face_or_card, card_or_pin, face_or_card_or_pin, etc. — conforme lista retornada pelo GET)
- [ ] 4.1.2 Ao abrir o card, buscar `GET /admin/api/devices/{id}/preferences/auth-mode` e pré-popular o dropdown com o modo atual
- [ ] 4.1.3 Botão "Salvar": envia `PUT /admin/api/devices/{id}/preferences/auth-mode` com `{verifyMode}` selecionado; exibe sucesso ou mensagem de erro acionável

### 4.2 Seção "Layout de Tela" (Display) `[M]`

Ref: spec.md §US2, §FR-003, §FR-004, §FR-005, §SC-002

- [ ] 4.2.1 Adicionar card "Layout de Tela": 3 botões/tabs para `normal` / `full` / `split`; 3 sliders/inputs numéricos para `screenOffTimeout`, `previewShowTime`, `standbyTimeout`
- [ ] 4.2.2 Ao abrir, buscar `GET .../preferences/display` e pré-popular os controles; opcionalmente carregar thumbnails via `GET .../preferences/display/thumbnails` para preview visual
- [ ] 4.2.3 Botão "Salvar": envia `PUT .../preferences/display`; exibe sucesso ou erro do firmware (ex: valor de timeout rejeitado)

### 4.3 Seção "Imagens de Branding" (Standby + Boot) `[M]`

Ref: spec.md §US3, §FR-006 a §FR-011, §SC-003

- [ ] 4.3.1 Adicionar card "Standby Picture": lista de imagens (`GET .../preferences/standby-pictures`), botão de upload (input file com `accept="image/*"`, envia `POST` multipart), botão de remover por UUID, botão "Desativar standby customizado"
- [ ] 4.3.2 Adicionar card "Boot Logo": botão de upload JPEG, botão de remover; mensagem de erro de rejeição de tamanho pelo firmware exibida de forma acionável
- [ ] 4.3.3 Feedback de operações de upload: spinner durante upload + mensagem de sucesso/erro; NUNCA exibir silêncio para falha

### 4.4 Seção "Estatísticas de Capacidade" `[M]`

Ref: spec.md §US4, §FR-016, §SC-004, §SC-005

- [ ] 4.4.1 Adicionar card "Estatísticas" com 6 counters: `Usuários (total/max)`, `Faces (total)`, `Cartões (total)`, `Eventos (total/max)` — todos buscados de `GET /admin/api/devices/{id}/stats`
- [ ] 4.4.2 Device offline → exibir "Indisponível" em cada counter, NUNCA zero por padrão (US4-AC2)
- [ ] 4.4.3 Botão de refresh; exibição da última atualização em timestamp

### 4.5 Seção "Configuração Avançada de Face" `[M]`

Ref: spec.md §US5, §FR-017, §FR-018, §SC-001

- [ ] 4.5.1 Adicionar card "Face Avançado": input numérico para `maxDistance` (m), botão "Salvar" que envia `PUT .../preferences/face-config`
- [ ] 4.5.2 Botão "Captura ao Vivo": envia `POST .../preferences/face-capture`, exibe a imagem retornada (base64 → `<img src="data:image/jpeg;base64,...">`); mensagem acionável se captura falhar (câmera obstruída, firmware não suporta)
- [ ] 4.5.3 Seção de propaganda (Material/Program/Schedule): formulário de upload de imagem + botão "Remover toda propaganda"; iniciar com layout funcional ou "aguardando backend" se FASE 3 não estiver completa

---

## Notes

- Items `[ ]` = pendente; `[~]` = em andamento; `[x]` = concluído; `[!]` = bloqueado
- Tasks de segurança **1.12** e **1.13** têm critério de aceite verificável (teto de 20 MB para uploads, validação de host anti-SSRF) — não podem ser consideradas concluídas sem os subtests correspondentes passando
- Constitution II: toda operação de escrita ISAPI nova (AuthMode, Display, FaceCompareCond, DisableStandby) tem teste de idempotência obrigatório marcado nas tasks 1.2.2, 1.4.2, 1.7.5, 1.11.3
- Dependência de deploy: nenhuma nova migration; feature totalmente stateless
