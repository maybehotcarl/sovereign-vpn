import { useEffect, useMemo, useState } from 'react';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { useAccount, useSignMessage } from 'wagmi';
import {
  NODE_REGISTRY_ADDRESS,
  PAYOUT_VAULT_ADDRESS,
  SUBSCRIPTION_MANAGER_ADDRESS,
} from './contracts';

const INSTALL_SCRIPT_URL = 'https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh';
const DEFAULT_GATEWAY_PORT = '8080';
const DEFAULT_WG_PORT = '51820';
const DEFAULT_REGION = 'us-east';
const OPERATOR_API_BASE = (import.meta.env.VITE_CONTROL_PLANE_URL || '').replace(/\/+$/, '');

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

function operatorApi(path) {
  return `${OPERATOR_API_BASE}${path}`;
}

function currentControlPlaneUrl() {
  if (OPERATOR_API_BASE) return OPERATOR_API_BASE;
  return typeof window === 'undefined' ? '' : window.location.origin;
}

async function readJson(resp) {
  try {
    return await resp.json();
  } catch {
    return {};
  }
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
  controlPlaneUrl,
}) {
  const argLines = [
    `--enroll ${shellQuote(enrollmentToken)}`,
    `--region ${shellQuote(region)}`,
    `--control-plane-url ${shellQuote(controlPlaneUrl)}`,
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
  const { signMessageAsync } = useSignMessage();
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
  const [enrollment, setEnrollment] = useState(null);
  const [enrollmentState, setEnrollmentState] = useState({ status: 'idle', message: '' });

  const controlPlaneUrl = currentControlPlaneUrl();
  const enrollmentToken = enrollment?.token || '';

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
    controlPlaneUrl,
  }), [
    address,
    controlPlaneUrl,
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
  const hasEnrollment = Boolean(enrollmentToken);

  useEffect(() => {
    setEnrollment(null);
    setEnrollmentState({ status: 'idle', message: '' });
  }, [address]);

  useEffect(() => {
    if (!enrollment?.token) return undefined;

    const poll = async () => {
      try {
        const resp = await fetch(operatorApi(`/operator/enrollments/${enrollment.token}`));
        if (!resp.ok) return;
        const data = await resp.json();
        setEnrollment(data);
      } catch {
        // Keep the last known status; polling is best-effort.
      }
    };

    const id = setInterval(poll, 5000);
    return () => clearInterval(id);
  }, [enrollment?.token]);

  const createEnrollment = async () => {
    if (!address) {
      setEnrollmentState({ status: 'error', message: 'Connect your operator wallet first.' });
      return;
    }

    setEnrollmentState({ status: 'creating', message: 'Preparing wallet signature...' });
    try {
      const challengeResp = await fetch(operatorApi('/auth/challenge'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address }),
      });
      const challenge = await readJson(challengeResp);
      if (!challengeResp.ok) throw new Error(challenge.error || 'Failed to get signing challenge');

      setEnrollmentState({ status: 'creating', message: 'Sign the operator enrollment message...' });
      const signature = await signMessageAsync({ message: challenge.message });

      setEnrollmentState({ status: 'creating', message: 'Creating enrollment token...' });
      const resp = await fetch(operatorApi('/operator/enrollments'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          operator: address,
          region,
          message: challenge.message,
          signature,
        }),
      });
      const data = await readJson(resp);
      if (!resp.ok) throw new Error(data.error || 'Enrollment failed');
      setEnrollment(data);
      setEnrollmentState({ status: 'ready', message: 'Enrollment token ready.' });
    } catch (err) {
      setEnrollmentState({
        status: 'error',
        message: err instanceof Error ? err.message : 'Enrollment API unavailable',
      });
    }
  };

  const copyCommand = async () => {
    if (!hasEnrollment) return;
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
        <div className={`operator-check${enrollment?.report ? ' done' : hasEnrollment ? '' : enrollmentState.status === 'error' ? ' error' : ''}`}>
          <span>2</span>
          <strong>Install</strong>
          <small>
            {enrollment?.report
              ? 'Node reported from VPS'
              : hasEnrollment
                ? 'Paste command on Ubuntu VPS'
                : enrollmentState.message || 'Create enrollment token'}
          </small>
        </div>
        <div className={`operator-check${enrollment?.status === 'healthy' || healthState.status === 'ok' ? ' done' : healthState.status === 'error' ? ' error' : ''}`}>
          <span>3</span>
          <strong>Verify</strong>
          <small>
            {enrollment?.status === 'healthy'
              ? 'Installer reported healthy gateway'
              : healthState.message || 'Run health check'}
          </small>
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
            <small>
              {hasEnrollment
                ? 'Run this on the VPS as root.'
                : 'Connect your wallet and create an enrollment token first.'}
            </small>
          </div>
          <div className="command-actions">
            <button
              className="btn-secondary small"
              onClick={createEnrollment}
              disabled={!isConnected || enrollmentState.status === 'creating'}
            >
              {enrollmentState.status === 'creating'
                ? 'Creating...'
                : hasEnrollment
                  ? 'New Enrollment'
                  : 'Create Enrollment'}
            </button>
            <button className="btn-primary small" onClick={copyCommand} disabled={!hasEnrollment}>
              {copyLabel}
            </button>
          </div>
        </div>
        {enrollmentState.status === 'error' && (
          <div className="operator-error">{enrollmentState.message}</div>
        )}
        {hasEnrollment ? (
          <pre className="command-output">{installCommand}</pre>
        ) : (
          <div className="command-placeholder">No enrollment token yet.</div>
        )}
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
