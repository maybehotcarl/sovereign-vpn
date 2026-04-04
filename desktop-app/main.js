const { app, BrowserWindow, dialog, ipcMain, shell } = require('electron');
const { execFile } = require('node:child_process');
const fs = require('node:fs/promises');
const http = require('node:http');
const path = require('node:path');
const { promisify } = require('node:util');

const execFileAsync = promisify(execFile);

const APP_PROTOCOL = 'sovereignvpn';
const APP_URL = 'https://6529vpn.io';
const LOCAL_HANDOFF_PORT = 9469;
const INTERFACE_NAME = '6529vpn';
const ALLOWED_HANDOFF_ORIGINS = new Set([
  APP_URL,
  'http://127.0.0.1:5173',
  'http://localhost:5173',
]);

let mainWindow = null;
let disconnectTimer = null;
let queuedPayload = null;
let handoffServer = null;

function appPaths() {
  const userData = app.getPath('userData');
  return {
    userData,
    stateFile: path.join(userData, 'desktop-state.json'),
    configFile: path.join(userData, `${INTERFACE_NAME}.conf`),
  };
}

function emitState(state) {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('desktop:state-changed', redactState(state));
  }
}

function redactState(state) {
  if (!state) {
    return {
      connected: false,
      accessMode: null,
      expiresAt: null,
      serverEndpoint: null,
      sessionLabel: null,
      lastError: null,
      lastDisconnectReason: null,
      connectedAt: null,
      disconnectAt: null,
    };
  }

  return {
    connected: Boolean(state.connected),
    accessMode: state.accessMode || null,
    expiresAt: state.expiresAt || null,
    serverEndpoint: state.serverEndpoint || null,
    sessionLabel: state.sessionLabel || null,
    lastError: state.lastError || null,
    lastDisconnectReason: state.lastDisconnectReason || null,
    connectedAt: state.connectedAt || null,
    disconnectAt: state.disconnectAt || null,
  };
}

async function loadState() {
  try {
    const raw = await fs.readFile(appPaths().stateFile, 'utf8');
    return JSON.parse(raw);
  } catch (error) {
    return {
      connected: false,
      accessMode: null,
      expiresAt: null,
      serverEndpoint: null,
      sessionLabel: null,
      lastError: null,
      lastDisconnectReason: null,
      connectedAt: null,
      disconnectAt: null,
    };
  }
}

async function saveState(state) {
  await fs.mkdir(appPaths().userData, { recursive: true });
  await fs.writeFile(appPaths().stateFile, JSON.stringify(state, null, 2), 'utf8');
}

function shellEscape(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

function escapeAppleScript(value) {
  return String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

function isIsoDate(value) {
  if (typeof value !== 'string' || !value.trim()) {
    return false;
  }
  return !Number.isNaN(Date.parse(value));
}

function normalizeConnectPayload(payload) {
  if (!payload || typeof payload !== 'object') {
    throw new Error('Desktop handoff payload is missing.');
  }

  if (typeof payload.vpnConfig !== 'string' || !payload.vpnConfig.includes('[Interface]')) {
    throw new Error('Desktop handoff payload is missing a valid WireGuard config.');
  }

  if (!isIsoDate(payload.expiresAt)) {
    throw new Error('Desktop handoff payload is missing a valid expiry time.');
  }

  return {
    vpnConfig: payload.vpnConfig,
    expiresAt: new Date(payload.expiresAt).toISOString(),
    accessMode: payload.accessMode === 'anonymous' ? 'anonymous' : 'direct',
    serverEndpoint: typeof payload.serverEndpoint === 'string' ? payload.serverEndpoint : '',
    sessionLabel: typeof payload.sessionLabel === 'string' ? payload.sessionLabel : 'Sovereign VPN',
  };
}

function parseConnectUrl(urlValue) {
  const parsed = new URL(urlValue);
  if (parsed.protocol !== `${APP_PROTOCOL}:`) {
    throw new Error('Unsupported URL protocol.');
  }
  const payload = parsed.searchParams.get('payload');
  if (!payload) {
    return null;
  }
  const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized + '='.repeat((4 - (normalized.length % 4 || 4)) % 4);
  return normalizeConnectPayload(JSON.parse(Buffer.from(padded, 'base64').toString('utf8')));
}

async function findExecutable(candidates) {
  for (const candidate of candidates) {
    try {
      await fs.access(candidate);
      return candidate;
    } catch {
      // keep trying
    }
  }
  return null;
}

async function runElevatedConnect(configPath) {
  if (process.platform === 'win32') {
    const wireguardExe = await findExecutable([
      path.join(process.env['ProgramFiles'] || 'C:\\Program Files', 'WireGuard', 'wireguard.exe'),
    ]);
    if (!wireguardExe) {
      throw new Error('WireGuard for Windows is not installed. Install it from wireguard.com/install first.');
    }
    await execFileAsync(wireguardExe, ['/installtunnelservice', configPath]);
    return;
  }

  if (process.platform === 'darwin') {
    const wgQuick = await findExecutable([
      '/opt/homebrew/bin/wg-quick',
      '/usr/local/bin/wg-quick',
      '/usr/bin/wg-quick',
    ]);
    if (!wgQuick) {
      throw new Error('Install wireguard-tools first, for example with `brew install wireguard-tools`.');
    }
    const command = `${shellEscape(wgQuick)} up ${shellEscape(configPath)}`;
    await execFileAsync('osascript', [
      '-e',
      `do shell script "${escapeAppleScript(command)}" with administrator privileges`,
    ]);
    return;
  }

  const pkexec = await findExecutable(['/usr/bin/pkexec', '/bin/pkexec']);
  const wgQuick = await findExecutable(['/usr/bin/wg-quick', '/bin/wg-quick']);
  if (!wgQuick) {
    throw new Error('Install wireguard-tools first, for example with `sudo apt install wireguard-tools openresolv`.');
  }
  if (!pkexec) {
    throw new Error('pkexec is required for the desktop helper on Linux so the app can bring the WireGuard tunnel up with root privileges.');
  }
  await execFileAsync(pkexec, [wgQuick, 'up', configPath]);
}

async function runElevatedDisconnect() {
  if (process.platform === 'win32') {
    const wireguardExe = await findExecutable([
      path.join(process.env['ProgramFiles'] || 'C:\\Program Files', 'WireGuard', 'wireguard.exe'),
    ]);
    if (!wireguardExe) {
      return;
    }
    await execFileAsync(wireguardExe, ['/uninstalltunnelservice', INTERFACE_NAME]);
    return;
  }

  if (process.platform === 'darwin') {
    const wgQuick = await findExecutable([
      '/opt/homebrew/bin/wg-quick',
      '/usr/local/bin/wg-quick',
      '/usr/bin/wg-quick',
    ]);
    if (!wgQuick) {
      return;
    }
    const command = `${shellEscape(wgQuick)} down ${shellEscape(appPaths().configFile)}`;
    await execFileAsync('osascript', [
      '-e',
      `do shell script "${escapeAppleScript(command)}" with administrator privileges`,
    ]);
    return;
  }

  const pkexec = await findExecutable(['/usr/bin/pkexec', '/bin/pkexec']);
  const wgQuick = await findExecutable(['/usr/bin/wg-quick', '/bin/wg-quick']);
  if (!pkexec || !wgQuick) {
    return;
  }
  await execFileAsync(pkexec, [wgQuick, 'down', appPaths().configFile]);
}

function clearDisconnectTimer() {
  if (disconnectTimer) {
    clearTimeout(disconnectTimer);
    disconnectTimer = null;
  }
}

async function disconnectTunnel(reason = 'user') {
  clearDisconnectTimer();

  const currentState = await loadState();
  try {
    await runElevatedDisconnect();
  } catch (error) {
    if (!String(error.stderr || error.message || '').includes('not a WireGuard interface')) {
      currentState.lastError = error.message || String(error);
    }
  }

  try {
    await fs.rm(appPaths().configFile, { force: true });
  } catch {
    // ignore
  }

  const nextState = {
    ...currentState,
    connected: false,
    lastDisconnectReason: reason,
    disconnectAt: new Date().toISOString(),
  };
  await saveState(nextState);
  emitState(nextState);
  return redactState(nextState);
}

async function scheduleExpiryGuard(state) {
  clearDisconnectTimer();

  if (!state.connected || !state.expiresAt) {
    return;
  }

  const msUntilExpiry = new Date(state.expiresAt).getTime() - Date.now();
  if (msUntilExpiry <= 0) {
    await disconnectTunnel('expired');
    return;
  }

  disconnectTimer = setTimeout(() => {
    disconnectTunnel('expired').catch((error) => {
      console.error('Failed to auto-disconnect expired tunnel:', error);
    });
  }, msUntilExpiry);
}

async function connectTunnel(payload) {
  const normalized = normalizeConnectPayload(payload);
  const { configFile, userData } = appPaths();

  await fs.mkdir(userData, { recursive: true });
  await fs.writeFile(configFile, normalized.vpnConfig, { mode: 0o600 });

  try {
    await runElevatedConnect(configFile);
  } catch (error) {
    const failedState = {
      ...(await loadState()),
      connected: false,
      lastError: error.message || String(error),
      lastDisconnectReason: null,
      disconnectAt: null,
    };
    await saveState(failedState);
    emitState(failedState);
    throw error;
  }

  const nextState = {
    connected: true,
    accessMode: normalized.accessMode,
    expiresAt: normalized.expiresAt,
    serverEndpoint: normalized.serverEndpoint,
    sessionLabel: normalized.sessionLabel,
    lastError: null,
    lastDisconnectReason: null,
    connectedAt: new Date().toISOString(),
    disconnectAt: null,
  };

  await saveState(nextState);
  await scheduleExpiryGuard(nextState);
  emitState(nextState);
  return redactState(nextState);
}

function currentPlatformInfo() {
  if (process.platform === 'win32') {
    return {
      platform: 'windows',
      protocol: APP_PROTOCOL,
      appUrl: APP_URL,
      handoffUrl: `http://127.0.0.1:${LOCAL_HANDOFF_PORT}/handoff`,
      nativeSupport: true,
      helperSummary: 'Handles WireGuard tunnel install/uninstall using the Windows WireGuard client.',
    };
  }
  if (process.platform === 'darwin') {
    return {
      platform: 'macos',
      protocol: APP_PROTOCOL,
      appUrl: APP_URL,
      handoffUrl: `http://127.0.0.1:${LOCAL_HANDOFF_PORT}/handoff`,
      nativeSupport: true,
      helperSummary: 'Uses wireguard-tools plus an administrator prompt to bring the tunnel up and down automatically.',
    };
  }
  return {
    platform: 'linux',
    protocol: APP_PROTOCOL,
    appUrl: APP_URL,
    handoffUrl: `http://127.0.0.1:${LOCAL_HANDOFF_PORT}/handoff`,
    nativeSupport: true,
    helperSummary: 'Uses pkexec plus wg-quick so the app can manage the tunnel lifecycle and auto-disconnect on expiry.',
  };
}

function queueOrDispatchPayload(payload) {
  queuedPayload = payload;
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('desktop:handoff', payload);
  }
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 980,
    height: 720,
    minWidth: 900,
    minHeight: 640,
    title: '6529 VPN Desktop',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, 'index.html'));
  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  mainWindow.webContents.once('did-finish-load', () => {
    if (queuedPayload) {
      mainWindow.webContents.send('desktop:handoff', queuedPayload);
    }
  });
}

async function bootstrapExistingState() {
  const currentState = await loadState();
  emitState(currentState);
  await scheduleExpiryGuard(currentState);
}

function handleProtocolUrl(urlValue) {
  try {
    const payload = parseConnectUrl(urlValue);
    if (payload) {
      queueOrDispatchPayload(payload);
      return;
    }
    if (mainWindow) {
      if (mainWindow.isMinimized()) {
        mainWindow.restore();
      }
      mainWindow.focus();
    }
  } catch (error) {
    dialog.showErrorBox('Invalid 6529 VPN desktop handoff', error.message || String(error));
  }
}

function scanArgvForProtocolUrl(argv) {
  const match = argv.find((value) => typeof value === 'string' && value.startsWith(`${APP_PROTOCOL}://`));
  if (match) {
    handleProtocolUrl(match);
  }
}

ipcMain.handle('desktop:get-state', async () => redactState(await loadState()));
ipcMain.handle('desktop:get-platform-info', async () => currentPlatformInfo());
ipcMain.handle('desktop:connect-payload', async (_event, payload) => connectTunnel(payload));
ipcMain.handle('desktop:disconnect', async () => disconnectTunnel('user'));
ipcMain.handle('desktop:open-website', async () => shell.openExternal(APP_URL));

function writeCorsHeaders(response, origin) {
  if (origin && ALLOWED_HANDOFF_ORIGINS.has(origin)) {
    response.setHeader('Access-Control-Allow-Origin', origin);
    response.setHeader('Vary', 'Origin');
    response.setHeader('Access-Control-Allow-Private-Network', 'true');
  }
  response.setHeader('Access-Control-Allow-Headers', 'Content-Type');
  response.setHeader('Access-Control-Allow-Methods', 'POST, OPTIONS, GET');
}

function startLocalHandoffServer() {
  if (handoffServer) {
    return;
  }

  handoffServer = http.createServer(async (request, response) => {
    const origin = request.headers.origin;
    writeCorsHeaders(response, origin);

    if (request.method === 'OPTIONS') {
      response.writeHead(204);
      response.end();
      return;
    }

    if (request.method === 'GET' && request.url === '/health') {
      response.writeHead(200, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({
        status: 'ok',
        protocol: APP_PROTOCOL,
        handoffUrl: `http://127.0.0.1:${LOCAL_HANDOFF_PORT}/handoff`,
      }));
      return;
    }

    if (request.method === 'POST' && request.url === '/handoff') {
      if (origin && !ALLOWED_HANDOFF_ORIGINS.has(origin)) {
        response.writeHead(403, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify({ error: 'origin_not_allowed' }));
        return;
      }

      try {
        const chunks = [];
        for await (const chunk of request) {
          chunks.push(chunk);
        }
        const raw = Buffer.concat(chunks).toString('utf8');
        const payload = normalizeConnectPayload(JSON.parse(raw));

        if (mainWindow) {
          if (mainWindow.isMinimized()) {
            mainWindow.restore();
          }
          mainWindow.focus();
        }

        const state = await connectTunnel(payload);
        response.writeHead(200, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify({ success: true, state }));
      } catch (error) {
        response.writeHead(400, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify({ error: error.message || String(error) }));
      }
      return;
    }

    response.writeHead(404, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ error: 'not_found' }));
  });

  handoffServer.listen(LOCAL_HANDOFF_PORT, '127.0.0.1');
}

const gotSingleInstanceLock = app.requestSingleInstanceLock();
if (!gotSingleInstanceLock) {
  app.quit();
} else {
  app.on('second-instance', (_event, argv) => {
    if (mainWindow) {
      if (mainWindow.isMinimized()) {
        mainWindow.restore();
      }
      mainWindow.focus();
    }
    scanArgvForProtocolUrl(argv);
  });

  app.on('open-url', (event, urlValue) => {
    event.preventDefault();
    handleProtocolUrl(urlValue);
  });

  app.whenReady().then(async () => {
    app.setAsDefaultProtocolClient(APP_PROTOCOL);
    startLocalHandoffServer();
    createWindow();
    await bootstrapExistingState();
    scanArgvForProtocolUrl(process.argv);

    app.on('activate', () => {
      if (BrowserWindow.getAllWindows().length === 0) {
        createWindow();
      }
    });
  });
}

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('will-quit', () => {
  if (handoffServer) {
    handoffServer.close();
    handoffServer = null;
  }
});
