import { useState, useEffect, useCallback } from 'react';
import { useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { formatEther } from 'viem';
import { SUBSCRIPTION_MANAGER_ADDRESS, SUBSCRIPTION_MANAGER_ABI } from './contracts';

const TIER_LABELS = {
  '24h': '24 Hours',
  '7d': '7 Days',
  '30d': '30 Days',
  '90d': '90 Days',
  '365d': '365 Days',
};

function formatDuration(seconds) {
  const days = Math.floor(seconds / 86400);
  if (days >= 365) return '365d';
  if (days >= 90) return '90d';
  if (days >= 30) return '30d';
  if (days >= 7) return '7d';
  return '24h';
}

function formatTimeLeft(expiresAt) {
  const diff = new Date(expiresAt) - Date.now();
  if (diff <= 0) return null;
  const days = Math.floor(diff / 86400000);
  const hours = Math.floor((diff % 86400000) / 3600000);
  const mins = Math.floor((diff % 3600000) / 60000);
  const secs = Math.floor((diff % 60000) / 1000);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m ${secs}s`;
  if (mins > 0) return `${mins}m ${secs}s`;
  return `${secs}s`;
}

function daysRemaining(expiresAt) {
  const diff = new Date(expiresAt) - Date.now();
  return diff / 86400000;
}

function formatDateTime(value) {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return date.toLocaleString();
}

export default function SessionDashboard({ session, onDisconnect, onReconnect, onRenew }) {
  const [timeLeft, setTimeLeft] = useState(() => formatTimeLeft(session.expiresAt));
  const [disconnecting, setDisconnecting] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [copyLabel, setCopyLabel] = useState('Copy');
  const [payoutInfo, setPayoutInfo] = useState(null);

  // Renewal state
  const [renewPhase, setRenewPhase] = useState('idle'); // idle|picking|sending|confirming|done|error
  const [renewTiers, setRenewTiers] = useState([]);
  const [renewSelected, setRenewSelected] = useState(null);
  const [renewTxHash, setRenewTxHash] = useState(null);
  const [renewError, setRenewError] = useState('');

  const { writeContractAsync } = useWriteContract();
  const { isSuccess: renewTxConfirmed, isError: renewTxFailed } = useWaitForTransactionReceipt({
    hash: renewTxHash,
  });

  const isAnonymousAccess = session.accessMode === 'anonymous' || session.address === 'anonymous';
  const subscriptionExpiresAt = session.subscriptionExpiresAt || null;
  const subscriptionTimeLeft = subscriptionExpiresAt ? formatTimeLeft(subscriptionExpiresAt) : null;
  const subscriptionExpired = Boolean(subscriptionExpiresAt) && !subscriptionTimeLeft;
  const formattedSubscriptionExpiry = formatDateTime(subscriptionExpiresAt);
  const expired = !timeLeft;
  const isSubscription = session.tier === 'subscription' || Boolean(subscriptionExpiresAt);
  const renewReferenceExpiry = subscriptionExpiresAt || session.expiresAt;
  const showRenew = isSubscription && !(subscriptionExpiresAt ? subscriptionExpired : expired) && daysRemaining(renewReferenceExpiry) < 7;
  const wireGuardInstallUrl = 'https://www.wireguard.com/install/';

  // Fetch payout info for the connected node operator
  useEffect(() => {
    if (!session.gatewayUrl || !session.nodeOperator) return;
    fetch(`${session.gatewayUrl}/payout/status?operator=${session.nodeOperator}`)
      .then(r => r.ok ? r.json() : null)
      .then(data => { if (data) setPayoutInfo(data); })
      .catch(() => {});
  }, [session.gatewayUrl, session.nodeOperator]);

  // Live countdown
  useEffect(() => {
    if (expired) return;
    const id = setInterval(() => {
      setTimeLeft(formatTimeLeft(session.expiresAt));
    }, 1000);
    return () => clearInterval(id);
  }, [session.expiresAt, expired]);

  // Watch renewal TX
  useEffect(() => {
    if (renewTxConfirmed && renewPhase === 'confirming' && renewSelected) {
      const currentExpiry = Math.floor(new Date(session.expiresAt).getTime() / 1000);
      const now = Math.floor(Date.now() / 1000);
      const newExpiresAt = Math.max(now, currentExpiry) + renewSelected.duration;
      if (onRenew) onRenew(newExpiresAt);
      setRenewPhase('done');
      setTimeout(() => setRenewPhase('idle'), 3000);
    }
    if (renewTxFailed && renewPhase === 'confirming') {
      setRenewError('Transaction failed on-chain');
      setRenewPhase('error');
    }
  }, [renewTxConfirmed, renewTxFailed]);

  const handleDisconnect = useCallback(async () => {
    setDisconnecting(true);
    try {
      const disconnect = async (baseUrl) => fetch(`${baseUrl}/vpn/disconnect`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_token: session.sessionToken,
          public_key: session.publicKey,
        }),
      });
      let resp = await disconnect(session.gatewayUrl);
      if (resp.status === 409) {
        const data = await resp.json().catch(() => null);
        if (data?.gateway_url && data.gateway_url !== session.gatewayUrl) {
          resp = await disconnect(data.gateway_url);
        }
      }
    } catch (err) {
      console.error('Disconnect error:', err);
    }
    onDisconnect();
  }, [session, onDisconnect]);

  const handleRenewClick = useCallback(async () => {
    setRenewPhase('picking');
    setRenewError('');
    setRenewSelected(null);
    setRenewTxHash(null);
    try {
      let resp = await fetch('/subscription/tiers');
      if (!resp.ok) {
        resp = await fetch(`${session.gatewayUrl}/subscription/tiers`);
      }
      if (!resp.ok) throw new Error('Failed to fetch tiers');
      const data = await resp.json();
      setRenewTiers(data.tiers.map(t => ({
        id: t.id,
        durationKey: formatDuration(t.duration_seconds),
        duration: t.duration_seconds,
        costWei: t.price_wei,
        costEth: formatEther(BigInt(t.price_wei)),
        contract: data.contract,
      })));
    } catch (err) {
      setRenewError(err.message);
      setRenewPhase('error');
    }
  }, [session.gatewayUrl]);

  const handleConfirmRenew = useCallback(async () => {
    if (!renewSelected) return;
    setRenewPhase('sending');
    try {
      const hash = await writeContractAsync({
        address: renewSelected.contract,
        abi: SUBSCRIPTION_MANAGER_ABI,
        functionName: 'renewSubscription',
        args: [renewSelected.id, '0x0000000000000000000000000000000000000000'],
        value: BigInt(renewSelected.costWei),
      });
      setRenewTxHash(hash);
      setRenewPhase('confirming');
    } catch (err) {
      setRenewError(err.message || 'Renewal failed');
      setRenewPhase('error');
    }
  }, [renewSelected, writeContractAsync]);

  const downloadConfig = () => {
    const blob = new Blob([session.vpnConfig], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = '6529vpn.conf';
    a.click();
    URL.revokeObjectURL(url);
  };

  const copyConfig = async () => {
    await navigator.clipboard.writeText(session.vpnConfig);
    setCopyLabel('Copied!');
    setTimeout(() => setCopyLabel('Copy'), 2000);
  };

  // Expired state
  if (expired) {
    return (
      <div className="dashboard">
        <div className="dashboard-header expired">
          <div className="dashboard-status">
            <div className="status-dot offline" />
            <span>
              {isAnonymousAccess
                ? 'Anonymous Session Expired'
                : isSubscription
                  ? 'Subscription Expired'
                  : 'Session Expired'}
            </span>
          </div>
        </div>
        <div className="dashboard-body" style={{ textAlign: 'center' }}>
          <p style={{ color: 'var(--muted)', marginBottom: 20 }}>
            {isAnonymousAccess && formattedSubscriptionExpiry && !subscriptionExpired
              ? `Your current anonymous VPN session expired. Your paid subscription is still active until ${formattedSubscriptionExpiry}. Reconnect to issue a fresh anonymous session.`
              : `Your VPN ${isSubscription ? 'subscription' : 'session'} has expired. Reconnect to get a new configuration.`}
          </p>
          <div className="btn-row">
            <button className="btn-primary" onClick={onReconnect}>Reconnect</button>
            <button className="btn-secondary" onClick={onDisconnect} style={{ padding: '14px 24px' }}>Dismiss</button>
          </div>
        </div>
      </div>
    );
  }

  // Active state
  return (
    <div className="dashboard">
      <div className="dashboard-header active">
        <div className="dashboard-status">
          <div className="status-dot" />
          <span>VPN Connected</span>
        </div>
        <div className="dashboard-tier">
          {isAnonymousAccess
            ? 'Anonymous Session'
            : isSubscription
              ? 'Subscription'
              : session.tier}
        </div>
      </div>

      <div className="dashboard-body">
        {isAnonymousAccess && (
          <div
            style={{
              marginBottom: 16,
              padding: '12px',
              border: '1px solid var(--border, #333)',
              borderRadius: 8,
              background: 'rgba(255,255,255,0.03)',
            }}
          >
            <div style={{ fontWeight: 600, marginBottom: 6 }}>
              Purchased access vs current connection
            </div>
            <div style={{ color: 'var(--muted)', fontSize: '0.85rem', lineHeight: 1.5 }}>
              You bought a wallet subscription. This WireGuard config is a short-lived anonymous session issued from that subscription.
              {formattedSubscriptionExpiry ? ` Subscription active until ${formattedSubscriptionExpiry}.` : ''}
            </div>
          </div>
        )}

        <div className="dashboard-stats">
          <div className="stat">
            <div className="stat-label">
              {isAnonymousAccess ? 'Anonymous Session Lease' : 'Time Remaining'}
            </div>
            <div className="stat-value">{timeLeft}</div>
          </div>
          {isAnonymousAccess && (
            <div className="stat">
              <div className="stat-label">Purchased Subscription</div>
              <div className="stat-value">
                {subscriptionTimeLeft || (formattedSubscriptionExpiry ? 'Active' : 'Unknown')}
              </div>
            </div>
          )}
          {isAnonymousAccess && formattedSubscriptionExpiry && (
            <div className="stat">
              <div className="stat-label">Subscription Active Until</div>
              <div className="stat-value" style={{ fontSize: '0.9rem' }}>{formattedSubscriptionExpiry}</div>
            </div>
          )}
          <div className="stat">
            <div className="stat-label">Client IP</div>
            <div className="stat-value mono">{session.clientAddress}</div>
          </div>
          <div className="stat">
            <div className="stat-label">Server</div>
            <div className="stat-value mono">{session.serverEndpoint}</div>
          </div>
          {payoutInfo && payoutInfo.pending_payout_wei && payoutInfo.pending_payout_wei !== '0' && (
            <div className="stat">
              <div className="stat-label">Pending Payout</div>
              <div className="stat-value mono">
                {formatEther(BigInt(payoutInfo.pending_payout_wei))} ETH
              </div>
            </div>
          )}
          {payoutInfo && (
            <div className="stat">
              <div className="stat-label">RAILGUN Address</div>
              <div className="stat-value mono" style={{ fontSize: '0.75rem', wordBreak: 'break-all' }}>
                {payoutInfo.railgun_address || 'Not set'}
              </div>
            </div>
          )}
        </div>

        <div className="dashboard-actions">
          {showRenew && (
            <button
              className="btn-primary"
              onClick={handleRenewClick}
              disabled={renewPhase !== 'idle' && renewPhase !== 'error' && renewPhase !== 'done'}
              style={{ padding: '10px 20px', fontSize: '0.85rem' }}
            >
              Renew
            </button>
          )}
          <button
            className="btn-primary"
            onClick={downloadConfig}
            style={{ padding: '10px 20px', fontSize: '0.85rem' }}
          >
            Download Config
          </button>
          <button
            className="btn-secondary"
            onClick={() => setShowConfig(!showConfig)}
            style={{ padding: '10px 20px', fontSize: '0.85rem' }}
          >
            {showConfig ? 'Hide Raw Config' : 'Advanced: View Raw Config'}
          </button>
          <button
            className="btn-disconnect"
            onClick={handleDisconnect}
            disabled={disconnecting}
          >
            {disconnecting ? 'Disconnecting...' : 'Disconnect'}
          </button>
        </div>

        <div className="setup-panel">
          <h3>Use With WireGuard</h3>
          <p className="setup-copy">
            1. Download your config. 2. Open the WireGuard app. 3. Import the downloaded file and switch the tunnel on.
          </p>
          <div className="btn-row" style={{ justifyContent: 'flex-start' }}>
            <button className="btn-primary" onClick={downloadConfig} style={{ padding: '10px 20px', fontSize: '0.85rem' }}>
              Download Config
            </button>
            <a
              className="btn-secondary"
              href={wireGuardInstallUrl}
              target="_blank"
              rel="noreferrer"
              style={{ padding: '10px 20px', fontSize: '0.85rem', textDecoration: 'none' }}
            >
              Get WireGuard
            </a>
          </div>
          <div className="setup-grid">
            <div className="setup-card">
              <div className="setup-label">Windows / macOS</div>
              <div className="setup-text">Install WireGuard, then choose “Import tunnel(s) from file” and select `6529vpn.conf`.</div>
            </div>
            <div className="setup-card">
              <div className="setup-label">iPhone / Android</div>
              <div className="setup-text">Install the WireGuard app, then import the downloaded config file into the app and activate it.</div>
            </div>
            <div className="setup-card">
              <div className="setup-label">Linux</div>
              <div className="setup-text">Install `wireguard-tools` and `openresolv`, then save the file as `/etc/wireguard/6529vpn.conf` and run `sudo wg-quick up 6529vpn`.</div>
            </div>
          </div>
        </div>

        {/* Renewal panel */}
        {renewPhase === 'picking' && (
          <div className="renew-panel">
            <h3 style={{ marginBottom: 12 }}>Renew Subscription</h3>
            <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: 16 }}>
              Extend your subscription. Your VPN stays connected.
            </p>
            <div className="tier-picker">
              {renewTiers.map(t => (
                <button
                  key={t.id}
                  className={`tier-option${renewSelected === t ? ' selected' : ''}`}
                  onClick={() => setRenewSelected(t)}
                >
                  <span className="tier-option-label">{TIER_LABELS[t.durationKey] || `${Math.floor(t.duration / 86400)}d`}</span>
                  <span className="tier-option-price">{t.costEth} ETH</span>
                </button>
              ))}
            </div>
            <div className="btn-row" style={{ marginTop: 16 }}>
              <button
                className="btn-primary"
                onClick={handleConfirmRenew}
                disabled={!renewSelected}
                style={{ padding: '10px 20px', fontSize: '0.85rem' }}
              >
                {renewSelected ? 'Confirm Renewal' : 'Select a plan'}
              </button>
              <button
                className="btn-secondary"
                onClick={() => setRenewPhase('idle')}
                style={{ padding: '10px 20px', fontSize: '0.85rem' }}
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {(renewPhase === 'sending' || renewPhase === 'confirming') && (
          <div className="renew-panel" style={{ textAlign: 'center' }}>
            <div className="spinner" style={{ margin: '0 auto 12px' }} />
            <p style={{ color: 'var(--muted)', fontSize: '0.85rem' }}>
              {renewPhase === 'sending' ? 'Confirm transaction in your wallet...' : 'Waiting for confirmation...'}
            </p>
          </div>
        )}

        {renewPhase === 'done' && (
          <div className="renew-panel" style={{ textAlign: 'center' }}>
            <p style={{ color: 'var(--success)', fontWeight: 600 }}>Subscription renewed!</p>
          </div>
        )}

        {renewPhase === 'error' && (
          <div className="renew-panel" style={{ textAlign: 'center' }}>
            <p style={{ color: 'var(--error)', marginBottom: 12 }}>{renewError}</p>
            <button
              className="btn-secondary"
              onClick={() => setRenewPhase('idle')}
              style={{ padding: '10px 20px', fontSize: '0.85rem' }}
            >
              Dismiss
            </button>
          </div>
        )}

        {showConfig && (
          <div style={{ marginTop: 16 }}>
            <div className="config-display">{session.vpnConfig}</div>
            <div className="btn-row">
              <button className="btn-primary" onClick={downloadConfig} style={{ padding: '10px 20px', fontSize: '0.85rem' }}>
                Download
              </button>
              <button className="btn-secondary" onClick={copyConfig} style={{ padding: '10px 20px', fontSize: '0.85rem' }}>
                {copyLabel}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
