# Relatorio Global de Features — presenca-facial-mvp

**Data:** 2026-06-20
**Projeto:** /Users/jot/Projects/_lab/Jot/face-attendance
**Diretorio:** docs/specs/
**Features analisadas:** 1
**Fase:** TERMINAL (review-features, onda-010) — agente-00c encerra aqui
**Branch:** agente-00c/presenca-facial (commit be9a2b0)

---

## Veredito final

> **MVP FUNCIONAL NO NUCLEO — NAO production-ready.**
> O servico Go esta implementado e verde no nucleo (FASES 1-7), porem o
> escopo de Observabilidade (FASE 8) e Testes E2E (FASE 9) NAO foi
> concluido, e ha lacunas de cobertura de testes. **26/34 tarefas**
> efetivamente registradas como `pass`. NAO classificar como 100% nem como
> production-ready ate fechar FASES 8-9 + cobertura.

---

## Tabela comparativa

| Feature | Descricao | % (state) | % (checkbox) | Criticidade Pendente | Sugestao |
|---------|-----------|-----------|--------------|----------------------|----------|
| presenca-facial-mvp | Microservico Go de presenca facial (GOB + HikVision ISAPI, Postgres, RabbitMQ) | ~76% (26/34) | 0% (drift) | C | CONTINUAR/PRIORIZAR resto |

Nota: a coluna "% (checkbox)" vem do `aggregate.sh` deterministico e marca
0% porque o executor NUNCA tickou os checkboxes em `tasks.md` (146 `[ ]`,
0 `[x]`). A coluna "% (state)" reflete o estado REAL registrado em
`state.json` (`.tasks[]` = 26 entradas `pass`). A verdade operacional e a
do state + filesystem, nao a dos checkboxes.

---

## Reconciliacao de tres fontes (Principio VI — veracidade)

| Sinal | Valor | Fonte |
|-------|-------|-------|
| Arquivos Go | 44 | `find *.go` |
| Build | VERDE | `go build ./...` exit 0 |
| Vet | VERDE | `go vet ./...` exit 0 |
| Unit tests | VERDE (6 pacotes) | `go test -short ./...`: config, domain, gob, hikvision, http, logging `ok` |
| Tarefas registradas pass | 26/34 | `state.json .tasks[]` |
| Headings de tarefa | 34 | `tasks.md` `### N.M` |
| Checkboxes marcados | 0/146 | `tasks.md` (drift do executor) |
| Pendencia humana | CHK052 | checklist (carga extrema) |

---

## Escopo concluido (FASES 1-7)

- **FASE 1** Requisitos residuais e fundacao (1.1-1.5) — pass
- **FASE 2** Dominio, configuracao, logging (2.1-2.4) — pass
- **FASE 3** Clientes HTTP externos GOB + HikVision ISAPI (3.1-3.5) — pass
- **FASE 4** Repositorios PostgreSQL (4.1-4.4) — pass
- **FASE 5** Seguranca, findings S1-S5 `[C]` (5.1-5.5) — pass
- **FASE 6** Fila RabbitMQ e resiliencia (6.1-6.3) — pass
- **FASE 7** Handlers HTTP e scheduler (7.1-7.4) — CODIGO PRESENTE
  (`internal/{http,scheduler}` compilam, unit tests de http verdes), porem
  **NAO registrado como task pass no state** (26/34). Tratar como
  "implementado, nao formalmente validado".

## Escopo INCOMPLETO (real)

- **FASE 8 — Observabilidade e Operacoes (8.1-8.2): NAO implementada.**
- **FASE 9 — Testes de Integracao e E2E (9.1-9.2): NAO implementada.**
- Total faltante: **8 de 34 tarefas** (7.1-7.4 nao-validadas formalmente +
  8.1-8.2 + 9.1-9.2).

## Lacunas de cobertura (real)

- `internal/queue`, `internal/scheduler`, `internal/worker`: **zero**
  `*_test.go`.
- `internal/repository`: tem `integration_test.go` e `roundtrip_test.go`,
  porem **gated por `//go:build integration`** — exigem PostgreSQL ativo
  (docker-compose) e **nao foram re-rodados** nesta execucao.

## Discrepancia de artefato

- Commit `be9a2b0` declara "implement Go microservice FASEs 1-9 (34/34
  tasks)" — **overclaim**: o state registra 26/34 e FASES 8-9 nao existem
  no codigo. A mensagem de commit nao reflete o estado real.

---

## Destaques

### Quase pronto (push final)
- Nucleo (FASES 1-6) solido e testado; FASE 7 com codigo presente. Falta
  fechar 8 tarefas para escopo MVP completo.

### Risco / divida
- Sem observabilidade (FASE 8) o servico nao tem como ser operado/monitorado
  em producao.
- Sem E2E (FASE 9) o fluxo ponta-a-ponta GOB→HikVision→Postgres→fila nao foi
  validado integrado.
- Pacotes queue/scheduler/worker sem testes — risco de regressao silenciosa.

---

## Acoes recomendadas (para retomada futura por humano ou feature-00c)

1. Implementar **FASE 8** (observabilidade: metricas/health/logs estruturados
   de operacao) — `docs/specs/presenca-facial-mvp/tasks.md` §8.1-8.2.
2. Implementar **FASE 9** (testes E2E + rodar integration tests com Postgres
   via docker-compose) — §9.1-9.2.
3. Adicionar `*_test.go` em `internal/{queue,scheduler,worker}`.
4. Re-rodar `go test -tags integration ./internal/repository/...` com
   Postgres ativo.
5. Resolver pendencia humana **CHK052** (comportamento sob carga extrema).
6. Corrigir a mensagem do commit be9a2b0 ou commit subsequente para refletir
   26/34 (nao 34/34); tickar checkboxes em `tasks.md` conforme o state.
7. So entao reavaliar release/production-readiness.

---

## Sugestao do agregador (deterministico)

`aggregate.sh` sugeriu `PRIORIZAR` (criticidade C pendente, <50% por
checkbox). Ajustado pelo estado real para **CONTINUAR** o nucleo e
**PRIORIZAR** o fechamento de FASES 8-9 + cobertura. Read-only: nenhum
arquivo movido/arquivado.
