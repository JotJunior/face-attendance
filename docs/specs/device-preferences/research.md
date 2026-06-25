# Research — device-preferences

**Feature**: `device-preferences`
**Status**: Complete
**Origem dos contratos**: todos os endpoints, verbos e payloads abaixo são
**SOURCED** com citação `arquivo:linha` em `legacy/hik2go/src/Hik2go/`. Nenhum
endpoint, campo ou limite foi inventado (Constitution I — Zero fabricação).

> **Aviso de verificação empírica**: os contratos vêm da biblioteca PHP de
> referência `hik2go`, que é fonte rastreável de **nomes de rota, verbo e shape
> de payload**. Os contratos NÃO foram (todos) exercitados contra firmware real
> nesta feature. A implementação DEVE seguir a mesma disciplina de
> `client_verify.go` / `client_system.go`: valores recusados pelo firmware
> falham alto (PUT/POST != 2xx → erro repassado), nunca são silenciados.

---

## 1. Contratos ISAPI por grupo (SOURCED)

### Grupo 1 — Modo de verificação (AuthMode)

Fonte: `legacy/hik2go/src/Hik2go/Preferences/AuthMode.php`

| Operação | Verbo | Path | Formato | Fonte |
|----------|-------|------|---------|-------|
| Ler plano semanal | GET | `/ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json` | JSON | AuthMode.php:68-76 |
| Atualizar verifyMode | PUT | `/ISAPI/AccessControl/VerifyWeekPlanCfg/1?format=json` | JSON | AuthMode.php:27-32 |

- Resposta do GET: `VerifyWeekPlanCfg.WeekPlanCfg[]`, cada item com campo
  `verifyMode` (AuthMode.php:21-23).
- O PUT é **read-modify-write**: lê o corpo completo, substitui `verifyMode` em
  **todos** os `WeekPlanCfg`, reenvia o corpo inteiro. O firmware rejeita payloads
  parciais.
- **Já existe** em `internal/hikvision/client_verify.go`:
  `GetVerifyWeekPlan` (corpo cru) + `EnsureFaceVerifyMode` (read-modify-write
  idempotente do mesmo endpoint). FR-001/FR-002 reaproveitam `GetVerifyWeekPlan`
  e generalizam o write para qualquer `verifyMode` (não só face).

### Grupo 2 — Layout de tela (IdentityTerminal)

Fonte: `legacy/hik2go/src/Hik2go/Preferences/IdentityTerminal.php`

| Operação | Verbo | Path | Formato | Fonte |
|----------|-------|------|---------|-------|
| Ler config de exibição | GET | `/ISAPI/AccessControl/IdentityTerminal` | **XML** (sem `?format=json`) | IdentityTerminal.php:23-27 |
| Atualizar config de exibição | PUT | `/ISAPI/AccessControl/IdentityTerminal` | **XML** (`application/xml`) | IdentityTerminal.php:94-102 |
| Listar thumbnails de modo | GET | `/ISAPI/AccessControl/Reader/GetShowModeThumbnailsList?format=json` | JSON | IdentityTerminal.php:34-40 |

- **XML root**: `IdentityTerminal version="<da-resposta>" xmlns="http://www.isapi.org/ver20/XMLSchema"` (IdentityTerminal.php:66-91).
- O atributo `version` do root XML vem da **resposta do GET** (IdentityTerminal.php:68)
  → leitura prévia é obrigatória (read-modify-write).
- Campos **read-only a preservar** (IdentityTerminal.php:66-91): `camera`,
  `fingerPrintModule`, `faceAlgorithm`, `saveCertifiedImage`, `readInfoOfCard`,
  `workMode`, `ecoMode` (subárvore: `eco`, `faceMatchThreshold1`,
  `faceMatchThresholdN`, `changeThreshold`, `maskFaceMatchThresholdN`,
  `maskFaceMatchThreshold1`), `enableScreenOff`, `popUpPreviewWindow`.
- Campos **configuráveis**: `screenOffTimeout`, `previewShowTime`, `standbyTimeout`,
  `showMode`, `advertisingDisplayType`.
- **Mapeamento de showMode** (IdentityTerminal.php:50-52):
  - `normal` → `showMode=normal`, `advertisingDisplayType=full`
  - `full` → `showMode=advertising`, `advertisingDisplayType=full`
  - `split` → `showMode=advertising`, `advertisingDisplayType=split`

### Grupo 3 — Imagem de standby (StandbyPicture)

Fonte: `legacy/hik2go/src/Hik2go/Preferences/StandbyPicture.php`

| Operação | Verbo | Path | Formato | Fonte |
|----------|-------|------|---------|-------|
| Listar standby pics | GET | `/ISAPI/Publish/StandbyPictureMgr/GetCustomStandbyPicList?format=json` | JSON | StandbyPicture.php:93-97 |
| Upload standby pic | POST | `/ISAPI/Publish/StandbyPictureMgr/UploadCustomStandbyPic?format=json` | **multipart** | StandbyPicture.php:13-37 |
| Ativar custom | PUT | `/ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` | JSON | StandbyPicture.php:76-85 |
| Desativar (default) | PUT | `/ISAPI/Publish/StandbyPictureMgr/StandbyPicDisplayParams?format=json` | JSON | StandbyPicture.php:59-68 |
| Remover standby pic | POST | `/ISAPI/Publish/StandbyPictureMgr/DeleteCustomStandbyPic?format=json` | JSON | StandbyPicture.php:44-51 |

- **Upload multipart** (StandbyPicture.php:15-33):
  - Campo `UploadCustomStandbyPic` (JSON): `{filePathType:"multipart", filePath:<nome>}`
  - Campo `filePath` (binário): conteúdo da imagem.
- **Ativar** (StandbyPicture.php:79-80): body `{standbyPicType:"custom", displayEffect:"stretch", switchingTime:20}`.
- **Desativar** (StandbyPicture.php:62-63): body `{standbyPicType:"default", displayEffect:"stretch", switchingTime:20}`.
- **Remover** (StandbyPicture.php:44-51): body `{customStandbyPicUUIDList:[{customStandbyPicUUID:"<uuid>"}]}`. Verbo é **POST** (não DELETE) no firmware; o endpoint admin será `DELETE` mas traduz para POST ISAPI.

### Grupo 4 — Imagem de boot (InitializationScreen / powerUpPicture)

Fonte: `legacy/hik2go/src/Hik2go/Preferences/InitializationScreen.php`

| Operação | Verbo | Path | Formato | Fonte |
|----------|-------|------|---------|-------|
| Upload boot pic | POST | `/ISAPI/System/powerUpPicture?format=json` | **multipart** | InitializationScreen.php:12-35 |
| Remover boot pic | DELETE | `/ISAPI/System/powerUpPicture?format=json` | (sem corpo) | InitializationScreen.php:43-47 |

- **Upload multipart** (InitializationScreen.php:15-31):
  - Campo `picture_info` (JSON): `{type:"filePathType", faceLibType:"binay"}`
    — **NB**: `binay` é a grafia literal do legado (InitializationScreen.php:20),
    não corrigir sem evidência de que o firmware aceita `binary`.
  - Campo `picture_name` (binário): imagem JPEG, nome `<id>.jpg` (InitializationScreen.php:27).

### Grupo 5 — Material de propaganda (Media + Presentation)

Fontes: `Preferences/Media.php`, `Preferences/Presentation.php`

| Etapa | Verbo | Path | Formato | Fonte |
|-------|-------|------|---------|-------|
| (a) Criar material | POST | `/ISAPI/Publish/MaterialMgr/material` | **XML** | Media.php:49-56 |
| (b) Upload binário | POST | `/ISAPI/Publish/MaterialMgr/material/{ID}/upload` | **multipart** | Media.php:102-122 |
| (c) Criar programa | POST | `/ISAPI/Publish/ProgramMgr/program` | **XML** | Presentation.php:35-42 |
| (d) Atualizar página | PUT | `/ISAPI/Publish/ProgramMgr/program/1/page/1` | **XML** | Presentation.php:95-102 |
| (e) Atualizar agenda | PUT | `/ISAPI/Publish/ScheduleMgr/playSchedule/1` | **XML** | Presentation.php:131-138 |
| Listar materiais | GET | `/ISAPI/Publish/MaterialMgr/material` | XML/JSON | Media.php:90-96 |
| Remover material | DELETE | `/ISAPI/Publish/MaterialMgr/material/{id}` | (sem corpo) | Media.php:78-82 |

- (a) XML root `Material` com filhos incl. `materialName`, `materialType=static`,
  `StaticMaterial{staticMaterialType=picture, picFormat, fileSize}` (Media.php:28-47).
  `picFormat` deriva do MIME (Media.php:43); `fileSize` do tamanho real (Media.php:44).
- (b) multipart: campos `name`, `type`, `size` (valores) + `file` (binário) (Media.php:105-117).
- (c) XML root `Program version="2.0" xmlns=".../ver20/XMLSchema"`, `Resolution`
  hardcoded `imageWidth=580 imageHeight=884` (Presentation.php:13,18-20).
- (d) XML root `Page version="2.0"`, `backgroundPic=<media_id>`,
  `BackgroundColor.RGB=16777215` (Presentation.php:75,82,84).
- (e) XML root `PlaySchedule version="2.0"`, `scheduleMode=screensaver`,
  `TimeRange{beginTime=00:00:00, endTime=24:00:00}` (Presentation.php:111,114,122-123).

### Grupo 6 — Estatísticas (Stats)

Fonte: `legacy/hik2go/src/Hik2go/Stats.php`

| Operação | Verbo | Path | Formato | Campos | Fonte |
|----------|-------|------|---------|--------|-------|
| Contagem usuários | GET | `/ISAPI/AccessControl/UserInfo/Count?format=json` | JSON | `UserInfoCount.userNumber/bindFaceUserNumber/bindCardUserNumber` | Stats.php:77-83 |
| Capacidade usuários | GET | `/ISAPI/AccessControl/UserInfo/capabilities?format=json` | JSON | `UserInfo.maxRecordNum` | Stats.php:65-71 |
| Contagem eventos | POST | `/ISAPI/AccessControl/AcsEventTotalNum?format=json` | JSON | body `{AcsEventTotalNumCond:{major:0,minor:0}}` → `AcsEventTotalNum.totalNum` | Stats.php:24-35 |
| Capacidade eventos | GET | `/ISAPI/AccessControl/AcsEventTotalNum/capabilities?format=json` | JSON | `AcsEvent.totalNum.@max` (atributo) | Stats.php:12-17 |

- FR-016 agrega as **4** chamadas numa resposta única. `events.max` é um **atributo
  XML** (`@max`) mesmo na resposta JSON do device (Stats.php:12-17) — o parser deve
  tolerar a chave `@max` (HikVision serializa atributos com prefixo `@` no JSON).

### Grupo 7 — Face avançado (Face)

Fonte: `legacy/hik2go/src/Hik2go/Face.php`

| Operação | Verbo | Path | Formato | Fonte |
|----------|-------|------|---------|-------|
| Config distância máx | PUT | `/ISAPI/AccessControl/FaceCompareCond` | **XML** | Face.php:137-145 |
| Captura facial ao vivo | POST | `/ISAPI/AccessControl/CaptureFaceData` | **XML** | Face.php:23-30 |

- **FaceCompareCond** (Face.php:121-134): XML root `FaceCompareCond version="2.0" xmlns=".../ver20/XMLSchema"`.
  - Configurável: `maxDistance` (float, Face.php:131).
  - Fixos (Face.php): `pitch=45`, `yaw=45`, `leftBorder=0`, `rightBorder=0`,
    `upBorder=0`, `bottomBorder=0`, `faceScore=0`, `faceScoreThreshold1=0`,
    `ROIRegionMode=manual` (Face.php:133).
- **CaptureFaceData** (Face.php:21-30): XML body
  `<CaptureFaceDataCond version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema"><captureInfrared>false</captureInfrared><dataType>url</dataType></CaptureFaceDataCond>`.
  Resposta: campo `faceDataUrl` (Face.php:33), que aponta para a imagem capturada.

---

## 2. Decisões técnicas

### D1 — Read-modify-write para escritas estruturadas (IdentityTerminal, AuthMode)

Endpoints `IdentityTerminal` (FR-004) e `VerifyWeekPlanCfg` (FR-002) exigem que o
PUT carregue o corpo **completo** que o device devolveu, com apenas os campos-alvo
substituídos. Motivo: o firmware rejeita payloads parciais e o `version` do
`IdentityTerminal` muda entre leituras (IdentityTerminal.php:68). **Padrão já
provado** em `client_verify.go:EnsureFaceVerifyMode` (read → mutate keys → marshal →
PUT). Para `IdentityTerminal` a leitura é XML; preserva-se a árvore via
`encoding/xml` com structs que cobrem read-only + configuráveis, OU via re-serialização
fiel. Decisão de implementação (a detalhar em tasks): structs Go espelhando os campos
de IdentityTerminal.php:66-91; campos não mapeados não existem no contrato do legado,
logo não há perda.

### D2 — Três mecanismos distintos de upload multipart de imagem

| Mecanismo | Campo JSON | Campo binário | Endpoint |
|-----------|------------|---------------|----------|
| Standby (FR-007) | `UploadCustomStandbyPic` | `filePath` | `StandbyPictureMgr/UploadCustomStandbyPic` |
| Boot (FR-010) | `picture_info` | `picture_name` | `System/powerUpPicture` |
| Material (FR-013b) | `name`/`type`/`size` (3 campos de valor) | `file` | `MaterialMgr/material/{ID}/upload` |

O helper multipart de `client.go:UploadFace` (`multipart.NewWriter` + `CreateFormField`
para JSON + `CreatePart` com header `Content-Disposition`/`Content-Type` para binário) é
o **template reusável**. Cada um dos 3 métodos novos do cliente monta seu próprio par de
campos conforme a tabela. Não há helper genérico no legado — cada upload tem nomes de
campo próprios e não-intercambiáveis.

### D3 — JSON vs XML por endpoint

A escolha de formato é **ditada pelo firmware**, não de design. Resumo:
- **JSON** (`?format=json`): AuthMode, StandbyPicture (params/list/delete), Stats,
  powerUpPicture (multipart com JSON embarcado).
- **XML** (`application/xml`, sem `?format=json`): IdentityTerminal, FaceCompareCond,
  CaptureFaceData, Material (create), Program, Page, PlaySchedule.
O cliente já fala ambos (cf. `client_doors.go` JSON e `client_webhooks.go` XML).

### D4 — Digest auth + HTTP reuso

Toda chamada passa por `Client.doRequest` (`internal/hikvision/client.go:198`), que já
implementa digest auth (`icholy/digest`), timeout e retorno `(body, status, err)`.
Nenhum cliente HTTP novo é criado. Construção do client por requisição:
`loadDeviceAndISAPIClient` (`admin_device_config_handlers.go:103`) já resolve device por
ID + credenciais cifradas (AES-GCM via `secrets.Cipher`) — reusado integralmente.

### D5 — Limites de tamanho de imagem por modelo (Constitution I, dec-010)

- **NÃO há pré-validação de tamanho no servidor.** Sem fonte rastreável para o limite
  do DS-K1T681DBX, hardcodar seria fabricação (Constitution I).
- Limite **observado** (não normativo, da memória do projeto / CLAUDE.md): DS-K1T673*
  ~200 KB. DS-K1T681DBX: **desconhecido** → a descobrir via `capabilities` em runtime
  ou repassando o erro do firmware.
- Estratégia: FR-022 valida apenas `image/*` (tipo). O erro de tamanho **vem do
  firmware** e é repassado ao operador (FR-020). Exemplo de mensagem acionável:
  > "O dispositivo rejeitou a imagem (HTTP 400). Provável causa: arquivo acima do
  > limite do modelo. Reduza a resolução/qualidade e tente novamente."
- **Nota de transcodificação**: `UploadFace` transcodifica PNG→JPEG q85 para caber no
  limite de face. Para standby/boot/material a transcodificação pode NÃO ser desejada
  (perda de qualidade de branding). Decisão de tasks: validar tipo, repassar bytes como
  recebidos, deixar o firmware ser a autoridade de tamanho.

### D6 — Material de propaganda órfão após falha parcial (dec-012)

O fluxo de 5 etapas de FR-013 não é transacional. Se (a) cria o material mas (b)/(c)/(d)/(e)
falham, o material fica órfão no device. Estratégia (dec-012):
- A resposta de erro de FR-013 **inclui o `id`** do material criado na etapa (a).
- O operador usa `DELETE /admin/api/devices/{id}/preferences/media/{material_id}` (FR-014)
  para limpeza manual.
- `DELETE .../preferences/media` (FR-015, bulk) lista via FR-012 e remove cada um — rede
  de segurança para órfãos acumulados.
- **Sem rollback automático**: tentar desfazer parcialmente pode mascarar o estado real
  do device. Documentar o risco e expor a ferramenta de limpeza é a escolha auditável.

### D7 — Captura facial ao vivo retorna base64 (dec-011)

`CaptureFaceData` devolve uma **URL** (`faceDataUrl`) apontando para imagem no device
(Face.php:33). Expor essa URL ao painel vazaria IP interno do device (Constitution V).
Estratégia: o backend faz **download** da URL (reusando o helper de download interno,
cf. `downloadImage` em `client.go`) e devolve a imagem como **base64 em JSON** ao painel.
Consistente com o padrão JSON de `/admin/api/*`.

### D8 — Read-back loop (onda plan)

`cstk recall --context` retornou K=4, porém os achados eram meta-decisões recursivas de
outras features (`dynamic-forms`, `product-management`) sem relação com o domínio ISAPI.
Tratados como ruído não-autoritativo (dec-021). Nenhum aprendizado de design aplicável foi
incorporado deste read-back.

---

## 3. Riscos rastreados

| Risco | Mitigação | FR/dec |
|-------|-----------|--------|
| Limite de imagem do DS-K1T681DBX desconhecido | Não hardcodar; repassar erro do firmware com mensagem acionável | dec-010, FR-020/022 |
| Material órfão em falha parcial de FR-013 | Retornar `id` do material; expor DELETE individual + bulk | dec-012, FR-013/014/015 |
| `version` do IdentityTerminal muda entre leituras | Read-modify-write obrigatório; sempre GET antes de PUT | FR-004, IdentityTerminal.php:68 |
| Grafia `binay` no powerUpPicture | Manter literal do legado; só mudar com evidência de firmware | FR-010, InitializationScreen.php:20 |
| `events.max` é atributo `@max` no JSON | Parser tolera chave prefixada `@` | FR-016, Stats.php:12-17 |
| Contratos não exercitados no firmware desta feature | Falha alta (status != 2xx → erro); nunca silenciar | Constitution I, FR-020 |
