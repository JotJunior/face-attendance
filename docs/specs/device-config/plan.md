# Implementation Plan: Configuração Completa do Dispositivo HikVision

**Feature**: `device-config` | **Date**: 2026-06-21 | **Spec**: [spec.md](./spec.md)

## Summary

Entregar os endpoints de backend Go sob `/admin/api/devices/{id}/...` e a
extensão da camada ISAPI (`internal/hikvision/client.go`) que ativam as 9 seções
da tela "Configuração do dispositivo" já desenhada na SPA (`internal/web/dist`,
estado "aguardando backend"). 6 user stories, 23 FRs: overview/identidade,
credenciais ISAPI (cifradas AES-GCM), data-hora/manutenção (reboot/factory
reset/time), controle de portas, gestão de usuários/faces no dispositivo, e
webhooks.

**Abordagem técnica** (da pesquisa): estender o `hikvision.Client` existente com
novos métodos ISAPI, reusando `doRequest` + digest auth + `NonRetriableError`;
estender o `adminDevicesRouter` (net/http ServeMux) para os novos subpaths sob a
sessão admin HMAC já vigente. Todos os contratos ISAPI são SOURCED do legacy
`hik-api` (que serve como fonte de contrato, NÃO é chamado em runtime — research
Decision 1). Persistência limitada a 2 colunas nullable de cache de capacidades
(`max_users`, `max_faces`); doors/webhooks/device-users são read-through ISAPI
sem persistência.

## Technical Context

**Language/Version**: Go 1.26.4 (SOURCED — `go.mod`; module
`github.com/jotjunior/face-attendance`)
**Primary Dependencies**: `net/http.ServeMux` (sem framework de roteamento),
`github.com/jackc/pgx/v5` (Postgres), `github.com/icholy/digest` (ISAPI digest
auth) — todos SOURCED (admin_api_handlers.go:16, client.go:28)
**Storage**: PostgreSQL (migrations em `migrations/`; última = 000006). Nova:
000007 (2 colunas nullable)
**Testing**: `go test ./... -count=1` (SOURCED — `Makefile`); integration via
`-tags integration` + `TEST_DATABASE_URL`
**Target Platform**: on-premise / Docker no Raspberry Pi arm64 (MEMORY.md;
Constitution §Stack)
**Project Type**: web-service (Go backend + SPA embarcada)
**Performance Goals**: SC-005 — controle de porta confirmado em <5s em rede
local; operações ISAPI síncronas sob demanda (não há requisito de throughput)
**Constraints**: Constitution V (segredos como config de runtime — senha ISAPI
nunca em log/JSON); Constitution IV (reuso restrito do legacy); operações ISAPI
têm `defaultTimeout = 30s` (SOURCED — client.go:43)
**Scale/Scope**: poucas a dezenas de dispositivos (Constitution §Volume MVP);
tela administrativa de baixa frequência de uso

## Constitution Check

*GATE: Deve passar antes do Phase 0. Re-checado após Phase 1 (§Re-check abaixo).*

| Princípio | Status | Notas |
|-----------|--------|-------|
| I. Veracidade de Dados — Zero Fabricação (NON-NEGOTIABLE) | PASS | Todo contrato ISAPI é SOURCED do legacy/client Go (citado com arquivo:linha em hikvision-isapi.md). Itens sem fonte verificada (NTP set-time, clear de faces, parser maxUsers/maxFaces) marcados `[PROPOSTA]`, NÃO afirmados como reais. Nenhum nome de campo/path inventado. |
| II. Idempotência Chaveada por CPF (NON-NEGOTIABLE) | N/A | Esta feature não escreve em recurso externo no caminho de enrollment. As ações são administrativas idempotentes por natureza (reboot, set-time, clear, control). `UserInfo/Clear` é idempotente (limpar já-limpo = no-op). Enrollment (UpsertUser/UploadFace) permanece fora de escopo. |
| III. Resiliência por Filas (retry + DLQ) | N/A | Operações são síncronas sob demanda do operador (spec §Contexto: "operações ISAPI são síncronas"). NÃO passam por RabbitMQ. Erros são retornados ao operador (504/502), não re-enfileirados — comportamento correto para ação interativa. |
| IV. Reuso Restrito do Legacy hik-api | PASS COM JUSTIFICATIVA | **Tensão central.** O texto limita o REUSO de código legacy a 3 ops ISAPI. Esta feature NÃO reusa código legacy (não chama cache/auth/controllers PHP em runtime — research Decision 1); usa o legacy como FONTE DE CONTRATO verificado e implementa em Go. A expansão da superfície ISAPI do Go (reboot/doors/etc.) é administrativa, fora do caminho crítico de enrollment. IV não é NON-NEGOTIABLE; expansão documentada em Complexity Tracking + aprovada via Governance §Exceções. |
| V. Segredos como Configuração de Runtime | PASS | `ISAPI_CRED_KEY` via env (SOURCED — config.go:149-156). Senha cifrada AES-GCM (`secrets.Cipher`, aesgcm.go); `BYTEA` no banco; NUNCA em JSON/log/erro (FR-005, marcada "sensitive — never log" client.go:151). `503` se key ausente (FR-007). |
| VI. Observabilidade Operacional | PASS | FR-011 exige log estruturado (`device_id`, `stage`, ação, operador) para ações destrutivas. Reusa `internal/logging.Logger` (SOURCED — admin_api_handlers.go:19,49). Erros ISAPI mapeados com contexto (504/502/503), não genéricos (SC-006). |
| VII. Idioma da Sintaxe | PASS | Identifiers em inglês (colunas `max_users`/`max_faces`, campos JSON snake_case `device_identifier`, métodos Go); mensagens de erro em português (`adminJSONError`, SOURCED admin_api_handlers.go:206). |

**Resultado do gate**: PASS. Nenhuma violação de princípio NON-NEGOTIABLE (I,
II). O Princípio IV passa com justificativa documentada (não é NON-NEGOTIABLE;
Governance permite desvio com rationale aprovado). Prossegue para Phase 0/1.

## Project Structure

### Documentation (this feature)

```
docs/specs/device-config/
├── spec.md                      # existente (6 stories, 23 FRs)
├── plan.md                      # Este arquivo
├── research.md                  # Phase 0 — 9 decisions
├── data-model.md                # Phase 1 — Device + 3 entidades read-through
├── quickstart.md                # Phase 1 — 7 cenários + roundtrip E2E
└── contracts/
    ├── hikvision-isapi.md        # ISAPI (SOURCED do legacy/Go)
    └── admin-api.md              # /admin/api/devices/{id}/* ([PROPOSTA])
```

### Source Code (repository root) — árvore real (verificada)

```
.
├── cmd/presenca-facial/                 # entrypoint do binário
├── internal/
│   ├── config/config.go                 # ISAPI_CRED_KEY (L149-156) — consumir
│   ├── domain/                          # domain.Device — estender com max_users/max_faces
│   ├── hikvision/
│   │   ├── client.go                    # ESTENDER: novos métodos ISAPI (reusa doRequest)
│   │   ├── client_system.go             # NOVO — reboot/factoryReset/time
│   │   ├── client_doors.go              # NOVO — capabilities/status/control + mapa cmd
│   │   ├── client_users.go              # NOVO — list/clear users
│   │   └── client_webhooks.go           # NOVO — list/delete httpHosts
│   ├── http/
│   │   ├── server.go                    # ESTENDER adminDevicesRouter (L115-117)
│   │   ├── admin_api_handlers.go        # ESTENDER deviceResponse (L110-124) + novos handlers
│   │   ├── admin_device_config_handlers.go  # NOVO — handlers dos subpaths
│   │   ├── session.go                   # reuso (auth HMAC, L95-114) — sem mudança
│   │   └── admin_api_test.go            # ESTENDER testes
│   ├── repository/device_repository.go  # ESTENDER: SetCapabilities (reusa SetCredentials/SetDeviceInfo)
│   ├── secrets/aesgcm.go                # reuso (Encrypt/Decrypt) — sem mudança
│   ├── logging/                         # reuso (log estruturado FR-011)
│   └── web/dist/                        # SPA já desenhada (consome os endpoints)
├── migrations/
│   ├── 000007_device_capabilities.up.sql    # NOVO — max_users/max_faces
│   └── 000007_device_capabilities.down.sql  # NOVO
└── legacy/hik-api/                      # FONTE DE CONTRATO (read-only; NÃO chamado em runtime)
```

**Structure Decision**: estender os pontos de extensão existentes em vez de criar
camadas novas. Novos métodos ISAPI agrupados por domínio no MESMO pacote
`hikvision` (research Decision 2), preservando o `Client` único como fronteira
ISAPI auditável (Constitution IV). Handlers dos subpaths num arquivo dedicado
`admin_device_config_handlers.go` para não inchar `admin_api_handlers.go`, mas
reusando `adminJSONError`/`extractLastPathSegment`/`toDeviceResponse` existentes.
Roteamento estende `adminDevicesRouter` (sob `sessionMW` já vigente) — zero
mudança no esquema de auth.

## Convenções de Borda

Feature multi-camada (DB ↔ backend Go ↔ frontend SPA ↔ device HikVision).
Fonte da verdade de cada convenção:

| Camada | Case style | Validação | Fonte da verdade |
|--------|------------|-----------|------------------|
| DB columns (PostgreSQL) | snake_case | migration + constraint | `migrations/*.sql` (000002, 000006, 000007) |
| Backend DTO / response (Go) | snake_case (json tags) | json tags + handler | `internal/http/admin_api_handlers.go` (`deviceResponse` L110-124) |
| Frontend DTO (SPA) | snake_case | consumo direto do JSON | `internal/web/dist` (SPA já consome os 4 endpoints atuais em snake_case) |
| API payload (request/response admin) | snake_case | handler Go | `contracts/admin-api.md` |
| Sub-objetos de dados do DEVICE (`users[]`, `doors[]`) | camelCase (nomes ISAPI: `employeeNo`, `numOfFace`, `doorID`) | passthrough do parse ISAPI | `contracts/hikvision-isapi.md` (fonte = legacy parsers) |
| ISAPI payload (device) | camelCase / XML do firmware | client Go monta/parseia | `internal/hikvision/client*.go` |
| URL path params (`{id}`, `{door_id}`, `{webhook_id}`) | inteiros 1-based | `extractLastPathSegment` + ParseInt | `internal/http/server.go` + admin_api_handlers.go:377-384 |
| `command` de porta (API) → `cmd` (ISAPI) | snake_case → camelCase | mapper no client | `internal/hikvision/client_doors.go` (mapa SOURCED research Decision 4) |

**Mapper layer (DB ↔ DTO)**:
- Backend: conversão `domain.Device` → `deviceResponse` em
  `toDeviceResponse` (admin_api_handlers.go:144-159) — ESTENDER com os 3 campos
  novos (`max_users`, `max_faces`, `isapi_credentials_set`).
- Mapper ISAPI ↔ DTO admin: no handler de cada subpath (traduz shape camelCase
  do device → envelope snake_case da resposta admin). Os sub-objetos de dados do
  device (`users[]`, `doors[]`) preservam os nomes ISAPI deliberadamente
  (são dados externos, não do nosso domínio) — ver data-model.md.
- ORM auto-mapping: NÃO — pgx com SQL explícito; sem gorm/sqlc.

**Validação**: snake_case validado por json tags (Go) em ambas as bordas; não há
Zod (projeto Go, SPA consome JSON direto). A borda crítica de drift
(backend→SPA) é coberta pelo cenário Roundtrip E2E (quickstart §6), que compara
o payload REAL contra as chaves do contrato.

## Complexity Tracking

> Preenchido porque o Constitution Check tem 1 item (Princípio IV) que passa
> COM JUSTIFICATIVA e requer registro (Governance §Exceções).

| Violação | Por Que Necessário | Alternativa Simples Rejeitada Porque |
|----------|--------------------|--------------------------------------|
| Expansão da superfície ISAPI do client Go além das 3 ops do Princípio IV (reboot, factory-reset, time, doors, user-list/clear, webhook-list/delete, faces-clear) | A spec entrega uma tela administrativa de dispositivo (9 seções já desenhadas na SPA) que requer essas operações ISAPI. Implementá-las em Go reusando o `Client` existente é a forma de não acoplar o runtime ao legacy PHP (o que VIOLARIA o Princípio IV de forma mais grave). | (a) *Chamar o serviço PHP legacy em runtime*: arrasta cache/auth/controllers do legacy (violação direta de IV), exige 2º runtime Swoole no Pi + Redis. (b) *Não entregar a tela*: a feature inteira é a tela; fora de questão. O Princípio IV não é NON-NEGOTIABLE; o desvio é administrativo (fora do caminho crítico de enrollment, que permanece nas 3 ops) e documentado conforme Governance. |

## Security Considerations (gate owasp-security — OWASP API Top 10:2023 / ASVS)

Threat model do design (não bloqueante — controles existentes cobrem os riscos
materiais). Findings:

| Risco (OWASP/CWE) | Avaliação | Controle (SOURCED) |
|-------------------|-----------|--------------------|
| **API1 BOLA/IDOR** no `{id}` | BAIXO | Modelo single-admin: devices são globais, sem ownership por tenant (Constitution §Volume MVP; MEMORY single-org). Toda a árvore `/admin/api/devices/` exige sessão (deny-by-default, server.go:54). FR-023 → 404 para `{id}` inexistente. Não há escalada de privilégio possível (um só papel admin). |
| **CWE-352 CSRF** em ações destrutivas (reboot/factory-reset/door-control/clear/PUT credentials via cookie) | MITIGADO | Cookie `admin_session` com `SameSite: http.SameSiteStrictMode` + `HttpOnly` + `Secure` (admin_ui_handlers.go:86-88). Requisições cross-site não carregam o cookie → POST/PUT/DELETE destrutivos protegidos por construção. Os novos endpoints herdam o mesmo cookie/middleware. |
| **A04/API Sensitive data** — vazamento da senha ISAPI | MITIGADO | FR-005: senha nunca em JSON/log/erro; `BYTEA` cifrado AES-GCM; `DeviceConfig.Password` "sensitive — never log" (client.go:151); `isapi_credentials_set` derivado expõe só presença. |
| **A04 Crypto** — AES-GCM da credencial | PASS | `secrets.Cipher` AES-256-GCM (nonce‖ciphertext), `ParseKey` exige 32 bytes (aesgcm.go); key via env `ISAPI_CRED_KEY` (config.go:149-156). 503 sem key (FR-007). |
| **A10 Exception handling** — erro genérico vazando interno | PASS | Mapa 504/502/503/404 com mensagens em português acionáveis; nunca 500 genérico (SC-006); erro nunca inclui a senha. |
| **API4 Resource consumption** — controle de porta física (FR-014) | INFORMATIVO | Recomendação: rate-limit nas ações destrutivas/de porta (não há hoje). Risco baixo dado SameSite + sessão admin local; registrar como melhoria futura (fora do escopo MVP). |

**Recomendações informativas para a implementação** (não bloqueiam o plan):
- Manter os handlers destrutivos restritos a métodos não-GET (já é o padrão dos
  handlers atuais, ex: resync exige POST, admin_resync_handler.go:39) para que o
  SameSite=Strict seja efetivo.
- Garantir `CookieSecure=true` em produção (deploy HTTPS) — config existente.
- Log estruturado de FR-011 NÃO deve incluir corpo de request de credenciais.

## Re-check de Constitution (pós-Phase 1)

Revalidação após design completo (data-model + contracts + quickstart):

- **Princípio I (PASS)**: o design NÃO introduziu nenhuma afirmação factual sem
  fonte. Os 3 itens sem fonte verificada permanecem rotulados `[PROPOSTA]` nos
  contracts (NTP set-time, clear de faces, parser de maxUsers/maxFaces) — a
  implementação deve confirmá-los empiricamente ou registrar bloqueio humano.
- **Princípio IV (PASS com justificativa, inalterado)**: o design confina toda
  a fala ISAPI ao `Client` único; o legacy permanece read-only como fonte de
  contrato. Complexity Tracking registra a expansão.
- **Princípio V (PASS)**: o design de credenciais (PUT credentials → Encrypt →
  BYTEA; `isapi_credentials_set` derivado; 503 sem key) reforça o princípio; o
  cenário 2/2b do quickstart valida que a senha nunca vaza.
- **Princípio VI (PASS)**: FR-011 + log estruturado reusado; mapa de erros
  504/502/503 garante mensagens com contexto.
- **Demais (II, III N/A; VII PASS)**: inalterados.

Design NÃO introduziu camada/serviço extra não justificado (reusa Client +
router + repo existentes). Gate final: **PASS**.
