/**
 * Face Attendance Admin — SPA-lite
 * Hash-based routing, cookie session auth, AbortController timeouts.
 * All API paths match contracts/admin-api.md exactly.
 */

// ─── CONSTANTS ────────────────────────────────────────────────
const API_TIMEOUT_MS      = 10_000;   // CHK-P20: 10s for data calls
const SYNC_TIMEOUT_MS     = 60_000;   // CHK-P20: 60s for sync
const DEBOUNCE_MEMBERS_MS = 300;      // search debounce
const TOAST_DURATION_MS   = 4_500;

// ─── STATE ────────────────────────────────────────────────────
const state = {
  currentRoute: null,
  members: { items: [], nextCursor: null, hasMore: false, query: '' },
  events:  { items: [], nextCursor: null, hasMore: false },
  syncInProgress: false,
};

// ─── HELPERS: FETCH ───────────────────────────────────────────

/**
 * Wrapped fetch with AbortController timeout and global 401 interception.
 * @param {string} url
 * @param {RequestInit} options
 * @param {number} timeoutMs
 * @returns {Promise<Response>}
 */
async function apiFetch(url, options = {}, timeoutMs = API_TIMEOUT_MS) {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), timeoutMs);

  try {
    const res = await fetch(url, {
      ...options,
      credentials: 'same-origin',
      signal: ctrl.signal,
    });
    clearTimeout(timer);

    // Global 401 interception (FR-012)
    if (res.status === 401) {
      const current = window.location.hash.replace('#', '') || 'dashboard';
      const redirect = encodeURIComponent(current);
      navigate(`login?redirect=${redirect}`, false);
      return res;
    }

    return res;
  } catch (err) {
    clearTimeout(timer);
    if (err.name === 'AbortError') {
      throw new Error('timeout');
    }
    throw err;
  }
}

async function apiGet(path, timeoutMs) {
  return apiFetch(`/admin/api/${path}`, { method: 'GET' }, timeoutMs);
}

async function apiPost(path, body, timeoutMs) {
  return apiFetch(`/admin/api/${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }, timeoutMs);
}

// ─── HELPERS: TOAST ───────────────────────────────────────────

/**
 * @param {'success'|'error'|'info'} type
 * @param {string} title
 * @param {string} [message]
 */
function showToast(type, title, message) {
  const container = document.getElementById('toast-container');
  const icons = {
    success: `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`,
    error:   `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`,
    info:    `<svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>`,
  };

  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.setAttribute('role', 'status');
  toast.innerHTML = `
    <span class="toast-icon" aria-hidden="true">${icons[type]}</span>
    <div class="toast-body">
      <div class="toast-title">${escHtml(title)}</div>
      ${message ? `<div class="toast-message">${escHtml(message)}</div>` : ''}
    </div>
    <button class="toast-close" aria-label="Fechar notificação">
      <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12"/></svg>
    </button>
  `;

  const dismiss = () => {
    toast.classList.add('removing');
    toast.addEventListener('animationend', () => toast.remove(), { once: true });
  };

  toast.querySelector('.toast-close').addEventListener('click', dismiss);
  container.appendChild(toast);
  setTimeout(dismiss, TOAST_DURATION_MS);
}

// ─── HELPERS: DOM ─────────────────────────────────────────────

function $(id) { return document.getElementById(id); }

function escHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function formatDateTime(iso) {
  if (!iso) return '—';
  try {
    return new Intl.DateTimeFormat('pt-BR', {
      day: '2-digit', month: '2-digit', year: 'numeric',
      hour: '2-digit', minute: '2-digit',
    }).format(new Date(iso));
  } catch {
    return iso;
  }
}

function showEl(id)  { const e = $(id); if (e) { e.classList.remove('hidden'); e.style.display = ''; } }
function hideEl(id)  { const e = $(id); if (e) e.classList.add('hidden'); }

function setLoading(screenPrefix, loading) {
  if (loading) {
    showEl(`${screenPrefix}-loading`);
    hideEl(`${screenPrefix}-list`);
    hideEl(`${screenPrefix}-list-view`);
  } else {
    hideEl(`${screenPrefix}-loading`);
    showEl(`${screenPrefix}-list`);
    showEl(`${screenPrefix}-list-view`);
  }
}

// ─── ROUTING ──────────────────────────────────────────────────

const PROTECTED_ROUTES = new Set(['dashboard', 'devices', 'members', 'events']);

/**
 * Navigate to a route, pushing to hash.
 * @param {string} route  e.g. "dashboard", "login?redirect=dashboard"
 * @param {boolean} push  whether to push to history
 */
function navigate(route, push = true) {
  if (push) {
    window.location.hash = route;
  } else {
    // Replace to avoid polluting history on auth redirect
    window.history.replaceState(null, '', `#${route}`);
  }
  renderRoute(route);
}

function getRouteFromHash() {
  const hash = window.location.hash.replace('#', '') || 'dashboard';
  return hash;
}

function renderRoute(fullRoute) {
  const [route] = fullRoute.split('?');
  state.currentRoute = route;

  const screens = ['login', 'dashboard', 'devices', 'members', 'events'];
  screens.forEach(s => {
    const el = $(`screen-${s}`);
    if (el) el.classList.remove('active');
  });

  // Show/hide sidebar based on route
  const isLogin = route === 'login';
  const sidebar = document.getElementById('sidebar');
  const layout  = document.querySelector('.app-layout');
  if (sidebar)  sidebar.style.display = isLogin ? 'none' : '';
  if (layout)   layout.style.gridTemplateColumns = isLogin ? '1fr' : '';

  // Update active sidebar link
  document.querySelectorAll('.sidebar-link').forEach(a => {
    a.classList.toggle('active', a.dataset.route === route);
  });

  // Activate screen
  const screen = $(`screen-${route}`);
  if (screen) screen.classList.add('active');

  // Load screen data
  switch (route) {
    case 'dashboard': loadDashboard(); break;
    case 'devices':   loadDevices();   break;
    case 'members':   loadMembers(true); break;
    case 'events':    loadEvents(true);  break;
    case 'login':     break;
    default:
      // Unknown route → dashboard
      navigate('dashboard');
  }
}

// ─── AUTH ─────────────────────────────────────────────────────

function initLogin() {
  const form = $('login-form');
  if (!form) return;

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const btn = $('login-submit-btn');
    const errEl = $('login-error');
    const username = $('login-username').value.trim();
    const password = $('login-password').value;

    if (!username || !password) {
      errEl.textContent = 'Preencha usuário e senha.';
      errEl.classList.add('visible');
      return;
    }

    btn.disabled = true;
    btn.textContent = 'Entrando...';
    errEl.classList.remove('visible');

    try {
      const res = await apiPost('login', { username, password });

      if (res.status === 204) {
        // Sucesso: redirecionar para destino (FR-012)
        const params = new URLSearchParams(window.location.hash.split('?')[1] || '');
        const dest = params.get('redirect') || 'dashboard';
        navigate(dest);
      } else if (res.status === 401) {
        let msg = 'Credenciais inválidas.';
        try {
          const data = await res.json();
          if (data.error) msg = data.error;
        } catch { /* ignore */ }
        errEl.textContent = msg;
        errEl.classList.add('visible');
        $('login-password').value = '';
        $('login-password').focus();
      } else if (res.status === 400) {
        errEl.textContent = 'Requisição inválida.';
        errEl.classList.add('visible');
      } else {
        errEl.textContent = 'Erro inesperado. Tente novamente.';
        errEl.classList.add('visible');
      }
    } catch (err) {
      errEl.textContent = err.message === 'timeout'
        ? 'Tempo de resposta esgotado. Verifique a conexão.'
        : 'Erro de conexão. Tente novamente.';
      errEl.classList.add('visible');
    } finally {
      btn.disabled = false;
      btn.textContent = 'Entrar';
    }
  });
}

async function doLogout() {
  try {
    await apiPost('logout', undefined);
  } catch { /* ignore */ }
  navigate('login', false);
}

// ─── DASHBOARD ────────────────────────────────────────────────

async function loadDashboard() {
  showEl('dashboard-loading');
  hideEl('dashboard-content');

  try {
    const res = await apiGet('stats');
    if (res.status === 401) return; // intercepted

    if (!res.ok) {
      showToast('error', 'Erro ao carregar métricas', `Status ${res.status}`);
      return;
    }

    const stats = await res.json();
    renderDashboard(stats);
  } catch (err) {
    showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
  } finally {
    hideEl('dashboard-loading');
    showEl('dashboard-content');
  }
}

function renderDashboard(stats) {
  const grid = $('metrics-grid');
  const offlineAlert = $('offline-alert');

  const membersWithSelfie  = stats.members_with_selfie ?? 0;
  const devicesActive      = stats.devices_active ?? 0;
  const devicesInactive    = stats.devices_inactive ?? 0;
  const attendance24h      = stats.attendance_last_24h ?? 0;
  const thresholdHours     = stats.device_offline_threshold_hours ?? 24;

  // Show offline alert if any inactive devices
  if (devicesInactive > 0) {
    const txt = $('offline-alert-text');
    if (txt) txt.textContent =
      `${devicesInactive} dispositivo${devicesInactive > 1 ? 's' : ''} sem heartbeat nas últimas ${thresholdHours}h.`;
    offlineAlert.classList.remove('hidden');
  } else {
    offlineAlert.classList.add('hidden');
  }

  // Empty state: zero devices
  const isEmpty = devicesActive === 0 && devicesInactive === 0;

  grid.innerHTML = `
    <div class="metric-card" role="listitem">
      <div class="metric-card-icon">
        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z"/>
        </svg>
      </div>
      <div class="metric-card-value">${membersWithSelfie}</div>
      <div class="metric-card-label">Membros com selfie</div>
    </div>

    <div class="metric-card" role="listitem">
      <div class="metric-card-icon ${devicesActive > 0 ? 'success' : ''}">
        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18"/>
        </svg>
      </div>
      <div class="metric-card-value">${devicesActive}</div>
      <div class="metric-card-label">Dispositivos ativos</div>
    </div>

    <div class="metric-card" role="listitem">
      <div class="metric-card-icon ${devicesInactive > 0 ? 'danger' : ''}">
        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M18.364 5.636a9 9 0 010 12.728m0 0l-2.829-2.829m2.829 2.829L21 21M15.536 8.464a5 5 0 010 7.072m0 0l-2.829-2.829m-4.243 2.829a4.978 4.978 0 01-1.414-2.83m-1.414 5.658a9 9 0 01-2.167-9.238m7.824 2.167a1 1 0 111.414 1.414m-1.414-1.414L3 3"/>
        </svg>
      </div>
      <div class="metric-card-value">${devicesInactive}</div>
      <div class="metric-card-label">Dispositivos offline</div>
      ${devicesInactive > 0 ? `<span class="metric-card-trend alert" aria-label="Alerta: ${devicesInactive} offline">! offline</span>` : ''}
    </div>

    <div class="metric-card" role="listitem">
      <div class="metric-card-icon ${attendance24h > 0 ? 'success' : ''}">
        <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01"/>
        </svg>
      </div>
      <div class="metric-card-value">${attendance24h}</div>
      <div class="metric-card-label">Presenças (24h)</div>
    </div>
  `;

  if (isEmpty) {
    grid.insertAdjacentHTML('afterend', `
      <div class="empty-state" style="margin-top: var(--space-5);" role="status">
        <div class="empty-state-icon" aria-hidden="true">
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18"/>
          </svg>
        </div>
        <h3 class="empty-state-title">Nenhum dispositivo configurado</h3>
        <p class="empty-state-text">Configure os leitores biométricos para começar a registrar presenças.</p>
      </div>
    `);
  }
}

// ─── SYNC ─────────────────────────────────────────────────────

function initSync() {
  const btn = $('sync-btn');
  if (!btn) return;

  btn.addEventListener('click', async () => {
    if (state.syncInProgress) return;

    state.syncInProgress = true;
    btn.classList.add('sync-btn--loading');
    btn.disabled = true;
    const label = $('sync-btn-label');
    if (label) label.textContent = 'Sincronizando...';

    try {
      const res = await apiPost('sync', undefined, SYNC_TIMEOUT_MS);

      if (res.status === 202) {
        showToast('success', 'Sincronização iniciada', 'Os membros serão sincronizados em segundo plano.');
      } else if (res.status === 409) {
        showToast('info', 'Sincronização em andamento', 'Aguarde a conclusão antes de iniciar nova sincronização.');
      } else if (res.status === 401) {
        // Already handled by interceptor
      } else {
        showToast('error', 'Falha na sincronização', `Status ${res.status}`);
      }
    } catch (err) {
      showToast('error', 'Falha na sincronização',
        err.message === 'timeout' ? 'Tempo esgotado após 60s.' : err.message);
    } finally {
      state.syncInProgress = false;
      btn.classList.remove('sync-btn--loading');
      btn.disabled = false;
      if (label) label.textContent = 'Sincronizar';
    }
  });
}

// ─── DEVICES ──────────────────────────────────────────────────

async function loadDevices() {
  // Show list view, hide detail view
  showEl('devices-list-view');
  hideEl('devices-detail-view');
  setLoading('devices', true);

  try {
    const res = await apiGet('devices');
    if (res.status === 401) return;

    if (!res.ok) {
      showToast('error', 'Erro ao carregar dispositivos', `Status ${res.status}`);
      return;
    }

    const data = await res.json();
    renderDevices(data);
  } catch (err) {
    showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
  } finally {
    setLoading('devices', false);
  }
}

function renderDevices(data) {
  const devices  = data.devices ?? [];
  const tbody    = $('devices-tbody');
  const emptyEl  = $('devices-empty');
  const countEl  = $('devices-count');

  if (countEl) countEl.textContent = devices.length;

  if (devices.length === 0) {
    tbody.innerHTML = '';
    showEl('devices-empty');
    return;
  }
  hideEl('devices-empty');

  tbody.innerHTML = devices.map(d => {
    const statusBadge = d.status === 'active'
      ? `<span class="badge badge-active">Ativo</span>`
      : `<span class="badge badge-offline">Offline</span>`;
    const webhookBadge = d.webhook_configured
      ? `<span class="badge badge-synced">Sim</span>`
      : `<span class="badge badge-unauthorized">Não</span>`;

    return `
      <tr class="clickable" data-device-id="${escHtml(String(d.id))}" tabindex="0"
          role="button" aria-label="Ver detalhes do dispositivo ${escHtml(d.device_identifier)}">
        <td><span class="td-mono">${escHtml(d.device_identifier)}</span></td>
        <td class="hide-mobile"><span class="td-mono">${escHtml(d.ip_address ?? '—')}</span></td>
        <td>${statusBadge}</td>
        <td class="hide-mobile">${webhookBadge}</td>
        <td class="hide-mobile td-muted"><span class="td-mono">${formatDateTime(d.last_heartbeat_at)}</span></td>
      </tr>
    `;
  }).join('');

  // Row click → device detail
  tbody.querySelectorAll('tr[data-device-id]').forEach(row => {
    const handler = () => loadDeviceDetail(Number(row.dataset.deviceId));
    row.addEventListener('click', handler);
    row.addEventListener('keydown', e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handler(); } });
  });
}

async function loadDeviceDetail(id) {
  hideEl('devices-list-view');
  showEl('devices-detail-view');

  const card = $('device-detail-card');
  card.innerHTML = `<div class="loading-spinner" role="status"><div class="spinner" aria-hidden="true"></div><span class="loading-text">Carregando...</span></div>`;

  try {
    const res = await apiGet(`devices/${id}`);
    if (res.status === 401) return;

    if (res.status === 404) {
      card.innerHTML = `<div class="empty-state"><h3 class="empty-state-title">Dispositivo não encontrado</h3></div>`;
      return;
    }
    if (!res.ok) {
      showToast('error', 'Erro ao carregar dispositivo', `Status ${res.status}`);
      return;
    }

    const d = await res.json();
    const statusBadge = d.status === 'active'
      ? `<span class="badge badge-active">Ativo</span>`
      : `<span class="badge badge-offline">Offline</span>`;

    card.innerHTML = `
      <div class="device-detail-header">
        <div class="device-icon" aria-hidden="true">
          <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M9 3H5a2 2 0 00-2 2v4m6-6h10a2 2 0 012 2v4M9 3v18m0 0h10a2 2 0 002-2V9M9 21H5a2 2 0 01-2-2V9m0 0h18"/>
          </svg>
        </div>
        <div>
          <div class="device-detail-name">${escHtml(d.device_identifier)}</div>
          <div class="device-detail-id">#${escHtml(String(d.id))}</div>
        </div>
        <div style="margin-left:auto;">${statusBadge}</div>
      </div>
      <div class="device-detail-body">
        <div class="device-field">
          <span class="device-field-label">IP</span>
          <span class="device-field-value">${escHtml(d.ip_address ?? '—')}</span>
        </div>
        <div class="device-field">
          <span class="device-field-label">MAC</span>
          <span class="device-field-value">${escHtml(d.mac_address ?? '—')}</span>
        </div>
        <div class="device-field">
          <span class="device-field-label">Webhook</span>
          <span class="device-field-value">${d.webhook_configured ? 'Configurado' : 'Não configurado'}</span>
        </div>
        <div class="device-field">
          <span class="device-field-label">Ativo</span>
          <span class="device-field-value">${d.is_active ? 'Sim' : 'Não'}</span>
        </div>
        <div class="device-field">
          <span class="device-field-label">Último heartbeat</span>
          <span class="device-field-value">${formatDateTime(d.last_heartbeat_at)}</span>
        </div>
        <div class="device-field">
          <span class="device-field-label">Cadastrado em</span>
          <span class="device-field-value">${formatDateTime(d.created_at)}</span>
        </div>
      </div>
    `;
  } catch (err) {
    showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
    // Fall back to list
    showEl('devices-list-view');
    hideEl('devices-detail-view');
  }
}

function initDevicesBackBtn() {
  const btn = $('devices-back-btn');
  if (btn) {
    btn.addEventListener('click', () => {
      hideEl('devices-detail-view');
      showEl('devices-list-view');
    });
  }

  const refreshBtn = $('devices-refresh-btn');
  if (refreshBtn) {
    refreshBtn.addEventListener('click', () => loadDevices());
  }
}

// ─── MEMBERS ──────────────────────────────────────────────────

let membersDebounceTimer = null;

async function loadMembers(reset = false) {
  if (reset) {
    state.members.items = [];
    state.members.nextCursor = null;
    state.members.hasMore = false;
  }

  if (reset) setLoading('members', true);

  const params = new URLSearchParams();
  if (state.members.query) params.set('q', state.members.query);
  if (state.members.nextCursor) params.set('cursor', state.members.nextCursor);

  try {
    const res = await apiGet(`members?${params}`);
    if (res.status === 401) return;

    if (!res.ok) {
      showToast('error', 'Erro ao carregar membros', `Status ${res.status}`);
      return;
    }

    const data = await res.json();
    const newItems = data.members ?? [];

    if (reset) {
      state.members.items = newItems;
    } else {
      state.members.items = [...state.members.items, ...newItems];
    }
    state.members.nextCursor = data.next_cursor ?? null;
    state.members.hasMore    = data.has_more ?? false;

    renderMembers();
  } catch (err) {
    showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
  } finally {
    if (reset) setLoading('members', false);
  }
}

function renderMembers() {
  const tbody    = $('members-tbody');
  const emptyEl  = $('members-empty');
  const countEl  = $('members-count');
  const footer   = $('members-footer');
  const loadMore = $('members-load-more');
  const items    = state.members.items;

  if (countEl) countEl.textContent = items.length + (state.members.hasMore ? '+' : '');

  if (items.length === 0) {
    tbody.innerHTML = '';
    showEl('members-empty');
    // Differentiate empty states (CHK-U09)
    const titleEl = $('members-empty-title');
    const textEl  = $('members-empty-text');
    if (state.members.query) {
      if (titleEl) titleEl.textContent = 'Nenhum resultado encontrado';
      if (textEl)  textEl.textContent  = `Nenhum membro corresponde a "${state.members.query}".`;
    } else {
      if (titleEl) titleEl.textContent = 'Nenhum membro cadastrado';
      if (textEl)  textEl.textContent  = 'Os membros aparecerão aqui após a primeira sincronização.';
    }
    if (footer) footer.style.display = 'none';
    return;
  }

  hideEl('members-empty');

  const syncBadgeMap = {
    synced:  `<span class="badge badge-synced">Sincronizado</span>`,
    failed:  `<span class="badge badge-failed">Falha</span>`,
    pending: `<span class="badge badge-pending">Pendente</span>`,
  };

  tbody.innerHTML = items.map(m => {
    const syncBadge = syncBadgeMap[m.sync_status] ?? `<span class="badge badge-pending">${escHtml(m.sync_status ?? '—')}</span>`;
    // Reenvio só faz sentido p/ "failed": têm selfie mas erraram o enrollment.
    // Os "pending" da lista são membros SEM selfie (nada a enrolar); "synced" já ok.
    const action = m.sync_status === 'failed'
      ? `<button class="btn btn-ghost btn-sm" data-resync-id="${escHtml(String(m.id))}" data-resync-name="${escHtml(m.name ?? 'membro')}">Reenviar</button>`
      : `<span class="td-muted">—</span>`;
    return `
      <tr>
        <td>${escHtml(m.name ?? '—')}</td>
        <td class="hide-mobile"><span class="td-mono">${escHtml(m.federal_document_masked ?? '—')}</span></td>
        <td>${syncBadge}</td>
        <td class="hide-tablet hide-mobile"><span class="td-muted">${escHtml(m.status ?? '—')}</span></td>
        <td class="hide-mobile td-muted td-mono">${escHtml(m.last_failed_stage ?? '—')}</td>
        <td>${action}</td>
      </tr>
    `;
  }).join('');

  if (footer) footer.style.display = state.members.hasMore ? '' : 'none';
  if (loadMore) loadMore.disabled = false;
}

function initMembersSearch() {
  const input = $('members-search');
  if (!input) return;

  input.addEventListener('input', () => {
    clearTimeout(membersDebounceTimer);
    membersDebounceTimer = setTimeout(() => {
      state.members.query = input.value.trim();
      loadMembers(true);
    }, DEBOUNCE_MEMBERS_MS);
  });

  const loadMoreBtn = $('members-load-more');
  if (loadMoreBtn) {
    loadMoreBtn.addEventListener('click', () => {
      if (state.members.hasMore && state.members.nextCursor) {
        loadMoreBtn.disabled = true;
        loadMembers(false);
      }
    });
  }
}

// ─── EVENTS ───────────────────────────────────────────────────

async function loadEvents(reset = false) {
  if (reset) {
    state.events.items = [];
    state.events.nextCursor = null;
    state.events.hasMore = false;
  }

  if (reset) setLoading('events', true);

  const params = new URLSearchParams();
  const fromVal = $('events-from')?.value;
  const toVal   = $('events-to')?.value;
  if (fromVal) params.set('from', fromVal);
  if (toVal)   params.set('to', toVal);
  if (state.events.nextCursor) params.set('cursor', state.events.nextCursor);

  try {
    const res = await apiGet(`events?${params}`);
    if (res.status === 401) return;

    if (!res.ok) {
      showToast('error', 'Erro ao carregar eventos', `Status ${res.status}`);
      return;
    }

    const data = await res.json();
    const newItems = data.events ?? [];

    if (reset) {
      state.events.items = newItems;
    } else {
      state.events.items = [...state.events.items, ...newItems];
    }
    state.events.nextCursor = data.next_cursor ?? null;
    state.events.hasMore    = data.has_more ?? false;

    renderEvents();
  } catch (err) {
    showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
  } finally {
    if (reset) setLoading('events', false);
  }
}

function renderEvents() {
  const tbody    = $('events-tbody');
  const emptyEl  = $('events-empty');
  const countEl  = $('events-count');
  const footer   = $('events-footer');
  const loadMore = $('events-load-more');
  const items    = state.events.items;

  if (countEl) countEl.textContent = items.length + (state.events.hasMore ? '+' : '');

  if (items.length === 0) {
    tbody.innerHTML = '';
    showEl('events-empty');
    if (footer) footer.style.display = 'none';
    return;
  }

  hideEl('events-empty');

  const markingBadgeMap = {
    marked:       `<span class="badge badge-marked">Marcado</span>`,
    pending:      `<span class="badge badge-pending">Pendente</span>`,
    failed:       `<span class="badge badge-failed">Falha</span>`,
    unauthorized: `<span class="badge badge-unauthorized">Não autorizado</span>`,
  };

  tbody.innerHTML = items.map(ev => {
    const statusBadge = markingBadgeMap[ev.marking_status]
      ?? `<span class="badge badge-pending">${escHtml(ev.marking_status ?? '—')}</span>`;

    return `
      <tr>
        <td><span class="td-mono">${formatDateTime(ev.event_datetime || ev.created_at)}</span></td>
        <td class="hide-mobile"><span class="td-mono">${escHtml(ev.device_identifier ?? '—')}</span></td>
        <td>${escHtml(ev.member_name ?? '—')}</td>
        <td class="hide-mobile"><span class="td-mono">${escHtml(ev.federal_document_masked ?? '—')}</span></td>
        <td>${statusBadge}</td>
      </tr>
    `;
  }).join('');

  if (footer) footer.style.display = state.events.hasMore ? '' : 'none';
  if (loadMore) loadMore.disabled = false;
}

function initEventsFilter() {
  const filterBtn = $('events-filter-btn');
  if (filterBtn) {
    filterBtn.addEventListener('click', () => loadEvents(true));
  }

  const loadMoreBtn = $('events-load-more');
  if (loadMoreBtn) {
    loadMoreBtn.addEventListener('click', () => {
      if (state.events.hasMore && state.events.nextCursor) {
        loadMoreBtn.disabled = true;
        loadEvents(false);
      }
    });
  }
}

// ─── MOBILE SIDEBAR ───────────────────────────────────────────

function initMobileSidebar() {
  const menuBtn  = $('mobile-menu-btn');
  const sidebar  = document.getElementById('sidebar');
  const overlay  = $('sidebar-overlay');
  if (!menuBtn || !sidebar || !overlay) return;

  const open = () => {
    sidebar.classList.add('open');
    overlay.classList.add('active');
    menuBtn.setAttribute('aria-expanded', 'true');
    // Focus trap: move focus to first sidebar link
    const firstLink = sidebar.querySelector('.sidebar-link');
    if (firstLink) firstLink.focus();
  };

  const close = () => {
    sidebar.classList.remove('open');
    overlay.classList.remove('active');
    menuBtn.setAttribute('aria-expanded', 'false');
    menuBtn.focus();
  };

  menuBtn.addEventListener('click', open);
  overlay.addEventListener('click', close);

  // Close on Escape (focus trap for drawer — dec-031)
  document.addEventListener('keydown', e => {
    if (e.key === 'Escape' && sidebar.classList.contains('open')) {
      close();
    }
  });

  // Close on nav link click (mobile)
  sidebar.querySelectorAll('.sidebar-link').forEach(link => {
    link.addEventListener('click', () => {
      if (window.innerWidth < 768) close();
    });
  });
}

// ─── KEYBOARD NAVIGATION ──────────────────────────────────────

function initKeyboardNav() {
  // Tab between sidebar links via arrow keys
  const nav = document.querySelector('.sidebar-nav');
  if (!nav) return;

  nav.addEventListener('keydown', e => {
    const links = [...nav.querySelectorAll('.sidebar-link')];
    const idx   = links.indexOf(document.activeElement);
    if (idx === -1) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      links[(idx + 1) % links.length].focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      links[(idx - 1 + links.length) % links.length].focus();
    }
  });
}

// ─── LOGOUT ───────────────────────────────────────────────────

function initLogout() {
  const btn = $('logout-btn');
  if (btn) btn.addEventListener('click', doLogout);
}

// ─── ROUTING INIT ─────────────────────────────────────────────

function handleHashChange() {
  const route = getRouteFromHash();
  renderRoute(route);
}

// ─── BOOT ─────────────────────────────────────────────────────

// Reenvio individual: delega o clique no botão "Reenviar" das linhas de membro.
// O #members-tbody persiste entre renders (só o innerHTML troca), então 1 listener basta.
function initMembersActions() {
  const tbody = $('members-tbody');
  if (!tbody) return;
  tbody.addEventListener('click', async (e) => {
    const btn = e.target.closest('[data-resync-id]');
    if (!btn || btn.disabled) return;
    const id   = btn.dataset.resyncId;
    const name = btn.dataset.resyncName || 'membro';
    const original = btn.textContent;
    btn.disabled = true;
    btn.textContent = 'Enviando...';
    try {
      const res = await apiPost(`members/${id}/resync`);
      if (res.status === 401) return; // interceptado → login
      if (res.status === 202) {
        showToast('success', 'Reenviado', `${name} foi enfileirado para reprocessamento.`);
      } else {
        let msg = `Status ${res.status}`;
        try { const d = await res.json(); if (d.error) msg = d.error; } catch { /* ignore */ }
        showToast('error', 'Falha ao reenviar', msg);
      }
    } catch (err) {
      showToast('error', 'Falha na conexão', err.message === 'timeout' ? 'Tempo esgotado' : err.message);
    } finally {
      btn.disabled = false;
      btn.textContent = original;
    }
  });
}

function init() {
  initLogin();
  initSync();
  initDevicesBackBtn();
  initMembersSearch();
  initMembersActions();
  initEventsFilter();
  initMobileSidebar();
  initKeyboardNav();
  initLogout();

  // Hash-based routing
  window.addEventListener('hashchange', handleHashChange);

  // Sidebar link navigation
  document.querySelectorAll('.sidebar-link[data-route]').forEach(link => {
    link.addEventListener('click', e => {
      e.preventDefault();
      navigate(link.dataset.route);
    });
    // Keyboard: Enter activates
    link.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        e.preventDefault();
        navigate(link.dataset.route);
      }
    });
  });

  // Initial route
  handleHashChange();
}

// DOM ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', init);
} else {
  init();
}
