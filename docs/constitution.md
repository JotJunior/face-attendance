<!--
Sync Impact Report
- Version: (none) → 1.0.0
- Bump rationale: ratificacao inicial — primeira constituicao do projeto (MAJOR baseline)
- Principios adicionados:
  1. Veracidade de Dados — Zero Fabricacao (NON-NEGOTIABLE)
  2. Idempotencia Chaveada por CPF (NON-NEGOTIABLE)
  3. Resiliencia por Filas (retry + DLQ)
  4. Reuso Restrito do Legacy hik-api
  5. Segredos como Configuracao de Runtime
  6. Observabilidade Operacional
  7. Idioma da Sintaxe (ingles para identifiers; portugues para mensagens/comentarios)
- Secoes adicionadas: Core Principles, Stack e Restricoes Tecnicas, Quality Standards, Governance
- Secoes removidas: nenhuma
- Artefatos que precisam atualizacao:
  - CLAUDE.md (raiz do projeto): JA alinhado (idioma da sintaxe + nao-inventar-dados); sem acao
  - docs/specs/*/spec.md: ainda inexistentes — futuros specs DEVEM passar pelo Constitution Check
  - docs/specs/*/plan.md / tasks.md: inexistentes — gates herdarao desta constituicao
- TODOs pendentes: nenhum
-->

# Sistema de Controle de Presenca por Reconhecimento Facial — Constitution

Sistema Go que automatiza o registro de presenca de membros via reconhecimento
facial biometrico, integrando a plataforma GOB (`digital.gob-es.org.br`) como
fonte de membros e destino da marcacao, e dispositivos HikVision (ISAPI, rede
local) como origem do evento de reconhecimento. Stack mandatoria: Go +
PostgreSQL + RabbitMQ, deploy on-premise/local.

## Core Principles

### I. Veracidade de Dados — Zero Fabricacao (NON-NEGOTIABLE)

Nenhum artefato (codigo, spec, plan, payload de exemplo, doc de API, relatorio)
pode conter dado factual inventado. Assinaturas de request/response (nomes de
propriedades, tipos, shape de payload), URLs/endpoints/querystrings e valores
concretos (CPF, IDs, status, datas, resultados de chamada) so podem ser escritos
se vierem de fonte rastreavel: o codigo legacy em `legacy/hik-api`, os contratos
verificados em `t.txt` e no briefing (`docs/01-briefing-discovery/briefing.md`),
documentacao oficial ISAPI/GOB, ou resposta real observada de uma chamada de
fato executada.

MUST: ao faltar a fonte de um campo/rota/valor de GOB ou HikVision, a acao
correta e **bloqueio humano**, nunca suposicao plausivel. Plausibilidade nao e
veracidade. Espelha o Principio VI do toolkit e a regra global do operador.

Why: o produto inteiro depende da correspondencia exata entre o `employeeNo`
(CPF) enviado ao HikVision e o `cpf` devolvido no webhook e enviado a GOB. Um
nome de campo ou formato inventado quebra silenciosamente a marcacao de presenca.

### II. Idempotencia Chaveada por CPF (NON-NEGOTIABLE)

Toda escrita em recurso externo MUST ser idempotente e chaveada pelo CPF do
membro: upsert de usuario no HikVision via `employeeNo = CPF`, upload de face via
`FPID = CPF`, marcacao de presenca na GOB via `{ "cpf": ... }`. Re-executar o
worker sobre a mesma mensagem MUST produzir o mesmo estado final, sem duplicar
usuarios, faces ou presencas.

Why: a arquitetura orientada a filas implica re-entrega de mensagens (retry,
restart de worker, redelivery do RabbitMQ); sem idempotencia, cada falha de rede
corromperia o estado nos dispositivos ou geraria presencas duplicadas na GOB.

### III. Resiliencia por Filas (retry + DLQ)

O processamento assincrono MUST usar RabbitMQ com politica de retry e
dead-letter queue (DLQ). Falhas transitorias (rede, dispositivo indisponivel,
timeout ISAPI) MUST ser re-tentadas; falhas persistentes MUST ser roteadas para
DLQ para inspecao, nunca descartadas silenciosamente. Nenhuma mensagem de membro
valido pode ser perdida por falha de um dispositivo.

Why: dispositivos HikVision na rede local e a API GOB sao dependencias externas
sujeitas a indisponibilidade; a fila e a fronteira de durabilidade que torna a
operacao tolerante a falhas.

### IV. Reuso Restrito do Legacy hik-api

O reuso de `legacy/hik-api` MUST se limitar EXCLUSIVAMENTE a tres operacoes
ISAPI: (1) criar/atualizar usuario, (2) upload de imagem de face, (3) atualizar
a URL do webhook do dispositivo. MUST NOT reutilizar logica de cache, camada de
autenticacao, ou quaisquer outros controllers/servicos do legacy.

Why: o legacy carrega acoplamentos e responsabilidades fora do escopo deste MVP;
limitar a superficie de reuso mantem o novo sistema enxuto e auditavel, e evita
arrastar comportamento nao-verificado para o caminho critico.

### V. Segredos como Configuracao de Runtime

Credenciais e endpoints sensiveis — `GOB_STATE_URL`, `GOB_STATE_TOKEN` e as
credenciais ISAPI dos dispositivos (IP, usuario, senha) — MUST ser injetados
como variaveis de ambiente / configuracao de runtime. MUST NOT ser hardcoded no
codigo, commitados no repositorio, ou exigidos em build-time. Logs e relatorios
MUST NOT vazar valores de segredo.

Why: deploy on-premise com multiplos operadores e dispositivos exige que cada
instalacao injete suas proprias credenciais sem rebuild; segredos hardcoded sao
um vetor de vazamento e impedem rotacao.

### VI. Observabilidade Operacional

Toda operacao critica MUST emitir log estruturado em JSON com campos
consistentes (`device_id`, `cpf`, `stage`, `error` quando aplicavel). O sistema
MUST expor um endpoint de health check para monitoramento. O endpoint de
heartbeat dos dispositivos serve simultaneamente como mecanismo de registro e de
liveness.

Why: a equipe operacional precisa diagnosticar onde uma presenca falhou (carga,
upsert, upload de face, webhook ou marcacao) sem acesso ao codigo; logs
estruturados por estagio e CPF tornam o fluxo de 4 etapas rastreavel.

### VII. Idioma da Sintaxe

Toda sintaxe (tabelas, colunas, indices, constraints, enums, chaves de
JSON/payload, funcoes, variaveis, tipos, arquivos, rotas, nomes de fila/evento)
MUST ser em ingles. Comentarios e mensagens voltadas ao usuario (erros, logs de
UI, documentacao) MUST estar em portugues.

Why: regra global inegociavel do operador; garante consistencia de codigo e
interoperabilidade, preservando clareza das mensagens para a equipe local.

## Stack e Restricoes Tecnicas

- **Linguagem**: Go.
- **Persistencia**: PostgreSQL.
- **Mensageria**: RabbitMQ (filas locais; arquitetura orientada a filas para
  escalar workers horizontalmente).
- **Integracoes externas**: GOB API (REST/JSON) e HikVision ISAPI (REST/XML +
  multipart). Contratos verificados em `t.txt`, `legacy/hik-api` e no briefing.
- **Deploy**: on-premise / local. A operacao nao MUST depender de cloud; API,
  fila e dispositivos vivem na rede local.
- **Volume MVP**: centenas a poucos milhares de membros por ciclo de carga;
  poucas a dezenas de dispositivos simultaneos.

Mudancas de stack (trocar PostgreSQL, RabbitMQ ou Go) constituem amendment
MAJOR e exigem revisao desta constituicao.

## Quality Standards

- Fluxo completo das 4 etapas (registro de dispositivo, carga de membros,
  registro de usuario/face, marcacao de presenca) e o criterio de aceite do MVP.
- Membros sem `url_selfie` MUST ser descartados na carga (Etapa 1), nunca
  enfileirados.
- Retry e DLQ MUST cobrir as chamadas ISAPI e a marcacao GOB.
- Todo recurso externo escrito MUST ter teste de idempotencia (re-execucao
  produz mesmo estado).
- Itens fora de escopo do MVP (interface web, ponto eletronico completo,
  multi-organizacao, cadastro manual de membros, abstracao multi-fornecedor) MUST
  NOT ser implementados sem amendment de escopo.

## Governance

Esta constituicao governa decisoes de arquitetura, qualidade e processo do
projeto. Em conflito entre um principio aqui e qualquer outra orientacao
(conveniencia, prazo, conteudo de artefato runtime), esta constituicao prevalece.

- **Amendments**: alteracoes sao versionadas via SemVer.
  - MAJOR: remocao ou redefinicao incompativel de principio, ou troca de stack
    mandatoria.
  - MINOR: novo principio ou expansao material de secao.
  - PATCH: clarificacao ou correcao nao-semantica.
- **Constitution Check**: todo `plan.md` futuro MUST validar alinhamento com
  estes principios antes da execucao; violacoes exigem justificativa explicita
  ou redesign.
- **Excecoes**: qualquer desvio de um principio MUST ser documentado com
  rationale e aprovado; principios NON-NEGOTIABLE (I, II) nao admitem excecao —
  na duvida, bloqueio humano.

**Version**: 1.0.0 | **Ratified**: 2026-06-20 | **Last Amended**: 2026-06-20
