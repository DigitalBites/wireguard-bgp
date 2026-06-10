async function refreshStatus() {
  const res = await fetch('/api/status');
  if (!res.ok) return;
  const status = await res.json();
  const wg = document.querySelector('#wg-status');
  const bird = document.querySelector('#bird-status');
  if (wg) setDefinitionList(wg, [['Interface', status.wireGuard.interface], ['State', status.wireGuard.state]]);
  if (bird) setDefinitionList(bird, [['State', status.bird.state]]);
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
  message.textContent = 'Saved';
}

document.querySelectorAll('[data-diag]').forEach(refreshDiag);
refreshStatus();
loadBirdConfig();

const birdForm = document.querySelector('#bird-form');
if (birdForm) birdForm.addEventListener('submit', saveBirdConfig);

const saveWG = document.querySelector('#save-wg');
if (saveWG) saveWG.addEventListener('click', saveWireGuardConfig);

document.querySelectorAll('[data-routing-action]').forEach((button) => {
  button.addEventListener('click', () => runRoutingAction(button.dataset.routingAction));
});

if (window.EventSource) {
  const events = new EventSource('/api/events');
  events.addEventListener('status', refreshStatus);
}
