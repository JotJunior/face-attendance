/**
 * Presença Facial — Admin SPA (vanilla, embeddable, sem build step)
 * Implementa o design Presenca Facial.dc.html sobre os contratos reais /admin/api/*.
 *
 * Telas existentes (Login, Visão geral, Dispositivos, Membros, Eventos) são ligadas
 * aos endpoints reais. As telas novas (Configuração do dispositivo, Perfil do membro)
 * seguem o design-brief §7/§8: a UI é construída contra o modelo de dados documentado;
 * onde o endpoint de backend ainda não existe, mostramos estado "aguardando backend"
 * em vez de inventar dados (degradação graciosa).
 *
 * Roteamento por hash · sessão por cookie admin_session · timeouts via AbortController.
 */

// ─── CONSTANTS ────────────────────────────────────────────────
const API_TIMEOUT_MS      = 10_000;
const SYNC_TIMEOUT_MS      = 60_000;
const DEBOUNCE_MS          = 300;
const TOAST_MS             = 4_200;
const THEME_KEY            = 'pf-theme';

// ─── STATE ────────────────────────────────────────────────────
const state = {
  route: null,
  devices: { items: [], byId: {}, loaded: false, query: '', filter: 'all' },
  members: { items: [], byId: {}, nextCursor: null, hasMore: false, query: '', filter: 'all' },
  events:  { items: [], nextCursor: null, hasMore: false, period: '24h' },
  syncInProgress: false,
};

// ─── THEME ────────────────────────────────────────────────────
function getTheme() {
  return localStorage.getItem(THEME_KEY) || 'dark';
}
function applyTheme(t) {
  document.documentElement.setAttribute('data-theme', t);
  localStorage.setItem(THEME_KEY, t);
  const btn = $('theme-btn');
  if (btn) btn.innerHTML = t === 'dark' ? ICON.moon : ICON.sun;
}
function toggleTheme() {
  applyTheme(getTheme() === 'dark' ? 'light' : 'dark');
}

// ─── ICONS ────────────────────────────────────────────────────
const ICON = {
  moon: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3 7 7 0 0 0 21 12.8z"/></svg>`,
  sun:  `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>`,
  resync: `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.5 15a9 9 0 1 1-2.1-9.4L23 10"/></svg>`,
  chevron: `<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="9 6 15 12 9 18"/></svg>`,
  back: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="15 18 9 12 15 6"/></svg>`,
  search: `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="7"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>`,
  plus: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>`,
  trash: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`,
  upload: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 9 12 4 17 9"/><line x1="12" y1="4" x2="12" y2="16"/></svg>`,
  warnTri: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><line x1="12" y1="9" x2="12" y2="13.5"/><line x1="12" y1="17" x2="12" y2="17"/></svg>`,
  info: `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="9"/><path d="M12 16v-4M12 8h0"/></svg>`,
  device: `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/></svg>`,
  camera: `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 7l-7 5 7 5V7z"/><rect x="1" y="5" width="15" height="14" rx="2"/></svg>`,
  members: `<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/><path d="M16 5.5a3 3 0 0 1 0 5.5"/></svg>`,
  bolt: `<svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M22 12h-4l-3 8-6-16-3 8H2"/></svg>`,
};

const CFG_SECTIONS = [
  { id:'overview',     label:'Visão geral',          icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16"/><line x1="12" y1="8" x2="12" y2="8"/></svg>` },
  { id:'system',       label:'Sistema & manutenção',  icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><line x1="4" y1="8" x2="20" y2="8"/><circle cx="9" cy="8" r="2"/><line x1="4" y1="16" x2="20" y2="16"/><circle cx="15" cy="16" r="2"/></svg>` },
  { id:'auth',         label:'Autenticação',          icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><path d="M4 8V6a2 2 0 0 1 2-2h2M16 4h2a2 2 0 0 1 2 2v2M20 16v2a2 2 0 0 1-2 2h-2M8 20H6a2 2 0 0 1-2-2v-2"/><circle cx="12" cy="11" r="2"/><path d="M9 15.5a3.5 3.5 0 0 1 6 0"/></svg>` },
  { id:'doors',        label:'Portas & acesso',       icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="5" y="3" width="14" height="18" rx="1.5"/><circle cx="15" cy="12" r="1"/></svg>` },
  { id:'users',        label:'Usuários no device',    icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/><path d="M16 5.5a3 3 0 0 1 0 5.5"/></svg>` },
  { id:'cards',        label:'Cartões',               icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="5" width="18" height="14" rx="2"/><line x1="3" y1="10" x2="21" y2="10"/></svg>` },
  { id:'faces',        label:'Biblioteca de faces',   icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><circle cx="9.5" cy="11" r="1"/><circle cx="14.5" cy="11" r="1"/><path d="M9 15a3.5 3.5 0 0 0 6 0"/></svg>` },
  { id:'events',       label:'Eventos & webhooks',    icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M10 21a2 2 0 0 0 4 0"/></svg>` },
  { id:'preferences',  label:'Preferências',          icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>` },
  { id:'media',        label:'Mídia',                 icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m4 18 5-5 4 4 3-3 4 4"/></svg>` },
];

// ─── FETCH ────────────────────────────────────────────────────
async function apiFetch(url, options = {}, timeoutMs = API_TIMEOUT_MS) {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);
  try {
    const res = await fetch(url, { ...options, credentials: 'same-origin', signal: ctrl.signal });
    clearTimeout(timer);
    if (res.status === 401) {
      const current = (window.location.hash.replace('#', '') || 'dashboard').split('?')[0];
      navigate(`login?redirect=${encodeURIComponent(current)}`, false);
      return res;
    }
    return res;
  } catch (err) {
    clearTimeout(timer);
    if (err.name === 'AbortError') throw new Error('timeout');
    throw err;
  }
}
const apiGet    = (path, t)        => apiFetch(`/admin/api/${path}`, { method: 'GET' }, t);
const apiPost   = (path, body, t)  => apiFetch(`/admin/api/${path}`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: body !== undefined ? JSON.stringify(body) : undefined,
}, t);
const apiPut    = (path, body, t)  => apiFetch(`/admin/api/${path}`, {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: body !== undefined ? JSON.stringify(body) : undefined,
}, t);
const apiDelete = (path, body, t)  => apiFetch(`/admin/api/${path}`, {
  method: 'DELETE',
  headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
  body: body !== undefined ? JSON.stringify(body) : undefined,
}, t);

// ─── DOM HELPERS ──────────────────────────────────────────────
function $(id) { return document.getElementById(id); }
function setView(html) { $('view').innerHTML = html; }
function escHtml(s) {
  return String(s ?? '').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}
function initials(name) {
  const p = String(name || '').trim().split(/\s+/);
  return (((p[0]||'')[0]||'') + ((p[p.length-1]||'')[0]||'')).toUpperCase() || '?';
}
function fmtDateTime(iso) {
  if (!iso) return '—';
  try {
    return new Intl.DateTimeFormat('pt-BR', { day:'2-digit', month:'2-digit', year:'numeric', hour:'2-digit', minute:'2-digit' }).format(new Date(iso));
  } catch { return iso; }
}
function fmtShort(iso) {
  if (!iso) return '—';
  try {
    return new Intl.DateTimeFormat('pt-BR', { day:'2-digit', month:'2-digit', hour:'2-digit', minute:'2-digit' }).format(new Date(iso)).replace(',', ' ·');
  } catch { return iso; }
}
function badge(kind, label) { return `<span class="badge badge-${kind}">${escHtml(label)}</span>`; }

// Mapeamentos de status → badge (a partir dos campos reais da API)
function deviceStatusBadge(s) { return s === 'active' ? badge('ok','Online') : badge('off','Offline'); }
function webhookBadge(b) { return b ? badge('muted','Configurado') : badge('warn','Ausente'); }
function memberStatusBadge(s) {
  const l = String(s || '').toLowerCase();
  if (l === 'ativo')   return badge('ok','Ativo');
  if (l === 'inativo') return badge('muted','Inativo');
  return badge('muted', s || '—');
}
const PROV = { synced:['ok','Provisionado'], failed:['off','Falhou'], pending:['muted','Pendente'] };
function provBadge(syncStatus) { const m = PROV[syncStatus] || ['muted', syncStatus || '—']; return badge(m[0], m[1]); }
const MARK = { marked:['ok','Marcado'], pending:['warn','Pendente'], failed:['off','Falhou'], unauthorized:['muted','Não-autorizado'] };
function markBadge(s) { const m = MARK[s] || ['muted', s || '—']; return badge(m[0], m[1]); }
// "Resultado" derivado do que a API expõe: EventView traz marking_status, não attendance_status.
// 'unauthorized' = match negado; demais estados implicam membro reconhecido (autorizado).
function resultBadge(markingStatus) { return markingStatus === 'unauthorized' ? badge('off','Negado') : badge('ok','Autorizado'); }

function pendingNote(text) {
  return `<div class="pending-note">${ICON.info}<span>${escHtml(text)}</span></div>`;
}
function emptyState(icon, title, sub) {
  return `<div class="empty">${icon}<div class="et">${escHtml(title)}</div><div class="es">${escHtml(sub)}</div></div>`;
}
function loadingState() {
  return `<div class="loading" role="status"><div class="spinner" aria-hidden="true"></div><span>Carregando…</span></div>`;
}

// ─── TOAST ────────────────────────────────────────────────────
function showToast(type, msg) {
  const wrap = $('toast-wrap');
  const el = document.createElement('div');
  el.className = `toast ${type}`;
  el.setAttribute('role', 'status');
  el.innerHTML = `<span class="tdot" aria-hidden="true"></span><span class="tmsg">${escHtml(msg)}</span>
    <button class="tclose" aria-label="Fechar"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 18L18 6M6 6l12 12"/></svg></button>`;
  const dismiss = () => { el.classList.add('removing'); el.addEventListener('animationend', () => el.remove(), { once:true }); };
  el.querySelector('.tclose').addEventListener('click', dismiss);
  wrap.appendChild(el);
  setTimeout(dismiss, TOAST_MS);
}
function netError(err) { showToast('error', err.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Falha de conexão.'); }
// Ação que depende de um endpoint de backend ainda não implementado (design-brief §7).
function pendingBackend(label) { showToast('info', `${label}: integração com o terminal ainda não disponível neste build.`); }

// ─── CONFIRM MODAL ────────────────────────────────────────────
let modalCtx = null;
function openConfirm(opts) {
  // opts: { title, body, confirmLabel, tone:'warn'|'danger', strong:bool, target:{name,ip,mac,status}, onConfirm }
  modalCtx = opts;
  const t = opts.target || {};
  const strongBlock = opts.strong ? `
    <div style="margin-top:16px;">
      <div style="font-size:12px; color:var(--text-2); margin-bottom:7px;">Para confirmar, digite o identificador do dispositivo — <strong class="mono" style="color:var(--text);">${escHtml(t.name)}</strong></div>
      <input id="modal-confirm-input" class="input" placeholder="Digite o identificador" autocomplete="off" />
    </div>` : '';
  $('modal-root').innerHTML = `
    <div class="modal-overlay" id="modal-overlay" role="dialog" aria-modal="true" aria-label="${escHtml(opts.title)}">
      <div class="modal" id="modal-card">
        <div class="modal-body">
          <div style="display:flex; gap:14px; align-items:flex-start;">
            <div class="modal-icon ${opts.tone === 'danger' ? 'danger' : 'warn'}">${ICON.warnTri}</div>
            <div style="flex:1; min-width:0;">
              <div class="modal-title">${escHtml(opts.title)}</div>
              <div class="modal-text">${escHtml(opts.body)}</div>
            </div>
          </div>
          <div class="modal-target">
            <div class="k">Dispositivo alvo</div>
            <div style="display:flex; align-items:center; gap:9px;">
              <div style="flex:1; min-width:0;">
                <div style="font-size:13px; font-weight:600;">${escHtml(t.name || '—')}</div>
                <div class="mono" style="font-size:11px; color:var(--text-3);">${escHtml(t.ip || '—')}${t.mac ? ' · ' + escHtml(t.mac) : ''}</div>
              </div>
              ${deviceStatusBadge(t.status)}
            </div>
          </div>
          ${strongBlock}
        </div>
        <div class="modal-foot">
          <button class="btn btn-ghost" id="modal-cancel">Cancelar</button>
          <button class="btn ${opts.tone === 'danger' ? 'btn-danger' : 'btn-warn-outline'}" id="modal-confirm" ${opts.strong ? 'disabled' : ''}>${escHtml(opts.confirmLabel)}</button>
        </div>
      </div>
    </div>`;

  $('modal-overlay').addEventListener('click', e => { if (e.target.id === 'modal-overlay') closeConfirm(); });
  $('modal-cancel').addEventListener('click', closeConfirm);
  $('modal-confirm').addEventListener('click', () => {
    if (modalCtx && modalCtx.onConfirm) modalCtx.onConfirm();
    closeConfirm();
  });
  if (opts.strong) {
    const input = $('modal-confirm-input');
    input.addEventListener('input', () => { $('modal-confirm').disabled = input.value.trim() !== opts.target.name; });
    input.focus();
  } else {
    $('modal-confirm').focus();
  }
  document.addEventListener('keydown', modalEsc);
}
function modalEsc(e) { if (e.key === 'Escape') closeConfirm(); }
function closeConfirm() { $('modal-root').innerHTML = ''; modalCtx = null; document.removeEventListener('keydown', modalEsc); }

// ─── ROUTING ──────────────────────────────────────────────────
const ROUTES = ['dashboard','devices','device-config','members','member-profile','events','login','flows','flows-edit','flows-logs'];
const TITLES = {
  dashboard:      ['Visão geral', 'Panorama da operação · presença, dispositivos e provisão'],
  devices:        ['Dispositivos', 'Terminais HikVision na rede local'],
  'device-config':['Configuração do dispositivo', 'Configuração completa do terminal'],
  members:        ['Membros', 'Membros sincronizados do GOB'],
  'member-profile':['Perfil do membro', 'Dados, provisão e histórico de acessos'],
  events:         ['Eventos', 'Log cronológico de reconhecimentos'],
  flows:          ['Fluxos', 'Automação pós-reconhecimento facial'],
  'flows-edit':   ['Editor de fluxo', 'Editor visual de automação de presença'],
  'flows-logs':   ['Logs de execução', 'Histórico de execuções do fluxo'],
};

function parseHash() {
  const raw = window.location.hash.replace('#', '') || 'dashboard';
  const [route, qs] = raw.split('?');
  const params = Object.fromEntries(new URLSearchParams(qs || ''));
  return { route: ROUTES.includes(route) ? route : 'dashboard', params };
}
function navigate(route, push = true) {
  if (push) window.location.hash = route;
  else window.history.replaceState(null, '', `#${route}`);
  renderRoute();
}

function renderRoute() {
  const { route, params } = parseHash();
  state.route = route;
  const app = $('app');
  const isLogin = route === 'login';
  app.classList.toggle('is-login', isLogin);
  app.classList.remove('nav-open');

  // topbar title
  const [title, sub] = TITLES[route] || TITLES.dashboard;
  $('page-title').textContent = title;
  $('page-sub').textContent = sub;

  // active nav (device-config counts as devices; member-profile as members; flows-edit/logs as flows)
  const navKey = route === 'device-config' ? 'devices' : route === 'member-profile' ? 'members' : (route === 'flows-edit' || route === 'flows-logs') ? 'flows' : route;
  document.querySelectorAll('.nav-item').forEach(b => b.classList.toggle('active', b.dataset.route === navKey));

  window.scrollTo(0, 0);
  switch (route) {
    case 'login':           renderLogin(params); break;
    case 'dashboard':       loadDashboard(); break;
    case 'devices':         mountDevices(); break;
    case 'device-config':   mountDeviceConfig(params); break;
    case 'members':         mountMembers(); break;
    case 'member-profile':  mountMemberProfile(params); break;
    case 'events':          mountEvents(); break;
    case 'flows':           mountFlowsList(); break;
    case 'flows-edit':      mountFlowsEditor(params); break;
    case 'flows-logs':      mountFlowsLogs(params); break;
  }
}

// ─── LOGIN ────────────────────────────────────────────────────
function renderLogin(params) {
  setView(`
    <div class="login-wrap">
      <div class="login-box">
        <div class="login-brand">
          <div class="brand-lockup">
            <img class="brand-icon" src="/admin/assets/acceno-icon.png" alt="" />
            <span class="brand-name">Acceno</span>
          </div>
          <div style="text-align:center;">
            <div class="t2">Painel administrativo · on-premise</div>
          </div>
        </div>
        <form class="login-card" id="login-form" novalidate>
          <label class="label" for="login-user">Usuário</label>
          <input class="input" id="login-user" name="username" autocomplete="username" placeholder="operador" required style="margin-bottom:14px;" />
          <label class="label" for="login-pass">Senha</label>
          <input class="input" id="login-pass" name="password" type="password" autocomplete="current-password" placeholder="••••••••" required style="margin-bottom:18px;" />
          <button type="submit" class="btn btn-accent block" id="login-submit">Entrar</button>
          <div class="login-err" id="login-err" role="alert" aria-live="assertive"></div>
          <div class="login-foot">
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="11" width="16" height="10" rx="2"/><path d="M8 11V8a4 4 0 0 1 8 0v3"/></svg>
            Conexão local segura · admin_session
          </div>
        </form>
      </div>
    </div>`);

  const form = $('login-form');
  const err = $('login-err');
  form.addEventListener('submit', async e => {
    e.preventDefault();
    const btn = $('login-submit');
    const username = $('login-user').value.trim();
    const password = $('login-pass').value;
    if (!username || !password) { err.textContent = 'Preencha usuário e senha.'; err.classList.add('show'); return; }
    btn.disabled = true; btn.textContent = 'Entrando…'; err.classList.remove('show');
    try {
      const res = await apiPost('login', { username, password });
      if (res.status === 204) {
        const dest = params.redirect || 'dashboard';
        navigate(dest);
      } else if (res.status === 401) {
        let msg = 'Credenciais inválidas.';
        try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
        err.textContent = msg; err.classList.add('show');
        $('login-pass').value = ''; $('login-pass').focus();
      } else {
        err.textContent = 'Erro inesperado. Tente novamente.'; err.classList.add('show');
      }
    } catch (e2) {
      err.textContent = e2.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Erro de conexão.';
      err.classList.add('show');
    } finally {
      btn.disabled = false; btn.textContent = 'Entrar';
    }
  });
}

async function doLogout() {
  try { await apiPost('logout', undefined); } catch {}
  navigate('login', false);
}

// ─── SYNC ─────────────────────────────────────────────────────
async function doSync() {
  if (state.syncInProgress) return;
  state.syncInProgress = true;
  const btn = $('sync-btn'), label = $('sync-label');
  btn.disabled = true; if (label) label.textContent = 'Sincronizando…';
  try {
    const res = await apiPost('sync', undefined, SYNC_TIMEOUT_MS);
    if (res.status === 202)      showToast('success', 'Sincronização iniciada — puxando membros do GOB.');
    else if (res.status === 409) showToast('info', 'Sincronização já em andamento. Aguarde a conclusão.');
    else if (res.status !== 401) showToast('error', `Falha na sincronização (status ${res.status}).`);
  } catch (err) {
    showToast('error', err.message === 'timeout' ? 'Sincronização: tempo esgotado após 60s.' : 'Falha na sincronização.');
  } finally {
    state.syncInProgress = false;
    btn.disabled = false; if (label) label.textContent = 'Sincronizar';
  }
}

// ─── DASHBOARD ────────────────────────────────────────────────
async function loadDashboard() {
  setView(loadingState());
  try {
    const [statsRes, devRes, evtRes] = await Promise.all([
      apiGet('stats'),
      apiGet('devices'),
      apiGet('events?limit=5'),
    ]);
    if ([statsRes, devRes, evtRes].some(r => r.status === 401)) return;
    if (!statsRes.ok) { setView(emptyState(ICON.bolt, 'Não foi possível carregar', `Métricas indisponíveis (status ${statsRes.status}).`)); return; }
    const stats = await statsRes.json();
    const devices = devRes.ok ? (await devRes.json()).devices || [] : [];
    const events = evtRes.ok ? (await evtRes.json()).events || [] : [];
    renderDashboard(stats, devices, events);
  } catch (err) {
    setView(emptyState(ICON.bolt, 'Falha de conexão', 'Verifique a rede e tente novamente.'));
    netError(err);
  }
}

function renderDashboard(stats, devices, events) {
  const active = stats.devices_active ?? 0;
  const inactive = stats.devices_inactive ?? 0;
  const total = active + inactive;
  const thr = stats.device_offline_threshold_hours ?? 24;
  const offline = devices.filter(d => d.status !== 'active');

  const alert = inactive > 0 ? `
    <div class="alert alert-warn">
      <span style="color:var(--warn); flex:none;">${ICON.warnTri}</span>
      <div class="grow"><strong style="color:var(--warn);">${inactive} dispositivo${inactive>1?'s':''} offline</strong> — sem heartbeat nas últimas ${thr}h. Presenças nesses pontos podem não estar sendo registradas.</div>
      <button class="btn btn-soft sm" data-route="devices" id="alert-go-devices">Ver dispositivos</button>
    </div>` : '';

  const recent = events.length ? events.slice(0, 5).map(ev => {
    const name = ev.member_name || 'Não reconhecido';
    const neg = ev.marking_status === 'unauthorized';
    return `
      <div class="trow" style="grid-template-columns:34px 1fr auto auto;">
        <div class="avatar ${neg ? 'neg' : ''}">${neg ? '??' : escHtml(initials(name))}</div>
        <div style="min-width:0;">
          <div class="cell-ellipsis" style="font-size:13px; font-weight:500;">${escHtml(name)}</div>
          <div class="mono" style="font-size:11px; color:var(--text-3);">${escHtml(ev.device_identifier || '—')} · ${escHtml(fmtShort(ev.event_datetime || ev.created_at))}</div>
        </div>
        ${resultBadge(ev.marking_status)}
        ${markBadge(ev.marking_status)}
      </div>`;
  }).join('') : `<div style="padding:18px;">${emptyState(ICON.bolt, 'Sem presenças recentes', 'Os reconhecimentos capturados pelos terminais aparecem aqui.')}</div>`;

  const rail = devices.length ? devices.slice(0, 5).map(d => `
    <div class="card-row" style="display:flex; align-items:center; gap:11px; cursor:pointer;" data-device-id="${escHtml(String(d.id))}">
      <span class="dot ${d.status === 'active' ? 'dot-on' : 'dot-off'}"></span>
      <div style="flex:1; min-width:0;">
        <div class="cell-ellipsis" style="font-size:12.5px; font-weight:500;">${escHtml(d.device_identifier)}</div>
        <div class="mono" style="font-size:10.5px; color:var(--text-3);">${escHtml(d.ip_address || '—')} · ${escHtml(fmtShort(d.last_heartbeat_at))}</div>
      </div>
      ${deviceStatusBadge(d.status)}
    </div>`).join('') : `<div style="padding:16px;">${emptyState(ICON.device, 'Nenhum dispositivo', 'Cadastre terminais para começar.')}</div>`;

  setView(`
    <div class="screen">
      ${alert}
      <div class="kpis">
        <div class="kpi">
          <div class="kpi-label">${svg14('<circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/>')} Membros com selfie</div>
          <div class="kpi-value">${(stats.members_with_selfie ?? 0).toLocaleString('pt-BR')}</div>
          <div class="kpi-sub">prontos para provisionar</div>
        </div>
        <div class="kpi">
          <div class="glow"></div>
          <div class="kpi-label">${svg14('<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/>')} Dispositivos online</div>
          <div class="kpi-value">${active}<span class="frac"> / ${total}</span></div>
          <div class="kpi-sub ${inactive>0?'warnt':'good'}">${inactive>0 ? inactive + ' offline · requer atenção' : 'todos online'}</div>
        </div>
        <div class="kpi">
          <div class="kpi-label">${svg14('<path d="M22 12h-4l-3 8-6-16-3 8H2"/>')} Presenças · 24h</div>
          <div class="kpi-value">${(stats.attendance_last_24h ?? 0).toLocaleString('pt-BR')}</div>
          <div class="kpi-sub">reconhecimentos registrados</div>
        </div>
        <div class="kpi">
          <div class="kpi-label">${svg14('<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><line x1="12" y1="9" x2="12" y2="13.5"/>')} Dispositivos offline</div>
          <div class="kpi-value">${inactive}</div>
          <div class="kpi-sub">sem heartbeat > ${thr}h</div>
        </div>
      </div>

      <div class="dash-cols">
        <div class="card flush">
          <div class="card-head"><div class="h">Presenças recentes</div><button class="btn-link" data-route="events" id="dash-go-events">Ver todas →</button></div>
          <div>${recent}</div>
        </div>
        <div class="rail">
          <div class="card flush">
            <div class="card-head"><div class="h">Dispositivos</div><button class="btn-link" data-route="devices" id="dash-go-devices">Todos →</button></div>
            ${rail}
          </div>
          <div class="card pad">
            <div class="h" style="font-size:14px; font-weight:600;">Sincronização GOB</div>
            <div style="font-size:12px; color:var(--text-2); margin-top:6px; line-height:1.5;">Puxa membros do GOB e enfileira quem tem selfie para provisionar nos terminais.</div>
            <button class="btn btn-soft block" id="dash-sync" style="margin-top:14px;">${ICON.resync} Sincronizar agora</button>
          </div>
        </div>
      </div>
    </div>`);

  $('view').querySelectorAll('[data-route]').forEach(b => b.addEventListener('click', () => navigate(b.dataset.route)));
  $('view').querySelectorAll('[data-device-id]').forEach(r => r.addEventListener('click', () => navigate(`device-config?id=${r.dataset.deviceId}`)));
  const ds = $('dash-sync'); if (ds) ds.addEventListener('click', doSync);
}
function svg14(inner) { return `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">${inner}</svg>`; }

// ─── DEVICES ──────────────────────────────────────────────────
function mountDevices() {
  setView(`
    <div class="screen">
      <div class="toolbar">
        <div class="searchbox" style="width:300px;">${ICON.search}<input id="dev-search" placeholder="Buscar por identificador ou IP…" aria-label="Buscar dispositivos" autocomplete="off" value="${escHtml(state.devices.query)}"></div>
        <div class="seg" id="dev-filter">
          <button class="seg-btn active" data-f="all">Todos</button>
          <button class="seg-btn" data-f="online">Online</button>
          <button class="seg-btn" data-f="offline">Offline</button>
        </div>
        <div class="spacer meta mono" id="dev-count">—</div>
      </div>
      <div class="card flush">
        <div class="thead" style="grid-template-columns:1.8fr 1.4fr .9fr 1fr 1.2fr 40px;">
          <div>Dispositivo</div><div>Endereço</div><div>Status</div><div>Webhook</div><div>Heartbeat</div><div></div>
        </div>
        <div id="dev-rows">${loadingState()}</div>
      </div>
      <div class="meta" style="display:flex; align-items:center; gap:7px;">${ICON.info} Clique em um dispositivo para abrir a configuração completa.</div>
    </div>`);

  const search = $('dev-search');
  let timer;
  search.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(() => { state.devices.query = search.value.trim(); renderDeviceRows(); }, DEBOUNCE_MS); });
  $('dev-filter').querySelectorAll('.seg-btn').forEach(b => b.addEventListener('click', () => {
    state.devices.filter = b.dataset.f;
    $('dev-filter').querySelectorAll('.seg-btn').forEach(x => x.classList.toggle('active', x === b));
    renderDeviceRows();
  }));

  loadDevices();
}

async function loadDevices() {
  try {
    const res = await apiGet('devices');
    if (res.status === 401) return;
    if (!res.ok) { $('dev-rows').innerHTML = emptyState(ICON.device, 'Erro ao carregar', `Status ${res.status}.`); return; }
    const data = await res.json();
    state.devices.items = data.devices || [];
    state.devices.byId = {};
    state.devices.items.forEach(d => { state.devices.byId[d.id] = d; });
    state.devices.loaded = true;
    renderDeviceRows();
  } catch (err) {
    if ($('dev-rows')) $('dev-rows').innerHTML = emptyState(ICON.device, 'Falha de conexão', 'Tente novamente.');
    netError(err);
  }
}

function filteredDevices() {
  const q = state.devices.query.toLowerCase();
  return state.devices.items.filter(d => {
    if (state.devices.filter === 'online' && d.status !== 'active') return false;
    if (state.devices.filter === 'offline' && d.status === 'active') return false;
    if (!q) return true;
    return (d.device_identifier || '').toLowerCase().includes(q) || (d.ip_address || '').toLowerCase().includes(q);
  });
}

function renderDeviceRows() {
  const rowsEl = $('dev-rows'); if (!rowsEl) return;
  const online = state.devices.items.filter(d => d.status === 'active').length;
  const cnt = $('dev-count'); if (cnt) cnt.textContent = `${online}/${state.devices.items.length} online`;
  const list = filteredDevices();
  if (!list.length) {
    rowsEl.innerHTML = state.devices.items.length
      ? emptyState(ICON.device, 'Nenhum resultado', 'Ajuste a busca ou os filtros.')
      : emptyState(ICON.device, 'Nenhum dispositivo cadastrado', 'Os terminais aparecem aqui após o primeiro registro.');
    return;
  }
  rowsEl.innerHTML = list.map(d => `
    <div class="trow clickable" style="grid-template-columns:1.8fr 1.4fr .9fr 1fr 1.2fr 40px;" data-device-id="${escHtml(String(d.id))}" tabindex="0" role="button" aria-label="Configurar ${escHtml(d.device_identifier)}">
      <div style="display:flex; align-items:center; gap:11px; min-width:0;">
        <span class="dot ${d.status === 'active' ? 'dot-on' : 'dot-off'}"></span>
        <div class="cell-ellipsis mono" style="font-size:12.5px; font-weight:500;">${escHtml(d.device_identifier)}</div>
      </div>
      <div class="mono" style="font-size:11.5px; color:var(--text-2); min-width:0;">
        <div>${escHtml(d.ip_address || '—')}</div><div style="color:var(--text-3); font-size:10px;">${escHtml(d.mac_address || '—')}</div>
      </div>
      <div>${deviceStatusBadge(d.status)}</div>
      <div>${webhookBadge(d.webhook_configured)}</div>
      <div class="mono" style="font-size:11.5px; color:var(--text-2);">${escHtml(fmtShort(d.last_heartbeat_at))}</div>
      <div class="chevron">${ICON.chevron}</div>
    </div>`).join('');

  rowsEl.querySelectorAll('[data-device-id]').forEach(row => {
    const go = () => navigate(`device-config?id=${row.dataset.deviceId}`);
    row.addEventListener('click', go);
    row.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); } });
  });
}

// ─── DEVICE CONFIG (nova tela — design-brief §7) ──────────────
async function mountDeviceConfig(params) {
  const id = params.id;
  const section = params.section || 'overview';
  setView(loadingState());

  // garante a lista para o seletor de dispositivos
  if (!state.devices.loaded) { try { const r = await apiGet('devices'); if (r.status === 401) return; if (r.ok) { const d = await r.json(); state.devices.items = d.devices || []; state.devices.items.forEach(x => state.devices.byId[x.id] = x); state.devices.loaded = true; } } catch {} }

  let dev = state.devices.byId[id];
  if (!dev && id) {
    try { const r = await apiGet(`devices/${id}`); if (r.status === 401) return; if (r.ok) { dev = await r.json(); state.devices.byId[id] = dev; } } catch {}
  }
  if (!dev) { setView(emptyState(ICON.device, 'Dispositivo não encontrado', 'Abra a configuração a partir da lista de Dispositivos.')); return; }

  renderDeviceConfig(dev, section);
}

function renderDeviceConfig(dev, section) {
  const opts = state.devices.items.map(d => `<option value="${escHtml(String(d.id))}" ${String(d.id) === String(dev.id) ? 'selected' : ''}>${escHtml(d.device_identifier)} · ${escHtml(d.ip_address || '—')}</option>`).join('');
  const nav = CFG_SECTIONS.map(s => `<button class="cfg-nav-item ${s.id === section ? 'active' : ''}" data-section="${s.id}">${s.icon}<span>${s.label}</span></button>`).join('');

  setView(`
    <div class="screen">
      <button class="btn-back" id="cfg-back">${ICON.back} Voltar para Dispositivos</button>
      <div class="card"><div class="cfg-id">
        <span class="dot ${dev.status === 'active' ? 'dot-on' : 'dot-off'}"></span>
        <div style="min-width:0;">
          <div style="display:flex; align-items:center; gap:9px; flex-wrap:wrap;"><span class="nm mono">${escHtml(dev.device_identifier)}</span>${deviceStatusBadge(dev.status)}</div>
          <div class="mono" style="font-size:11.5px; color:var(--text-3); margin-top:2px;">${escHtml(dev.ip_address || '—')} · ${escHtml(dev.mac_address || '—')}</div>
        </div>
        <div style="margin-left:auto; display:flex; align-items:center; gap:8px;">
          <span class="meta">Trocar:</span>
          <select class="select" id="cfg-switch" style="width:auto;">${opts}</select>
        </div>
      </div></div>
      <div class="cfg-split">
        <nav class="cfg-nav" id="cfg-nav">${nav}</nav>
        <div id="cfg-content"></div>
      </div>
    </div>`);

  $('cfg-back').addEventListener('click', () => navigate('devices'));
  $('cfg-switch').addEventListener('change', e => navigate(`device-config?id=${e.target.value}&section=${section}`));
  $('cfg-nav').querySelectorAll('[data-section]').forEach(b => b.addEventListener('click', () => navigate(`device-config?id=${dev.id}&section=${b.dataset.section}`)));
  renderCfgSection(dev, section);
}

function cfgTargetOf(dev) { return { name: dev.device_identifier, ip: dev.ip_address, mac: dev.mac_address, status: dev.status }; }

function renderCfgSection(dev, section) {
  const el = $('cfg-content');
  const previewNote = pendingNote('Pré-visualização — os controles desta seção são habilitados quando o backend de configuração do terminal (ISAPI, §7) estiver disponível.');
  const renderers = {
    overview:     () => cfgOverview(dev),
    system:       () => cfgSystem(dev),
    auth:         () => cfgCredentials(dev),
    doors:        () => cfgDoors(dev),
    users:        () => cfgUsers(dev),
    cards:        () => previewNote + cfgCards(),
    faces:        () => cfgFaces(dev),
    events:       () => cfgWebhooks(dev),
    preferences:  () => cfgPreferences(dev),
    media:        () => cfgMediaFull(dev),
  };
  el.innerHTML = `<div class="stack">${(renderers[section] || renderers.overview)()}</div>`;
  wireCfgActions(dev);
  // Wire async sections after DOM is ready
  if (section === 'auth')        wireCfgCredentials(dev);
  if (section === 'system')      wireCfgSystem(dev);
  if (section === 'doors')       wireCfgDoors(dev);
  if (section === 'users')       wireCfgUsers(dev);
  if (section === 'faces')       wireCfgFaces(dev);
  if (section === 'events')      wireCfgWebhooks(dev);
  if (section === 'preferences') wireCfgPreferences(dev);
  if (section === 'media')       wireCfgMediaFull(dev);
}

function cfgOverview(dev) {
  // serial/model/firmware vêm do deviceInfo (ISAPI) persistido no banco; IP/MAC/status/
  // heartbeat do registro do device. max_users/max_faces/isapi_credentials_set vêm do
  // GET /admin/api/devices/{id} estendido (FR-002/003).
  const kv = (k, v, mono) => `<div class="kv"><div class="k">${k}</div><div class="v ${mono?'mono':''}">${v}</div></div>`;

  // isapi_credentials_set: bool — null → não configurado (device antigo sem campo)
  let credBadge;
  if (dev.isapi_credentials_set === true) {
    credBadge = badge('ok', 'Configuradas');
  } else if (dev.isapi_credentials_set === false) {
    credBadge = badge('warn', 'Não configuradas');
  } else {
    credBadge = badge('muted', '—');
  }

  // max_users/max_faces: null → '—' (nunca exibir zero nem estimativa — FR-002)
  const maxUsers = dev.max_users != null ? String(dev.max_users) : '—';
  const maxFaces = dev.max_faces != null ? String(dev.max_faces) : '—';

  // Aviso quando credenciais ISAPI não configuradas (US1-AS3)
  const credWarn = dev.isapi_credentials_set === false ? `
    <div class="alert alert-warn" style="margin-bottom:0;">
      <span style="color:var(--warn); flex:none;">${ICON.warnTri}</span>
      <div class="grow"><strong style="color:var(--warn);">Credenciais ISAPI não configuradas</strong> — as seções Sistema, Portas, Usuários e Faces dependem da comunicação ISAPI e ficarão desabilitadas até que as credenciais sejam preenchidas. <button class="btn-link" data-section-goto="auth">Configurar agora →</button></div>
    </div>` : '';

  return `
    ${credWarn}
    <div class="card flush">
      <div class="card-head"><div class="h">Identificação & status</div></div>
      <div class="kv-grid">
        ${kv('Identificador', escHtml(dev.device_identifier), true)}
        ${kv('Modelo', escHtml(dev.model || '—'))}
        ${kv('Nº de série', escHtml(dev.serial_number || '—'), true)}
        ${kv('IP', escHtml(dev.ip_address || '—'), true)}
        ${kv('MAC', escHtml(dev.mac_address || '—'), true)}
        ${kv('Firmware', escHtml(dev.firmware_version || '—'), true)}
        ${kv('Status', dev.status === 'active' ? 'Online' : 'Offline')}
        ${kv('Último heartbeat', escHtml(fmtDateTime(dev.last_heartbeat_at)), true)}
        ${kv('Webhook', dev.webhook_configured ? 'Configurado' : 'Ausente')}
        ${kv('Credenciais ISAPI', credBadge)}
      </div>
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Capacidades</div></div>
      <div class="kv-grid">
        ${kv('Máx. usuários', maxUsers, true)}
        ${kv('Máx. faces', maxFaces, true)}
      </div>
    </div>`;
}

function cfgCredentials(dev) {
  // Formulário real de credenciais ISAPI (US2).
  // Senha usa type="password" e nunca é ecoada de volta (FR-005).
  // Submit faz PUT /admin/api/devices/{id}/credentials.
  const credSet = dev.isapi_credentials_set === true;
  const statusBadge = credSet ? badge('ok', 'Configuradas') : badge('warn', 'Não configuradas');
  return `
    <div class="card flush">
      <div class="card-head">
        <div class="h">Credenciais ISAPI</div>
        <span id="cred-status-badge">${statusBadge}</span>
      </div>
      <div style="padding:16px;">
        <div style="font-size:12px; color:var(--text-2); margin-bottom:16px; line-height:1.5;">
          Informe as credenciais da interface HTTP/ISAPI do terminal. A senha é cifrada antes de armazenar e nunca exibida novamente.
          ${credSet ? `<br><span style="color:var(--green);">● Credenciais já cadastradas</span> — preencha para atualizar.` : ''}
        </div>
        <form id="cred-form" novalidate>
          <div class="grid-2" style="margin-bottom:14px;">
            <div class="field">
              <label class="label" for="cred-user">Usuário ISAPI</label>
              <input class="input mono" id="cred-user" name="isapi_username" autocomplete="username" placeholder="admin" required />
            </div>
            <div class="field">
              <label class="label" for="cred-port">Porta HTTP</label>
              <input class="input mono" id="cred-port" name="isapi_port" type="number" min="1" max="65535" placeholder="80" required />
            </div>
          </div>
          <div class="field" style="margin-bottom:18px;">
            <label class="label" for="cred-pass">Senha ISAPI</label>
            <input class="input mono" id="cred-pass" name="isapi_password" type="password" autocomplete="new-password" placeholder="••••••••" required />
          </div>
          <div style="display:flex; align-items:center; gap:10px;">
            <button type="submit" class="btn btn-accent" id="cred-submit">Salvar credenciais</button>
            <span id="cred-feedback" role="alert" aria-live="assertive" style="font-size:12px;"></span>
          </div>
        </form>
      </div>
    </div>`;
}

function wireCfgCredentials(dev) {
  const form = $('cred-form');
  if (!form) return;
  form.addEventListener('submit', async e => {
    e.preventDefault();
    const username = $('cred-user').value.trim();
    const password = $('cred-pass').value;
    const portRaw  = $('cred-port').value.trim();
    const feedback = $('cred-feedback');
    const btn      = $('cred-submit');

    // Validação frontend (5.2.6)
    const port = parseInt(portRaw, 10);
    if (!username) { feedback.textContent = 'Usuário é obrigatório.'; feedback.style.color = 'var(--red)'; return; }
    if (!password) { feedback.textContent = 'Senha é obrigatória.'; feedback.style.color = 'var(--red)'; return; }
    if (!portRaw || isNaN(port) || port < 1 || port > 65535) {
      feedback.textContent = 'Porta inválida (1–65535).'; feedback.style.color = 'var(--red)'; return;
    }

    btn.disabled = true; btn.textContent = 'Salvando…'; feedback.textContent = '';
    try {
      const res = await apiPut(`devices/${dev.id}/credentials`, {
        isapi_username: username,
        isapi_password: password,
        isapi_port: port,
      });
      if (res.status === 401) return;
      if (res.ok) {
        const data = await res.json();
        // Nunca exibir a senha de volta (FR-005)
        feedback.textContent = `Credenciais salvas. Porta: ${data.isapi_port ?? port}.`;
        feedback.style.color = 'var(--green)';
        $('cred-pass').value = '';
        // Atualizar badge de status
        const badgeEl = $('cred-status-badge');
        if (badgeEl && data.isapi_credentials_set) badgeEl.innerHTML = badge('ok', 'Configuradas');
        // Atualizar estado local para que overview reflita
        dev.isapi_credentials_set = data.isapi_credentials_set ?? true;
        dev.isapi_port = data.isapi_port ?? port;
        showToast('success', 'Credenciais ISAPI salvas com sucesso.');
      } else {
        let msg = `Erro ao salvar (status ${res.status}).`;
        if (res.status === 503) msg = 'Chave de cifra ausente no servidor (ISAPI_CRED_KEY). Contate o administrador.';
        else if (res.status === 400) { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
        else if (res.status === 404) msg = 'Dispositivo não encontrado.';
        feedback.textContent = msg; feedback.style.color = 'var(--red)';
      }
    } catch (err) {
      feedback.textContent = err.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Falha de conexão.';
      feedback.style.color = 'var(--red)';
      netError(err);
    } finally {
      btn.disabled = false; btn.textContent = 'Salvar credenciais';
    }
  });
}

// ─── 5.3 SISTEMA (time + reboot + factory-reset) ─────────────
function cfgSystem(dev) {
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Data & hora</div><span class="badge badge-warn">Crítico</span></div>
      <div style="padding:16px; display:flex; flex-direction:column; gap:14px;">
        <div class="kv-grid" style="grid-template-columns:1fr 1fr;">
          <div class="kv"><div class="k">Hora atual no device</div><div class="v mono" id="sys-time-val">Carregando…</div></div>
          <div class="kv"><div class="k">Modo</div><div class="v mono" id="sys-time-mode">—</div></div>
        </div>
        <div id="sys-time-err" role="alert" style="display:none; color:var(--red); font-size:12px;"></div>
        <form id="sys-time-form" novalidate>
          <div class="grid-2" style="margin-bottom:12px;">
            <div class="field">
              <label class="label" for="sys-time-mode-sel">Modo</label>
              <select class="select" id="sys-time-mode-sel">
                <option value="manual">Manual</option>
                <option value="ntp">NTP</option>
              </select>
            </div>
            <div class="field">
              <label class="label" for="sys-tz">Fuso horário</label>
              <input class="input mono" id="sys-tz" placeholder="CST+3:00:00 (formato HikVision)" />
            </div>
          </div>
          <div id="sys-ntp-block" style="display:none; margin-bottom:12px;">
            <div class="field">
              <label class="label" for="sys-ntp">Servidor NTP</label>
              <input class="input mono" id="sys-ntp" placeholder="pool.ntp.br" />
            </div>
          </div>
          <div id="sys-manual-block" style="margin-bottom:12px;">
            <div class="field">
              <label class="label" for="sys-local-time">Data/hora local (YYYY-MM-DDThh:mm:ss)</label>
              <input class="input mono" id="sys-local-time" placeholder="2026-06-21T14:00:00" />
            </div>
          </div>
          <div style="display:flex; align-items:center; gap:10px;">
            <button type="submit" class="btn btn-accent" id="sys-time-submit">Aplicar</button>
            <span id="sys-time-feedback" role="alert" aria-live="assertive" style="font-size:12px;"></span>
          </div>
        </form>
      </div>
    </div>
    <div class="danger-card">
      <div class="dh">${ICON.warnTri} Zona de perigo</div>
      <div style="padding:16px; display:flex; flex-direction:column; gap:12px;">
        <div class="row-between">
          <div><div style="font-size:12.5px; font-weight:500;">Reiniciar dispositivo</div><div style="font-size:11px; color:var(--text-3);">Indisponível por ~40s durante o reboot.</div></div>
          <button class="btn btn-warn-outline sm" id="sys-reboot-btn">Reiniciar</button>
        </div>
        <div style="height:1px; background:var(--border);"></div>
        <div class="row-between">
          <div><div style="font-size:12.5px; font-weight:500;">Reset de fábrica</div><div style="font-size:11px; color:var(--text-3);">Irreversível — apaga usuários, faces, cartões e configurações.</div></div>
          <button class="btn btn-danger sm" id="sys-factory-btn">Reset de fábrica</button>
        </div>
      </div>
    </div>`;
}

function wireCfgSystem(dev) {
  // Carregar hora atual do device
  apiGet(`devices/${dev.id}/time`).then(async res => {
    const el = $('sys-time-val'), em = $('sys-time-mode'), errEl = $('sys-time-err');
    if (!el) return;
    if (res.status === 401) return;
    if (res.ok) {
      const d = await res.json();
      el.textContent = d.local_time || '—';
      em.textContent = (d.time_mode || '—').toUpperCase();
      // Preencher form com valores actuais
      const modeEl = $('sys-time-mode-sel');
      const tzEl = $('sys-tz');
      const ltEl = $('sys-local-time');
      const ntpEl = $('sys-ntp');
      if (modeEl) { modeEl.value = (d.time_mode || 'manual').toLowerCase(); modeEl.dispatchEvent(new Event('change')); }
      if (tzEl) tzEl.value = d.time_zone || '';
      if (ltEl) ltEl.value = d.local_time || '';
      if (ntpEl) ntpEl.value = d.ntp_server || '';
    } else {
      if (errEl) { errEl.textContent = `Não foi possível carregar hora do device (status ${res.status}).`; errEl.style.display = ''; }
      if (el) el.textContent = '—';
    }
  }).catch(err => {
    const el = $('sys-time-val'), errEl = $('sys-time-err');
    if (el) el.textContent = '—';
    if (errEl) { errEl.textContent = err.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Falha de conexão ao carregar hora.'; errEl.style.display = ''; }
  });

  // Toggle NTP/manual blocks
  const modeEl = $('sys-time-mode-sel');
  if (modeEl) {
    modeEl.addEventListener('change', () => {
      const isNtp = modeEl.value === 'ntp';
      const nb = $('sys-ntp-block'), mb = $('sys-manual-block');
      if (nb) nb.style.display = isNtp ? '' : 'none';
      if (mb) mb.style.display = isNtp ? 'none' : '';
    });
  }

  // Submit time form
  const form = $('sys-time-form');
  if (form) {
    form.addEventListener('submit', async e => {
      e.preventDefault();
      const btn = $('sys-time-submit'), fb = $('sys-time-feedback');
      const mode = $('sys-time-mode-sel').value;
      const tz = ($('sys-tz').value || '').trim();
      const body = { time_mode: mode, time_zone: tz };
      if (mode === 'ntp') body.ntp_server = ($('sys-ntp').value || '').trim();
      else body.local_time = ($('sys-local-time').value || '').trim();
      btn.disabled = true; btn.textContent = 'Aplicando…'; fb.textContent = '';
      try {
        const res = await apiPut(`devices/${dev.id}/time`, body);
        if (res.status === 401) return;
        if (res.ok) { fb.textContent = 'Configuração de hora aplicada.'; fb.style.color = 'var(--green)'; showToast('success', 'Hora do dispositivo atualizada.'); }
        else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} fb.textContent = msg; fb.style.color = 'var(--red)'; }
      } catch (err) { fb.textContent = err.message === 'timeout' ? 'Tempo esgotado.' : 'Falha de conexão.'; fb.style.color = 'var(--red)'; }
      finally { btn.disabled = false; btn.textContent = 'Aplicar'; }
    });
  }

  // Reboot
  const rebootBtn = $('sys-reboot-btn');
  if (rebootBtn) {
    rebootBtn.addEventListener('click', () => {
      openConfirm({
        title: 'Reiniciar dispositivo', confirmLabel: 'Reiniciar agora', tone: 'warn', strong: false,
        body: 'O terminal ficará indisponível por cerca de 40 segundos.',
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiPost(`devices/${dev.id}/actions/reboot`);
            if (res.status === 401) return;
            if (res.ok) showToast('success', 'Reboot iniciado — o terminal ficará offline por ~40s.');
            else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
          } catch (err) { netError(err); }
        },
      });
    });
  }

  // Factory reset
  const factoryBtn = $('sys-factory-btn');
  if (factoryBtn) {
    factoryBtn.addEventListener('click', () => {
      openConfirm({
        title: 'Reset de fábrica', confirmLabel: 'Apagar tudo e resetar', tone: 'danger', strong: true,
        body: 'IRREVERSÍVEL — apaga todos os usuários, faces, cartões e configurações. Os membros precisarão ser reprovisionados.',
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiPost(`devices/${dev.id}/actions/factory-reset`);
            if (res.status === 401) return;
            if (res.ok) {
              const d = await res.json();
              // Atualizar estado local webhook_configured (US3-AS3)
              dev.webhook_configured = d.webhook_configured ?? false;
              state.devices.byId[dev.id] = dev;
              showToast('success', 'Reset de fábrica iniciado. Webhook removido do registro.');
            } else {
              let msg = `Erro (status ${res.status}).`; try { const d2 = await res.json(); if (d2.error) msg = d2.error; } catch {}
              showToast('error', msg);
            }
          } catch (err) { netError(err); }
        },
      });
    });
  }
}

// ─── 5.4 PORTAS ───────────────────────────────────────────────
function cfgDoors(dev) {
  return `
    <div id="doors-list">${loadingState()}</div>
    <div class="card flush" style="margin-top:0;">
      <div class="card-head"><div class="h">Configuração de porta</div><span class="badge badge-muted">somente leitura neste build</span></div>
      <div style="padding:16px;" class="grid-2">
        <div class="field"><label class="label">Delay de destravamento (s)</label><input class="input" placeholder="5" disabled></div>
        <div class="field"><label class="label">Modo de alarme</label><select class="select" disabled><option>Porta aberta demais</option><option>Arrombamento</option><option>Desativado</option></select></div>
      </div>
    </div>`;
}

function wireCfgDoors(dev) {
  const listEl = $('doors-list');
  if (!listEl) return;
  apiGet(`devices/${dev.id}/doors`).then(async res => {
    if (!listEl) return;
    if (res.status === 401) return;
    if (!res.ok) {
      const msg = res.status === 504 ? 'Dispositivo offline — não foi possível carregar portas.' : `Erro ao carregar portas (status ${res.status}).`;
      listEl.innerHTML = emptyState(ICON.device, 'Erro ao carregar portas', msg);
      return;
    }
    const data = await res.json();
    const doors = data.doors || [];
    if (!doors.length) { listEl.innerHTML = emptyState(ICON.device, 'Nenhuma porta encontrada', 'O dispositivo não reportou portas configuradas.'); return; }
    listEl.innerHTML = doors.map(door => `
      <div class="card pad" style="margin-bottom:10px;">
        <div class="row-between">
          <div style="display:flex; align-items:center; gap:11px;">
            <div style="width:38px; height:38px; border-radius:10px; background:var(--green-soft); display:grid; place-items:center;"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--green)" stroke-width="2"><rect x="5" y="3" width="14" height="18" rx="1.5"/><circle cx="15" cy="12" r="1"/></svg></div>
            <div>
              <div style="font-size:13.5px; font-weight:600;">${escHtml(door.door_name || `Porta ${door.door_id}`)}</div>
              <div style="font-size:11.5px; color:var(--text-3);">ID ${escHtml(String(door.door_id))} · ${escHtml(String(door.reader_count || 1))} leitor(es) · <span id="door-state-${escHtml(String(door.door_id))}">carregando estado…</span></div>
            </div>
          </div>
          <div style="display:flex; gap:8px;">
            <button class="btn btn-warn-outline sm" data-door-open="${escHtml(String(door.door_id))}">Destravar 5s</button>
          </div>
        </div>
      </div>`).join('');

    // Carregar status de cada porta e wire botões de controle
    doors.forEach(door => {
      const did = door.door_id;
      apiGet(`devices/${dev.id}/doors/${did}/status`).then(async sr => {
        const stEl = $(`door-state-${did}`);
        if (!stEl) return;
        if (sr.ok) {
          const sd = await sr.json();
          // Exibir valores observados (CHK055 — não presumir enum)
          stEl.textContent = `porta: ${sd.door_state || '—'} · trava: ${sd.lock_state || '—'}`;
        } else {
          stEl.textContent = sr.status === 504 ? 'offline' : `status ${sr.status}`;
        }
      }).catch(() => { const stEl = $(`door-state-${did}`); if (stEl) stEl.textContent = 'erro de conexão'; });

      const openBtn = listEl.querySelector(`[data-door-open="${did}"]`);
      if (openBtn) {
        openBtn.addEventListener('click', () => {
          openConfirm({
            title: `Destravar porta ${did} remotamente`, confirmLabel: 'Destravar', tone: 'warn', strong: false,
            body: `A porta ${escHtml(door.door_name || `${did}`)} será destravada por 5 segundos. A ação fica registrada no log de auditoria.`,
            target: cfgTargetOf(dev),
            onConfirm: async () => {
              try {
                const res = await apiPost(`devices/${dev.id}/doors/${did}/control`, { command: 'open' });
                if (res.status === 401) return;
                if (res.ok) showToast('success', `Porta ${did} destravada com sucesso.`);
                else if (res.status === 504) showToast('error', 'Dispositivo offline — não foi possível destravar a porta.');
                else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
              } catch (err) { netError(err); }
            },
          });
        });
      }
    });
  }).catch(err => {
    const el = $('doors-list');
    if (el) el.innerHTML = emptyState(ICON.device, 'Falha de conexão', 'Não foi possível carregar a lista de portas.');
    netError(err);
  });
}

// ─── 5.5 USUÁRIOS & FACES ─────────────────────────────────────
function cfgUsers(dev) {
  return `
    <div class="toolbar">
      <div class="meta mono" id="users-count">Carregando…</div>
      <div class="spacer"></div>
      <button class="btn btn-ghost sm" id="users-prev" disabled>Anterior</button>
      <span class="meta mono" id="users-page">—</span>
      <button class="btn btn-ghost sm" id="users-next" disabled>Próxima</button>
    </div>
    <div class="card flush">
      <div class="thead" style="grid-template-columns:1fr 1fr 80px 60px;">
        <div>Usuário</div><div>Matrícula</div><div>Faces</div><div>Válido</div>
      </div>
      <div id="users-rows">${loadingState()}</div>
    </div>
    <div class="card" style="border-color:var(--red); padding:14px 16px; margin-top:0;">
      <div class="row-between">
        <div><div style="font-size:12.5px; font-weight:500; color:var(--red);">Limpar todos os usuários</div><div style="font-size:11px; color:var(--text-3);">Remove todos os usuários e faces deste terminal. Requer reprovisionamento.</div></div>
        <button class="btn btn-danger sm" id="users-clear-btn">Limpar todos</button>
      </div>
    </div>`;
}

function wireCfgUsers(dev) {
  let page = 1;
  const perPage = 50;

  async function loadUsersPage(p) {
    const rowsEl = $('users-rows'), cntEl = $('users-count'), pgEl = $('users-page');
    const prevBtn = $('users-prev'), nextBtn = $('users-next');
    if (!rowsEl) return;
    rowsEl.innerHTML = loadingState();
    if (cntEl) cntEl.textContent = 'Carregando…';
    if (prevBtn) prevBtn.disabled = true;
    if (nextBtn) nextBtn.disabled = true;
    try {
      const res = await apiGet(`devices/${dev.id}/users?page=${p}&per_page=${perPage}`);
      if (res.status === 401) return;
      if (!res.ok) {
        const msg = res.status === 504 ? 'Dispositivo offline.' : `Erro (status ${res.status}).`;
        rowsEl.innerHTML = emptyState(ICON.members, 'Erro ao carregar', msg);
        return;
      }
      const data = await res.json();
      const users = data.users || [];
      const total = data.total ?? 0;
      page = data.page ?? p;
      const totalPages = Math.ceil(total / (data.per_page || perPage));
      if (cntEl) cntEl.textContent = `${total} usuário(s) no terminal`;
      if (pgEl) pgEl.textContent = `${page} / ${Math.max(1, totalPages)}`;
      if (prevBtn) prevBtn.disabled = page <= 1;
      if (nextBtn) nextBtn.disabled = page >= totalPages;
      if (!users.length) {
        rowsEl.innerHTML = emptyState(ICON.members, 'Nenhum usuário cadastrado', 'O terminal não possui usuários no momento.');
        return;
      }
      // camelCase do ISAPI preservado no payload (contracts/admin-api.md §Users)
      rowsEl.innerHTML = users.map(u => `
        <div class="trow" style="grid-template-columns:1fr 1fr 80px 60px;">
          <div class="cell-ellipsis" style="font-size:12.5px; font-weight:500;">${escHtml(u.name || '—')}</div>
          <div class="mono" style="font-size:11.5px; color:var(--text-2);">${escHtml(u.employeeNo || '—')}</div>
          <div class="mono" style="font-size:12px;">${escHtml(String(u.numOfFace ?? 0))}</div>
          <div>${u.valid ? badge('ok','Sim') : badge('muted','Não')}</div>
        </div>`).join('');
    } catch (err) {
      if ($('users-rows')) $('users-rows').innerHTML = emptyState(ICON.members, 'Falha de conexão', 'Tente novamente.');
      netError(err);
    }
  }

  loadUsersPage(1);

  const prevBtn = $('users-prev'), nextBtn = $('users-next');
  if (prevBtn) prevBtn.addEventListener('click', () => { if (page > 1) loadUsersPage(page - 1); });
  if (nextBtn) nextBtn.addEventListener('click', () => loadUsersPage(page + 1));

  // Limpar usuários (US5-AS2 — confirmação forte)
  const clearBtn = $('users-clear-btn');
  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      openConfirm({
        title: 'Limpar todos os usuários', confirmLabel: 'Limpar usuários', tone: 'danger', strong: true,
        body: 'Remove TODOS os usuários e faces deste terminal. Os membros precisarão ser reprovisionados.',
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiDelete(`devices/${dev.id}/users`);
            if (res.status === 401) return;
            if (res.ok) { showToast('success', 'Usuários removidos do terminal.'); loadUsersPage(1); }
            else {
              let msg = `Erro (status ${res.status}).`;
              if (res.status === 504) msg = 'Dispositivo offline.';
              else { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
              showToast('error', msg);
            }
          } catch (err) { netError(err); }
        },
      });
    });
  }
}

function cfgCards() {
  return `
    <div class="row-between"><div class="meta">Cartões RFID/NFC cadastrados no terminal</div><button class="btn btn-accent sm" data-pending="Novo cartão">${ICON.plus} Novo cartão</button></div>
    <div class="card flush">${emptyState(ICON.device, 'Cartões do terminal', 'O CRUD de cartões (§7.6) será habilitado com o backend correspondente.')}</div>
    <div class="card" style="border-color:var(--red); padding:14px 16px;">
      <div class="row-between"><div><div style="font-size:12.5px; font-weight:500; color:var(--red);">Limpar todos os cartões</div><div style="font-size:11px; color:var(--text-3);">Remove todos os cartões RFID/NFC deste terminal.</div></div>
      <button class="btn btn-danger sm" data-modal="clearCards">Limpar todos</button></div>
    </div>`;
}

function cfgFaces(dev) {
  // DELETE faces é stub (501) — botão desabilitado com tooltip (US5-AS3, 5.5.5)
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Biblioteca de faces</div><div id="faces-count" class="meta mono">—</div></div>
      <div id="faces-list">${loadingState()}</div>
    </div>
    <div class="card" style="border-color:var(--red); padding:14px 16px;">
      <div class="row-between">
        <div>
          <div style="font-size:12.5px; font-weight:500; color:var(--red);">Limpar biblioteca de faces</div>
          <div style="font-size:11px; color:var(--text-3);">Remove todas as faces — usuários permanecem sem reconhecimento até reenvio.</div>
        </div>
        <button class="btn btn-danger sm" id="faces-clear-btn">Limpar faces</button>
      </div>
    </div>`;
}

function wireCfgFaces(dev) {
  // A lista de faces não tem endpoint dedicado de listagem; exibir estado informativo.
  const listEl = $('faces-list');
  if (listEl) {
    listEl.innerHTML = emptyState(ICON.members, 'Visualização não disponível', 'A listagem de faces individuais não é exposta pela ISAPI. Use a seção Usuários para verificar o campo "Faces" por usuário.');
  }
  // Botão "Limpar faces" — SOURCED: FaceService.php:38/283 (ENDPOINT_FACE_CLEAR)
  const clearBtn = $('faces-clear-btn');
  if (clearBtn) {
    clearBtn.addEventListener('click', () => {
      openConfirm({
        ...CFG_MODALS.clearFaces,
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiDelete(`devices/${dev.id}/faces`);
            if (res.status === 401) return;
            if (res.ok) { showToast('success', 'Faces removidas do terminal.'); }
            else {
              let msg = `Erro (status ${res.status}).`;
              if (res.status === 504) msg = 'Dispositivo offline.';
              else { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
              showToast('error', msg);
            }
          } catch (err) { netError(err); }
        },
      });
    });
  }
}

// ─── 5.6 WEBHOOKS ─────────────────────────────────────────────
function cfgWebhooks(dev) {
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Destinos de notificação</div><div id="webhooks-count" class="meta mono">—</div></div>
      <div id="webhooks-list">${loadingState()}</div>
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Stream ao vivo</div><span class="badge badge-muted">desconectado</span></div>
      <div style="padding:16px;">${pendingNote('O monitor de eventos em tempo real do device será habilitado com o stream ISAPI (§7.8).')}</div>
    </div>`;
}

function wireCfgWebhooks(dev) {
  const listEl = $('webhooks-list'), cntEl = $('webhooks-count');
  if (!listEl) return;
  apiGet(`devices/${dev.id}/webhooks`).then(async res => {
    if (!listEl) return;
    if (res.status === 401) return;
    if (!res.ok) {
      const msg = res.status === 504 ? 'Dispositivo offline.' : `Erro (status ${res.status}).`;
      listEl.innerHTML = emptyState(ICON.device, 'Erro ao carregar webhooks', msg);
      return;
    }
    const data = await res.json();
    const hooks = data.webhooks || [];
    if (cntEl) cntEl.textContent = `${hooks.length} webhook(s)`;
    if (!hooks.length) { listEl.innerHTML = emptyState(ICON.device, 'Nenhum webhook configurado', 'Os destinos de notificação do terminal aparecem aqui.'); return; }
    listEl.innerHTML = hooks.map(h => `
      <div class="trow" style="grid-template-columns:1fr auto auto;" id="wh-row-${escHtml(h.id)}">
        <div style="min-width:0;">
          <div class="cell-ellipsis mono" style="font-size:12px;">${escHtml(h.url || '—')}</div>
          <div style="font-size:11px; color:var(--text-3);">${escHtml(h.protocol || '—')} · ID: ${escHtml(String(h.id))}</div>
        </div>
        <div style="display:flex; align-items:center; gap:6px;">${h._is_primary ? badge('ok','Principal') : ''}</div>
        <div><button class="btn btn-danger sm" data-wh-delete="${escHtml(String(h.id))}" data-wh-url="${escHtml(h.url || '')}">${ICON.trash}</button></div>
      </div>`).join('');

    listEl.querySelectorAll('[data-wh-delete]').forEach(btn => {
      btn.addEventListener('click', () => {
        const whId = btn.dataset.whDelete;
        const whUrl = btn.dataset.whUrl;
        openConfirm({
          title: 'Remover webhook', confirmLabel: 'Remover', tone: 'danger', strong: false,
          body: `Remove o destino de notificação "${whUrl}". Se for o webhook principal do sistema, os eventos de presença deixarão de ser recebidos.`,
          target: cfgTargetOf(dev),
          onConfirm: async () => {
            try {
              const res = await apiDelete(`devices/${dev.id}/webhooks/${whId}`);
              if (res.status === 401) return;
              if (res.ok) {
                const d = await res.json();
                showToast('success', 'Webhook removido.');
                // Atualizar webhook_configured no estado local (FR-019)
                dev.webhook_configured = d.webhook_configured ?? dev.webhook_configured;
                state.devices.byId[dev.id] = dev;
                // Remover linha da UI
                const row = $(`wh-row-${whId}`);
                if (row) row.remove();
                const remaining = (listEl.querySelectorAll('.trow') || []).length;
                if (cntEl) cntEl.textContent = `${remaining} webhook(s)`;
                if (remaining === 0) listEl.innerHTML = emptyState(ICON.device, 'Nenhum webhook configurado', '');
              } else {
                let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
                showToast('error', msg);
              }
            } catch (err) { netError(err); }
          },
        });
      });
    });
  }).catch(err => {
    const el = $('webhooks-list');
    if (el) el.innerHTML = emptyState(ICON.device, 'Falha de conexão', 'Não foi possível carregar webhooks.');
    netError(err);
  });
}

// ─── PREFERÊNCIAS (tasks 4.1–4.5) ─────────────────────────────

// Modos de verificação disponíveis no HikVision (SOURCED AuthMode.php)
const VERIFY_MODES = [
  { value: 'face',                label: 'Reconhecimento facial' },
  { value: 'card',                label: 'Cartão' },
  { value: 'pin',                 label: 'PIN' },
  { value: 'face_or_card',        label: 'Face ou cartão' },
  { value: 'card_or_pin',         label: 'Cartão ou PIN' },
  { value: 'face_or_card_or_pin', label: 'Face, cartão ou PIN' },
  { value: 'face_and_pin',        label: 'Face + PIN (duplo)' },
  { value: 'card_and_pin',        label: 'Cartão + PIN (duplo)' },
  { value: 'face_and_card',       label: 'Face + cartão (duplo)' },
];

// Modos de display suportados (SOURCED IdentityTerminal.php)
const DISPLAY_MODES = [
  { value: 'normal', label: 'Normal' },
  { value: 'full',   label: 'Tela cheia' },
  { value: 'split',  label: 'Dividido' },
];

function cfgPreferences(dev) {
  return `
    <!-- 4.1 Modo de Verificação -->
    <div class="card flush" id="pref-authmode-card">
      <div class="card-head">
        <div class="h">Modo de Verificação</div>
        <span id="pref-authmode-badge" class="meta mono" style="font-size:11px;">Carregando…</span>
      </div>
      <div style="padding:16px;">
        <div style="font-size:12px; color:var(--text-2); margin-bottom:14px; line-height:1.5;">
          Define como o terminal autentica os usuários (aplicado a todos os dias da semana).
        </div>
        <div id="pref-authmode-err" style="display:none; color:var(--red); font-size:12px; margin-bottom:10px;"></div>
        <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap;">
          <select class="select" id="pref-authmode-sel" style="width:280px;" disabled>
            <option value="">Carregando…</option>
          </select>
          <button class="btn btn-accent sm" id="pref-authmode-save" disabled>Salvar modo</button>
          <span id="pref-authmode-fb" role="alert" aria-live="assertive" style="font-size:12px;"></span>
        </div>
      </div>
    </div>

    <!-- 4.2 Layout de Tela -->
    <div class="card flush" id="pref-display-card">
      <div class="card-head"><div class="h">Layout de Tela</div></div>
      <div style="padding:16px;">
        <div style="font-size:12px; color:var(--text-2); margin-bottom:14px; line-height:1.5;">
          Controla como o terminal exibe informações na tela durante o reconhecimento.
        </div>
        <div id="pref-display-err" style="display:none; color:var(--red); font-size:12px; margin-bottom:10px;"></div>
        <!-- Seletor de modo (tabs) -->
        <div style="margin-bottom:16px;">
          <div class="label">Modo de exibição</div>
          <div class="seg" id="pref-display-mode" style="display:inline-flex;">
            ${DISPLAY_MODES.map(m => `<button class="seg-btn" data-mode="${m.value}">${m.label}</button>`).join('')}
          </div>
        </div>
        <!-- Inputs numéricos -->
        <div class="grid-2" style="margin-bottom:16px;">
          <div class="field">
            <label class="label" for="pref-screen-off">Desligar tela após (s)</label>
            <input class="input mono" id="pref-screen-off" type="number" min="0" placeholder="60" />
          </div>
          <div class="field">
            <label class="label" for="pref-preview-time">Tempo de pré-visualização (s)</label>
            <input class="input mono" id="pref-preview-time" type="number" min="0" placeholder="5" />
          </div>
          <div class="field">
            <label class="label" for="pref-standby-timeout">Timeout standby (s)</label>
            <input class="input mono" id="pref-standby-timeout" type="number" min="0" placeholder="30" />
          </div>
        </div>
        <div style="display:flex; align-items:center; gap:10px;">
          <button class="btn btn-accent sm" id="pref-display-save" disabled>Salvar layout</button>
          <span id="pref-display-fb" role="alert" aria-live="assertive" style="font-size:12px;"></span>
        </div>
      </div>
    </div>

    <!-- 4.3 Standby Picture -->
    <div class="card flush" id="pref-standby-card">
      <div class="card-head">
        <div class="h">Imagem de Standby</div>
        <div style="display:flex; align-items:center; gap:8px;">
          <label class="btn btn-accent sm" style="cursor:pointer; margin:0;">
            ${ICON.upload} Enviar imagem
            <input type="file" id="pref-standby-file" accept="image/*" style="display:none;" />
          </label>
          <button class="btn btn-ghost sm" id="pref-standby-disable">Desativar customizada</button>
        </div>
      </div>
      <div id="pref-standby-list" style="padding:16px;">${loadingState()}</div>
    </div>

    <!-- 4.3 Boot Logo -->
    <div class="card flush" id="pref-bootpic-card">
      <div class="card-head">
        <div class="h">Boot Logo</div>
        <div style="display:flex; align-items:center; gap:8px;">
          <label class="btn btn-accent sm" style="cursor:pointer; margin:0;">
            ${ICON.upload} Enviar logo
            <input type="file" id="pref-bootpic-file" accept="image/jpeg,image/jpg" style="display:none;" />
          </label>
          <button class="btn btn-ghost sm" id="pref-bootpic-delete">Remover logo</button>
        </div>
      </div>
      <div id="pref-bootpic-status" style="padding:16px;">
        <div style="font-size:12.5px; color:var(--text-2); line-height:1.5;">
          Imagem exibida na inicialização do terminal. Somente JPEG. Tamanho máximo aceito pelo firmware varia por modelo.
        </div>
      </div>
    </div>

    <!-- 4.4 Estatísticas de Capacidade -->
    <div class="card flush" id="pref-stats-card">
      <div class="card-head">
        <div class="h">Estatísticas de Capacidade</div>
        <div style="display:flex; align-items:center; gap:8px;">
          <span class="meta mono" id="pref-stats-updated" style="font-size:11px;"></span>
          <button class="btn btn-ghost sm" id="pref-stats-refresh">Atualizar</button>
        </div>
      </div>
      <div id="pref-stats-body" style="padding:16px;">${loadingState()}</div>
    </div>

    <!-- 4.5 Configuração Avançada de Face -->
    <div class="card flush" id="pref-faceadv-card">
      <div class="card-head"><div class="h">Configuração Avançada de Face</div></div>
      <div style="padding:16px;">
        <div style="font-size:12px; color:var(--text-2); margin-bottom:14px; line-height:1.5;">
          Distância máxima para reconhecimento facial. Valores maiores aumentam a área de detecção.
        </div>
        <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap; margin-bottom:20px;">
          <div class="field" style="width:220px;">
            <label class="label" for="pref-maxdistance">Distância máxima (m)</label>
            <input class="input mono" id="pref-maxdistance" type="number" step="0.1" min="0.1" max="5" placeholder="1.5" />
          </div>
          <button class="btn btn-accent sm" id="pref-faceadv-save" style="margin-top:18px;">Salvar configuração</button>
          <span id="pref-faceadv-fb" role="alert" aria-live="assertive" style="font-size:12px; margin-top:18px;"></span>
        </div>
        <div style="border-top:1px solid var(--border); padding-top:16px;">
          <div style="font-size:13px; font-weight:600; margin-bottom:8px;">Captura ao Vivo</div>
          <div style="font-size:12px; color:var(--text-2); margin-bottom:12px; line-height:1.5;">
            Captura uma imagem do que o terminal está vendo agora (câmera frontal).
          </div>
          <button class="btn btn-soft sm" id="pref-facecapture-btn">Capturar imagem</button>
          <div id="pref-facecapture-result" style="margin-top:14px; display:none;">
            <img id="pref-facecapture-img" src="" alt="Captura ao vivo do terminal" style="max-width:320px; border-radius:10px; border:1px solid var(--border);" />
          </div>
          <div id="pref-facecapture-err" style="display:none; margin-top:10px; font-size:12px; color:var(--red);"></div>
        </div>
      </div>
    </div>
  `;
}

function wireCfgPreferences(dev) {
  // ── 4.1 Modo de Verificação ──────────────────────────────────
  const authmodeCard = $('pref-authmode-card');
  if (authmodeCard) {
    const sel = $('pref-authmode-sel');
    const saveBtn = $('pref-authmode-save');
    const fb = $('pref-authmode-fb');
    const badge = $('pref-authmode-badge');
    const errEl = $('pref-authmode-err');

    apiGet(`devices/${dev.id}/preferences/auth-mode`).then(async res => {
      if (!sel) return;
      if (res.status === 401) return;
      if (!res.ok) {
        const msg = res.status === 504 ? 'Dispositivo offline — não foi possível carregar.' : `Erro (status ${res.status}).`;
        errEl.textContent = msg; errEl.style.display = '';
        if (badge) badge.textContent = 'Indisponível';
        return;
      }
      const data = await res.json();
      const plans = data.weekPlans || [];

      // Todos os dias devem ter o mesmo modo; pega o do primeiro
      const currentMode = plans.length > 0 ? (plans[0].verifyMode || '') : '';

      // Popular select com opções disponíveis
      sel.innerHTML = VERIFY_MODES.map(m =>
        `<option value="${escHtml(m.value)}" ${m.value === currentMode ? 'selected' : ''}>${escHtml(m.label)}</option>`
      ).join('');

      // Adicionar opção desconhecida se o modo atual não constar na lista
      if (currentMode && !VERIFY_MODES.find(m => m.value === currentMode)) {
        sel.innerHTML = `<option value="${escHtml(currentMode)}" selected>${escHtml(currentMode)}</option>` + sel.innerHTML;
      }

      sel.disabled = false;
      if (saveBtn) saveBtn.disabled = false;
      if (badge) badge.textContent = currentMode || '—';
    }).catch(err => {
      if (errEl) { errEl.textContent = err.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Falha de conexão.'; errEl.style.display = ''; }
      if (badge) badge.textContent = 'Indisponível';
    });

    if (saveBtn) {
      saveBtn.addEventListener('click', async () => {
        const mode = sel.value;
        if (!mode) return;
        saveBtn.disabled = true; saveBtn.textContent = 'Salvando…'; fb.textContent = '';
        try {
          const res = await apiPut(`devices/${dev.id}/preferences/auth-mode`, { verifyMode: mode });
          if (res.status === 401) return;
          if (res.ok) {
            fb.textContent = 'Modo de verificação salvo.'; fb.style.color = 'var(--green)';
            if (badge) badge.textContent = mode;
            showToast('success', 'Modo de verificação atualizado.');
          } else {
            let msg = `Erro (status ${res.status}).`;
            if (res.status === 504) msg = 'Dispositivo offline.';
            else { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
            fb.textContent = msg; fb.style.color = 'var(--red)';
          }
        } catch (err) {
          fb.textContent = err.message === 'timeout' ? 'Tempo esgotado.' : 'Falha de conexão.'; fb.style.color = 'var(--red)';
        } finally {
          saveBtn.disabled = false; saveBtn.textContent = 'Salvar modo';
        }
      });
    }
  }

  // ── 4.2 Layout de Tela ──────────────────────────────────────
  const displayCard = $('pref-display-card');
  if (displayCard) {
    const saveBtn = $('pref-display-save');
    const fb = $('pref-display-fb');
    const errEl = $('pref-display-err');
    let currentDisplayMode = 'normal';

    // Wire mode tabs
    const modeGroup = $('pref-display-mode');
    if (modeGroup) {
      modeGroup.querySelectorAll('.seg-btn').forEach(b => b.addEventListener('click', () => {
        currentDisplayMode = b.dataset.mode;
        modeGroup.querySelectorAll('.seg-btn').forEach(x => x.classList.toggle('active', x === b));
      }));
    }

    apiGet(`devices/${dev.id}/preferences/display`).then(async res => {
      if (res.status === 401) return;
      if (!res.ok) {
        const msg = res.status === 504 ? 'Dispositivo offline.' : `Erro (status ${res.status}).`;
        if (errEl) { errEl.textContent = msg; errEl.style.display = ''; }
        return;
      }
      const data = await res.json();
      currentDisplayMode = data.showMode || 'normal';

      // Ativar tab do modo atual
      if (modeGroup) {
        modeGroup.querySelectorAll('.seg-btn').forEach(b => b.classList.toggle('active', b.dataset.mode === currentDisplayMode));
      }

      const soEl = $('pref-screen-off');
      const ptEl = $('pref-preview-time');
      const stEl = $('pref-standby-timeout');
      if (soEl) soEl.value = data.screenOffTimeout != null ? data.screenOffTimeout : '';
      if (ptEl) ptEl.value = data.previewShowTime  != null ? data.previewShowTime  : '';
      if (stEl) stEl.value = data.standbyTimeout   != null ? data.standbyTimeout   : '';

      if (saveBtn) saveBtn.disabled = false;
    }).catch(err => {
      if (errEl) { errEl.textContent = err.message === 'timeout' ? 'Tempo de resposta esgotado.' : 'Falha de conexão.'; errEl.style.display = ''; }
    });

    if (saveBtn) {
      saveBtn.addEventListener('click', async () => {
        const soEl = $('pref-screen-off');
        const ptEl = $('pref-preview-time');
        const stEl = $('pref-standby-timeout');
        const screenOffTimeout = parseInt(soEl ? soEl.value : '', 10);
        const previewShowTime  = parseInt(ptEl ? ptEl.value : '', 10);
        const standbyTimeout   = parseInt(stEl ? stEl.value : '', 10);

        if (isNaN(screenOffTimeout) || isNaN(previewShowTime) || isNaN(standbyTimeout)) {
          fb.textContent = 'Preencha todos os campos numéricos.'; fb.style.color = 'var(--red)'; return;
        }

        saveBtn.disabled = true; saveBtn.textContent = 'Salvando…'; fb.textContent = '';
        try {
          const res = await apiPut(`devices/${dev.id}/preferences/display`, {
            showMode: currentDisplayMode,
            screenOffTimeout,
            previewShowTime,
            standbyTimeout,
          });
          if (res.status === 401) return;
          if (res.ok) {
            fb.textContent = 'Layout de tela salvo.'; fb.style.color = 'var(--green)';
            showToast('success', 'Layout de tela atualizado.');
          } else {
            let msg = `Erro (status ${res.status}).`;
            if (res.status === 504) msg = 'Dispositivo offline.';
            else if (res.status === 400) { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
            fb.textContent = msg; fb.style.color = 'var(--red)';
          }
        } catch (err) {
          fb.textContent = err.message === 'timeout' ? 'Tempo esgotado.' : 'Falha de conexão.'; fb.style.color = 'var(--red)';
        } finally {
          saveBtn.disabled = false; saveBtn.textContent = 'Salvar layout';
        }
      });
    }
  }

  // ── 4.3 Standby Picture ──────────────────────────────────────
  function loadStandbyList() {
    const listEl = $('pref-standby-list');
    if (!listEl) return;
    listEl.innerHTML = loadingState();
    apiGet(`devices/${dev.id}/preferences/standby-pictures`).then(async res => {
      if (!listEl) return;
      if (res.status === 401) return;
      if (!res.ok) {
        const msg = res.status === 504 ? 'Dispositivo offline.' : `Erro (status ${res.status}).`;
        listEl.innerHTML = emptyState(ICON.upload, 'Erro ao carregar', msg);
        return;
      }
      const data = await res.json();
      const pics = data.pictures || [];
      if (!pics.length) {
        listEl.innerHTML = emptyState(ICON.upload, 'Nenhuma imagem de standby', 'Envie uma imagem para personalizar a tela de espera do terminal.');
        return;
      }
      listEl.innerHTML = pics.map(p => `
        <div class="trow" style="grid-template-columns:1fr auto;" id="standby-row-${escHtml(p.uuid)}">
          <div style="min-width:0;">
            <div class="cell-ellipsis mono" style="font-size:12px;">${escHtml(p.fileName || p.uuid)}</div>
            <div style="font-size:11px; color:var(--text-3);">UUID: ${escHtml(p.uuid)}</div>
          </div>
          <button class="btn btn-danger sm" data-standby-del="${escHtml(p.uuid)}">${ICON.trash}</button>
        </div>`).join('');

      listEl.querySelectorAll('[data-standby-del]').forEach(btn => {
        btn.addEventListener('click', async () => {
          const uuid = btn.dataset.standbyDel;
          btn.disabled = true;
          try {
            const res = await apiDelete(`devices/${dev.id}/preferences/standby-pictures/${uuid}`);
            if (res.status === 401) return;
            if (res.ok) {
              showToast('success', 'Imagem de standby removida.');
              const row = $(`standby-row-${uuid}`);
              if (row) row.remove();
              if (!listEl.querySelector('.trow')) listEl.innerHTML = emptyState(ICON.upload, 'Nenhuma imagem de standby', '');
            } else {
              let msg = `Erro (status ${res.status}).`;
              try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
              showToast('error', msg);
              btn.disabled = false;
            }
          } catch (err) { netError(err); btn.disabled = false; }
        });
      });
    }).catch(err => {
      const el = $('pref-standby-list');
      if (el) el.innerHTML = emptyState(ICON.upload, 'Falha de conexão', '');
      netError(err);
    });
  }

  loadStandbyList();

  const standbyFile = $('pref-standby-file');
  if (standbyFile) {
    standbyFile.addEventListener('change', async () => {
      const file = standbyFile.files[0];
      if (!file) return;
      standbyFile.value = '';
      if (!file.type.startsWith('image/')) { showToast('error', 'O arquivo deve ser uma imagem (image/*).'); return; }
      showToast('info', 'Enviando imagem de standby…');
      const fd = new FormData();
      fd.append('file', file, file.name);
      try {
        const ctrl = new AbortController();
        const timer = setTimeout(() => ctrl.abort(), 60_000);
        const res = await fetch(`/admin/api/devices/${dev.id}/preferences/standby-pictures`, {
          method: 'POST', body: fd, credentials: 'same-origin', signal: ctrl.signal,
        });
        clearTimeout(timer);
        if (res.status === 401) { navigate('login', false); return; }
        if (res.ok) {
          showToast('success', 'Imagem enviada e standby customizado ativado.');
          loadStandbyList();
        } else if (res.status === 413) {
          showToast('error', 'Arquivo muito grande (máximo 20 MB).');
        } else {
          let msg = `Erro ao enviar (status ${res.status}).`;
          try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
          showToast('error', msg);
        }
      } catch (err) {
        netError(err);
      }
    });
  }

  const standbyDisableBtn = $('pref-standby-disable');
  if (standbyDisableBtn) {
    standbyDisableBtn.addEventListener('click', async () => {
      standbyDisableBtn.disabled = true; standbyDisableBtn.textContent = 'Desativando…';
      try {
        const res = await apiPut(`devices/${dev.id}/preferences/standby-pictures/disable`);
        if (res.status === 401) return;
        if (res.ok) showToast('success', 'Standby customizado desativado — exibição padrão restaurada.');
        else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
      } catch (err) { netError(err); }
      finally { standbyDisableBtn.disabled = false; standbyDisableBtn.textContent = 'Desativar customizada'; }
    });
  }

  // ── 4.3 Boot Logo ───────────────────────────────────────────
  const bootpicFile = $('pref-bootpic-file');
  if (bootpicFile) {
    bootpicFile.addEventListener('change', async () => {
      const file = bootpicFile.files[0];
      if (!file) return;
      bootpicFile.value = '';
      if (!file.type.startsWith('image/')) { showToast('error', 'O arquivo deve ser uma imagem.'); return; }
      showToast('info', 'Enviando boot logo…');
      const fd = new FormData();
      fd.append('file', file, file.name);
      try {
        const ctrl = new AbortController();
        const timer = setTimeout(() => ctrl.abort(), 60_000);
        const res = await fetch(`/admin/api/devices/${dev.id}/preferences/boot-picture`, {
          method: 'POST', body: fd, credentials: 'same-origin', signal: ctrl.signal,
        });
        clearTimeout(timer);
        if (res.status === 401) { navigate('login', false); return; }
        if (res.ok) {
          showToast('success', 'Boot logo enviado com sucesso.');
        } else if (res.status === 413) {
          showToast('error', 'Arquivo muito grande (máximo 20 MB).');
        } else {
          let msg = `Erro ao enviar (status ${res.status}).`;
          try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
          showToast('error', msg);
        }
      } catch (err) { netError(err); }
    });
  }

  const bootpicDeleteBtn = $('pref-bootpic-delete');
  if (bootpicDeleteBtn) {
    bootpicDeleteBtn.addEventListener('click', () => {
      openConfirm({
        title: 'Remover boot logo', confirmLabel: 'Remover', tone: 'warn', strong: false,
        body: 'Remove a imagem de inicialização customizada. O terminal voltará a exibir o logo padrão HikVision.',
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiDelete(`devices/${dev.id}/preferences/boot-picture`);
            if (res.status === 401) return;
            if (res.ok) showToast('success', 'Boot logo removido.');
            else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
          } catch (err) { netError(err); }
        },
      });
    });
  }

  // ── 4.4 Estatísticas de Capacidade ──────────────────────────
  function loadStats() {
    const body = $('pref-stats-body');
    const updEl = $('pref-stats-updated');
    if (!body) return;
    body.innerHTML = loadingState();
    if (updEl) updEl.textContent = 'Atualizando…';
    apiGet(`devices/${dev.id}/stats`).then(async res => {
      if (!body) return;
      if (res.status === 401) return;
      if (!res.ok) {
        // US4-AC2: offline → Indisponível, nunca zeros
        body.innerHTML = statGrid('Indisponível', 'Indisponível', 'Indisponível', 'Indisponível', 'Indisponível', 'Indisponível');
        if (updEl) updEl.textContent = res.status === 504 ? 'Dispositivo offline' : `Erro status ${res.status}`;
        return;
      }
      const data = await res.json();
      const u = data.users || {};
      const e = data.events || {};
      const fmt = v => v != null ? v.toLocaleString('pt-BR') : 'Indisponível';
      body.innerHTML = statGrid(fmt(u.total), fmt(u.max), fmt(u.faces), fmt(u.cards), fmt(e.total), fmt(e.max));
      if (updEl) updEl.textContent = `Atualizado ${fmtShort(new Date().toISOString())}`;
    }).catch(err => {
      const el = $('pref-stats-body');
      if (el) el.innerHTML = statGrid('Indisponível', 'Indisponível', 'Indisponível', 'Indisponível', 'Indisponível', 'Indisponível');
      const u = $('pref-stats-updated'); if (u) u.textContent = 'Falha de conexão';
      netError(err);
    });
  }

  function statGrid(usrTotal, usrMax, faces, cards, evtTotal, evtMax) {
    const cell = (label, value) => `
      <div style="padding:14px 16px;">
        <div style="font-size:10px; text-transform:uppercase; letter-spacing:.06em; color:var(--text-3); font-weight:600; margin-bottom:4px;">${label}</div>
        <div style="font-size:18px; font-weight:600; font-variant-numeric:tabular-nums;">${escHtml(String(value))}</div>
      </div>`;
    return `<div style="display:grid; grid-template-columns:repeat(3,1fr); gap:1px; background:var(--border);">
      ${cell('Usuários cadastrados', usrTotal)}
      ${cell('Capacidade máx. usuários', usrMax)}
      ${cell('Faces cadastradas', faces)}
      ${cell('Cartões cadastrados', cards)}
      ${cell('Eventos registrados', evtTotal)}
      ${cell('Capacidade máx. eventos', evtMax)}
    </div>`;
  }

  loadStats();

  const refreshBtn = $('pref-stats-refresh');
  if (refreshBtn) refreshBtn.addEventListener('click', loadStats);

  // ── 4.5 Configuração Avançada de Face ───────────────────────
  const faceadvSave = $('pref-faceadv-save');
  if (faceadvSave) {
    faceadvSave.addEventListener('click', async () => {
      const maxDistEl = $('pref-maxdistance');
      const fb = $('pref-faceadv-fb');
      const maxDistance = parseFloat(maxDistEl ? maxDistEl.value : '');
      if (isNaN(maxDistance) || maxDistance <= 0) {
        fb.textContent = 'Informe um valor maior que zero.'; fb.style.color = 'var(--red)'; return;
      }
      faceadvSave.disabled = true; faceadvSave.textContent = 'Salvando…'; fb.textContent = '';
      try {
        const res = await apiPut(`devices/${dev.id}/preferences/face-config`, { maxDistance });
        if (res.status === 401) return;
        if (res.ok) {
          fb.textContent = 'Configuração salva.'; fb.style.color = 'var(--green)';
          showToast('success', 'Configuração de face atualizada.');
        } else {
          let msg = `Erro (status ${res.status}).`;
          if (res.status === 504) msg = 'Dispositivo offline.';
          else if (res.status === 400) { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
          fb.textContent = msg; fb.style.color = 'var(--red)';
        }
      } catch (err) {
        fb.textContent = err.message === 'timeout' ? 'Tempo esgotado.' : 'Falha de conexão.'; fb.style.color = 'var(--red)';
      } finally {
        faceadvSave.disabled = false; faceadvSave.textContent = 'Salvar configuração';
      }
    });
  }

  const captureBtn = $('pref-facecapture-btn');
  if (captureBtn) {
    captureBtn.addEventListener('click', async () => {
      const resultEl = $('pref-facecapture-result');
      const imgEl = $('pref-facecapture-img');
      const errEl = $('pref-facecapture-err');
      captureBtn.disabled = true; captureBtn.textContent = 'Capturando…';
      if (resultEl) resultEl.style.display = 'none';
      if (errEl) errEl.style.display = 'none';
      try {
        const res = await apiFetch(`/admin/api/devices/${dev.id}/preferences/face-capture`, { method: 'POST', credentials: 'same-origin' }, 30_000);
        if (res.status === 401) { navigate('login', false); return; }
        if (res.ok) {
          const data = await res.json();
          if (data.image && imgEl && resultEl) {
            imgEl.src = `data:image/jpeg;base64,${data.image}`;
            resultEl.style.display = '';
          }
        } else {
          let msg = `Não foi possível capturar (status ${res.status}).`;
          if (res.status === 504) msg = 'Dispositivo offline — câmera inacessível.';
          else if (res.status === 502) msg = 'Falha de segurança ou dispositivo inacessível.';
          else { try { const d = await res.json(); if (d.error) msg = d.error; } catch {} }
          if (errEl) { errEl.textContent = msg; errEl.style.display = ''; }
        }
      } catch (err) {
        if (errEl) { errEl.textContent = err.message === 'timeout' ? 'Captura esgotou o tempo (30s).' : 'Falha de conexão.'; errEl.style.display = ''; }
        netError(err);
      } finally {
        captureBtn.disabled = false; captureBtn.textContent = 'Capturar imagem';
      }
    });
  }
}

// ─── MÍDIA (propaganda — task 4.5.3) ──────────────────────────

function cfgMediaFull(dev) {
  return `
    <div class="card flush" id="media-list-card">
      <div class="card-head">
        <div class="h">Material de propaganda</div>
        <div style="display:flex; align-items:center; gap:8px;">
          <label class="btn btn-accent sm" style="cursor:pointer; margin:0;">
            ${ICON.upload} Enviar imagem
            <input type="file" id="media-upload-file" accept="image/*" style="display:none;" />
          </label>
          <button class="btn btn-danger sm" id="media-delete-all">${ICON.trash} Remover tudo</button>
        </div>
      </div>
      <div id="media-list-body" style="padding:16px;">${loadingState()}</div>
    </div>
  `;
}

function wireCfgMediaFull(dev) {
  function loadMediaList() {
    const body = $('media-list-body');
    if (!body) return;
    body.innerHTML = loadingState();
    apiGet(`devices/${dev.id}/preferences/media`).then(async res => {
      if (!body) return;
      if (res.status === 401) return;
      if (!res.ok) {
        const msg = res.status === 504 ? 'Dispositivo offline.' : `Erro (status ${res.status}).`;
        body.innerHTML = emptyState(ICON.upload, 'Erro ao carregar materiais', msg);
        return;
      }
      const data = await res.json();
      const items = data.materials || [];
      if (!items.length) {
        body.innerHTML = emptyState(ICON.upload, 'Nenhum material cadastrado', 'Envie uma imagem para exibir propaganda no terminal.');
        return;
      }
      body.innerHTML = items.map(m => `
        <div class="trow" style="grid-template-columns:1fr auto;" id="media-row-${escHtml(m.id)}">
          <div style="min-width:0;">
            <div class="cell-ellipsis" style="font-size:12.5px; font-weight:500;">${escHtml(m.name || m.id)}</div>
            <div class="mono" style="font-size:11px; color:var(--text-3);">ID: ${escHtml(m.id)}</div>
          </div>
          <button class="btn btn-danger sm" data-media-del="${escHtml(m.id)}">${ICON.trash}</button>
        </div>`).join('');

      body.querySelectorAll('[data-media-del]').forEach(btn => {
        btn.addEventListener('click', async () => {
          const id = btn.dataset.mediaDel;
          btn.disabled = true;
          try {
            const res = await apiDelete(`devices/${dev.id}/preferences/media/${id}`);
            if (res.status === 401) return;
            if (res.ok) {
              showToast('success', 'Material removido.');
              const row = $(`media-row-${id}`);
              if (row) row.remove();
              if (!body.querySelector('.trow')) body.innerHTML = emptyState(ICON.upload, 'Nenhum material cadastrado', '');
            } else {
              let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {}
              showToast('error', msg); btn.disabled = false;
            }
          } catch (err) { netError(err); btn.disabled = false; }
        });
      });
    }).catch(err => {
      const el = $('media-list-body');
      if (el) el.innerHTML = emptyState(ICON.upload, 'Falha de conexão', '');
      netError(err);
    });
  }

  loadMediaList();

  const uploadFile = $('media-upload-file');
  if (uploadFile) {
    uploadFile.addEventListener('change', async () => {
      const file = uploadFile.files[0];
      if (!file) return;
      uploadFile.value = '';
      if (!file.type.startsWith('image/')) { showToast('error', 'O arquivo deve ser uma imagem (image/*).'); return; }
      showToast('info', 'Criando material de propaganda…');
      const fd = new FormData();
      fd.append('file', file, file.name);
      try {
        const ctrl = new AbortController();
        const timer = setTimeout(() => ctrl.abort(), 120_000);
        const res = await fetch(`/admin/api/devices/${dev.id}/preferences/media`, {
          method: 'POST', body: fd, credentials: 'same-origin', signal: ctrl.signal,
        });
        clearTimeout(timer);
        if (res.status === 401) { navigate('login', false); return; }
        if (res.ok) {
          showToast('success', 'Material criado e programação configurada.');
          loadMediaList();
        } else if (res.status === 413) {
          showToast('error', 'Arquivo muito grande (máximo 20 MB).');
        } else {
          let msg = `Erro ao criar material (status ${res.status}).`;
          try {
            const d = await res.json();
            if (d.error) msg = d.error;
            if (d.orphanMaterialId) msg += ` Material órfão: ${d.orphanMaterialId}`;
          } catch {}
          showToast('error', msg);
        }
      } catch (err) { netError(err); }
    });
  }

  const deleteAllBtn = $('media-delete-all');
  if (deleteAllBtn) {
    deleteAllBtn.addEventListener('click', () => {
      openConfirm({
        title: 'Remover toda propaganda', confirmLabel: 'Remover tudo', tone: 'danger', strong: false,
        body: 'Remove todos os materiais de propaganda cadastrados no terminal.',
        target: cfgTargetOf(dev),
        onConfirm: async () => {
          try {
            const res = await apiDelete(`devices/${dev.id}/preferences/media`);
            if (res.status === 401) return;
            if (res.ok) { showToast('success', 'Todos os materiais removidos.'); loadMediaList(); }
            else { let msg = `Erro (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
          } catch (err) { netError(err); }
        },
      });
    });
  }
}

function cfgMedia() { return cfgMediaFull({}); } // alias legado — não deveria ser chamado

const CFG_MODALS = {
  reboot:     { title:'Reiniciar dispositivo', confirmLabel:'Reiniciar agora', tone:'warn', strong:false, body:'O terminal ficará indisponível por cerca de 40 segundos. Reconhecimentos não serão registrados durante a reinicialização.' },
  factory:    { title:'Reset de fábrica', confirmLabel:'Apagar tudo e resetar', tone:'danger', strong:true, body:'Esta ação é IRREVERSÍVEL. Todos os usuários, faces, cartões e configurações deste terminal serão apagados. Será necessário reprovisionar todos os membros a partir do GOB.' },
  doorOpen:   { title:'Destravar porta remotamente', confirmLabel:'Destravar porta', tone:'warn', strong:false, body:'A porta será destravada imediatamente por 5 segundos. A ação fica registrada no log de auditoria com o seu usuário.' },
  clearUsers: { title:'Limpar todos os usuários', confirmLabel:'Limpar usuários', tone:'danger', strong:true, body:'Remove TODOS os usuários deste terminal — as faces associadas também são apagadas. Os membros precisarão ser reprovisionados.' },
  clearCards: { title:'Limpar todos os cartões', confirmLabel:'Limpar cartões', tone:'danger', strong:true, body:'Remove TODOS os cartões RFID/NFC cadastrados neste terminal.' },
  clearFaces: { title:'Limpar biblioteca de faces', confirmLabel:'Limpar faces', tone:'danger', strong:true, body:'Remove TODAS as faces cadastradas. Os usuários permanecem, mas sem reconhecimento facial até reenviar as faces.' },
};

function wireCfgActions(dev) {
  $('cfg-content').querySelectorAll('[data-modal]').forEach(b => b.addEventListener('click', () => {
    const m = CFG_MODALS[b.dataset.modal]; if (!m) return;
    openConfirm({ ...m, target: cfgTargetOf(dev), onConfirm: () => pendingBackend(m.confirmLabel) });
  }));
  $('cfg-content').querySelectorAll('[data-pending]').forEach(b => b.addEventListener('click', () => pendingBackend(b.dataset.pending)));
  $('cfg-content').querySelectorAll('.switch').forEach(sw => sw.addEventListener('click', () => sw.classList.toggle('on')));
  // "Configurar agora" link no overview navega para aba auth
  $('cfg-content').querySelectorAll('[data-section-goto]').forEach(b => b.addEventListener('click', () => navigate(`device-config?id=${dev.id}&section=${b.dataset.sectionGoto}`)));
}

// ─── MEMBERS ──────────────────────────────────────────────────
function mountMembers() {
  setView(`
    <div class="screen">
      <div class="toolbar">
        <div class="searchbox" style="width:320px;">${ICON.search}<input id="mem-search" placeholder="Buscar por nome ou CPF…" aria-label="Buscar membros" autocomplete="off" value="${escHtml(state.members.query)}"></div>
        <div class="seg" id="mem-filter">
          <button class="seg-btn active" data-f="all">Todos</button>
          <button class="seg-btn" data-f="ativos">Ativos</button>
          <button class="seg-btn" data-f="falhas">Falhas</button>
        </div>
        <div class="spacer meta mono" id="mem-count">—</div>
      </div>
      <div class="card flush">
        <div class="thead" style="grid-template-columns:1.8fr 1.1fr .8fr 1fr 1.1fr 70px;">
          <div>Membro</div><div>CPF</div><div>Status</div><div>Provisão</div><div class="hide-mobile">Última falha</div><div></div>
        </div>
        <div id="mem-rows">${loadingState()}</div>
        <div class="table-foot" id="mem-foot" style="display:none;"><button class="btn btn-soft sm" id="mem-more">Carregar mais</button></div>
      </div>
    </div>`);

  const search = $('mem-search');
  let timer;
  search.addEventListener('input', () => { clearTimeout(timer); timer = setTimeout(() => { state.members.query = search.value.trim(); loadMembers(true); }, DEBOUNCE_MS); });
  $('mem-filter').querySelectorAll('.seg-btn').forEach(b => b.addEventListener('click', () => {
    state.members.filter = b.dataset.f;
    $('mem-filter').querySelectorAll('.seg-btn').forEach(x => x.classList.toggle('active', x === b));
    renderMemberRows();
  }));
  $('mem-more').addEventListener('click', () => { if (state.members.hasMore && state.members.nextCursor != null) { $('mem-more').disabled = true; loadMembers(false); } });

  loadMembers(true);
}

async function loadMembers(reset) {
  if (reset) { state.members.items = []; state.members.nextCursor = null; state.members.hasMore = false; $('mem-rows').innerHTML = loadingState(); }
  const params = new URLSearchParams();
  if (state.members.query) params.set('q', state.members.query);
  if (state.members.nextCursor != null) params.set('cursor', state.members.nextCursor);
  try {
    const res = await apiGet(`members?${params}`);
    if (res.status === 401) return;
    if (!res.ok) { $('mem-rows').innerHTML = emptyState(ICON.members, 'Erro ao carregar', `Status ${res.status}.`); return; }
    const data = await res.json();
    const items = data.members || [];
    state.members.items = reset ? items : [...state.members.items, ...items];
    state.members.items.forEach(m => state.members.byId[m.id] = m);
    state.members.nextCursor = data.next_cursor ?? null;
    state.members.hasMore = data.has_more ?? false;
    renderMemberRows();
  } catch (err) {
    if ($('mem-rows')) $('mem-rows').innerHTML = emptyState(ICON.members, 'Falha de conexão', 'Tente novamente.');
    netError(err);
  }
}

function filteredMembers() {
  return state.members.items.filter(m => {
    if (state.members.filter === 'ativos') return String(m.status || '').toLowerCase() === 'ativo';
    if (state.members.filter === 'falhas') return m.sync_status === 'failed';
    return true;
  });
}

function renderMemberRows() {
  const rowsEl = $('mem-rows'); if (!rowsEl) return;
  const cnt = $('mem-count'); if (cnt) cnt.textContent = `${state.members.items.length}${state.members.hasMore ? '+' : ''} membros`;
  const list = filteredMembers();
  const foot = $('mem-foot');
  if (!list.length) {
    rowsEl.innerHTML = state.members.query
      ? emptyState(ICON.members, 'Nenhum resultado', `Nenhum membro corresponde a "${state.members.query}".`)
      : emptyState(ICON.members, 'Nenhum membro', 'Os membros aparecem aqui após a primeira sincronização com o GOB.');
    if (foot) foot.style.display = 'none';
    return;
  }
  rowsEl.innerHTML = list.map(m => `
    <div class="trow clickable" style="grid-template-columns:1.8fr 1.1fr .8fr 1fr 1.1fr 70px;" data-member-id="${escHtml(String(m.id))}" tabindex="0" role="button" aria-label="Perfil de ${escHtml(m.name)}">
      <div style="display:flex; align-items:center; gap:11px; min-width:0;">
        <div class="avatar">${escHtml(initials(m.name))}</div>
        <div class="cell-ellipsis" style="font-size:13px; font-weight:500;">${escHtml(m.name || '—')}</div>
      </div>
      <div class="mono" style="font-size:11.5px; color:var(--text-2);">${escHtml(m.federal_document_masked || '—')}</div>
      <div>${memberStatusBadge(m.status)}</div>
      <div>${provBadge(m.sync_status)}</div>
      <div class="hide-mobile mono" style="font-size:11px; color:var(--text-3);">${escHtml(m.last_failed_stage || '—')}</div>
      <div style="display:flex; justify-content:flex-end;">
        <button class="icon-btn sm" data-resync-id="${escHtml(String(m.id))}" data-resync-name="${escHtml(m.name || 'membro')}" title="Reenviar provisionamento" aria-label="Reenviar ${escHtml(m.name || '')}">${ICON.resync}</button>
      </div>
    </div>`).join('');

  rowsEl.querySelectorAll('[data-member-id]').forEach(row => {
    const go = e => { if (e.target.closest('[data-resync-id]')) return; navigate(`member-profile?id=${row.dataset.memberId}`); };
    row.addEventListener('click', go);
    row.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); navigate(`member-profile?id=${row.dataset.memberId}`); } });
  });
  rowsEl.querySelectorAll('[data-resync-id]').forEach(b => b.addEventListener('click', e => { e.stopPropagation(); resyncMember(b.dataset.resyncId, b.dataset.resyncName, b); }));
  if (foot) foot.style.display = state.members.hasMore ? '' : 'none';
  const more = $('mem-more'); if (more) more.disabled = false;
}

async function resyncMember(id, name, btn) {
  if (btn) btn.disabled = true;
  try {
    const res = await apiPost(`members/${id}/resync`);
    if (res.status === 401) return;
    if (res.status === 202) showToast('success', `${name} enfileirado para reprocessamento.`);
    else { let msg = `Falha ao reenviar (status ${res.status}).`; try { const d = await res.json(); if (d.error) msg = d.error; } catch {} showToast('error', msg); }
  } catch (err) { netError(err); }
  finally { if (btn) btn.disabled = false; }
}

// ─── MEMBER PROFILE (nova tela — design-brief §8) ─────────────
function mountMemberProfile(params) {
  const m = state.members.byId[params.id];
  if (!m) {
    setView(`<div class="screen"><button class="btn-back" id="mp-back">${ICON.back} Voltar para Membros</button>${emptyState(ICON.members, 'Abra o perfil pela lista', 'O perfil completo do membro é carregado a partir da lista de Membros (não há endpoint de detalhe por id neste build).')}</div>`);
    $('mp-back').addEventListener('click', () => navigate('members'));
    return;
  }
  const tab = params.tab || 'prov';
  renderMemberProfile(m, tab);
}

function renderMemberProfile(m, tab) {
  const fact = (k, v, mono) => `<div class="mp-fact"><div class="k">${k}</div><div class="v ${mono?'mono':''}">${v}</div></div>`;
  setView(`
    <div class="screen">
      <button class="btn-back" id="mp-back">${ICON.back} Voltar para Membros</button>
      <div class="card mp-head">
        <div class="mp-photo"><span class="ini">${escHtml(initials(m.name))}</span></div>
        <div style="flex:1; min-width:220px;">
          <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap;">
            <h2 class="mp-name">${escHtml(m.name || '—')}</h2>
            ${memberStatusBadge(m.status)}
            ${provBadge(m.sync_status)}
          </div>
          <div class="mp-facts">
            ${fact('CPF', escHtml(m.federal_document_masked || '—'), true)}
            ${fact('Telefone', '—')}
            ${fact('GOB ID', '—', true)}
            ${fact('Sincronizado', '—', true)}
            ${fact('Atualizado', '—', true)}
          </div>
        </div>
      </div>
      ${pendingNote('Foto (url_selfie), telefone, GOB ID e datas vêm de um endpoint de detalhe do membro (§8.1) ainda não disponível — exibindo o que a listagem fornece.')}
      <div class="tabs">
        <button class="tab ${tab==='prov'?'active':''}" id="mp-tab-prov">Provisão nos dispositivos</button>
        <button class="tab ${tab==='hist'?'active':''}" id="mp-tab-hist">Histórico de acessos</button>
      </div>
      <div id="mp-tabbody"></div>
    </div>`);

  $('mp-back').addEventListener('click', () => navigate('members'));
  $('mp-tab-prov').addEventListener('click', () => navigate(`member-profile?id=${m.id}&tab=prov`));
  $('mp-tab-hist').addEventListener('click', () => navigate(`member-profile?id=${m.id}&tab=hist`));
  $('mp-tabbody').innerHTML = tab === 'hist' ? mpHistory() : mpProvision(m);

  const rb = $('mp-resync'); if (rb) rb.addEventListener('click', () => resyncMember(m.id, m.name, rb));
}

function mpProvision(m) {
  // member_processing_status (por device) não é exposto pelo contrato atual (§8.2).
  // Mostramos o estado de sincronização que a listagem fornece + ação de reenvio real.
  return `
    <div class="stack">
      <div class="card pad">
        <div class="row-between">
          <div><div style="font-size:14px; font-weight:600;">Situação de provisionamento</div><div style="font-size:12px; color:var(--text-2); margin-top:4px;">Estado consolidado (GOB): ${provBadge(m.sync_status)} ${m.last_failed_stage ? `· última falha em <span class="mono">${escHtml(m.last_failed_stage)}</span>` : ''}</div></div>
          <button class="btn btn-soft sm" id="mp-resync">${ICON.resync} Reenviar</button>
        </div>
        <div class="steps">
          <div class="bar"></div>
          <div class="row">
            ${provStep('Usuário criado', m.sync_status)}
            ${provStep('Face enviada', m.sync_status)}
            ${provStep('Webhook configurado', m.sync_status)}
          </div>
        </div>
      </div>
      ${pendingNote('O detalhamento por dispositivo (3 etapas por terminal, motivo de falha por device) será exibido quando o endpoint de provisionamento por membro × dispositivo (§8.2) estiver disponível.')}
    </div>`;
}
function provStep(label, sync) {
  // Aproximação visual a partir do sync_status consolidado (sem dados por-etapa reais).
  const st = sync === 'synced' ? 'done' : sync === 'failed' ? 'failed' : 'pending';
  const icon = st === 'done'
    ? '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>'
    : st === 'failed'
    ? '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>'
    : '<span style="width:6px; height:6px; border-radius:50%; background:var(--text-3);"></span>';
  const sub = st === 'done' ? 'ok' : st === 'failed' ? 'falhou' : 'aguardando';
  return `<div class="step"><div class="step-circle ${st}">${icon}</div><div><div class="lbl">${label}</div><div class="sub">${sub}</div></div></div>`;
}

function mpHistory() {
  // Não há endpoint de eventos por membro (events só filtra por data) — §8.3 aguarda backend.
  const sum = (label, val, sub) => `<div class="card pad"><div style="font-size:11.5px; color:var(--text-2);">${label}</div><div style="font-size:18px; font-weight:600; margin-top:6px;">${val}</div><div style="font-size:11px; color:var(--text-3); margin-top:2px;">${sub}</div></div>`;
  return `
    <div class="stack">
      <div class="grid-3">
        ${sum('Último acesso', '—', 'aguardando backend')}
        ${sum('Acessos · 7 dias', '—', 'aguardando backend')}
        ${sum('Acessos · 30 dias', '—', 'aguardando backend')}
      </div>
      <div class="card pad">
        <div class="row-between" style="margin-bottom:8px;"><div class="h" style="font-size:14px; font-weight:600;">Histórico de acessos</div><div class="meta mono">§8.3</div></div>
        ${emptyState(ICON.bolt, 'Histórico por membro', 'O histórico e a frequência de acessos deste membro serão exibidos quando o endpoint de eventos por membro (§8.3) estiver disponível. A lista global continua em Eventos.')}
      </div>
    </div>`;
}

// ─── EVENTS ───────────────────────────────────────────────────
function mountEvents() {
  setView(`
    <div class="screen">
      <div class="grid-3" id="evt-summary">
        ${evtSummaryCard('Acessos carregados', '—', '')}
        ${evtSummaryCard('Marcados no GOB', '—', 'var(--green)')}
        ${evtSummaryCard('Negados / falhas', '—', 'var(--red)')}
      </div>
      <div class="toolbar">
        <div class="seg plain" id="evt-period">
          <button class="seg-btn ${state.events.period==='24h'?'active':''}" data-p="24h">24h</button>
          <button class="seg-btn ${state.events.period==='7d'?'active':''}" data-p="7d">7 dias</button>
          <button class="seg-btn ${state.events.period==='30d'?'active':''}" data-p="30d">30 dias</button>
        </div>
        <div class="spacer meta mono">mais recentes primeiro</div>
      </div>
      <div class="card flush">
        <div class="thead" style="grid-template-columns:1.8fr 1.3fr 1fr 1fr 110px;">
          <div>Membro</div><div>Dispositivo</div><div>Resultado</div><div>Marcação GOB</div><div style="text-align:right;">Data/hora</div>
        </div>
        <div id="evt-rows">${loadingState()}</div>
        <div class="table-foot" id="evt-foot" style="display:none;"><button class="btn btn-soft sm" id="evt-more">Carregar mais</button></div>
      </div>
    </div>`);

  $('evt-period').querySelectorAll('.seg-btn').forEach(b => b.addEventListener('click', () => {
    state.events.period = b.dataset.p;
    $('evt-period').querySelectorAll('.seg-btn').forEach(x => x.classList.toggle('active', x === b));
    loadEvents(true);
  }));
  $('evt-more').addEventListener('click', () => { if (state.events.hasMore && state.events.nextCursor) { $('evt-more').disabled = true; loadEvents(false); } });

  loadEvents(true);
}
function evtSummaryCard(label, val, color) {
  return `<div class="card pad"><div style="font-size:11.5px; color:var(--text-2);">${label}</div><div style="font-size:23px; font-weight:600; margin-top:6px; font-variant-numeric:tabular-nums; ${color?`color:${color};`:''}">${val}</div></div>`;
}

function periodFrom() {
  const now = Date.now();
  const ms = state.events.period === '7d' ? 7*864e5 : state.events.period === '30d' ? 30*864e5 : 864e5;
  return new Date(now - ms).toISOString();
}

async function loadEvents(reset) {
  if (reset) { state.events.items = []; state.events.nextCursor = null; state.events.hasMore = false; $('evt-rows').innerHTML = loadingState(); }
  const params = new URLSearchParams();
  params.set('from', periodFrom());
  if (state.events.nextCursor) params.set('cursor', JSON.stringify(state.events.nextCursor));
  try {
    const res = await apiGet(`events?${params}`);
    if (res.status === 401) return;
    if (!res.ok) { $('evt-rows').innerHTML = emptyState(ICON.bolt, 'Erro ao carregar', `Status ${res.status}.`); return; }
    const data = await res.json();
    const items = data.events || [];
    state.events.items = reset ? items : [...state.events.items, ...items];
    state.events.nextCursor = data.next_cursor ?? null;
    state.events.hasMore = data.has_more ?? false;
    renderEvents();
  } catch (err) {
    if ($('evt-rows')) $('evt-rows').innerHTML = emptyState(ICON.bolt, 'Falha de conexão', 'Tente novamente.');
    netError(err);
  }
}

function renderEvents() {
  const rowsEl = $('evt-rows'); if (!rowsEl) return;
  const items = state.events.items;
  // Resumo a partir da amostra carregada (rótulos deixam claro que é o carregado, não o período inteiro).
  const plus = state.events.hasMore ? '+' : '';
  const marked = items.filter(e => e.marking_status === 'marked').length;
  const denied = items.filter(e => e.marking_status === 'unauthorized' || e.marking_status === 'failed').length;
  const sum = $('evt-summary');
  if (sum) sum.innerHTML = evtSummaryCard('Acessos carregados', items.length + plus, '') + evtSummaryCard('Marcados no GOB', marked + plus, 'var(--green)') + evtSummaryCard('Negados / falhas', denied + plus, 'var(--red)');

  const foot = $('evt-foot');
  if (!items.length) {
    rowsEl.innerHTML = emptyState(ICON.bolt, 'Nenhum evento no período', 'Os reconhecimentos capturados pelos terminais aparecem aqui.');
    if (foot) foot.style.display = 'none';
    return;
  }
  rowsEl.innerHTML = items.map(e => {
    const name = e.member_name || 'Não reconhecido';
    const neg = e.marking_status === 'unauthorized';
    return `
      <div class="trow" style="grid-template-columns:1.8fr 1.3fr 1fr 1fr 110px;">
        <div style="display:flex; align-items:center; gap:11px; min-width:0;">
          <div class="avatar ${neg ? 'neg' : ''}">${neg ? '??' : escHtml(initials(name))}</div>
          <div style="min-width:0;">
            <div class="cell-ellipsis" style="font-size:13px; font-weight:500;">${escHtml(name)}</div>
            <div class="mono" style="font-size:10.5px; color:var(--text-3);">${escHtml(e.federal_document_masked || '—')}</div>
          </div>
        </div>
        <div style="font-size:12.5px; color:var(--text-2);" class="cell-ellipsis">${escHtml(e.device_identifier || '—')}</div>
        <div>${resultBadge(e.marking_status)}</div>
        <div>${markBadge(e.marking_status)}</div>
        <div class="mono" style="text-align:right; font-size:11.5px; color:var(--text-2);">${escHtml(fmtShort(e.event_datetime || e.created_at))}</div>
      </div>`;
  }).join('');
  if (foot) foot.style.display = state.events.hasMore ? '' : 'none';
  const more = $('evt-more'); if (more) more.disabled = false;
}

// ─── SHELL WIRING ─────────────────────────────────────────────
function initShell() {
  applyTheme(getTheme());
  $('theme-btn').addEventListener('click', toggleTheme);
  $('sync-btn').addEventListener('click', doSync);
  $('logout-btn').addEventListener('click', doLogout);

  document.querySelectorAll('.nav-item[data-route]').forEach(b => b.addEventListener('click', () => navigate(b.dataset.route)));

  // mobile drawer
  const app = $('app');
  $('hamburger').addEventListener('click', () => { app.classList.toggle('nav-open'); $('hamburger').setAttribute('aria-expanded', app.classList.contains('nav-open')); });
  $('scrim').addEventListener('click', () => app.classList.remove('nav-open'));

  // top search → members
  const ts = $('top-search');
  ts.addEventListener('keydown', e => {
    if (e.key === 'Enter') { state.members.query = ts.value.trim(); ts.value = ''; navigate('members'); }
  });

  window.addEventListener('hashchange', renderRoute);
}

function init() {
  initShell();
  renderRoute();
}

if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
else init();

// ─────────────────────────────────────────────────────────────────────────────
// FLOWS MODULE — Tasks 5.1 (lista), 5.2–5.4 (editor canvas), 5.5 (bg images), 5.6 (logs)
// APIs: /admin/api/flows*, /admin/api/background-images*
// ─────────────────────────────────────────────────────────────────────────────

// Node type registry (9 tipos conforme spec.md §FR-001 e domain/flow.go)
const ND = {
  start:             { label:'Início',         blocked:false,   h:52, inputs:0, out:1, outLabels:[''],               cv:'--green'  },
  camera_on:         { label:'Câmera ON',       blocked:'ISAPI', h:52, inputs:1, out:1, outLabels:[''],               cv:'--text-3' },
  camera_off:        { label:'Câmera OFF',      blocked:'ISAPI', h:52, inputs:1, out:1, outLabels:[''],               cv:'--text-3' },
  wait:              { label:'Aguardar',        blocked:false,   h:52, inputs:1, out:1, outLabels:[''],               cv:'--blue'   },
  change_background: { label:'Trocar fundo',    blocked:false,   h:52, inputs:1, out:1, outLabels:[''],               cv:'--accent' },
  https_call:        { label:'HTTPS Call',      blocked:false,   h:52, inputs:1, out:1, outLabels:[''],               cv:'--blue'   },
  qrcode_background: { label:'QR Code fundo',   blocked:false,   h:52, inputs:1, out:1, outLabels:[''],               cv:'--cyan'   },
  decision:          { label:'Decisão',         blocked:false,   h:70, inputs:1, out:2, outLabels:['valid','invalid'], cv:'--warn'   },
  send_message:      { label:'Enviar mensagem', blocked:'API',   h:52, inputs:1, out:1, outLabels:[''],               cv:'--text-3' },
};

const FLOW_NODE_W = 160;
const FLOW_PORT_R = 6;
const FLOW_CANVAS_W = 2400;
const FLOW_CANVAS_H = 1400;

// Variáveis de interpolação disponíveis (de internal/flow/interpolator.go — vocabulário fechado)
const FLOW_VARS = [
  '[user.name]','[user.document]','[user.status]','[user.mobile]',
  '[device.id]','[device.identifier]','[device.ip]','[device.mac]',
  '[event.authorized]','[event.datetime]',
];

// ─── Estado do editor (singleton, null quando não está na tela do editor) ───
let ES = null;
let _edDocHandlers = null;

function genNodeId() { return 'n' + Date.now().toString(36) + Math.random().toString(36).slice(2,5); }

function flIcon(sz) {
  return `<svg width="${sz}" height="${sz}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M4 12h4m0 0 2-4 4 8 2-4h4"/><circle cx="4" cy="12" r="2"/><circle cx="20" cy="8" r="2"/><circle cx="20" cy="16" r="2"/></svg>`;
}
function flowBadge(s) { return s === 'active' ? badge('ok','Ativo') : badge('muted','Inativo'); }

// ─── 5.1 Lista de fluxos ─────────────────────────────────────────────────────

async function mountFlowsList() {
  cleanupEditor();
  setView(loadingState());
  try {
    const res = await apiGet('flows');
    if (res.status === 401) return;
    if (!res.ok) { setView(emptyState(flIcon(32),'Erro ao carregar fluxos',`Status ${res.status}.`)); return; }
    const data = await res.json();
    renderFlowsList(data.flows || []);
  } catch(err) {
    setView(emptyState(flIcon(32),'Falha de conexão','Tente novamente.'));
    netError(err);
  }
}

function renderFlowsList(flows) {
  const cols = '50px 1fr 110px 120px 180px';
  const rows = flows.map(f => {
    const devLabel = f.device_id ? `#${f.device_id}` : '—';
    return `
      <div class="trow" style="grid-template-columns:${cols};">
        <div class="mono" style="font-size:11px;color:var(--text-3);">#${f.id}</div>
        <div style="font-size:13px;font-weight:500;">${escHtml(f.name)}</div>
        <div>${flowBadge(f.status)}</div>
        <div style="font-size:12px;color:var(--text-2);">${escHtml(devLabel)}</div>
        <div style="display:flex;gap:5px;justify-content:flex-end;">
          <button class="btn btn-ghost sm" data-fl-edit="${f.id}">Editar</button>
          ${f.status==='inactive'
            ? `<button class="btn btn-soft sm" data-fl-act="${f.id}">Ativar</button>`
            : `<button class="btn btn-ghost sm" data-fl-deact="${f.id}">Pausar</button>`}
          <button class="icon-btn sm" data-fl-del="${f.id}" title="Excluir">${ICON.trash}</button>
        </div>
      </div>`;
  }).join('');

  setView(`
    <div class="screen" style="max-width:900px;">
      <div class="toolbar" style="margin-bottom:14px;">
        <div style="font-size:15px;font-weight:600;">Fluxos de automação</div>
        <div class="spacer"></div>
        <button class="btn btn-accent" id="fl-new-btn">${ICON.plus} Novo fluxo</button>
      </div>
      <div class="card flush">
        <div class="thead" style="grid-template-columns:${cols};">
          <div>#</div><div>Nome</div><div>Status</div><div>Dispositivo</div><div></div>
        </div>
        ${flows.length ? rows : `<div style="padding:18px;">${emptyState(flIcon(36),'Nenhum fluxo criado','Crie seu primeiro fluxo de automação pós-reconhecimento facial.')}</div>`}
      </div>
    </div>`);

  $('fl-new-btn')?.addEventListener('click', flCreateNew);
  document.querySelectorAll('[data-fl-edit]').forEach(b => b.addEventListener('click', e => { e.stopPropagation(); navigate(`flows-edit?id=${b.dataset.flEdit}`); }));
  document.querySelectorAll('[data-fl-act]').forEach(b => b.addEventListener('click', async e => {
    e.stopPropagation(); b.disabled = true;
    try {
      const r = await apiPut(`flows/${b.dataset.flAct}/activate`, undefined);
      if (r.status === 422) { const d = await r.json(); showToast('error',`Validação: ${(d.errors||[]).map(e=>e.message).join('; ')}`); }
      else if (r.ok) { showToast('success','Fluxo ativado.'); mountFlowsList(); }
      else showToast('error',`Erro ${r.status}`);
    } catch(err) { netError(err); } finally { b.disabled = false; }
  }));
  document.querySelectorAll('[data-fl-deact]').forEach(b => b.addEventListener('click', async e => {
    e.stopPropagation(); b.disabled = true;
    try {
      const r = await apiPut(`flows/${b.dataset.flDeact}/deactivate`, undefined);
      if (r.ok) { showToast('success','Fluxo pausado.'); mountFlowsList(); }
      else showToast('error',`Erro ${r.status}`);
    } catch(err) { netError(err); } finally { b.disabled = false; }
  }));
  document.querySelectorAll('[data-fl-del]').forEach(b => b.addEventListener('click', async e => {
    e.stopPropagation();
    if (!confirm('Excluir este fluxo? Esta ação não pode ser desfeita.')) return;
    b.disabled = true;
    try {
      const r = await apiDelete(`flows/${b.dataset.flDel}`);
      if (r.ok || r.status === 204) { showToast('success','Fluxo excluído.'); mountFlowsList(); }
      else showToast('error',`Erro ${r.status}`);
    } catch(err) { netError(err); } finally { b.disabled = false; }
  }));
}

async function flCreateNew() {
  const name = prompt('Nome do novo fluxo:');
  if (!name?.trim()) return;
  try {
    const res = await apiPost('flows', { name: name.trim() });
    if (res.status === 201 || res.ok) {
      const flow = await res.json();
      navigate(`flows-edit?id=${flow.id}`);
    } else {
      const d = await res.json().catch(() => ({}));
      showToast('error', d.error || `Erro ${res.status}`);
    }
  } catch(err) { netError(err); }
}

// ─── 5.2 + 5.3 + 5.4 + 5.5 Editor de fluxo ──────────────────────────────────

async function mountFlowsEditor(params) {
  cleanupEditor();
  const id = params.id;
  if (!id) { navigate('flows'); return; }
  setView(loadingState());
  try {
    const [fRes, imgRes] = await Promise.all([apiGet(`flows/${id}`), apiGet('background-images')]);
    if (fRes.status === 401) return;
    if (!fRes.ok) { setView(emptyState(flIcon(32),'Fluxo não encontrado',`Status ${fRes.status}.`)); return; }
    const flow = await fRes.json();
    ES = {
      id: flow.id, name: flow.name, status: flow.status,
      nodes: (flow.nodes || []).map(n => ({
        id: n.id, type: n.type,
        config: (n.config && typeof n.config === 'object') ? n.config : {},
        x: typeof n.x === 'number' ? n.x : 80,
        y: typeof n.y === 'number' ? n.y : 80,
      })),
      edges: (flow.edges || []).map(e => ({ from: e.from, to: e.to, label: e.label || '' })),
      selectedId: null, palette_drag: null, node_drag: null, connecting: null,
      bgImages: imgRes.ok ? (await imgRes.json()).images || [] : [],
      valErrors: [], dirty: false,
    };
    renderEditor();
  } catch(err) {
    setView(emptyState(flIcon(32),'Falha ao carregar editor','Tente novamente.'));
    netError(err);
  }
}

function renderEditor() {
  if (!ES) return;
  const valBanner = ES.valErrors.length ? `
    <div class="val-errors" style="margin-bottom:12px;">
      <div class="ve-title">Erros de validação</div>
      ${ES.valErrors.map(e => `<div class="ve-item">· ${escHtml(e.message)}${e.node_id ? ` <span class="mono" style="font-size:10px;color:var(--text-3);">(${escHtml(e.node_id)})</span>` : ''}</div>`).join('')}
    </div>` : '';

  setView(`
    ${valBanner}
    <div class="ed-shell" id="ed-shell">
      <div class="ed-palette" id="ed-palette">
        <div class="pal-section">Tipos de nó</div>
        ${Object.entries(ND).map(([type, def]) => `
          <div class="pal-item${def.blocked ? ' pal-blocked' : ''}" data-pal="${type}"
               title="${def.blocked ? `Aguardando contrato ${def.blocked} — pode posicionar e conectar` : def.label}">
            <span class="pal-dot" style="background:var(${def.cv});"></span>
            <span class="pal-label">${escHtml(def.label)}</span>
            ${def.blocked ? `<span class="pal-badge">${def.blocked}</span>` : ''}
          </div>`).join('')}
        <div class="pal-section" style="margin-top:8px;">Variáveis</div>
        <div style="padding:2px 7px 8px;font-size:10.5px;color:var(--text-3);line-height:1.9;">
          ${FLOW_VARS.map(v => `<span class="var-chip" data-palvar="${escHtml(v)}">${escHtml(v)}</span>`).join(' ')}
        </div>
      </div>
      <div class="ed-middle">
        <div class="ed-bar">
          <button class="btn-back" id="ed-back-btn">${ICON.back} Fluxos</button>
          <span class="ed-name">${escHtml(ES.name)}</span>
          ${flowBadge(ES.status)}
          <div class="spacer"></div>
          <button class="btn btn-ghost sm" id="ed-images-btn">Imagens de fundo</button>
          <button class="btn btn-soft sm" id="ed-save-btn">Salvar</button>
          <button class="btn btn-accent sm" id="ed-pub-btn">Publicar (Ativar)</button>
        </div>
        <div class="ed-canvas-wrap" id="ed-canvas-wrap">
          <div class="ed-canvas-inner" id="ed-canvas-inner"
               style="position:relative;width:${FLOW_CANVAS_W}px;height:${FLOW_CANVAS_H}px;">
            <svg id="ed-svg" style="position:absolute;inset:0;width:100%;height:100%;pointer-events:none;overflow:visible;">
              <defs>
                <marker id="arr" markerWidth="7" markerHeight="5" refX="6" refY="2.5" orient="auto">
                  <polygon points="0 0,7 2.5,0 5" fill="var(--text-3)"/>
                </marker>
                <marker id="arr-v" markerWidth="7" markerHeight="5" refX="6" refY="2.5" orient="auto">
                  <polygon points="0 0,7 2.5,0 5" fill="var(--green)"/>
                </marker>
                <marker id="arr-i" markerWidth="7" markerHeight="5" refX="6" refY="2.5" orient="auto">
                  <polygon points="0 0,7 2.5,0 5" fill="var(--red)"/>
                </marker>
              </defs>
              <g id="ed-edges-g"></g>
              <path id="ed-conn" stroke="var(--accent)" stroke-width="1.5" fill="none" stroke-dasharray="5,4" style="display:none;"/>
            </svg>
            <div id="ed-nodes-g" style="position:absolute;inset:0;"></div>
          </div>
        </div>
      </div>
      <div class="ed-panel">
        <div class="ep-head">Configuração do nó</div>
        <div class="ep-body" id="ep-body">${renderCfgPanel()}</div>
      </div>
    </div>`);

  renderNodes();
  renderEdges();
  wireEditorShell();
}

// ─── Canvas rendering ─────────────────────────────────────────────────────────

function portOutY(def, label) {
  if (def.out === 1) return def.h / 2;
  return label === 'valid' ? def.h * 0.28 : def.h * 0.72;
}

function renderNodes() {
  const layer = $('ed-nodes-g'); if (!layer || !ES) return;
  layer.innerHTML = ES.nodes.map(n => {
    const def = ND[n.type] || ND.start;
    const h = def.h;
    const sel = ES.selectedId === n.id;
    const hasErr = ES.valErrors.some(e => e.node_id === n.id);
    const summary = nodeSummary(n);

    const inPort = def.inputs > 0
      ? `<div class="ed-port ed-port-in" style="top:${h/2-FLOW_PORT_R}px;" data-ptype="in" data-nid="${n.id}"></div>` : '';

    let outPorts = '';
    if (def.out === 1) {
      outPorts = `<div class="ed-port ed-port-out" style="top:${h/2-FLOW_PORT_R}px;" data-ptype="out" data-nid="${n.id}" data-plbl="" title="Saída"></div>`;
    } else {
      outPorts = `
        <div class="ed-port ed-port-out" style="top:${h*0.28-FLOW_PORT_R}px;" data-ptype="out" data-nid="${n.id}" data-plbl="valid" title="Saída: válido"></div>
        <div class="ed-port ed-port-out" style="top:${h*0.72-FLOW_PORT_R}px;" data-ptype="out" data-nid="${n.id}" data-plbl="invalid" title="Saída: inválido"></div>
        <div class="ed-port-lbl" style="right:-50px;top:${h*0.28-6}px;color:var(--green);">válido</div>
        <div class="ed-port-lbl" style="right:-54px;top:${h*0.72-6}px;color:var(--red);">inválido</div>`;
    }

    return `<div class="ed-node${sel?' ed-node-sel':''}${hasErr?' ed-node-err':''}${def.blocked?' ed-node-blocked':''}"
         style="left:${n.x}px;top:${n.y}px;height:${h}px;"
         data-nid="${n.id}">
      <div class="ed-node-head" style="border-left:3px solid var(${def.cv});">
        <span class="pal-dot" style="background:var(${def.cv});flex:none;"></span>
        <span class="ed-node-label">${escHtml(def.label)}</span>
        ${def.blocked ? `<span class="pal-badge" style="margin-left:auto;">${def.blocked}</span>` : ''}
        <button class="ed-node-del" data-ndel="${n.id}" title="Remover nó">×</button>
      </div>
      ${summary ? `<div class="ed-node-sub">${escHtml(summary)}</div>` : ''}
      ${inPort}${outPorts}
    </div>`;
  }).join('');

  wireNodeHandlers();
}

function renderEdges() {
  const g = $('ed-edges-g'); if (!g || !ES) return;
  g.innerHTML = ES.edges.map(e => {
    const fn = ES.nodes.find(n => n.id === e.from);
    const tn = ES.nodes.find(n => n.id === e.to);
    if (!fn || !tn) return '';
    const fd = ND[fn.type] || ND.start;
    const td = ND[tn.type] || ND.start;
    const sy = fn.y + portOutY(fd, e.label);
    const sx = fn.x + FLOW_NODE_W;
    const ty = tn.y + td.h / 2;
    const tx = tn.x;
    const cx = Math.max(40, Math.abs(tx - sx) * 0.5);
    const d = `M${sx},${sy} C${sx+cx},${sy} ${tx-cx},${ty} ${tx},${ty}`;
    let clr = 'var(--text-3)', mEnd = 'url(#arr)';
    if (e.label === 'valid') { clr = 'var(--green)'; mEnd = 'url(#arr-v)'; }
    if (e.label === 'invalid') { clr = 'var(--red)'; mEnd = 'url(#arr-i)'; }
    const lx = (sx + tx) / 2, ly = (sy + ty) / 2 - 10;
    const lbl = e.label ? `<text x="${lx}" y="${ly}" fill="${clr}" font-size="10" text-anchor="middle" font-family="Inter,sans-serif">${e.label}</text>` : '';
    return `<g class="ed-edge" data-efrom="${e.from}" data-eto="${e.to}" data-elbl="${e.label}">
      <path d="${d}" stroke="${clr}" stroke-width="1.8" fill="none" marker-end="${mEnd}" style="pointer-events:stroke;"/>
      <path d="${d}" stroke="transparent" stroke-width="10" fill="none" style="pointer-events:stroke;cursor:pointer;" title="Clique para remover aresta"/>
      ${lbl}
    </g>`;
  }).join('');

  // Click-to-remove edge
  setTimeout(() => {
    const eg = $('ed-edges-g'); if (!eg) return;
    eg.querySelectorAll('.ed-edge').forEach(el => {
      el.addEventListener('click', () => {
        if (!ES) return;
        ES.edges = ES.edges.filter(e => !(e.from===el.dataset.efrom && e.to===el.dataset.eto && e.label===el.dataset.elbl));
        ES.dirty = true;
        renderEdges();
      });
    });
  }, 0);
}

function wireNodeHandlers() {
  const layer = $('ed-nodes-g'); if (!layer) return;
  layer.querySelectorAll('[data-ndel]').forEach(btn => {
    btn.addEventListener('mousedown', e => e.stopPropagation());
    btn.addEventListener('click', e => { e.stopPropagation(); removeEdNode(btn.dataset.ndel); });
  });
  layer.querySelectorAll('.ed-node').forEach(el => {
    el.addEventListener('mousedown', e => {
      if (e.target.closest('[data-ptype]') || e.target.closest('[data-ndel]')) return;
      const id = el.dataset.nid;
      const node = ES?.nodes.find(n => n.id === id); if (!node) return;
      selectEdNode(id);
      ES.node_drag = { id, sx: e.clientX, sy: e.clientY, nx: node.x, ny: node.y };
      e.preventDefault();
    });
  });
  layer.querySelectorAll('[data-ptype="out"]').forEach(port => {
    port.addEventListener('mousedown', e => {
      e.stopPropagation(); e.preventDefault();
      if (ES) ES.connecting = { fromId: port.dataset.nid, portLabel: port.dataset.plbl || '' };
      redrawConnPath(e);
    });
  });
}

function removeEdNode(id) {
  if (!ES) return;
  ES.nodes = ES.nodes.filter(n => n.id !== id);
  ES.edges = ES.edges.filter(e => e.from !== id && e.to !== id);
  if (ES.selectedId === id) { ES.selectedId = null; refreshCfg(); }
  ES.dirty = true;
  renderNodes(); renderEdges();
}

function selectEdNode(id) {
  if (!ES) return;
  ES.selectedId = id;
  refreshCfg();
  $('ed-nodes-g')?.querySelectorAll('.ed-node').forEach(el => el.classList.toggle('ed-node-sel', el.dataset.nid === id));
}

function refreshCfg() {
  const body = $('ep-body'); if (!body) return;
  body.innerHTML = renderCfgPanel();
  wireCfg();
}

// ─── Config panel per node type (5.3) ────────────────────────────────────────

function renderCfgPanel() {
  if (!ES?.selectedId) return `<div class="ep-empty">${flIcon(26)}<div>Clique num nó no canvas para configurá-lo</div></div>`;
  const node = ES.nodes.find(n => n.id === ES.selectedId);
  if (!node) { ES.selectedId = null; return renderCfgPanel(); }
  const def = ND[node.type] || ND.start;
  const cfg = (node.config && typeof node.config === 'object') ? node.config : {};
  let fields = '';

  if (def.blocked === 'ISAPI') {
    fields = `<div class="pending-note" style="margin-top:8px;">${ICON.warnTri}<span><strong>BLOCKED_ISAPI</strong> — Execução aguarda contrato ISAPI verificado. Nó pode ser posicionado e conectado no canvas.</span></div>`;
  } else if (def.blocked === 'API') {
    fields = `
      <div class="pal-section">Mensagem</div>
      <label class="label" for="cfg-msg">Template da mensagem</label>
      <textarea class="input" id="cfg-msg" rows="4" placeholder="Olá [user.name], presença registrada em [event.datetime].">${escHtml(cfg.message_template||'')}</textarea>
      ${varHintHtml('cfg-msg')}
      <div class="pending-note" style="margin-top:10px;">${ICON.warnTri}<span><strong>BLOCKED_API</strong> — Envio aguarda contrato de API. Template pode ser configurado; o envio não ocorrerá até o contrato ser fornecido.</span></div>`;
  } else if (node.type === 'wait') {
    fields = `
      <div class="pal-section">Espera</div>
      <label class="label" for="cfg-secs">Duração (segundos)</label>
      <input class="input" id="cfg-secs" type="number" min="1" max="3600" value="${escHtml(String(cfg.duration_seconds||5))}" />
      <div style="font-size:11px;color:var(--text-3);margin-top:4px;">Entre 1 e 3600 segundos.</div>`;
  } else if (node.type === 'change_background') {
    const opts = (ES.bgImages||[]).map(img =>
      `<option value="${img.id}"${String(cfg.image_id)===String(img.id)?' selected':''}>${escHtml(img.name||img.file_path||String(img.id))}</option>`
    ).join('');
    fields = `
      <div class="pal-section">Imagem de fundo</div>
      <label class="label" for="cfg-img">Imagem</label>
      <select class="select" id="cfg-img">
        <option value="">— Selecione —</option>
        ${opts}
      </select>
      ${!ES.bgImages.length ? `<div style="font-size:11px;color:var(--text-3);margin-top:4px;">Nenhuma imagem cadastrada. Clique em "Imagens de fundo" para fazer upload.</div>` : ''}`;
  } else if (node.type === 'https_call') {
    const headers = (cfg.headers && typeof cfg.headers === 'object') ? cfg.headers : {};
    const hdrPairs = Object.entries(headers);
    const hdrRows = hdrPairs.map(([k,v],i) => {
      const isSealed = v === '__sealed__' || String(v).startsWith('__secret__:');
      return `<div class="hdr-row" data-hdr-i="${i}">
        <input class="input" placeholder="Chave" value="${escHtml(k)}" data-hk="${i}" />
        <div style="position:relative;">
          <input class="input" type="${isSealed?'password':'text'}" placeholder="${isSealed?'(secreto — digite novo valor para alterar)':'Valor'}"
            value="${isSealed?'':escHtml(v)}" data-hv="${i}" style="padding-right:76px;" />
          <label style="position:absolute;right:7px;top:50%;transform:translateY(-50%);display:flex;align-items:center;gap:3px;font-size:10px;color:var(--text-3);cursor:pointer;white-space:nowrap;">
            <input type="checkbox" data-hsec="${i}" ${isSealed?'checked':''} style="margin:0;" /> secreto
          </label>
        </div>
        <button class="icon-btn sm" data-hdel="${i}" title="Remover">${ICON.trash}</button>
      </div>`;
    }).join('');
    fields = `
      <div class="pal-section">URL e método</div>
      <label class="label" for="cfg-url">URL</label>
      <input class="input" id="cfg-url" placeholder="https://..." value="${escHtml(cfg.url||'')}" />
      <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;margin-top:8px;">
        <div><label class="label" for="cfg-mth">Método</label>
          <select class="select" id="cfg-mth">${['GET','POST','PUT','PATCH'].map(m=>`<option${(cfg.method||'POST')===m?' selected':''}>${m}</option>`).join('')}</select></div>
        <div><label class="label" for="cfg-to">Timeout (s)</label>
          <input class="input" id="cfg-to" type="number" min="1" max="300" value="${escHtml(String(cfg.timeout_seconds||30))}" /></div>
      </div>
      <div class="pal-section" style="margin-top:10px;">Headers</div>
      <div id="hdr-list" style="display:flex;flex-direction:column;gap:6px;">${hdrRows}</div>
      <button class="btn btn-ghost sm" id="add-hdr-btn" style="margin-top:6px;">${ICON.plus} Header</button>
      <div class="pal-section" style="margin-top:10px;">Body</div>
      <textarea class="input mono" id="cfg-body" rows="4" placeholder='{"key":"[user.name]"}'>${escHtml(cfg.body||'')}</textarea>
      ${varHintHtml('cfg-body')}`;
  } else if (node.type === 'qrcode_background') {
    fields = `
      <div class="pal-section">QR Code</div>
      <label class="label" for="cfg-qr">Conteúdo (template)</label>
      <textarea class="input" id="cfg-qr" rows="4" placeholder="CPF: [user.document]">${escHtml(cfg.content_template||'')}</textarea>
      ${varHintHtml('cfg-qr')}`;
  } else {
    // start, decision — sem config adicional
    fields = `<div style="font-size:12px;color:var(--text-3);margin-top:10px;line-height:1.6;">Este nó não possui configuração adicional.</div>`;
  }

  return `
    <div style="display:flex;align-items:center;gap:7px;margin-bottom:12px;">
      <span style="width:10px;height:10px;border-radius:50%;background:var(${def.cv});flex:none;display:inline-block;"></span>
      <span style="font-size:13px;font-weight:600;">${escHtml(def.label)}</span>
      <span class="mono" style="font-size:10px;color:var(--text-3);margin-left:auto;">${escHtml(node.id)}</span>
    </div>
    ${fields}
    <div style="display:flex;gap:8px;margin-top:14px;">
      <button class="btn btn-soft sm" id="cfg-apply-btn">Aplicar</button>
      <button class="btn btn-ghost sm" id="cfg-del-btn" style="margin-left:auto;color:var(--red);">${ICON.trash} Remover</button>
    </div>`;
}

function varHintHtml(targetId) {
  return `<div class="var-hint">Variáveis disponíveis (clique para inserir):<br>${FLOW_VARS.map(v=>`<span class="var-chip" data-v="${escHtml(v)}" data-vtgt="${targetId}">${escHtml(v)}</span>`).join('')}</div>`;
}

function nodeSummary(node) {
  const cfg = node.config || {};
  switch(node.type) {
    case 'wait': return cfg.duration_seconds ? `${cfg.duration_seconds}s` : '';
    case 'change_background': return cfg.image_id ? `img #${cfg.image_id}` : '';
    case 'https_call': return cfg.url ? (cfg.url.length>24?cfg.url.slice(0,24)+'…':cfg.url) : '';
    case 'qrcode_background': return cfg.content_template ? cfg.content_template.slice(0,22)+'…' : '';
    case 'send_message': return cfg.message_template ? cfg.message_template.slice(0,22)+'…' : '';
    default: return '';
  }
}

function wireCfg() {
  if (!ES?.selectedId) return;
  const node = ES.nodes.find(n => n.id === ES.selectedId); if (!node) return;

  $('cfg-apply-btn')?.addEventListener('click', () => applyCfg(node));
  $('cfg-del-btn')?.addEventListener('click', () => removeEdNode(node.id));

  // Variable chips — insert into target textarea/input
  document.querySelectorAll('[data-v][data-vtgt]').forEach(chip => {
    chip.addEventListener('click', () => {
      const el = $(chip.dataset.vtgt); if (!el) return;
      const v = chip.dataset.v;
      const s = el.selectionStart ?? el.value.length;
      const e2 = el.selectionEnd ?? s;
      el.value = el.value.slice(0,s) + v + el.value.slice(e2);
      el.selectionStart = el.selectionEnd = s + v.length;
      el.focus();
    });
  });

  $('add-hdr-btn')?.addEventListener('click', () => {
    const hdrs = { ...((node.config||{}).headers||{}), '': '' };
    node.config = { ...(node.config||{}), headers: hdrs };
    refreshCfg();
  });

  document.querySelectorAll('[data-hdel]').forEach(btn => {
    btn.addEventListener('click', () => {
      const i = parseInt(btn.dataset.hdel);
      const hdrs = { ...((node.config||{}).headers||{}) };
      const keys = Object.keys(hdrs);
      if (keys[i] !== undefined) delete hdrs[keys[i]];
      node.config = { ...(node.config||{}), headers: hdrs };
      refreshCfg();
    });
  });
}

function applyCfg(node) {
  let cfg = {};
  switch(node.type) {
    case 'wait': {
      const v = parseInt($('cfg-secs')?.value||'5');
      cfg = { duration_seconds: Math.min(3600, Math.max(1, isNaN(v)?5:v)) };
      break;
    }
    case 'change_background': {
      const v = $('cfg-img')?.value;
      if (v) cfg = { image_id: parseInt(v) };
      break;
    }
    case 'https_call': {
      const url = $('cfg-url')?.value?.trim()||'';
      const method = $('cfg-mth')?.value||'POST';
      const timeout = parseInt($('cfg-to')?.value||'30')||30;
      const body = $('cfg-body')?.value||'';
      const hdrs = {};
      const hdrList = $('hdr-list');
      if (hdrList) {
        hdrList.querySelectorAll('.hdr-row').forEach((_,i) => {
          const k = hdrList.querySelector(`[data-hk="${i}"]`)?.value?.trim();
          const vEl = hdrList.querySelector(`[data-hv="${i}"]`);
          const secEl = hdrList.querySelector(`[data-hsec="${i}"]`);
          if (!k) return;
          const v = vEl?.value?.trim()||'';
          const isSecret = secEl?.checked;
          if (isSecret && v) hdrs[k] = `__secret__:${v}`;
          else if (isSecret && !v) {
            // Keep existing sealed value for this key
            const existing = ((node.config||{}).headers||{})[k];
            hdrs[k] = existing||'';
          } else hdrs[k] = v;
        });
      }
      cfg = { url, method, timeout_seconds: Math.min(300, Math.max(1,timeout)), headers: hdrs, body };
      break;
    }
    case 'qrcode_background':
      cfg = { content_template: $('cfg-qr')?.value||'' }; break;
    case 'send_message':
      cfg = { message_template: $('cfg-msg')?.value||'' }; break;
  }
  node.config = cfg;
  ES.dirty = true;
  // Update summary in the node card without full re-render
  const subEl = document.querySelector(`[data-nid="${node.id}"] .ed-node-sub`);
  const sum = nodeSummary(node);
  if (subEl) subEl.textContent = sum;
  else if (sum) {
    const head = document.querySelector(`[data-nid="${node.id}"] .ed-node-head`);
    if (head) head.insertAdjacentHTML('afterend', `<div class="ed-node-sub">${escHtml(sum)}</div>`);
  }
  showToast('success','Configuração aplicada.');
  refreshCfg();
}

// ─── Editor shell wiring ──────────────────────────────────────────────────────

function wireEditorShell() {
  $('ed-back-btn')?.addEventListener('click', () => navigate('flows'));
  $('ed-save-btn')?.addEventListener('click', () => saveFlow(false));
  $('ed-pub-btn')?.addEventListener('click', () => saveFlow(true));
  $('ed-images-btn')?.addEventListener('click', openBgModal);

  // Palette drag
  document.querySelectorAll('[data-pal]').forEach(item => {
    item.addEventListener('mousedown', e => { e.preventDefault(); startPalDrag(item.dataset.pal, e); });
  });

  // Palette var chips → copy to clipboard
  document.querySelectorAll('[data-palvar]').forEach(chip => {
    chip.addEventListener('click', () => {
      navigator.clipboard?.writeText(chip.dataset.palvar).catch(()=>{});
      showToast('success', `Copiado: ${chip.dataset.palvar}`);
    });
  });

  // Document-level mouse/key handlers
  const mm = onEdMM, mu = onEdMU, kd = onEdKD;
  document.addEventListener('mousemove', mm);
  document.addEventListener('mouseup', mu);
  document.addEventListener('keydown', kd);
  _edDocHandlers = { mm, mu, kd };

  wireCfg();
}

function cleanupEditor() {
  if (ES?.palette_drag?.ghostEl) ES.palette_drag.ghostEl.remove();
  if (_edDocHandlers) {
    document.removeEventListener('mousemove', _edDocHandlers.mm);
    document.removeEventListener('mouseup', _edDocHandlers.mu);
    document.removeEventListener('keydown', _edDocHandlers.kd);
    _edDocHandlers = null;
  }
  ES = null;
}

function startPalDrag(type, e) {
  const def = ND[type]; if (!def || !ES) return;
  const ghost = document.createElement('div');
  ghost.className = 'ed-node ed-ghost';
  ghost.style.cssText = `width:${FLOW_NODE_W}px;height:${def.h}px;left:${e.clientX-FLOW_NODE_W/2}px;top:${e.clientY-def.h/2}px;`;
  ghost.innerHTML = `<div class="ed-node-head" style="border-left:3px solid var(${def.cv});"><span class="pal-dot" style="background:var(${def.cv});flex:none;"></span><span class="ed-node-label">${escHtml(def.label)}</span></div>`;
  document.body.appendChild(ghost);
  ES.palette_drag = { type, ghostEl: ghost };
}

function redrawConnPath(e) {
  const path = $('ed-conn'); if (!path || !ES?.connecting) return;
  const wrap = $('ed-canvas-wrap'); if (!wrap) return;
  const rect = wrap.getBoundingClientRect();
  const mx = e.clientX - rect.left + wrap.scrollLeft;
  const my = e.clientY - rect.top + wrap.scrollTop;
  const fn = ES.nodes.find(n => n.id === ES.connecting.fromId); if (!fn) return;
  const fd = ND[fn.type]||ND.start;
  const sy = fn.y + portOutY(fd, ES.connecting.portLabel);
  const sx = fn.x + FLOW_NODE_W;
  const cx = Math.max(30, Math.abs(mx-sx)*0.5);
  path.setAttribute('d',`M${sx},${sy} C${sx+cx},${sy} ${mx-cx},${my} ${mx},${my}`);
  path.style.display='';
}

function onEdMM(e) {
  if (!ES || !$('ed-shell')) return;
  if (ES.palette_drag?.ghostEl) {
    ES.palette_drag.ghostEl.style.left=(e.clientX-FLOW_NODE_W/2)+'px';
    ES.palette_drag.ghostEl.style.top=(e.clientY-20)+'px';
  }
  if (ES.node_drag) {
    const {id,sx,sy,nx,ny} = ES.node_drag;
    const node = ES.nodes.find(n=>n.id===id); if (!node) return;
    node.x = Math.max(0, nx + e.clientX - sx);
    node.y = Math.max(0, ny + e.clientY - sy);
    const el = document.querySelector(`[data-nid="${id}"]`);
    if (el) { el.style.left=node.x+'px'; el.style.top=node.y+'px'; }
    renderEdges();
  }
  if (ES.connecting) redrawConnPath(e);
}

function onEdMU(e) {
  if (!ES || !$('ed-shell')) return;

  if (ES.palette_drag) {
    const {type, ghostEl} = ES.palette_drag;
    if (ghostEl) ghostEl.remove();
    ES.palette_drag = null;
    const wrap = $('ed-canvas-wrap');
    if (wrap) {
      const rect = wrap.getBoundingClientRect();
      if (e.clientX>=rect.left&&e.clientX<=rect.right&&e.clientY>=rect.top&&e.clientY<=rect.bottom) {
        const x = Math.max(10, e.clientX - rect.left + wrap.scrollLeft - FLOW_NODE_W/2);
        const y = Math.max(10, e.clientY - rect.top + wrap.scrollTop - (ND[type]||ND.start).h/2);
        addCanvasNode(type, x, y);
      }
    }
    return;
  }

  if (ES.node_drag) { ES.node_drag = null; ES.dirty = true; }

  if (ES.connecting) {
    const cp = $('ed-conn'); if (cp) cp.style.display='none';
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const inPort = el?.closest('[data-ptype="in"]');
    if (inPort) {
      const toId = inPort.dataset.nid, fromId = ES.connecting.fromId, label = ES.connecting.portLabel||'';
      if (fromId !== toId) {
        const exists = ES.edges.some(e2 => e2.from===fromId && e2.to===toId && e2.label===label);
        if (!exists) { ES.edges.push({from:fromId,to:toId,label}); ES.dirty=true; renderEdges(); }
      }
    }
    ES.connecting = null;
  }
}

function onEdKD(e) {
  if (!ES || !$('ed-shell')) return;
  if ((e.key==='Delete'||e.key==='Backspace') && ES.selectedId) {
    const tag = document.activeElement?.tagName?.toLowerCase();
    if (tag==='input'||tag==='textarea'||tag==='select') return;
    removeEdNode(ES.selectedId);
  }
}

function addCanvasNode(type, x, y) {
  const def = ND[type]; if (!def || !ES) return;
  if (type==='start' && ES.nodes.some(n=>n.type==='start')) { showToast('error','Só pode existir um nó de início.'); return; }
  const defCfg = type==='wait' ? {duration_seconds:5} : type==='https_call' ? {url:'',method:'POST',headers:{},body:'',timeout_seconds:30} : {};
  const node = { id: genNodeId(), type, config: defCfg, x, y };
  ES.nodes.push(node);
  ES.dirty = true;
  renderNodes();
  selectEdNode(node.id);
}

// ─── 5.4 Salvar / Publicar ───────────────────────────────────────────────────

async function saveFlow(andActivate) {
  if (!ES) return;
  const saveBtn=$('ed-save-btn'), pubBtn=$('ed-pub-btn');
  if (saveBtn) saveBtn.disabled=true;
  if (pubBtn) pubBtn.disabled=true;
  try {
    const payload = {
      name: ES.name,
      nodes: ES.nodes.map(n => ({ id:n.id, type:n.type, config:n.config||{}, x:n.x, y:n.y })),
      edges: ES.edges,
    };
    const r = await apiPut(`flows/${ES.id}`, payload);
    if (r.status === 422) {
      const d = await r.json();
      ES.valErrors = d.errors || [];
      renderEditor();
      showToast('error','Erros de validação. Corrija os nós destacados em vermelho.');
      return;
    }
    if (!r.ok) {
      const d = await r.json().catch(() => ({}));
      showToast('error', d.error || `Salvar falhou (${r.status}).`);
      return;
    }
    ES.valErrors = []; ES.dirty = false;
    showToast('success','Fluxo salvo.');
    if (andActivate) {
      const ar = await apiPut(`flows/${ES.id}/activate`, undefined);
      if (ar.status === 422) {
        const d = await ar.json();
        ES.valErrors = d.errors || [];
        renderEditor();
        showToast('error','Erros de validação ao ativar. Corrija e tente novamente.');
        return;
      }
      if (!ar.ok) {
        const d = await ar.json().catch(() => ({}));
        showToast('error', d.error || `Ativar falhou (${ar.status}).`);
        return;
      }
      ES.status = 'active';
      showToast('success','Fluxo publicado e ativado!');
      renderEditor();
    }
  } catch(err) { netError(err); }
  finally {
    const s=$('ed-save-btn'), p=$('ed-pub-btn');
    if (s) s.disabled=false; if (p) p.disabled=false;
  }
}

// ─── 5.5 Imagens de fundo (modal) ────────────────────────────────────────────

function openBgModal() {
  const imgs = ES?.bgImages || [];
  const rows = imgs.map(img => `
    <div class="trow" style="grid-template-columns:1fr auto;align-items:center;">
      <div style="font-size:12.5px;">${escHtml(img.name||img.file_path||String(img.id))}</div>
      <button class="icon-btn sm" data-imgdel="${img.id}" title="Remover">${ICON.trash}</button>
    </div>`).join('');

  $('modal-root').innerHTML = `
    <div class="modal-overlay" id="modal-overlay" role="dialog" aria-modal="true" aria-label="Imagens de fundo">
      <div class="modal" id="modal-card" style="max-width:520px;">
        <div class="modal-body">
          <div class="modal-title">Imagens de fundo</div>
          <div class="modal-text">Imagens disponíveis para o nó "Trocar fundo". Formatos: JPEG e PNG. Máximo 5 MB.</div>
          <div style="margin-top:14px;">
            <label class="label">Enviar nova imagem</label>
            <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
              <input type="file" id="img-file" accept="image/jpeg,image/png" class="input" style="flex:1;min-width:0;" />
              <button class="btn btn-soft sm" id="img-send-btn">${ICON.upload} Enviar</button>
            </div>
          </div>
          <div style="margin-top:14px;">
            <div class="card flush" style="max-height:220px;overflow-y:auto;">
              ${imgs.length ? rows : `<div style="padding:14px;">${emptyState(flIcon(24),'Nenhuma imagem','Faça upload de uma imagem JPEG ou PNG.')}</div>`}
            </div>
          </div>
        </div>
        <div class="modal-foot">
          <button class="btn btn-ghost" id="bg-modal-close">Fechar</button>
        </div>
      </div>
    </div>`;

  $('modal-overlay').addEventListener('click', e => { if (e.target.id==='modal-overlay') closeBgModal(); });
  $('bg-modal-close').addEventListener('click', closeBgModal);

  $('img-send-btn')?.addEventListener('click', async () => {
    const fi = $('img-file'); const file = fi?.files?.[0];
    if (!file) { showToast('error','Selecione um arquivo.'); return; }
    if (file.size > 5*1024*1024) { showToast('error','Arquivo muito grande (máx 5 MB).'); return; }
    const btn = $('img-send-btn'); btn.disabled=true; btn.textContent='Enviando…';
    try {
      const fd = new FormData(); fd.append('file', file);
      const r = await apiFetch('/admin/api/background-images', { method:'POST', body:fd });
      if (r.status===201||r.ok) {
        const img = await r.json();
        if (ES) ES.bgImages.push(img);
        showToast('success','Imagem enviada.');
        openBgModal(); // re-render modal with updated list
      } else if (r.status===415) {
        showToast('error','Formato não suportado. Use JPEG ou PNG.');
      } else { showToast('error',`Upload falhou (${r.status}).`); }
    } catch(err) { netError(err); }
    finally { const b=$('img-send-btn'); if(b){b.disabled=false; b.innerHTML=`${ICON.upload} Enviar`;} }
  });

  document.querySelectorAll('[data-imgdel]').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.dataset.imgdel; btn.disabled=true;
      try {
        const r = await apiDelete(`background-images/${id}`);
        if (r.ok||r.status===204) {
          if (ES) ES.bgImages = ES.bgImages.filter(i => String(i.id)!==String(id));
          showToast('success','Imagem removida.'); openBgModal();
        } else { showToast('error',`Erro ${r.status}`); btn.disabled=false; }
      } catch(err) { netError(err); btn.disabled=false; }
    });
  });
}

function closeBgModal() {
  $('modal-root').innerHTML='';
  if (ES?.selectedId && ES.nodes.find(n=>n.id===ES.selectedId)?.type==='change_background') refreshCfg();
}

// ─── 5.6 Logs de execução ────────────────────────────────────────────────────

let _logsId=null, _logsOff=0;
const LOGS_LIM=20;

async function mountFlowsLogs(params) {
  cleanupEditor();
  const id = params.id; if (!id){navigate('flows');return;}
  _logsId=id; _logsOff=0;
  setView(loadingState());
  try {
    const r = await apiGet(`flows/${id}/logs?limit=${LOGS_LIM}&offset=0`);
    if (r.status===401) return;
    if (!r.ok){setView(emptyState(flIcon(28),'Erro ao carregar logs',`Status ${r.status}.`));return;}
    renderLogsView(id, await r.json());
  } catch(err){setView(emptyState(flIcon(28),'Falha de conexão','Tente novamente.'));netError(err);}
}

function logBadge(s) { return s==='completed'?badge('ok','Concluído'):badge('off','Circuit break'); }

function renderLogsView(flowId, data) {
  const logs = data.logs||[];
  const hasMore = logs.length >= LOGS_LIM;

  function logRow(l) {
    return `<div class="trow" style="grid-template-columns:1.4fr 1fr 100px 1fr;">
      <div>
        <div style="font-size:12.5px;font-weight:500;">${fmtDateTime(l.started_at)}</div>
        <div class="mono" style="font-size:10.5px;color:var(--text-3);">→ ${fmtDateTime(l.finished_at)}</div>
      </div>
      <div>${logBadge(l.status)}</div>
      <div class="mono" style="font-size:11px;color:var(--text-3);">dev #${l.device_id}</div>
      <div style="font-size:11.5px;">
        ${l.failed_node_id?`<div>nó: <span class="mono">${escHtml(l.failed_node_id)}</span></div>`:''}
        ${l.error?`<div class="mono" style="font-size:10px;color:var(--red);">${escHtml(l.error)}</div>`:''}
      </div>
    </div>`;
  }

  setView(`
    <div class="screen" style="max-width:860px;">
      <div class="toolbar" style="margin-bottom:14px;">
        <button class="btn-back" id="logs-back-btn">${ICON.back} Editor</button>
        <span style="font-size:15px;font-weight:600;margin-left:10px;">Logs de execução — fluxo #${flowId}</span>
      </div>
      <div class="card flush">
        <div class="thead" style="grid-template-columns:1.4fr 1fr 100px 1fr;">
          <div>Início / Término</div><div>Status</div><div>Dispositivo</div><div>Detalhes</div>
        </div>
        <div id="logs-rows">
          ${logs.length ? logs.map(logRow).join('') : `<div style="padding:18px;">${emptyState(flIcon(28),'Nenhuma execução registrada','O fluxo ainda não foi executado.')}</div>`}
        </div>
        ${hasMore?`<div class="table-foot" id="logs-foot"><button class="btn btn-soft sm" id="logs-more">Carregar mais</button></div>`:''}
      </div>
    </div>`);

  $('logs-back-btn')?.addEventListener('click', () => navigate(`flows-edit?id=${flowId}`));
  $('logs-more')?.addEventListener('click', async () => {
    _logsOff += LOGS_LIM;
    const btn=$('logs-more'); btn.disabled=true; btn.textContent='Carregando…';
    try {
      const r = await apiGet(`flows/${_logsId}/logs?limit=${LOGS_LIM}&offset=${_logsOff}`);
      if (!r.ok) return;
      const d = await r.json();
      const newLogs = d.logs||[];
      $('logs-rows').insertAdjacentHTML('beforeend', newLogs.map(logRow).join(''));
      if (newLogs.length<LOGS_LIM) { const f=$('logs-foot');if(f)f.style.display='none'; }
      else { btn.disabled=false; btn.textContent='Carregar mais'; }
    } catch(err) { netError(err); btn.disabled=false; btn.textContent='Carregar mais'; }
  });
}
