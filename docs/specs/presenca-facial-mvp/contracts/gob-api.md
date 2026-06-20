# Contract — GOB API (externa, consumida)

**Feature**: `presenca-facial-mvp` | **Date**: 2026-06-20
**Direcao**: OUTBOUND (o sistema chama a GOB)
**Base URL**: `{GOB_STATE_URL}` (env, ex. `https://digital.gob-es.org.br` — `t.txt:9`)

> Contrato REAL extraido de `t.txt` (fornecido pelo operador). Nenhum campo
> inventado (Constitution Principio I). Itens nao presentes em `t.txt` estao
> marcados `[PROPOSTA — a validar na implementacao]`.

## GET /api/face-detection/members — listar membros

**Fonte**: `t.txt:8-27`, `briefing.md:43-62`. FR-004, FR-005.

### Request
```
GET {GOB_STATE_URL}/api/face-detection/members
Authorization: Bearer {GOB_STATE_TOKEN}
```

### Response 200 (application/json)
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

### Campos verificados de `data[]`
| Campo | Tipo | Notas |
|-------|------|-------|
| `id` | number | id do membro na GOB |
| `status` | string | ex. `REGULAR` |
| `created_at` | string (ISO 8601) | |
| `updated_at` | string (ISO 8601) | |
| `federal_document` | string | CPF em digits (`00000000000`) — chave de correlacao |
| `name` | string | nome |
| `mobile_number` | string | telefone formatado |
| `url_selfie` | string\|ausente | URL da selfie; ausente/vazio → membro descartado (FR-006) |

### Regras de consumo
- `success != true` ou erro HTTP → log estruturado, sem publicar mensagens
  parciais (FR-005, US2 cenario 3).
- Membro sem `url_selfie` → descartado, nao enfileirado (FR-006).
- Cada membro valido → uma mensagem na fila `member.processing` (FR-007).

> `[PROPOSTA — a validar na implementacao]`: paginacao da resposta. `t.txt` mostra
> um array unico em `data[]` sem campos de paginacao. Se a GOB paginar
> (centenas/milhares de membros, briefing.md:149), a implementacao deve detectar e
> tratar — mas NAO assumimos um esquema de paginacao especifico aqui (seria
> fabricar contrato). Comportamento default: consumir `data[]` como veio.

---

## POST /attendance/3ff4708cb695ad1a6e9f87cb714e1f22 — marcar presenca

**Fonte**: `t.txt:41-49`, `briefing.md:81-83,185-188`. FR-015.

### Request
```
POST {GOB_STATE_URL}/attendance/3ff4708cb695ad1a6e9f87cb714e1f22
Authorization: {GOB_STATE_TOKEN}
Content-Type: application/json

{ "cpf": "00.000.000-00" }
```

> ATENCAO: o header `Authorization` aqui NAO usa o prefixo `Bearer` — token cru
> (`t.txt:46`: `'Authorization' => 'GOB_STATE_TOKEN'`), diferente do GET de membros
> (que usa `Bearer`, `t.txt:10`). Os dois headers sao distintos e foram verificados
> separadamente. NAO unificar.

### Body
| Campo | Tipo | Formato | Notas |
|-------|------|---------|-------|
| `cpf` | string | mascarado `00.000.000-00` | formato verificado em `t.txt:48` |

### Idempotencia (FR-016)
- A re-entrega do mesmo evento de reconhecimento NAO deve re-enviar marcacao; a
  dedup e local (chave `event_key` em `attendance_events`, ver data-model).

### Tratamento de erro (FR-015, Principio III)
- 4xx/5xx → aplicar retry com backoff (`RETRY_MAX_ATTEMPTS`,
  `RETRY_INITIAL_BACKOFF_MS`); esgotado → DLQ. Nunca perder o evento.

> `[PROPOSTA — a validar na implementacao]`: shape da resposta de sucesso da
> marcacao (`t.txt` mostra apenas request). Tratamento default: status 2xx =
> sucesso; demais = falha → retry/DLQ. NAO assumimos um body de resposta especifico.
