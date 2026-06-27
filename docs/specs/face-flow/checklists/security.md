# Security Checklist: face-flow (Motor que Executa Chamadas Externas com Variáveis do Usuário)

**Purpose**: Validar a QUALIDADE dos requisitos de segurança do motor de execução,
que dispara chamadas externas (HTTPS, mensagem, ISAPI) interpolando dados reais do
membro/dispositivo. Valida requisito, não implementação.
**Created**: 2026-06-26
**Feature**: [spec.md](../spec.md)

## Superfície de Chamada Externa (SSRF)

- [ ] CHK001 - Há requisito restringindo o destino da chamada HTTPS (allowlist de hosts / bloqueio de IPs internos/metadata) para mitigar SSRF? [Gap, Spec §FR-014] {auto} — Gap: o nó 5 dispara para URL arbitrária configurada e roda no host on-premise (mesma LAN dos devices e serviços internos). Nenhum requisito limita o alvo. Mesmo com admin autenticado, o motor dispara automaticamente a cada reconhecimento — vetor SSRF não endereçado.
- [ ] CHK002 - O QR code (nó 6) que pode conter `[user.document]` (CPF) exibido na tela do dispositivo tem requisito de privacidade/limitação de conteúdo sensível? [Gap, Spec §FR-015/FR-020] {auto} — Gap: nó 6 interpola variáveis (incl. CPF) em QR de fundo exibido publicamente no terminal; nenhum requisito restringe exposição de PII.

## Injeção via Interpolação de Variáveis

- [ ] CHK003 - Há requisito de sanitização dos valores interpolados ao compor HEADERS HTTP (prevenção de header/CRLF injection via `[user.name]` etc.)? [Gap, Spec §FR-020] {auto} — Gap: FR-020 trata variável ausente/sintaxe inválida, mas não define sanitização do valor injetado em contexto de header. Dados de membro vêm do GOB (parcialmente externos).
- [x] CHK004 - O comportamento para variável fora do vocabulário / ausente está definido de forma fechada (sem vazar contexto não previsto)? [Clareza, Spec §FR-020] {auto} — Satisfeito: vocabulário fechado (tabela FR-020); ausente→"", desconhecida→literal. Não há expansão arbitrária de contexto.

## Segredos em Configuração de Nó

- [ ] CHK005 - Há requisito de proteção (cifragem em repouso) para segredos embutidos na config do nó HTTPS (ex.: `Authorization: Bearer <token>`)? [Gap, Spec §US3-AS3 vs Constitution §segredos] {auto} — Gap: US3 Acceptance #3 prevê header `Authorization: Bearer <token>`; a config do nó é JSONB no DB (plan §4.1) sem requisito de cifragem, divergindo do tratamento de segredos do projeto (AES-256-GCM para credenciais ISAPI). Token armazenado em claro.
- [ ] CHK006 - Os segredos de config de nó têm requisito de não-exposição em respostas da API admin (ex.: GET /flows/{id} retorna headers com token)? [Gap, Spec §FR-001/plan §5] {auto} — Gap: nenhum requisito de mascaramento de segredos ao ler o fluxo via API.

## Logging e Auditoria sem Vazamento

- [ ] CHK007 - O FlowExecutionLog e o log estruturado (FR-021) têm requisito de NÃO persistir o body interpolado / segredos / CPF em claro? [Gap, Spec §FR-021/Key Entities vs Constitution §logging] {auto} — Gap: FR-021 loga `error`; em falha do nó HTTPS o erro pode conter URL+body interpolado (CPF, token). Constituição exige CPF mascarado em log (`MaskCPFForLog`); nenhum requisito aplica isso ao motor de fluxo.
- [x] CHK008 - Eventos relevantes (execução, circuit-break) são registrados para auditoria? [Cobertura, Spec §FR-021/CL-001] {auto} — Satisfeito: FlowExecutionLog (status completed/circuit_break, started/finished_at) + slog complementar.

## Autorização da Gestão de Fluxos

- [x] CHK009 - O acesso à gestão de fluxos está restrito ao painel admin autenticado? [Cobertura, Spec §FR-001/CL-002/plan §5] {auto} — Satisfeito: telas integram a SPA admin (CL-002); rotas sob AdminAuth+Session (plan §5). Editor não é exposto sem autenticação.
- [x] CHK010 - O gatilho do motor está restrito ao único evento válido (sem acionamento por evento forjado de outro tipo)? [Clareza, Spec §FR-018/SC-003] {auto} — Satisfeito: FR-018 (somente `AccessControllerEvent`), SC-003 (100% dos demais ignorados); webhook já tem IP allowlist (CLAUDE.md).

## Princípio I — Zero Fabricação (contratos externos)

- [x] CHK011 - Requisitos que dependem de contrato ISAPI/API não verificado estão marcados como bloqueados, sem assinatura inventada? [Completude, Spec §FR-010/011/013/015/017] {auto} — Satisfeito: nós 1/2/4/6/8 marcados `[BLOCKED_ISAPI]`/`[BLOCKED_API]`; nós 4/6 desbloqueados via fonte verificada `client_standby.go` (plan §9). Nenhum endpoint fabricado.

## Priorização (decisão do dono do produto)

- [ ] CHK012 - O risco de SSRF (CHK001) é aceitável dado que apenas o admin configura a URL, ou exige allowlist/controle de destino? [Risco] {humano}
- [ ] CHK013 - A exposição de PII (CPF) em QR de fundo (CHK002) e em logs (CHK007) está dentro da política de privacidade/LGPD do produto? [Risco, Compliance] {humano}
- [ ] CHK014 - Armazenar segredos de nó (Bearer token) em claro (CHK005/CHK006) é aceitável para o modelo de ameaça on-premise, ou exige cifragem como nas credenciais ISAPI? [Risco] {humano}

## Notes

- Achados de maior risco: CHK001 (SSRF), CHK005/CHK006 (segredos em claro), CHK007 (CPF/segredo em log) — todos `[Gap]` de requisito, não de implementação.
- Encaminhamento: `[Gap]` → `/create-tasks` ("definir requisito de X") ou `/clarify`; `{humano}` → decisão do dono do produto antes de `/execute-task` dos nós afetados.
- NÃO são bloqueios humanos de contrato externo (esses são B-001/B-002, já documentados); são gaps de requisito de segurança internos.
