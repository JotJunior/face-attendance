# Data Model — device-preferences

**Feature**: `device-preferences`
**Status**: Complete

## Resumo: feature stateless (passthrough ISAPI)

Esta feature **não adiciona nenhuma tabela, coluna ou migration**. Toda a
funcionalidade é passthrough síncrono sob demanda: o operador aciona uma operação
no painel → o backend traduz para uma (ou várias) chamada ISAPI → o estado vive no
**firmware do terminal**, não no Postgres.

A última migration é `000007_device_capabilities` (feature `device-config`). Esta
feature **não cria** `000008_*`.

### Por que nada vai ao Postgres

| Entidade | Onde vive | Persistência local? |
|----------|-----------|---------------------|
| Modo de verificação (verifyMode) | Firmware (`VerifyWeekPlanCfg`) | Não — lido/escrito sob demanda |
| Layout de tela (showMode/timeouts) | Firmware (`IdentityTerminal`) | Não — read-modify-write sob demanda |
| Imagens de standby | Firmware (`StandbyPictureMgr`), identificadas por UUID do device | Não — listadas/enviadas/removidas sob demanda |
| Imagem de boot | Firmware (`System/powerUpPicture`) | Não |
| Material/Program/Schedule de propaganda | Firmware (`Publish` namespace) | Não — IDs gerados/retornados pelo device |
| Estatísticas (contadores/capacidades) | Firmware (`UserInfo/Count`, `AcsEventTotalNum`) | Não — leitura agregada ao vivo, nunca cacheada |
| Config de face (maxDistance) | Firmware (`FaceCompareCond`) | Não |

> **Decisão (Constitution VI / Princípio I)**: estatísticas (FR-016) NUNCA são
> cacheadas. Zero e "indisponível" são estados distintos (spec US4 cenário 2): um
> device offline retorna erro de conectividade, jamais contadores zerados. Cachear
> abriria espaço para exibir dado factual desatualizado como atual — proibido.

## Reuso de schema existente

A única entidade persistida tocada (somente leitura) é **`devices`** (tabela já
existente, gerenciada pela feature `device-config`):

- `id` — chave usada em todos os paths `/admin/api/devices/{id}/preferences/*`.
- `host`, `isapi_port` — resolução do alvo ISAPI.
- `isapi_username`, `isapi_password_enc` — credenciais cifradas (AES-256-GCM,
  `internal/secrets`), descifradas em runtime por `hikvision.LoadDeviceConfig`.

Esta feature **não altera o schema de `devices`** (spec, Key Entities).

## Entidades efêmeras (no device, modeladas só em memória)

Modeladas como structs Go transitórios para serialização XML/JSON — nunca persistidos:

### IdentityTerminalConfig (XML, read-modify-write)
- Configuráveis: `showMode`, `advertisingDisplayType`, `screenOffTimeout`,
  `previewShowTime`, `standbyTimeout`.
- Read-only preservados: `version` (atributo root), `camera`, `fingerPrintModule`,
  `faceAlgorithm`, `saveCertifiedImage`, `readInfoOfCard`, `workMode`, `ecoMode`
  (subárvore), `enableScreenOff`, `popUpPreviewWindow`.
- Fonte de campos: `legacy/hik2go/src/Hik2go/Preferences/IdentityTerminal.php:66-91`.

### StandbyPicture (JSON)
- Identificada por `customStandbyPicUUID` (gerado pelo device).
- Não há tabela local de UUIDs; a listagem é sempre a verdade do device
  (`GetCustomStandbyPicList`).

### Material / Program / Schedule (XML)
- `Material.id` gerado pelo device na criação; retornado ao operador para limpeza
  manual em caso de falha parcial (dec-012).
- `Program`/`Schedule` usam IDs fixos `1` no fluxo de screensaver (Presentation.php).

### DeviceStats (JSON agregado, FR-016)
- Composição em memória de 4 respostas ISAPI:
  `users.{total,faces,cards,max}` + `events.{total,max}`.
- Sem persistência; montado a cada GET `/stats`.

### FaceCaptureResult (JSON, FR-018)
- `image_base64` — imagem capturada baixada da `faceDataUrl` do device e encodada
  (dec-011). Nunca persistida; nunca expõe a URL/IP interno.

## Invariantes de dados

- **I1 (Idempotência, Constitution II)**: escritas estruturadas (AuthMode FR-002,
  IdentityTerminal FR-004, FaceCompareCond FR-017) são read-modify-write — reenviar a
  mesma configuração produz o mesmo estado, sem efeito colateral. Standby
  enable/disable (FR-009) é idempotente por natureza (estado-alvo fixo).
- **I2 (Sem fabricação, Constitution I/VI)**: nenhum valor de contador, UUID, ID de
  material ou config é inventado; tudo provém da resposta do device ou é repassado como
  erro. Limite de tamanho de imagem NÃO é hardcodado (dec-010).
- **I3 (Sem cache de leitura)**: stats e configs são sempre lidos ao vivo; nunca
  servidos de cópia local que possa divergir do firmware.
