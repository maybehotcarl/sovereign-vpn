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

function downloadTextFile(filename, contents, mimeType = 'text/plain;charset=utf-8') {
  const blob = new Blob([contents], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function buildPosixAutoDisconnectScript(vpnConfig, expiresAt, platform) {
  const disconnectDelaySeconds = Math.max(15, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 1000));
  const humanExpiry = formatDateTime(expiresAt) || expiresAt;
  const installHint =
    platform === 'macos'
      ? 'Install wireguard-tools first, for example with `brew install wireguard-tools`.'
      : 'Install wireguard-tools first, for example with `sudo apt install wireguard-tools openresolv`.';

  return `#!/usr/bin/env bash
set -euo pipefail

WG_QUICK="$(command -v wg-quick || true)"
if [ -z "$WG_QUICK" ]; then
  echo "${installHint}" >&2
  exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this helper with sudo so it can bring the tunnel up and tear it down later." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG_PATH="$SCRIPT_DIR/6529vpn.conf"
DISCONNECT_DELAY=${disconnectDelaySeconds}

cat > "$CONFIG_PATH" <<'CONFIG_EOF'
${vpnConfig}
CONFIG_EOF
chmod 600 "$CONFIG_PATH"

"$WG_QUICK" down "$CONFIG_PATH" >/dev/null 2>&1 || true
"$WG_QUICK" up "$CONFIG_PATH"

cleanup_cmd=$(printf '%q down %q >/tmp/6529vpn-expiry-guard.log 2>&1; rm -f %q' "$WG_QUICK" "$CONFIG_PATH" "$CONFIG_PATH")
nohup bash -lc "sleep $DISCONNECT_DELAY; $cleanup_cmd" >/dev/null 2>&1 &

echo "6529 VPN is connected."
echo "This helper will automatically disconnect the tunnel at ${humanExpiry}."
echo "To disconnect sooner, run: sudo $WG_QUICK down $CONFIG_PATH"
`;
}

function buildWindowsAutoDisconnectScript(vpnConfig, expiresAt) {
  const disconnectDelaySeconds = Math.max(15, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / 1000));
  const humanExpiry = formatDateTime(expiresAt) || expiresAt;

  return `$ErrorActionPreference = "Stop"

$wireguard = Join-Path $env:ProgramFiles "WireGuard\\wireguard.exe"
if (-not (Test-Path $wireguard)) {
  throw "Install WireGuard for Windows first: https://www.wireguard.com/install/"
}

$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).
  IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
  throw "Run this helper from an elevated PowerShell window."
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$configPath = Join-Path $scriptDir "6529vpn.conf"
$cleanupPath = Join-Path $env:TEMP "6529vpn-expiry-guard.ps1"

@'
${vpnConfig}
'@ | Set-Content -LiteralPath $configPath -Encoding Ascii

try { & $wireguard /uninstalltunnelservice 6529vpn | Out-Null } catch {}
& $wireguard /installtunnelservice $configPath

$cleanup = @"
Start-Sleep -Seconds ${disconnectDelaySeconds}
& '$wireguard' /uninstalltunnelservice 6529vpn | Out-Null
Remove-Item -LiteralPath '$configPath' -ErrorAction SilentlyContinue
Remove-Item -LiteralPath '$cleanupPath' -ErrorAction SilentlyContinue
"@

Set-Content -LiteralPath $cleanupPath -Value $cleanup -Encoding Ascii
Start-Process powershell -WindowStyle Hidden -ArgumentList @('-ExecutionPolicy', 'Bypass', '-File', $cleanupPath)

Write-Host "6529 VPN is connected."
Write-Host "This helper will automatically disconnect the tunnel at ${humanExpiry}."
Write-Host "To disconnect sooner, run: & '$wireguard' /uninstalltunnelservice 6529vpn"
`;
}

function detectClientPlatform() {
  if (typeof navigator === 'undefined') {
    return 'desktop';
  }

  const source = [
    navigator.userAgentData?.platform,
    navigator.platform,
    navigator.userAgent,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();

  if (/iphone|ipad|ipod|android/.test(source)) return 'mobile';
  if (/mac/.test(source)) return 'macos';
  if (/win/.test(source)) return 'windows';
  if (/linux|x11/.test(source)) return 'linux';
  return 'desktop';
}

function getPlatformSetup(platform) {
  switch (platform) {
    case 'macos':
      return {
        name: 'Mac',
        installLabel: 'Get WireGuard For Mac',
        installCopy: 'If you already installed WireGuard, skip this step.',
        importCopy: 'Open WireGuard, choose “Import tunnel(s) from file,” select `6529vpn.conf`, then switch the tunnel on.',
        helperLabel: 'Download macOS auto-disconnect helper',
        helperCopy: 'Advanced: use this if you want the tunnel to shut itself off automatically when the session lease ends.',
      };
    case 'windows':
      return {
        name: 'Windows PC',
        installLabel: 'Get WireGuard For Windows',
        installCopy: 'If WireGuard is already installed, go straight to step 2.',
        importCopy: 'Open WireGuard, import `6529vpn.conf`, then activate the tunnel.',
        helperLabel: 'Download Windows auto-disconnect helper',
        helperCopy: 'Advanced: installs the tunnel service and schedules an automatic disconnect at lease expiry.',
      };
    case 'linux':
      return {
        name: 'Linux device',
        installLabel: 'Get WireGuard For Linux',
        installCopy: 'If your distro already has WireGuard installed, skip this step.',
        importCopy: 'Import `6529vpn.conf` into WireGuard, or use the Linux helper below if you want automatic disconnect at expiry.',
        helperLabel: 'Download Linux auto-disconnect helper',
        helperCopy: 'Advanced: writes the config, starts the tunnel with `wg-quick`, and tears it down automatically when the lease ends.',
      };
    case 'mobile':
      return {
        name: 'device',
        installLabel: 'Get WireGuard',
        installCopy: 'Install the WireGuard app for your device before downloading the config.',
        importCopy: 'Download `6529vpn.conf`, import it into WireGuard, then activate the tunnel.',
        helperLabel: null,
        helperCopy: '',
      };
    default:
      return {
        name: 'device',
        installLabel: 'Get WireGuard',
        installCopy: 'Install WireGuard first if this is your first time setting up the VPN.',
        importCopy: 'Import `6529vpn.conf` into WireGuard, then switch the tunnel on.',
        helperLabel: null,
        helperCopy: '',
      };
  }
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
  const sessionExpiryLabel = formatDateTime(session.expiresAt);
  const clientPlatform = detectClientPlatform();
  const platformSetup = getPlatformSetup(clientPlatform);
  const sessionTypeLabel = isAnonymousAccess ? 'Anonymous Session' : 'Direct Wallet Session';

  const showPlatformHelper = clientPlatform === 'macos' || clientPlatform === 'windows' || clientPlatform === 'linux';

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
  }, [onRenew, renewPhase, renewSelected, renewTxConfirmed, renewTxFailed, session.expiresAt]);

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
    downloadTextFile('6529vpn.conf', session.vpnConfig);
  };

  const openDesktopApp = async () => {
    const desktopPayload = {
      vpnConfig: session.vpnConfig,
      expiresAt: session.expiresAt,
      accessMode: isAnonymousAccess ? 'anonymous' : 'direct',
      serverEndpoint: session.serverEndpoint,
      sessionLabel: isAnonymousAccess ? 'Anonymous Session' : 'Direct Wallet Session',
    };

    const sendHandoff = async () => {
      const response = await fetch('http://127.0.0.1:9469/handoff', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(desktopPayload),
      });

      if (!response.ok) {
        const payload = await response.json().catch(() => null);
        const errorText = payload?.error ? ` (${payload.error})` : '';
        throw new Error(`Desktop handoff failed${errorText}`);
      }
    };

    try {
      await sendHandoff();
      return;
    } catch {
      window.location.href = 'sovereignvpn://open';
      window.setTimeout(() => {
        sendHandoff().catch(() => {
          window.alert('Desktop app not detected yet. Open the 6529 VPN Desktop app first, then try “Open in Desktop App” again.');
        });
      }, 1200);
    }
  };

  const downloadLinuxHelper = () => {
    downloadTextFile(
      '6529vpn-linux-connect.sh',
      buildPosixAutoDisconnectScript(session.vpnConfig, session.expiresAt, 'linux'),
    );
  };

  const downloadMacHelper = () => {
    downloadTextFile(
      '6529vpn-macos-connect.command',
      buildPosixAutoDisconnectScript(session.vpnConfig, session.expiresAt, 'macos'),
    );
  };

  const downloadWindowsHelper = () => {
    downloadTextFile(
      '6529vpn-windows-connect.ps1',
      buildWindowsAutoDisconnectScript(session.vpnConfig, session.expiresAt),
    );
  };

  const downloadPlatformHelper = () => {
    if (clientPlatform === 'macos') {
      downloadMacHelper();
      return;
    }
    if (clientPlatform === 'windows') {
      downloadWindowsHelper();
      return;
    }
    if (clientPlatform === 'linux') {
      downloadLinuxHelper();
    }
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
      <div className="dashboard-header ready">
        <div className="dashboard-status">
          <div className="status-dot pending" />
          <div className="dashboard-status-copy">
            <span className="dashboard-status-title">Session Ready</span>
            <span className="dashboard-status-subtitle">
              Finish the setup below before this device is actually routing traffic through 6529 VPN.
            </span>
          </div>
        </div>
        <div className="dashboard-tier">{sessionTypeLabel}</div>
      </div>

      <div className="dashboard-body">
        <div className="setup-hero">
          <div className="setup-hero-kicker">Next Step</div>
          <h3 className="setup-hero-title">Finish setup on this {platformSetup.name}</h3>
          <p className="setup-copy" style={{ marginBottom: 0 }}>
            {isAnonymousAccess
              ? `You already issued a short-lived anonymous VPN lease. Now import the config into WireGuard so this device actually uses it.${formattedSubscriptionExpiry ? ` Your wallet subscription remains active until ${formattedSubscriptionExpiry}.` : ''}`
              : 'Your wallet session is ready. The last step is importing the config into WireGuard and turning the tunnel on.'}
          </p>
        </div>

        <div className="dashboard-stats">
          <div className="stat">
            <div className="stat-label">
              {isAnonymousAccess ? 'Anonymous Session Lease' : 'Session Expires In'}
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
            <div className="stat-label">Assigned Tunnel IP</div>
            <div className="stat-value mono">{session.clientAddress}</div>
          </div>
          <div className="stat">
            <div className="stat-label">VPN Server</div>
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

        <div className="setup-panel">
          <h3>Use This Session With WireGuard</h3>
          <p className="setup-copy">
            WireGuard is the VPN app that actually creates the secure tunnel on your device. Keep this simple: install WireGuard, download the config, import it, then switch the tunnel on.
          </p>
          <div className="setup-grid setup-grid-steps">
            <div className="setup-card setup-step-card">
              <div className="setup-step-number">1</div>
              <div className="setup-label">Install WireGuard</div>
              <div className="setup-text">{platformSetup.installCopy}</div>
              <div className="btn-row setup-card-actions">
                <a
                  className="btn-secondary"
                  href={wireGuardInstallUrl}
                  target="_blank"
                  rel="noreferrer"
                  style={{ padding: '10px 20px', fontSize: '0.85rem', textDecoration: 'none' }}
                >
                  {platformSetup.installLabel}
                </a>
              </div>
            </div>
            <div className="setup-card setup-step-card">
              <div className="setup-step-number">2</div>
              <div className="setup-label">Download your config</div>
              <div className="setup-text">This file contains the exact tunnel settings for this session.</div>
              <div className="btn-row setup-card-actions">
                <button
                  className="btn-primary"
                  onClick={downloadConfig}
                  style={{ padding: '10px 20px', fontSize: '0.85rem' }}
                >
                  Download Config
                </button>
              </div>
            </div>
            <div className="setup-card setup-step-card">
              <div className="setup-step-number">3</div>
              <div className="setup-label">Import it and turn the tunnel on</div>
              <div className="setup-text">{platformSetup.importCopy}</div>
            </div>
          </div>
        </div>

        <div className="dashboard-actions">
          <button
            className="btn-disconnect"
            onClick={handleDisconnect}
            disabled={disconnecting}
          >
            {disconnecting ? 'Ending Session...' : 'End Session'}
          </button>
        </div>

        <details className="advanced-panel">
          <summary className="advanced-summary">
            <span>Advanced Options</span>
            <span className="advanced-summary-copy">Desktop app, auto-disconnect helpers, renew, and raw config</span>
          </summary>
          <div className="advanced-body">
            <div className="setup-note">
              This is a full-tunnel WireGuard profile. If it stays active after the lease ends, your device can keep sending traffic into a dead tunnel until you disconnect it manually.
              {sessionExpiryLabel ? ` This session is scheduled to expire at ${sessionExpiryLabel}.` : ''}
            </div>

            <div className="advanced-actions">
              {clientPlatform === 'linux' && (
                <button
                  className="btn-primary"
                  onClick={openDesktopApp}
                  style={{ padding: '10px 20px', fontSize: '0.85rem' }}
                >
                  Open In Desktop App
                </button>
              )}
              {showPlatformHelper && (
                <button
                  className="btn-secondary"
                  onClick={downloadPlatformHelper}
                  style={{ padding: '10px 20px', fontSize: '0.85rem' }}
                >
                  {platformSetup.helperLabel}
                </button>
              )}
              {showRenew && (
                <button
                  className="btn-primary"
                  onClick={handleRenewClick}
                  disabled={renewPhase !== 'idle' && renewPhase !== 'error' && renewPhase !== 'done'}
                  style={{ padding: '10px 20px', fontSize: '0.85rem' }}
                >
                  Renew Subscription
                </button>
              )}
              <button
                className="btn-secondary"
                onClick={() => setShowConfig(!showConfig)}
                style={{ padding: '10px 20px', fontSize: '0.85rem' }}
              >
                {showConfig ? 'Hide Raw Config' : 'View Raw Config'}
              </button>
            </div>

            {showPlatformHelper && (
              <p className="advanced-copy">{platformSetup.helperCopy}</p>
            )}
          </div>
        </details>

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
          <div className="advanced-config">
            <div className="config-display">{session.vpnConfig}</div>
            <div className="btn-row">
              <button className="btn-primary" onClick={downloadConfig} style={{ padding: '10px 20px', fontSize: '0.85rem' }}>
                Download Again
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
