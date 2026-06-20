# Contract — HikVision ISAPI (externa, consumida via Digest)

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20
**Direcao**: OUTBOUND (o sistema chama o dispositivo)
**Base URL**: `http://{device_ip}:{device_port}` (config de runtime por dispositivo)
**Auth**: HTTP **Digest** (usuario/senha ISAPI, env por dispositivo — `HttpClient.php:176`, `AuthType.php`)

> Contratos REAIS extraidos do codigo legacy `legacy/hik-api` (PHP/Hyperf).
> Cada operacao cita arquivo:linha. Reuso restrito as 3 operacoes (Constitution
> Principio IV, FR-013). Nenhum endpoint/campo inventado (Principio I).

## 1. Upsert de usuario — POST/PUT /ISAPI/AccessControl/UserInfo/Modify

**Fonte**: `UserService.php:34` (endpoint), `:146-184` (create POST), `:186-227`
(update PUT), `:193,435-447` (campos). FR-010.

### Request (create)
```
POST /ISAPI/AccessControl/UserInfo/Modify
Content-Type: application/xml
Authorization: Digest ...

<UserInfo>
  <employeeNo>{CPF}</employeeNo>
  <name>{NOME}</name>
</UserInfo>
```
- Status OK: **200 ou 201** (`UserService.php:161`).

### Request (update)
```
PUT /ISAPI/AccessControl/UserInfo/Modify
Content-Type: application/xml

<UserInfo>
  <employeeNo>{CPF}</employeeNo>
  <name>{NOME}</name>
</UserInfo>
```
- O legacy injeta `employeeNo` no `UserInfo` (`UserService.php:193`).
- Status OK: **200 ou 204** (`UserService.php:204`).

### Campos verificados de `UserInfo`
| Campo | Valor | Fonte |
|-------|-------|-------|
| `employeeNo` | CPF do membro | `UserService.php:193,435` |
| `name` | nome do membro | `parseUserData:436` |

> `[PROPOSTA — a validar na implementacao]`: o legacy tambem parseia
> `userType`, `Valid` (enable/beginTime/endTime), `doorRight`, `numOfCard`,
> `numOfFace` na RESPOSTA (`parseUserData:435-447`), e o briefing.md:70 lista
> `userType`, `Valid`, `doorRight` no payload de envio. O MVP envia o minimo
> verificado (`employeeNo` + `name`); campos adicionais (ex. `Valid` com janela de
> validade) sao opcionais e marcados proposta — NAO sao inventados, derivam do que o
> dispositivo aceita e podem ser adicionados na implementacao se o device exigir.

### Estrategia upsert (idempotencia, Principio II)
- Chave = `employeeNo` (CPF). O endpoint `Modify` faz upsert; reprocessar a mesma
  mensagem nao duplica usuario (FR-009, US3 cenario 4). A escolha POST-vs-PUT pode
  ser: tentar create (POST) e, se ja existe, update (PUT) — ou consultar
  `UserInfo/Search`/`Detail` antes (endpoints existem no legacy mas estao FORA do
  reuso permitido; preferir POST-then-PUT). Detalhe de execute-task.

---

## 2. Upload de face — POST /ISAPI/Intelligent/FDLib/faceDataRecord (multipart)

**Fonte**: `FaceService.php:30` (endpoint), `:42-44` (constantes), `:158-221`
(create), `HttpClient.php:107-116,204-233` (mecanica multipart). FR-011.

### Request
```
POST /ISAPI/Intelligent/FDLib/faceDataRecord?format=json
Content-Type: multipart/form-data
Authorization: Digest ...

--boundary
Content-Disposition: form-data; name="FaceDataRecord"

{"type":"concurrent","faceLibType":"blackFD","FDID":"1","FPID":"{CPF}"}
--boundary
Content-Disposition: form-data; name="FaceImage"; filename="{CPF}.jpg"
Content-Type: image/jpeg

<binario da imagem>
--boundary--
```
- Query: `format=json` (`FaceService.php:193`).
- Status OK: **200** (`FaceService.php:197`).

### Campos verificados
| Parte | Campo | Valor | Fonte |
|-------|-------|-------|-------|
| `FaceDataRecord` (field) | `type` | `concurrent` | `FaceService.php:177` |
| | `faceLibType` | `blackFD` | `:42,178` (`FACE_LIB_TYPE`) |
| | `FDID` | `1` | `:44,179` (`FACE_LIB_ID`) |
| | `FPID` | CPF | `:180` |
| `FaceImage` (file) | filename | `{CPF}.jpg` | `:187` (`sprintf('%s.jpg', id)`) |
| | mime | detectado da imagem | `:189` (`mime_content_type`) — esperado `image/jpeg` |

### Pipeline da imagem
- Baixar de `url_selfie` (do Member) → enviar como `FaceImage`. Falha de download
  ou imagem inexistente → retry e, persistindo, DLQ (Edge Case spec, Principio III).
- Idempotencia: `FPID` = CPF; reenviar a mesma face nao duplica (US3 cenario 4).

---

## 3. Configurar webhook — POST /ISAPI/Event/notification/httpHosts

**Fonte**: `NotificationService.php:33` (endpoint), `:47-90` (configure),
`:360-375` (`buildWebhookConfig`). FR-012.

### Request
```
POST /ISAPI/Event/notification/httpHosts
Content-Type: application/xml
Authorization: Digest ...

<HttpHostNotification>
  <id>{ID}</id>
  <url>{URL_DA_API_LOCAL}</url>
  <protocolType>HTTP</protocolType>
  <parameterFormatType>XML</parameterFormatType>
  <addressingFormatType>ipaddress</addressingFormatType>
  <ipAddress>{HOST_DA_API_LOCAL}</ipAddress>
  <portNo>{PORTA}</portNo>
  <path>{PATH_DO_WEBHOOK}</path>
  <httpAuthenticationMethod>none</httpAuthenticationMethod>
</HttpHostNotification>
```
- Status OK: **200 ou 201** (`NotificationService.php:67`).

### Campos verificados de `HttpHostNotification`
| Campo | Valor default (legacy) | Fonte |
|-------|------------------------|-------|
| `id` | id do host (uniqid no legacy) | `:364` |
| `url` | URL da API local | `:365` |
| `protocolType` | `HTTP` | `:366` |
| `parameterFormatType` | `XML` | `:367` |
| `addressingFormatType` | `ipaddress` | `:368` |
| `ipAddress` | host derivado da url | `:369` |
| `portNo` | porta derivada da url (default 80) | `:370` |
| `path` | path derivado da url | `:371` |
| `httpAuthenticationMethod` | `none` | `:372` |

### Idempotencia
- Reconfigurar para a mesma URL e idempotente do ponto de vista funcional (o
  dispositivo passa a apontar para a API local). A escolha de `id` estavel por
  dispositivo evita acumular hosts duplicados — detalhe de execute-task.

---

## Codigos de status e erros (transversal)

- O cliente HTTP legacy usa `http_errors=false` e inspeciona `statusCode`
  (`HttpClient.php:160`); o Go deve fazer o mesmo (nao lancar em 4xx/5xx, decidir
  por codigo).
- Respostas XML sao parseadas; o legacy converte XML→array
  (`HttpClient.php:266-279`). O Go le o status e, quando precisar, o corpo.
- Falha transitoria (timeout, 5xx, conexao recusada) → retry/backoff/DLQ
  (Principio III, FR-023).

## Reuso PROIBIDO (Principio IV / FR-013)
NAO reutilizar do legacy: cache (`UserRepository`/`FaceRepository`), camada de
auth da aplicacao (`JwtAuthMiddleware`, `AuthService`), demais controllers
(`DeviceController`, `WebhookCallbackController`), e os demais endpoints ISAPI
(Door, Card, Stats, Preferences, Image sync, etc.). Apenas as 3 operacoes acima.
