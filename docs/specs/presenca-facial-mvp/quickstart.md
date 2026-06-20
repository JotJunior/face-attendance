# Quickstart / Cenarios de Teste — MVP de Presenca Facial

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

> Cenarios cobrem happy path + ao menos um error case por fluxo. Os contratos
> externos sao stubados/verificados contra os shapes reais de `contracts/`. NAO
> usar payloads inventados — usar os shapes verificados (Principio I).

## Pre-requisitos de ambiente
- PostgreSQL + RabbitMQ rodando (docker-compose local).
- Env vars: `GOB_STATE_URL`, `GOB_STATE_TOKEN`, credenciais ISAPI por dispositivo,
  `MEMBER_SYNC_INTERVAL_MINUTES`, `RETRY_MAX_ATTEMPTS`, `RETRY_INITIAL_BACKOFF_MS`.
- Migrations aplicadas (members, devices, member_processing_status, attendance_events).

---

## Cenario 1 — Registro automatico de dispositivo (US4 / FR-001, FR-002)

1. Subir a API local. Garantir `devices` vazia.
2. Enviar um heartbeat de um dispositivo nao registrado (MAC `AA:BB:CC:DD:EE:FF`).
   → **Expected**: 200; nova linha em `devices` com `is_active=true`,
   `last_heartbeat_at` setado.
3. Enviar novo heartbeat do MESMO dispositivo.
   → **Expected**: 200; NENHUMA linha nova; `last_heartbeat_at` atualizado
   (sem duplicacao — FR-002).

---

## Cenario 2 — Carga de membros filtra por url_selfie (US2 / FR-004..FR-007, SC-001)

1. Stub da GOB `GET /api/face-detection/members` retornando `{"success":true,
   "data":[ membroA com url_selfie, membroB SEM url_selfie ]}` (shape de
   `contracts/gob-api.md`).
2. Disparar a carga (`POST /admin/sync`).
   → **Expected**: exatamente 1 mensagem publicada em `member.processing`
   (membroA); membroB descartado (FR-006). `members` reflete a carga.
3. **Error case**: stub da GOB respondendo 500.
   → **Expected**: log estruturado de erro; NENHUMA mensagem publicada (sem carga
   parcial — US2 cenario 3).

---

## Cenario 3 — Worker registra usuario + face no dispositivo (US3 / FR-009..FR-013)

1. Publicar manualmente uma `ProcessingMessage` valida (CPF, name, urlSelfie) em
   `member.processing`. Stub ISAPI (Digest) ativo.
2. Worker consome.
   → **Expected** (na ordem):
   - `POST/PUT /ISAPI/AccessControl/UserInfo/Modify` com XML `<UserInfo>` contendo
     `employeeNo`=CPF + `name` (status 200/201 ou 200/204).
   - `POST /ISAPI/Intelligent/FDLib/faceDataRecord?format=json` multipart com parte
     `FaceDataRecord`={"type":"concurrent","faceLibType":"blackFD","FDID":"1",
     "FPID":CPF} + parte `FaceImage` `<CPF>.jpg` (status 200).
   - `POST /ISAPI/Event/notification/httpHosts` XML `<HttpHostNotification>`
     apontando para a API local (status 200/201).
   - `member_processing_status` com `user_synced=face_uploaded=webhook_set=true`,
     `last_stage=done`.
3. **Idempotencia (SC-003)**: re-publicar a MESMA mensagem.
   → **Expected**: re-execucao nao cria usuario/face duplicado (upsert por CPF);
   estado final identico.
4. **Error case (DLQ, FR-023, SC-004)**: stub ISAPI retornando 500 em todas as
   tentativas.
   → **Expected**: re-tenta ate `RETRY_MAX_ATTEMPTS` com backoff; depois a
   mensagem vai para `member.processing.dlq` (nao perdida).
5. **Error case (download de selfie falha)**: `url_selfie` aponta para 404.
   → **Expected**: retry; persistindo, DLQ + log estruturado (Edge Case spec).

---

## Cenario 4 — Marcacao de presenca via webhook (US1 / FR-014..FR-017, SC-002)

1. Membro com CPF `12345678901` ja carregado (Cenario 3). Stub GOB
   `POST /attendance/3ff4708cb695ad1a6e9f87cb714e1f22` retornando 2xx.
2. Enviar ao webhook o payload de reconhecimento com `AccessControllerEvent`
   contendo `employeeNoString="12345678901"` e `attendanceStatus="authorized"`
   (shape de `contracts/inbound-http.md`).
   → **Expected**: a API extrai o CPF de `employeeNoString`, envia
   `POST /attendance/3ff4708cb695ad1a6e9f87cb714e1f22` com header
   `Authorization: <GOB_STATE_TOKEN>` (SEM Bearer) e body
   `{"cpf":"123.456.789-01"}` (formato mascarado). `attendance_events.marked=true`.
   Latencia < 5s (SC-002).
3. **Idempotencia (FR-016)**: entregar o MESMO evento de novo (redelivery).
   → **Expected**: dedup por `event_key`; NENHUMA segunda marcacao enviada a GOB.
4. **Error case (membro desconhecido, FR-017)**: webhook com
   `employeeNoString="00000000000"` sem membro correspondente.
   → **Expected**: log estruturado de evento desconhecido; NENHUMA marcacao;
   `attendance_events.marked=false`.
5. **Error case (GOB rejeita, Principio III)**: stub GOB retornando 500.
   → **Expected**: retry/backoff; persistindo, DLQ; evento de reconhecimento nao
   perdido (Edge Case spec).

---

## Cenario 5 — Roundtrip End-to-End (borda backend ↔ externos)

> Obrigatorio: chamada REAL ao stub externo, captura do payload, comparacao de
> shape contra `contracts/`. Expoe drift de nomes de campo (ex. `employeeNoString`
> vs `employeeNo`) antes de acumular.

1. Subir stub HTTP que grava cada request recebido (GOB attendance + ISAPI).
2. Rodar o fluxo completo: carga (Cenario 2) → worker (Cenario 3) → webhook
   (Cenario 4) com um membro real de ponta a ponta.
3. Capturar os requests gravados pelo stub.
   → **Expected**:
   - Request a `UserInfo/Modify` contem a TAG `<employeeNo>` (nao
     `<employeeNoString>` — esse e so o campo de ENTRADA do webhook).
   - Request a `faceDataRecord` tem `FPID` == CPF enviado.
   - Request a `/attendance/...` tem `cpf` no formato mascarado e o MESMO CPF que
     o `employeeNoString` recebido (normalizado para digits) — fechando o loop
     de correlacao (Principio II).
4. **Expected (anti-drift)**: o CPF que entrou em `employeeNoString` no webhook e
   exatamente o que saiu em `{"cpf":...}` para a GOB. Qualquer divergencia de
   formato/campo falha o teste.

---

## Mapa cenario → requisito

| Cenario | Cobre |
|---------|-------|
| 1 | US4, FR-001, FR-002 |
| 2 | US2, FR-004..FR-007, SC-001 |
| 3 | US3, FR-008..FR-013, FR-022, FR-023, SC-003, SC-004 |
| 4 | US1, FR-014..FR-017, FR-022, SC-002 |
| 5 | Principio II (correlacao CPF), anti-drift de borda |
