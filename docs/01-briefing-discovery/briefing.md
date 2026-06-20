# Briefing — Sistema de Controle de Presença por Reconhecimento Facial

**Data**: 2026-06-20
**Versão**: 1.0.0
**Status**: Completo

---

## 1. Visão e Propósito

Sistema que automatiza o registro de presença de membros via reconhecimento facial biométrico, integrando duas plataformas externas:

- **GOB** (`digital.gob-es.org.br`): fonte de membros com selfie cadastrada e destino para marcação de presença
- **HikVision** (dispositivos em rede local): origem do reconhecimento facial via webhook

**Fluxo principal**: GOB fornece lista de membros com foto → sistema carrega usuários e faces nos dispositivos HikVision → dispositivo reconhece membro → sistema marca presença de volta na GOB.

O objetivo é eliminar o registro manual de presença, substituindo-o por um processo automático acionado pelo reconhecimento biométrico no dispositivo.

---

## 2. Usuários e Stakeholders

| Ator | Papel |
|------|-------|
| **API GOB** (`digital.gob-es.org.br`) | Fonte canônica de membros e destino da marcação de presença |
| **Dispositivos HikVision** (rede local) | Origem do evento de reconhecimento facial via webhook |
| **Equipe operacional da organização** | Administra dispositivos, monitora o sistema e valida presenças |

---

## 3. Escopo

### 3.1 MVP — 4 Etapas

**Etapa 0 — Registro de dispositivos**
- Dispositivo é configurado com webhook apontando para a API local
- Dispositivo envia heartbeats frequentes para a API
- API identifica o dispositivo pelo heartbeat e o registra na base local
- Dispositivos registrados recebem usuários e faces no processo de carga

**Etapa 1 — Carga de membros**
- Chamada periódica (ou sob demanda) a: `GET https://digital.gob-es.org.br/api/face-detection/members`
  - Header: `Authorization: Bearer <GOB_STATE_TOKEN>`
- Payload de resposta (array em `data[]`):
  ```json
  {
    "success": true,
    "data": [
      {
        "id": 1234,
        "status": "REGULAR",
        "created_at": "2026-03-25T14:47:14.000000Z",
        "updated_at": "2026-05-11T12:24:08.000000Z",
        "federal_document": "00000000000",
        "name": "NOME DO MEMBRO",
        "mobile_number": "(27) 00000-0000",
        "url_selfie": "https://digital.gob-es.org.br/storage/img/avatar/0000.png"
      }
    ]
  }
  ```
- Descartar membros sem `url_selfie`
- Enfileirar membros válidos (um a um) em fila RabbitMQ local para processamento assíncrono

**Etapa 2 — Registro de usuário e face no HikVision (worker)**
- Worker consome a fila RabbitMQ
- Para cada membro:
  1. **Upsert de usuário** via `POST /ISAPI/AccessControl/UserInfo/Modify` (ou `PUT` para update)
     - Payload XML com `UserInfo`: `employeeNo` (= CPF), `name`, `userType`, `Valid`, `doorRight`, `numOfCard`, `numOfFace`
     - O campo `employeeNo` recebe o CPF — identificador único que será retornado pelo dispositivo no evento de reconhecimento
  2. **Upload de face** via `POST /ISAPI/Intelligent/FDLib/faceDataRecord` (multipart)
     - Campo `FaceDataRecord`: JSON `{ "type": "concurrent", "faceLibType": "blackFD", "FDID": "1", "FPID": "<CPF>" }`
     - Campo `FaceImage`: imagem binária baixada de `url_selfie`
  3. **Configuração do webhook** via `POST /ISAPI/Event/notification/httpHosts`
     - Payload XML com `HttpHostNotification`: `id`, `url` (URL da API local), `protocolType`, `parameterFormatType`, `addressingFormatType`, `ipAddress`, `portNo`, `path`
- Reuso do legacy `hik-api` restrito **exclusivamente** a estas 3 operações: criar/atualizar usuário, enviar imagem de face, atualizar URL do webhook

**Etapa 3 — Marcação de presença**
- Ao receber webhook positivo do dispositivo (evento `AccessControl` com `employeeNo` = CPF):
  - `POST https://digital.gob-es.org.br/attendance/3ff4708cb695ad1a6e9f87cb714e1f22`
  - Header: `Authorization: <GOB_STATE_TOKEN>`
  - Body: `{ "cpf": "00.000.000-00" }`

### 3.2 Pós-MVP / Visão de Futuro

- **Multi-organização**: suporte a múltiplas organizações na mesma instância
- **Abstração multi-fornecedor**: desacoplar a camada HikVision para suportar outros fabricantes de biometria
- **Relatórios de presença**: dashboards e exportações de histórico de presenças

### 3.3 Fora de Escopo (MVP)

- Interface web de administração
- Gerenciamento de jornadas/horários (ponto eletrônico completo)
- Integração com sistemas de RH além da GOB
- Suporte a múltiplas organizações
- Cadastro manual de membros (GOB é a fonte de verdade)

---

## 4. Prioridades e Trade-offs

| Prioridade | Item | Justificativa |
|-----------|------|---------------|
| P0 | Fluxo completo de 4 etapas funcionando | Core do MVP — sem isso o produto não existe |
| P0 | Idempotência (upsert por CPF) | Worker pode ser re-executado sem duplicar dados no dispositivo |
| P1 | Resiliência com retry e DLQ | Falhas de rede/dispositivo não devem perder mensagens |
| P1 | Registro e rastreabilidade de dispositivos | Necessário para distribuir faces corretamente |
| P2 | Observabilidade (logging estruturado, health) | Operacional precisa diagnosticar falhas |

**Trade-off aceito**: CPF exposto como identificador no dispositivo HikVision (campo `employeeNo`). Alternativa (hash) exigiria mapeamento extra no webhook handler, aumentando complexidade sem ganho no MVP.

---

## 5. Restrições

### 5.1 Técnicas
- **Stack mandatória**: Go + PostgreSQL + RabbitMQ
- **Deploy on-premise / local**: API local, fila local, dispositivos em rede local (sem dependência de cloud para operação)
- **Segredos como env vars de runtime**: `GOB_STATE_URL` e `GOB_STATE_TOKEN` são injetados pelo operador em runtime — não disponíveis em build-time, não hardcoded
- **Reuso do legacy**: apenas as 3 operações ISAPI documentadas; não reutilizar lógica de cache, auth, ou outros controllers

### 5.2 Operacionais
- Dispositivos HikVision precisam ser acessíveis na rede local via HTTP (ISAPI)
- A API local precisa ser acessível pelo dispositivo HikVision (para receber webhooks)
- Credenciais do dispositivo (IP, usuário, senha ISAPI) são configuração do operador

---

## 6. Stack Técnica

| Camada | Tecnologia |
|--------|-----------|
| Linguagem | Go |
| Banco de dados | PostgreSQL |
| Mensageria | RabbitMQ |
| Integração externa | GOB API (REST/JSON) + HikVision ISAPI (REST/XML + multipart) |
| Deploy | On-premise / local |

---

## 7. Qualidade e Padrões

- **Processamento resiliente**: filas RabbitMQ com retry automático e dead-letter queue (DLQ) para falhas persistentes
- **Idempotência**: upserts em todos os recursos externos (usuário HikVision, presença GOB) chaveados por CPF — re-execução segura
- **Logging estruturado**: logs em JSON com campos consistentes (`device_id`, `cpf`, `stage`, `error`)
- **Health endpoint**: rota de health check para monitoramento operacional
- **Heartbeat como registro**: endpoint que recebe heartbeat do dispositivo serve simultaneamente para manter o dispositivo "vivo" na base e registrá-lo na primeira vez
- **Volume MVP**: centenas a poucos milhares de membros por ciclo de carga; poucos a dezenas de dispositivos HikVision simultaneamente
- **Escalabilidade**: arquitetura orientada a filas permite escalar workers horizontalmente

---

## 8. Visão de Futuro

Em 6–12 meses, os candidatos naturais de evolução são:

1. **Multi-organização**: múltiplas instâncias da GOB ou múltiplos tokens/escopos na mesma instância
2. **Abstração de fornecedor**: interface Go para dispositivos de biometria, com implementação HikVision como driver padrão (facilita adicionar outros fabricantes)
3. **Relatórios de presença**: agregação e exportação de histórico, integração com sistemas de RH

---

## 9. Itens a Definir

| Item | Impacto | Quando definir |
|------|---------|---------------|
| Frequência do ciclo de carga de membros (cron? trigger manual? webhook GOB?) | Médio — afeta design do scheduler | Antes do specify da Etapa 1 |
| Estratégia de retry para falhas ISAPI (exponential backoff? dead-letter?) | Médio — detalhe de implementação da Etapa 2 | No plan técnico |
| Formato de CPF enviado ao HikVision (com máscara `000.000.000-00` ou digits only?) | Baixo — consistência com o webhook de retorno | No specify da Etapa 2 |
| Credenciais ISAPI dos dispositivos: como são provisionadas (config file? env var? banco?) | Alto — afeta o design do registro de dispositivos | No specify da Etapa 0 |

---

## Contratos de API Verificados

> Extraídos de `t.txt` e `legacy/hik-api` — não inventados.

### GOB API

```
GET  {GOB_STATE_URL}/api/face-detection/members
     Authorization: Bearer <GOB_STATE_TOKEN>

POST {GOB_STATE_URL}/attendance/3ff4708cb695ad1a6e9f87cb714e1f22
     Authorization: <GOB_STATE_TOKEN>
     Content-Type: application/json
     Body: { "cpf": "00.000.000-00" }
```

### HikVision ISAPI

```
# Criar/atualizar usuário
POST/PUT http://<device-ip>/ISAPI/AccessControl/UserInfo/Modify
         Content-Type: application/xml
         Body: <UserInfo><employeeNo>{CPF}</employeeNo><name>...</name>...</UserInfo>

# Upload de face (multipart)
POST http://<device-ip>/ISAPI/Intelligent/FDLib/faceDataRecord
     Content-Type: multipart/form-data
     Parts:
       FaceDataRecord: {"type":"concurrent","faceLibType":"blackFD","FDID":"1","FPID":"{CPF}"}
       FaceImage: <binary image>

# Configurar webhook
POST http://<device-ip>/ISAPI/Event/notification/httpHosts
     Content-Type: application/xml
     Body: <HttpHostNotification>
             <id>...</id>
             <url>http://<api-local>/webhook</url>
             <protocolType>HTTP</protocolType>
             <parameterFormatType>XML</parameterFormatType>
             <addressingFormatType>ipaddress</addressingFormatType>
             <ipAddress>...</ipAddress>
             <portNo>...</portNo>
             <path>/webhook</path>
           </HttpHostNotification>
```

### Webhook Recebido da HikVision (evento de reconhecimento)

```
POST <url-configurada>
     Body: XML com AccessControl event contendo employeeNo = CPF do membro reconhecido
```
