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
  { id:'overview', label:'Visão geral',          icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><line x1="12" y1="11" x2="12" y2="16"/><line x1="12" y1="8" x2="12" y2="8"/></svg>` },
  { id:'system',   label:'Sistema & manutenção',  icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><line x1="4" y1="8" x2="20" y2="8"/><circle cx="9" cy="8" r="2"/><line x1="4" y1="16" x2="20" y2="16"/><circle cx="15" cy="16" r="2"/></svg>` },
  { id:'auth',     label:'Autenticação',          icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><path d="M4 8V6a2 2 0 0 1 2-2h2M16 4h2a2 2 0 0 1 2 2v2M20 16v2a2 2 0 0 1-2 2h-2M8 20H6a2 2 0 0 1-2-2v-2"/><circle cx="12" cy="11" r="2"/><path d="M9 15.5a3.5 3.5 0 0 1 6 0"/></svg>` },
  { id:'doors',    label:'Portas & acesso',       icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="5" y="3" width="14" height="18" rx="1.5"/><circle cx="15" cy="12" r="1"/></svg>` },
  { id:'users',    label:'Usuários no device',    icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="9" cy="8" r="3.2"/><path d="M3.5 19a5.5 5.5 0 0 1 11 0"/><path d="M16 5.5a3 3 0 0 1 0 5.5"/></svg>` },
  { id:'cards',    label:'Cartões',               icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="5" width="18" height="14" rx="2"/><line x1="3" y1="10" x2="21" y2="10"/></svg>` },
  { id:'faces',    label:'Biblioteca de faces',   icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><circle cx="9.5" cy="11" r="1"/><circle cx="14.5" cy="11" r="1"/><path d="M9 15a3.5 3.5 0 0 0 6 0"/></svg>` },
  { id:'events',   label:'Eventos & webhooks',    icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M10 21a2 2 0 0 0 4 0"/></svg>` },
  { id:'media',    label:'Mídia',                 icon:`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9.5" r="1.5"/><path d="m4 18 5-5 4 4 3-3 4 4"/></svg>` },
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
const apiGet  = (path, t)        => apiFetch(`/admin/api/${path}`, { method: 'GET' }, t);
const apiPost = (path, body, t)  => apiFetch(`/admin/api/${path}`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
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
const ROUTES = ['dashboard','devices','device-config','members','member-profile','events','login'];
const TITLES = {
  dashboard:      ['Visão geral', 'Panorama da operação · presença, dispositivos e provisão'],
  devices:        ['Dispositivos', 'Terminais HikVision na rede local'],
  'device-config':['Configuração do dispositivo', 'Configuração completa do terminal'],
  members:        ['Membros', 'Membros sincronizados do GOB'],
  'member-profile':['Perfil do membro', 'Dados, provisão e histórico de acessos'],
  events:         ['Eventos', 'Log cronológico de reconhecimentos'],
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

  // active nav (device-config counts as devices; member-profile as members)
  const navKey = route === 'device-config' ? 'devices' : route === 'member-profile' ? 'members' : route;
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
  }
}

// ─── LOGIN ────────────────────────────────────────────────────
function renderLogin(params) {
  setView(`
    <div class="login-wrap">
      <div class="login-box">
        <div class="login-brand">
          <div class="mark"><svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1"><circle cx="12" cy="9" r="3.6"/><path d="M5.5 20a6.5 6.5 0 0 1 13 0"/></svg></div>
          <div style="text-align:center;">
            <div class="t1">Presença Facial</div>
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
    overview: () => cfgOverview(dev),
    system:   () => previewNote + cfgSystem(dev),
    auth:     () => previewNote + cfgAuth(),
    doors:    () => previewNote + cfgDoors(dev),
    users:    () => previewNote + cfgUsers(),
    cards:    () => previewNote + cfgCards(),
    faces:    () => previewNote + cfgFaces(),
    events:   () => previewNote + cfgEvents(),
    media:    () => previewNote + cfgMedia(),
  };
  el.innerHTML = `<div class="stack">${(renderers[section] || renderers.overview)()}</div>`;
  wireCfgActions(dev);
}

function cfgOverview(dev) {
  // Apenas IP/MAC/status/heartbeat vêm do contrato atual (deviceResponse). Demais campos
  // (modelo, série, firmware, uptime, capacidades) exigem o endpoint de telemetria (§7.1).
  const kv = (k, v, mono) => `<div class="kv"><div class="k">${k}</div><div class="v ${mono?'mono':''}">${v}</div></div>`;
  return `
    ${pendingNote('Telemetria completa do terminal (modelo, firmware, uptime, capacidades) será exibida quando o endpoint de status (§7.1) estiver disponível.')}
    <div class="card flush">
      <div class="card-head"><div class="h">Identificação & status</div></div>
      <div class="kv-grid">
        ${kv('Identificador', escHtml(dev.device_identifier), true)}
        ${kv('IP', escHtml(dev.ip_address || '—'), true)}
        ${kv('MAC', escHtml(dev.mac_address || '—'), true)}
        ${kv('Status', dev.status === 'active' ? 'Online' : 'Offline')}
        ${kv('Último heartbeat', escHtml(fmtDateTime(dev.last_heartbeat_at)), true)}
        ${kv('Webhook', dev.webhook_configured ? 'Configurado' : 'Ausente')}
        ${kv('Modelo', '—')}
        ${kv('Firmware', '—', true)}
        ${kv('Uptime', '—', true)}
      </div>
    </div>`;
}

function cfgSystem(dev) {
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Data & hora</div><span class="badge badge-warn">Crítico</span></div>
      <div style="padding:16px; display:flex; flex-direction:column; gap:14px;">
        <div class="row-between">
          <div><div style="font-size:12.5px; font-weight:500;">Sincronização NTP</div><div style="font-size:11px; color:var(--text-3);">Relógio errado quebra a validade dos usuários e o timestamp dos eventos.</div></div>
          <button class="switch on" aria-label="NTP" aria-pressed="true"><span></span></button>
        </div>
        <div class="grid-2">
          <div class="field"><label class="label">Servidor NTP</label><input class="input mono" placeholder="pool.ntp.br"></div>
          <div class="field"><label class="label">Fuso horário</label><input class="input mono" placeholder="America/Sao_Paulo"></div>
        </div>
        <div style="display:flex; gap:10px;"><button class="btn btn-accent" data-pending="Sincronizar relógio">Sincronizar relógio</button><button class="btn btn-ghost" data-pending="Ajuste manual">Ajuste manual</button></div>
      </div>
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Tela de inicialização</div></div>
      <div style="padding:16px;"><div class="dropzone">${ICON.upload}<div style="font-size:12.5px; color:var(--text-2); margin-top:6px;">Arraste a imagem de boot ou clique para enviar</div><div class="mono" style="font-size:10.5px; margin-top:4px;">JPG/PNG · máx 1024×600 · até 200KB</div></div></div>
    </div>
    <div class="danger-card">
      <div class="dh">${ICON.warnTri} Zona de perigo</div>
      <div style="padding:16px; display:flex; flex-direction:column; gap:12px;">
        <div class="row-between">
          <div><div style="font-size:12.5px; font-weight:500;">Reiniciar dispositivo</div><div style="font-size:11px; color:var(--text-3);">Indisponível por ~40s durante o reboot.</div></div>
          <button class="btn btn-warn-outline sm" data-modal="reboot">Reiniciar</button>
        </div>
        <div style="height:1px; background:var(--border);"></div>
        <div class="row-between">
          <div><div style="font-size:12.5px; font-weight:500;">Reset de fábrica</div><div style="font-size:11px; color:var(--text-3);">Irreversível — apaga usuários, faces, cartões e configurações.</div></div>
          <button class="btn btn-danger sm" data-modal="factory">Reset de fábrica</button>
        </div>
      </div>
    </div>`;
}

function cfgAuth() {
  const mode = (on, icon, label, sub) => `<div class="authmode ${on?'on':''}">${icon}<div><div style="font-size:13px; font-weight:${on?600:500};">${label}</div>${sub?`<div style="font-size:11px; color:var(--text-3);">${sub}</div>`:''}</div></div>`;
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Modo de autenticação</div></div>
      <div style="padding:16px; display:grid; grid-template-columns:1fr 1fr; gap:10px;">
        ${mode(true, '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--accent-text)" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><path d="M9 15a3.5 3.5 0 0 0 6 0"/><circle cx="9.5" cy="11" r="1"/><circle cx="14.5" cy="11" r="1"/></svg>', 'Face', 'recomendado')}
        ${mode(false, '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="3" y="5" width="18" height="14" rx="2"/><line x1="3" y1="10" x2="21" y2="10"/></svg>', 'Cartão', '')}
        ${mode(false, '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><rect x="4" y="4" width="16" height="16" rx="2"/><circle cx="8" cy="9" r="1"/><circle cx="12" cy="9" r="1"/><circle cx="16" cy="9" r="1"/></svg>', 'PIN', '')}
        ${mode(false, '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="9" cy="12" r="6"/><rect x="13" y="9" width="8" height="6" rx="1"/></svg>', 'Face + Cartão', '')}
      </div>
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Terminal & tela de espera</div></div>
      <div style="padding:16px; display:flex; flex-direction:column; gap:14px;">
        <div class="grid-2">
          <div class="field"><label class="label">Idioma do device</label><select class="select"><option>Português (BR)</option><option>English</option></select></div>
          <div class="field"><label class="label">Timeout de tela (s)</label><input class="input" placeholder="30"></div>
        </div>
        <div class="field"><label class="label">Mensagem de boas-vindas (standby)</label><input class="input" placeholder="Bem-vindo — aproxime o rosto"></div>
        <div class="dropzone" style="padding:18px;">Enviar mídia de espera (standby) · <span class="mono" style="font-size:10.5px;">JPG/PNG</span></div>
      </div>
    </div>`;
}

function cfgDoors(dev) {
  return `
    <div class="card pad">
      <div class="row-between">
        <div style="display:flex; align-items:center; gap:11px;">
          <div style="width:38px; height:38px; border-radius:10px; background:var(--green-soft); display:grid; place-items:center;"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--green)" stroke-width="2"><rect x="5" y="3" width="14" height="18" rx="1.5"/><circle cx="15" cy="12" r="1"/></svg></div>
          <div><div style="font-size:13.5px; font-weight:600;">Porta 1 · Principal</div><div style="font-size:11.5px; color:var(--text-3);">Estado em tempo real exige o endpoint de portas (§7.4).</div></div>
        </div>
        <div style="display:flex; gap:8px;">
          <button class="btn btn-warn-outline sm" data-modal="doorOpen">Destravar 5s</button>
          <button class="btn btn-ghost sm" data-pending="Manter travada">Manter travada</button>
        </div>
      </div>
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Configuração da porta</div></div>
      <div style="padding:16px;" class="grid-2">
        <div class="field"><label class="label">Delay de destravamento (s)</label><input class="input" placeholder="5"></div>
        <div class="field"><label class="label">Modo de alarme</label><select class="select"><option>Porta aberta demais</option><option>Arrombamento</option><option>Desativado</option></select></div>
      </div>
    </div>`;
}

function cfgUsers() {
  return `
    <div class="toolbar">
      <div class="searchbox" style="width:240px;">${ICON.search}<input placeholder="Buscar usuário…" aria-label="Buscar usuário no device"></div>
      <button class="btn btn-accent sm" data-pending="Novo usuário">${ICON.plus} Novo usuário</button>
      <button class="btn btn-ghost sm" data-pending="Captura ao vivo">${ICON.camera} Captura ao vivo</button>
    </div>
    <div class="card flush">${emptyState(ICON.members, 'Usuários no terminal', 'A lista de usuários cadastrados no device é carregada do endpoint §7.5, ainda não disponível neste build.')}</div>
    <div class="card" style="border-color:var(--red); padding:14px 16px;">
      <div class="row-between"><div><div style="font-size:12.5px; font-weight:500; color:var(--red);">Limpar todos os usuários</div><div style="font-size:11px; color:var(--text-3);">Remove todos os usuários e faces deste terminal.</div></div>
      <button class="btn btn-danger sm" data-modal="clearUsers">Limpar todos</button></div>
    </div>`;
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

function cfgFaces() {
  const ph = Array.from({length:5}).map(() => `<div class="face-ph"></div>`).join('');
  return `
    <div class="card pad">
      <div class="row-between"><div class="h" style="font-size:13.5px; font-weight:600;">Biblioteca de faces</div><div class="meta mono">§7.7</div></div>
      <div class="facegrid" style="margin-top:14px;">${ph}<div class="face-add" data-pending="Enviar face">${ICON.plus}</div></div>
      <div style="margin-top:12px;">${pendingNote('Faces reais carregam do endpoint da biblioteca; os blocos acima são apenas o layout.')}</div>
    </div>
    <div class="card pad">
      <div class="h" style="font-size:13.5px; font-weight:600;">Comparação 1:1</div>
      <div style="font-size:11.5px; color:var(--text-3); margin-top:3px;">Verifique se uma imagem corresponde a um usuário cadastrado.</div>
      <div style="display:flex; align-items:center; gap:14px; margin-top:14px;">
        <div style="width:88px; height:108px; border-radius:10px; background:repeating-linear-gradient(135deg,var(--surface-2) 0 7px,var(--surface-3) 7px 14px); display:grid; place-items:center; color:var(--text-3); font-size:10px;" class="mono">imagem</div>
        <div style="color:var(--text-3);"><svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14M13 6l6 6-6 6"/></svg></div>
        <div style="flex:1;"><select class="select"><option>Selecione um usuário…</option></select><button class="btn btn-accent" style="margin-top:10px;" data-pending="Comparar 1:1">Comparar</button></div>
      </div>
    </div>
    <div class="card" style="border-color:var(--red); padding:14px 16px;">
      <div class="row-between"><div><div style="font-size:12.5px; font-weight:500; color:var(--red);">Limpar biblioteca de faces</div><div style="font-size:11px; color:var(--text-3);">Remove todas as faces — usuários permanecem sem reconhecimento.</div></div>
      <button class="btn btn-danger sm" data-modal="clearFaces">Limpar faces</button></div>
    </div>`;
}

function cfgEvents() {
  return `
    <div class="card flush">
      <div class="card-head"><div class="h">Webhooks</div><button class="btn btn-soft sm" data-pending="Adicionar webhook">${ICON.plus} Adicionar</button></div>
      ${emptyState(ICON.device, 'Destinos de notificação', 'A lista de webhooks do terminal (§7.8) é carregada do backend de eventos.')}
    </div>
    <div class="card flush">
      <div class="card-head"><div class="h">Stream ao vivo</div><span class="badge badge-muted">desconectado</span></div>
      <div style="padding:16px;">${pendingNote('O monitor de eventos em tempo real do device será habilitado com o stream ISAPI (§7.8).')}</div>
    </div>`;
}

function cfgMedia() {
  return `
    <div class="row-between"><div class="meta">Imagens enviadas ao dispositivo</div><button class="btn btn-accent sm" data-pending="Enviar mídia">${ICON.upload} Enviar</button></div>
    <div class="facegrid" style="grid-template-columns:repeat(4,1fr);">
      <div class="face-ph" style="aspect-ratio:16/10;"></div>
      <div class="face-ph" style="aspect-ratio:16/10;"></div>
      <div class="face-add" style="aspect-ratio:16/10;" data-pending="Enviar mídia">${ICON.plus}</div>
    </div>
    ${pendingNote('O gerenciamento de mídia do terminal (§7.9) depende do backend correspondente.')}`;
}

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
