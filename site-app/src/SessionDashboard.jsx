import { useState, useEffect, useCallback } from 'react';

function formatTimeLeft(expiresAt) {
  const diff = new Date(expiresAt) - Date.now();
  if (diff <= 0) return null;
  const hours = Math.floor(diff / 3600000);
  const mins = Math.floor((diff % 3600000) / 60000);
  const secs = Math.floor((diff % 60000) / 1000);
  if (hours > 0) return `${hours}h ${mins}m ${secs}s`;
  if (mins > 0) return `${mins}m ${secs}s`;
  return `${secs}s`;
}

export default function SessionDashboard({ session, onDisconnect, onReconnect }) {
  const [timeLeft, setTimeLeft] = useState(() => formatTimeLeft(session.expiresAt));
  const [disconnecting, setDisconnecting] = useState(false);
  const [showConfig, setShowConfig] = useState(false);
  const [copyLabel, setCopyLabel] = useState('Copy');

  const expired = !timeLeft;

  // Live countdown
  useEffect(() => {
    if (expired) return;
    const id = setInterval(() => {
      setTimeLeft(formatTimeLeft(session.expiresAt));
    }, 1000);
    return () => clearInterval(id);
  }, [session.expiresAt, expired]);

  const handleDisconnect = useCallback(async () => {
    setDisconnecting(true);
    try {
      await fetch(`${session.gatewayUrl}/vpn/disconnect`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_token: session.address,
          public_key: session.publicKey,
        }),
      });
    } catch (err) {
      console.error('Disconnect error:', err);
    }
    onDisconnect();
  }, [session, onDisconnect]);

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
            <span>Session Expired</span>
          </div>
        </div>
        <div className="dashboard-body" style={{ textAlign: 'center' }}>
          <p style={{ color: 'var(--muted)', marginBottom: 20 }}>
            Your VPN session has expired. Reconnect to get a new configuration.
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
        <div className="dashboard-tier">{session.tier}</div>
      </div>

      <div className="dashboard-body">
        <div className="dashboard-stats">
          <div className="stat">
            <div className="stat-label">Time Remaining</div>
            <div className="stat-value">{timeLeft}</div>
          </div>
          <div className="stat">
            <div className="stat-label">Client IP</div>
            <div className="stat-value mono">{session.clientAddress}</div>
          </div>
          <div className="stat">
            <div className="stat-label">Server</div>
            <div className="stat-value mono">{session.serverEndpoint}</div>
          </div>
        </div>

        <div className="dashboard-actions">
          <button
            className="btn-secondary"
            onClick={() => setShowConfig(!showConfig)}
            style={{ padding: '10px 20px', fontSize: '0.85rem' }}
          >
            {showConfig ? 'Hide Config' : 'View Config'}
          </button>
          <button
            className="btn-disconnect"
            onClick={handleDisconnect}
            disabled={disconnecting}
          >
            {disconnecting ? 'Disconnecting...' : 'Disconnect'}
          </button>
        </div>

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
