import { useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { useAccount } from 'wagmi';
import {
  NODE_REGISTRY_ADDRESS,
  PAYOUT_VAULT_ADDRESS,
  SUBSCRIPTION_MANAGER_ADDRESS,
} from './contracts';

const INSTALL_SCRIPT_URL = 'https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh';
const DEFAULT_GATEWAY_PORT = '8080';
const DEFAULT_WG_PORT = '51820';
const DEFAULT_REGION = 'us-east';

function generateEnrollmentToken() {
  const bytes = new Uint8Array(12);
  if (window.crypto?.getRandomValues) {
    window.crypto.getRandomValues(bytes);
    return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
  }
  return `${Date.now().toString(16)}${Math.random().toString(16).slice(2, 10)}`;
}

function shellQuote(value) {
  const text = String(value);
  if (/^[A-Za-z0-9_./:=@%+-]+$/.test(text)) return text;
  return `'${text.replace(/'/g, `'\\''`)}'`;
}

function gatewayUrlFromHost(host, port) {
  const trimmed = host.trim();
  if (!trimmed) return `http://127.0.0.1:${port}`;
  if (/^https?:\/\//i.test(trimmed)) return trimmed.replace(/\/+$/, '');
  return `http://${trimmed}:${port}`;
}

function buildInstallCommand({
  address,
  enrollmentToken,
  region,
  publicHost,
  rpcUrl,
  gatewayPort,
  wgPort,
  nodeRegistry,
  subscriptionManager,
  payoutVault,
  delegationEnabled,
}) {
  const argLines = [
    `--enroll ${shellQuote(enrollmentToken)}`,
    `--region ${shellQuote(region)}`,
  ];

  if (address) argLines.push(`--operator ${shellQuote(address)}`);
  if (rpcUrl.trim()) argLines.push(`--eth-rpc ${shellQuote(rpcUrl.trim())}`);
  if (publicHost.trim()) argLines.push(`--public-ip ${shellQuote(publicHost.trim())}`);
  if (gatewayPort !== DEFAULT_GATEWAY_PORT) argLines.push(`--gateway-port ${shellQuote(gatewayPort)}`);
  if (wgPort !== DEFAULT_WG_PORT) argLines.push(`--wg-port ${shellQuote(wgPort)}`);
  if (nodeRegistry.trim()) argLines.push(`--node-registry ${shellQuote(nodeRegistry.trim())}`);
  if (subscriptionManager.trim()) argLines.push(`--subscription-manager ${shellQuote(subscriptionManager.trim())}`);
  if (payoutVault.trim()) argLines.push(`--payout-vault ${shellQuote(payoutVault.trim())}`);
  if (delegationEnabled) argLines.push('--enable-delegation');

  const formattedArgs = argLines.join(' \\\n      ');
  return `curl -fsSL ${INSTALL_SCRIPT_URL} \\\n  | sudo bash -s -- \\\n      ${formattedArgs}`;
}

export default function OperatorEnrollment() {
  const { address, isConnected } = useAccount();
  const [enrollmentToken, setEnrollmentToken] = useState(() => generateEnrollmentToken());
  const [publicHost, setPublicHost] = useState('');
  const [rpcUrl, setRpcUrl] = useState('');
  const [region, setRegion] = useState(DEFAULT_REGION);
  const [gatewayPort, setGatewayPort] = useState(DEFAULT_GATEWAY_PORT);
  const [wgPort, setWgPort] = useState(DEFAULT_WG_PORT);
  const [nodeRegistry, setNodeRegistry] = useState(NODE_REGISTRY_ADDRESS);
  const [subscriptionManager, setSubscriptionManager] = useState(SUBSCRIPTION_MANAGER_ADDRESS);
  const [payoutVault, setPayoutVault] = useState(PAYOUT_VAULT_ADDRESS);
  const [delegationEnabled, setDelegationEnabled] = useState(true);
  const [copyLabel, setCopyLabel] = useState('Copy');
  const [healthUrl, setHealthUrl] = useState('');
  const [healthState, setHealthState] = useState({ status: 'idle', message: '' });

  const installCommand = useMemo(() => buildInstallCommand({
    address,
    enrollmentToken,
    region,
    publicHost,
    rpcUrl,
    gatewayPort,
    wgPort,
    nodeRegistry,
    subscriptionManager,
    payoutVault,
    delegationEnabled,
  }), [
    address,
    delegationEnabled,
    enrollmentToken,
    gatewayPort,
    nodeRegistry,
    payoutVault,
    publicHost,
    region,
    rpcUrl,
    subscriptionManager,
    wgPort,
  ]);

  const defaultHealthUrl = gatewayUrlFromHost(publicHost, gatewayPort);

  const copyCommand = async () => {
    await navigator.clipboard.writeText(installCommand);
    setCopyLabel('Copied');
    setTimeout(() => setCopyLabel('Copy'), 1800);
  };

  const checkHealth = async () => {
    const baseUrl = (healthUrl.trim() || defaultHealthUrl).replace(/\/+$/, '');
    setHealthState({ status: 'checking', message: `Checking ${baseUrl}/health` });
    try {
      const resp = await fetch(`${baseUrl}/health`);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      if (data.status !== 'ok') throw new Error('Gateway did not report ok');
      setHealthState({
        status: 'ok',
        message: `Online with ${data.active_peers || 0} active peer${data.active_peers === 1 ? '' : 's'}`,
      });
    } catch (err) {
      setHealthState({
        status: 'error',
        message: err instanceof Error ? err.message : 'Health check failed',
      });
    }
  };

  return (
    <section className="operator-panel">
      <div className="operator-header">
        <div>
          <h2>Run a Node</h2>
          <p>Generate an installer command, run it on a VPS, then verify the gateway.</p>
        </div>
        <ConnectButton />
      </div>

      <div className="operator-checks">
        <div className={`operator-check${isConnected ? ' done' : ''}`}>
          <span>1</span>
          <strong>Wallet</strong>
          <small>{isConnected ? `${address.slice(0, 6)}...${address.slice(-4)}` : 'Connect operator wallet'}</small>
        </div>
        <div className="operator-check done">
          <span>2</span>
          <strong>Install</strong>
          <small>Paste command on Ubuntu VPS</small>
        </div>
        <div className={`operator-check${healthState.status === 'ok' ? ' done' : healthState.status === 'error' ? ' error' : ''}`}>
          <span>3</span>
          <strong>Verify</strong>
          <small>{healthState.message || 'Run health check'}</small>
        </div>
      </div>

      <div className="operator-grid">
        <label className="field">
          <span>Public IP or DNS</span>
          <input
            value={publicHost}
            onChange={e => setPublicHost(e.target.value)}
            placeholder="Auto-detect on server"
          />
        </label>

        <label className="field">
          <span>Region</span>
          <select value={region} onChange={e => setRegion(e.target.value)}>
            <option value="us-east">US East</option>
            <option value="us-west">US West</option>
            <option value="eu-west">EU West</option>
            <option value="eu-central">EU Central</option>
            <option value="ap-southeast">AP Southeast</option>
          </select>
        </label>

        <label className="field">
          <span>Gateway Port</span>
          <input value={gatewayPort} onChange={e => setGatewayPort(e.target.value)} inputMode="numeric" />
        </label>

        <label className="field">
          <span>WireGuard Port</span>
          <input value={wgPort} onChange={e => setWgPort(e.target.value)} inputMode="numeric" />
        </label>

        <label className="field wide">
          <span>Ethereum RPC</span>
          <input
            value={rpcUrl}
            onChange={e => setRpcUrl(e.target.value)}
            placeholder="Use installer default, or paste Alchemy/Infura URL"
          />
        </label>

        <label className="field wide">
          <span>NodeRegistry</span>
          <input value={nodeRegistry} onChange={e => setNodeRegistry(e.target.value)} placeholder="0x..." />
        </label>

        <label className="field wide">
          <span>SubscriptionManager</span>
          <input value={subscriptionManager} onChange={e => setSubscriptionManager(e.target.value)} placeholder="0x..." />
        </label>

        <label className="field wide">
          <span>PayoutVault</span>
          <input value={payoutVault} onChange={e => setPayoutVault(e.target.value)} placeholder="0x..." />
        </label>
      </div>

      <label className="toggle-row">
        <input
          type="checkbox"
          checked={delegationEnabled}
          onChange={e => setDelegationEnabled(e.target.checked)}
        />
        <span>Enable delegated wallet checks</span>
      </label>

      <div className="command-card">
        <div className="command-card-header">
          <div>
            <strong>Install Command</strong>
            <small>Run this on the VPS as root.</small>
          </div>
          <div className="command-actions">
            <button className="btn-secondary small" onClick={() => setEnrollmentToken(generateEnrollmentToken())}>
              New Token
            </button>
            <button className="btn-primary small" onClick={copyCommand}>
              {copyLabel}
            </button>
          </div>
        </div>
        <pre className="command-output">{installCommand}</pre>
      </div>

      <div className="verify-card">
        <label className="field">
          <span>Gateway URL</span>
          <input
            value={healthUrl}
            onChange={e => setHealthUrl(e.target.value)}
            placeholder={defaultHealthUrl}
          />
        </label>
        <button
          className="btn-primary"
          onClick={checkHealth}
          disabled={healthState.status === 'checking'}
        >
          {healthState.status === 'checking' ? 'Checking...' : 'Check Gateway'}
        </button>
      </div>
    </section>
  );
}
