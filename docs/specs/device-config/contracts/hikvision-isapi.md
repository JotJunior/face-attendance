# Contrato ISAPI — Operações de Configuração do Dispositivo

**Feature**: `device-config` | **Date**: 2026-06-21 | **Camada**: device (HikVision DS-K1T673DWX)

Contratos das operações ISAPI consumidas pelo backend Go. **Todos SOURCED**
do legacy `hik-api` (verificado por leitura de código) ou do client Go atual,
salvo os explicitamente marcados `[PROPOSTA]`. Auth: HTTP Digest
(`github.com/icholy/digest`, client.go:28). Base URL: `http://{ip}:{isapi_port}`
(client.go:181-187).

Legenda de proveniência:
- **SOURCED-LEGACY** `<arquivo>:<linha>` — path/verbo/shape extraído do legacy.
- **SOURCED-GO** `client.go:<linha>` — já implementado no client Go.
- **[PROPOSTA]** — endpoint/shape ainda não verificado; a validar na implementação.

---

## Grupo: Sistema (US3 — FR-008/009/010)

### Reboot
- **Verbo/Path**: `PUT /ISAPI/System/reboot`
- **Body**: vazio
- **Proveniência**: SOURCED-LEGACY `DeviceService.php:42` (`ENDPOINT_REBOOT`),
  `DeviceService.php:219-246` (`reboot()` envia PUT com array vazio `[]`).
- **Sucesso**: HTTP 200/204 (DeviceService.php:230).

### Factory reset
- **Verbo/Path**: `PUT /ISAPI/System/factoryReset`
- **Body**: `{"mode": "basic"}` (XML/form)
- **Proveniência**: SOURCED-LEGACY `DeviceService.php:44` (`ENDPOINT_RESET`),
  `DeviceService.php:186-217` (`clear()` PUT com `['mode' => 'basic']`).
- **Sucesso**: HTTP 200/204. **Pós-ação backend**: `webhook_configured=false` (FR-009).

### Get time
- **Verbo/Path**: `GET /ISAPI/System/time`
- **Response (parse)**: `{localTime, timeZone, timeMode}`
- **Proveniência**: SOURCED-LEGACY `DeviceService.php:40` (`ENDPOINT_TIME`),
  `DeviceService.php:248-276` (`getTime()`), `parseTimeData` (L395-406).

### Set time
- **Verbo/Path**: `PUT /ISAPI/System/time`
- **Body**: `{"Time": {"timeMode": "manual"|"NTP", "localTime": "YYYY-MM-DDThh:mm:ss", "timeZone": "<offset>"}}`
- **Proveniência**: SOURCED-LEGACY `DeviceService.php:278-320` (`setTime()`),
  body em L284-290 (`timeMode=manual`, `localTime`, `timeZone`).
- **NTP**: o legacy só implementa `manual`. Modo NTP (US3-AS1 "servidor NTP
  informado") usa `timeMode: "NTP"` + bloco NTPServer — **[PROPOSTA — shape do
  bloco NTP a validar; o legacy não o implementa]**.

### Get device info (já em uso)
- **Verbo/Path**: `GET /ISAPI/System/deviceInfo`
- **Response**: XML ou JSON `{DeviceInfo:{deviceName, model, serialNumber, firmwareVersion, macAddress}}`
- **Proveniência**: SOURCED-GO `client.go:42,58-103` (`FetchDeviceInfo`, já
  parseia XML+JSON). SOURCED-LEGACY `DeviceService.php:355-369` (`parseDeviceInfo`,
  inclui `macAddress`, `firmwareReleasedDate`, `deviceType`).

### Capabilities de contagem (FR-002) — PROPOSTA
- **Verbo/Path**: `GET /ISAPI/System/capabilities` (flags) — SOURCED-LEGACY
  `DeviceService.php:36` (`ENDPOINT_CAPABILITIES`), `parseCapabilities`
  (L375-390) — mas este parseia FLAGS booleanas (`isSupportFR`, `isSupportCard`),
  **NÃO** contadores `maxUsers`/`maxFaces`.
- **maxUsers/maxFaces**: `[PROPOSTA]` — shape de onde vêm os contadores máximos
  NÃO está verificado. A implementação deve descobrir (ex:
  `GET /ISAPI/AccessControl/UserInfo/capabilities` ou
  `/ISAPI/Intelligent/FDLib/capabilities`) via chamada observada, OU retornar
  `null` (FR-002: nunca estimar). Ver research.md Decision 7.

---

## Grupo: Portas (US4 — FR-012/013/014)

### Door capabilities (lista de portas)
- **Verbo/Path**: `GET /ISAPI/AccessControl/Door/capabilities?format=json`
- **Response (parse)**: `{AccessControlDoorCapabilities:{DoorInfo:[{doorID, doorName, readerCount}]}}`
- **Proveniência**: SOURCED-LEGACY `DoorService.php:34` (`ENDPOINT_DOOR_CAPABILITIES`),
  `list()` (L56-97), `parseDoorList` (L351-375).

### Door status
- **Verbo/Path**: `POST /ISAPI/AccessControl/Door/Status?format=json`
- **Body**: `{"DoorStatusList": {"DoorStatus": [{"doorID": <N>}]}}`
- **Response (parse)**: `{doorID, doorName, doorState, lockState, contactState, currentAction}`
- **Proveniência**: SOURCED-LEGACY `DoorService.php:30` (`ENDPOINT_DOOR_STATUS`),
  `getStatus()` (L99-146, body em L113-119), `parseDoorStatus` (L380-411).

### Door remote control
- **Verbo/Path**: `PUT /ISAPI/AccessControl/RemoteControl/door/{N}` (N = door_id, %d)
- **Body (XML)**:
  ```xml
  <RemoteControlDoor version="2.0" xmlns="http://www.isapi.org/ver20/XMLSchema">
      <cmd>{open|close|alwaysOpen|alwaysClosed|normalOpen}</cmd>
  </RemoteControlDoor>
  ```
- **Proveniência**: SOURCED-LEGACY `DoorService.php:28` (`ENDPOINT_DOOR_CONTROL`,
  `/ISAPI/AccessControl/RemoteControl/door/%d`), `sendCommand` XML em L307-311,
  comandos em L39-47.
- **Mapa command (API) → cmd (ISAPI)**: ver research.md Decision 4. Os 5 valores
  `cmd` são SOURCED das constantes `CMD_*`.
- **Caveat**: comportamento empírico de `alwaysOpen`/`alwaysClosed`/`normalOpen`
  no firmware a confirmar (research.md Decision 4).

### Door config (para ler open_duration — US4 destravar Ns)
- **Verbo/Path**: `GET /ISAPI/AccessControl/Door/{N}?format=json`
- **Response (parse)**: `{doorID, doorName, lockTime, openDuration, ...}`
- **Proveniência**: SOURCED-LEGACY `DoorService.php:32` (`ENDPOINT_DOOR_CONFIG`),
  `getConfig()` (L168-208), `parseDoorConfig` (L416-433, `openDuration` em L430).

---

## Grupo: Usuários no dispositivo (US5 — FR-016/016b)

### List users (paginado)
- **Verbo/Path**: `POST /ISAPI/AccessControl/UserInfo/Search`
- **Body**: `{"UserInfoSearchCond": {"searchID": "<uid>", "searchResultPosition": <(page-1)*perPage>, "maxResults": <perPage>}}`
- **Response (parse)**: `{items: [{employeeNo, name, userType, numOfFace, valid, beginTime, endTime}], total}`
- **Proveniência**: SOURCED-LEGACY `UserService.php:30` (`ENDPOINT_USER_LIST`),
  `list(page, perPage)` (L49-92, paginação em L52-58), `parseUserList`/`parseUserData`
  (L396-447). **Já confirmado na clarify (dec-005, score 3).**

### Clear all users
- **Verbo/Path**: `PUT /ISAPI/AccessControl/UserInfo/Clear`
- **Body**: vazio
- **Proveniência**: SOURCED-LEGACY `UserService.php:38` (`ENDPOINT_USER_CLEAR`),
  `clear()` (L269-299, PUT body `[]` em L273-277).
- **Efeito**: remove todos os usuários E suas faces (US5-AS2).

---

## Grupo: Faces (US5 — FR-017)

### Clear faces library — PROPOSTA
- **Biblioteca-alvo**: FDLib `faceLibType="blackFD"`, `FDID="1"` — SOURCED-GO
  `client.go:289-293` (mesma usada em `UploadFace`).
- **Verbo/Path de clear**: `[PROPOSTA]` — NÃO há endpoint de clear da FDLib
  verificado no legacy nem no Go. Hipótese a validar:
  `PUT /ISAPI/Intelligent/FDLib/FDSearch/Delete?format=json` com `FDID=1`, OU
  delete por FPID em batch. A implementação DEVE confirmar (doc ISAPI/chamada
  observada) ou registrar bloqueio humano. Ver research.md Decision 9.

---

## Grupo: Webhooks (US6 — FR-018/019)

### List notification hosts
- **Verbo/Path**: `GET /ISAPI/Event/notification/httpHosts`
- **Response (parse)**: `{total, webhooks: [{id, url, protocol, events}]}`
- **Proveniência**: SOURCED-LEGACY `NotificationService.php:33` (`ENDPOINT_WEBHOOK`),
  `getWebhookConfig(null)` (L180-214, GET na L187), `parseWebhookConfig` (L397-429).

### Delete notification host
- **Verbo/Path**: `DELETE /ISAPI/Event/notification/httpHosts/{webhook_id}`
- **Proveniência**: SOURCED-LEGACY `NotificationService.php:92-123` (`removeWebhook`,
  DELETE em `ENDPOINT_WEBHOOK . "/{$webhookId}"`, L99-104).
- **Efeito backend**: se `webhook_id` == `deterministicHostID(device.Host)`
  (client.go:341,408-411), marcar `webhook_configured=false` (FR-019).

### Create notification host (já em uso)
- **Verbo/Path**: `POST /ISAPI/Event/notification/httpHosts`
- **Body (XML)**: `<HttpHostNotification>` com `id, url, protocolType=HTTP, parameterFormatType=XML, addressingFormatType=ipaddress, ipAddress, portNo, path, httpAuthenticationMethod=none`
- **Proveniência**: SOURCED-GO `client.go:339-374` (`ConfigureWebhook`, XML em
  L346-363); SOURCED-LEGACY `NotificationService.php:360-375` (`buildWebhookConfig`).

---

## Tabela de status codes ISAPI → tratamento

| Situação | Detecção (SOURCED) | Tratamento backend |
|----------|---------------------|--------------------|
| Erro de transporte (timeout/conn refused) | `doRequest` retorna `error` não-nil (client.go:202-205) | `504` (FR-021) |
| 401 (digest auth falhou) | `status == 401` | `502` "falha de autenticação com o dispositivo" (FR-022) |
| 4xx de lógica do dispositivo | `status 4xx` + corpo | `502` + detalhe distinto de conectividade (FR-015) |
| 200/201/204 | status de sucesso | OK |

A senha NUNCA aparece em mensagem de erro (FR-005; `DeviceConfig.Password` é
"sensitive — never log", client.go:151).
