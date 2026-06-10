let statusStreamConnected = false;
let statusLastRefreshedAt = 0;

async function refreshStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) {
      setRefreshConnected(false);
      return;
    }
    const status = await res.json();
    const wg = document.querySelector('#wg-status');
    const bird = document.querySelector('#bird-status');
    if (wg) setDefinitionList(wg, statusRows('WireGuard', status.wireGuard));
    if (bird) setDefinitionList(bird, statusRows('BIRD', status.bird));
    statusLastRefreshedAt = Date.now();
    updateRefreshAges();
    setRefreshConnected(statusStreamConnected || !window.EventSource);
  } catch (_err) {
    setRefreshConnected(false);
  }
}

function setRefreshConnected(connected) {
  document.querySelectorAll('[data-refresh-indicator]').forEach((indicator) => {
    indicator.classList.toggle('is-connected', connected);
    indicator.classList.toggle('is-disconnected', !connected);
    indicator.title = connected ? 'Status stream connected' : 'Status stream disconnected';
    const label = indicator.querySelector('.sr-only');
    if (label) label.textContent = connected ? 'Connected' : 'Disconnected';
  });
}

function updateRefreshAges() {
  const text = statusLastRefreshedAt ? `Last refreshed: ${formatAge(Date.now() - statusLastRefreshedAt)} ago` : 'Last refreshed: never';
  document.querySelectorAll('[data-refresh-age]').forEach((el) => {
    el.textContent = text;
  });
}

function formatAge(ms) {
  const seconds = Math.max(0, Math.floor(ms / 1000));
  if (seconds < 60) return `${seconds} seconds`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} minutes`;
  const hours = Math.floor(minutes / 60);
  return `${hours} hours`;
}

function statusRows(kind, service) {
  const stats = service.stats || {};
  if (kind === 'WireGuard') {
    return [
      ['Interface', service.interface],
      ['State', service.state],
      ['Endpoint', stats.endpoint],
      ['Latest handshake', stats.latestHandshake],
      ['RX bytes', stats.rxBytes],
      ['TX bytes', stats.txBytes],
      ['Keepalive', stats.persistentKeepalive],
    ].filter(([, value]) => value);
  }
  return [
    ['State', service.state],
    ['BGP state', stats.bgpState],
    ['Neighbor', stats.neighbor],
    ['Neighbor AS', stats.neighborAS],
    ['Local AS', stats.localAS],
    ['Routes', stats.routes],
  ].filter(([, value]) => value);
}

function setDefinitionList(el, rows) {
  el.replaceChildren();
  rows.forEach(([label, value]) => {
    const dt = document.createElement('dt');
    const dd = document.createElement('dd');
    dt.textContent = label;
    dd.textContent = value || '';
    el.append(dt, dd);
  });
}

function csrfHeaders(extra = {}) {
  const token = document.querySelector('meta[name="csrf-token"]')?.content || '';
  return token ? {...extra, 'X-CSRF-Token': token} : extra;
}

async function runRoutingAction(action) {
  const message = document.querySelector('#routing-message');
  const steps = document.querySelector('#routing-steps');
  const buttons = [...document.querySelectorAll('[data-routing-action]')];
  if (!message || !steps) return;
  setRoutingBusy(buttons, true);
  message.textContent = `${labelForAction(action)} running`;
  steps.replaceChildren();
  try {
    const res = await fetch(`/api/routing/${action}`, {
      method: 'POST',
      headers: csrfHeaders(),
    });
    const body = await res.json().catch(() => ({}));
    renderRoutingResult(steps, body.steps || []);
    message.textContent = body.ok ? `${labelForAction(action)} complete` : `${labelForAction(action)} failed`;
    await refreshStatus();
  } catch (err) {
    message.textContent = `${labelForAction(action)} failed`;
    renderRoutingResult(steps, [{action, ok: false, error: err.message || String(err)}]);
  } finally {
    setRoutingBusy(buttons, false);
  }
}

function renderRoutingResult(el, steps) {
  el.replaceChildren();
  if (!steps.length) {
    const item = document.createElement('li');
    item.textContent = 'No steps returned';
    item.className = 'step-error';
    el.append(item);
    return;
  }
  steps.forEach((step) => {
    const item = document.createElement('li');
    const action = document.createElement('span');
    const state = document.createElement('span');
    const detail = document.createElement('pre');
    action.textContent = step.action || 'unknown';
    state.textContent = step.ok ? 'ok' : 'error';
    item.className = step.ok ? 'step-ok' : 'step-error';
    item.append(action, state);
    if (step.output || step.error) {
      detail.textContent = step.output || step.error;
      item.append(detail);
    }
    el.append(item);
  });
}

function setRoutingBusy(buttons, busy) {
  buttons.forEach((button) => {
    button.disabled = busy;
  });
}

function labelForAction(action) {
  return action.charAt(0).toUpperCase() + action.slice(1);
}

async function refreshDiag(el) {
  const name = el.dataset.diag;
  const res = await fetch(`/api/diag/${name}`);
  const body = await res.json();
  el.textContent = body.output || body.error || 'No output';
}

async function loadBirdConfig() {
  const form = document.querySelector('#bird-form');
  const preview = document.querySelector('#bird-preview');
  if (!form || !preview) return;
  const res = await fetch('/api/bird/config');
  const body = await res.json();
  const cfg = body.config || {};
  form.routerId.value = cfg.routerId || '';
  form.localAsn.value = cfg.localAsn || '';
  form.peerAsn.value = cfg.peerAsn || '';
  form.peerIp.value = cfg.peerIp || '';
  form.interface.value = cfg.interface || 'wg0';
  form.advertisedRoutes.value = (cfg.advertisedRoutes || []).join('\n');
  preview.textContent = body.generated || body.error || 'Incomplete BIRD settings';
}

async function loadWireGuardConfig() {
  const textarea = document.querySelector('#wg-config');
  const message = document.querySelector('#wg-message');
  if (!textarea) return;
  const res = await fetch('/api/wg/config');
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (message) message.textContent = body.error || 'Load failed';
    return;
  }
  textarea.value = body.config || '';
  if (message) message.textContent = body.exists ? 'Loaded with keys redacted' : 'No WireGuard config saved';
}

async function loadSettings() {
  const form = document.querySelector('#settings-form');
  const message = document.querySelector('#settings-message');
  if (!form) return;
  const res = await fetch('/api/settings');
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (message) message.textContent = body.error || 'Load failed';
    return;
  }
  form.autoStart.checked = Boolean(body.autoStart);
  form.pinDashboardClientRoute.checked = Boolean(body.pinDashboardClientRoute);
  form.pinnedClientRoutes.value = (body.pinnedClientRoutes || []).join('\n');
}

async function saveSettings(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const message = document.querySelector('#settings-message');
  const payload = {
    autoStart: form.autoStart.checked,
    pinDashboardClientRoute: form.pinDashboardClientRoute.checked,
    pinnedClientRoutes: form.pinnedClientRoutes.value.split('\n').map((route) => route.trim()).filter(Boolean),
  };
  const res = await fetch('/api/settings', {
    method: 'POST',
    headers: csrfHeaders({'Content-Type': 'application/json'}),
    body: JSON.stringify(payload),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    message.textContent = body.error || 'Save failed';
    return;
  }
  form.autoStart.checked = Boolean(body.autoStart);
  form.pinDashboardClientRoute.checked = Boolean(body.pinDashboardClientRoute);
  form.pinnedClientRoutes.value = (body.pinnedClientRoutes || []).join('\n');
  message.textContent = 'Saved';
}

async function saveBirdConfig(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const message = document.querySelector('#bird-message');
  const payload = {
    routerId: form.routerId.value.trim(),
    localAsn: Number(form.localAsn.value),
    peerAsn: Number(form.peerAsn.value),
    peerIp: form.peerIp.value.trim(),
    interface: form.interface.value.trim() || 'wg0',
    advertisedRoutes: form.advertisedRoutes.value.split('\n').map((route) => route.trim()).filter(Boolean),
  };
  const res = await fetch('/api/bird/config', {
    method: 'POST',
    headers: csrfHeaders({'Content-Type': 'application/json'}),
    body: JSON.stringify(payload),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    message.textContent = body.error || 'Save failed';
    return;
  }
  message.textContent = 'Saved';
  document.querySelector('#bird-preview').textContent = body.generated || '';
}

async function saveWireGuardConfig() {
  const textarea = document.querySelector('#wg-config');
  const message = document.querySelector('#wg-message');
  if (!textarea || !message) return;
  const res = await fetch('/api/wg/config', {method: 'POST', headers: csrfHeaders(), body: textarea.value});
  if (!res.ok) {
    message.textContent = await res.text();
    return;
  }
  const body = await res.json().catch(() => ({}));
  textarea.value = body.config || textarea.value;
  message.textContent = 'Saved with keys redacted';
}

async function loadLogs() {
  const list = document.querySelector('#app-logs');
  if (!list) return;
  const res = await fetch('/api/logs');
  const body = await res.json().catch(() => ({}));
  list.replaceChildren();
  if (!res.ok) {
    const item = document.createElement('li');
    item.textContent = body.error || 'Log load failed';
    list.append(item);
    return;
  }
  const entries = body.entries || [];
  if (!entries.length) {
    const item = document.createElement('li');
    item.textContent = 'No log entries';
    list.append(item);
    return;
  }
  entries.slice().reverse().forEach((entry) => {
    const item = document.createElement('li');
    const meta = document.createElement('span');
    const message = document.createElement('strong');
    const detail = document.createElement('pre');
    meta.textContent = `${entry.time || ''} ${entry.level || 'info'}`;
    message.textContent = entry.message || '';
    item.append(meta, message);
    if (entry.detail) {
      detail.textContent = entry.detail;
      item.append(detail);
    }
    list.append(item);
  });
}

document.querySelectorAll('[data-diag]').forEach(refreshDiag);
refreshStatus();
loadWireGuardConfig();
loadBirdConfig();
loadSettings();
loadLogs();

const settingsForm = document.querySelector('#settings-form');
if (settingsForm) settingsForm.addEventListener('submit', saveSettings);

const birdForm = document.querySelector('#bird-form');
if (birdForm) birdForm.addEventListener('submit', saveBirdConfig);

const saveWG = document.querySelector('#save-wg');
if (saveWG) saveWG.addEventListener('click', saveWireGuardConfig);

const refreshLogs = document.querySelector('#refresh-logs');
if (refreshLogs) refreshLogs.addEventListener('click', loadLogs);

document.querySelectorAll('[data-routing-action]').forEach((button) => {
  button.addEventListener('click', () => runRoutingAction(button.dataset.routingAction));
});

if (window.EventSource) {
  const events = new EventSource('/api/events');
  events.addEventListener('open', () => {
    statusStreamConnected = true;
    setRefreshConnected(true);
  });
  events.addEventListener('status', refreshStatus);
  events.addEventListener('error', () => {
    statusStreamConnected = false;
    setRefreshConnected(false);
  });
}

setInterval(updateRefreshAges, 1000);
updateRefreshAges();
