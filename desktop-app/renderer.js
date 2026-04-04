const stateText = document.getElementById('statusText');
const statusDot = document.getElementById('statusDot');
const logEl = document.getElementById('log');
const detailMode = document.getElementById('detailMode');
const detailExpiry = document.getElementById('detailExpiry');
const detailServer = document.getElementById('detailServer');
const detailDisconnect = document.getElementById('detailDisconnect');
const platformSummary = document.getElementById('platformSummary');
const disconnectButton = document.getElementById('disconnect');
const openWebsiteButton = document.getElementById('openWebsite');

let latestState = null;

function formatDateTime(value) {
  if (!value) {
    return '—';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function renderState(state) {
  latestState = state;
  if (state.connected) {
    stateText.textContent = 'Tunnel connected';
    statusDot.className = 'dot connected';
  } else if (state.lastError) {
    stateText.textContent = 'Tunnel needs attention';
    statusDot.className = 'dot error';
  } else {
    stateText.textContent = 'Desktop companion ready';
    statusDot.className = 'dot';
  }

  detailMode.textContent = state.connected
    ? (state.accessMode === 'anonymous' ? 'Anonymous session' : 'Direct wallet session')
    : 'Idle';
  detailExpiry.textContent = state.connected ? formatDateTime(state.expiresAt) : 'Not connected';
  detailServer.textContent = state.serverEndpoint || '—';
  detailDisconnect.textContent = state.lastDisconnectReason
    ? `${state.lastDisconnectReason} at ${formatDateTime(state.disconnectAt)}`
    : '—';

  disconnectButton.disabled = !state.connected;
}

function setLog(message) {
  logEl.textContent = message;
}

async function connectPayload(payload) {
  try {
    setLog(`Incoming handoff received.\nConnecting ${payload.accessMode === 'anonymous' ? 'anonymous' : 'direct'} session and requesting OS permissions…`);
    const nextState = await window.sovereignDesktop.connectPayload(payload);
    renderState(nextState);
    setLog(`Connected.\nThe tunnel will auto-disconnect at ${formatDateTime(nextState.expiresAt)}.`);
  } catch (error) {
    setLog(`Connect failed.\n${error.message || String(error)}`);
  }
}

async function bootstrap() {
  const [state, platform] = await Promise.all([
    window.sovereignDesktop.getState(),
    window.sovereignDesktop.getPlatformInfo(),
  ]);

  renderState(state);
  platformSummary.textContent = platform.helperSummary;

  if (state.connected) {
    setLog(`Tunnel connected.\nAuto-disconnect is scheduled for ${formatDateTime(state.expiresAt)}.`);
  }
}

window.sovereignDesktop.onStateChanged((state) => {
  renderState(state);
});

window.sovereignDesktop.onHandoff((payload) => {
  connectPayload(payload);
});

disconnectButton.addEventListener('click', async () => {
  try {
    setLog('Disconnecting tunnel…');
    const nextState = await window.sovereignDesktop.disconnect();
    renderState(nextState);
    setLog('Tunnel disconnected.');
  } catch (error) {
    setLog(`Disconnect failed.\n${error.message || String(error)}`);
  }
});

openWebsiteButton.addEventListener('click', async () => {
  await window.sovereignDesktop.openWebsite();
});

bootstrap().catch((error) => {
  setLog(`Startup failed.\n${error.message || String(error)}`);
});
