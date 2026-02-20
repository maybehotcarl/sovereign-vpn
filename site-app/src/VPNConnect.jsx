import { useState, useCallback } from 'react';
import { useAccount, useSignMessage } from 'wagmi';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { generateKeyPair } from './wgkeys';

const STEPS = [
  { id: 'challenge', label: 'Requesting challenge...' },
  { id: 'sign', label: 'Sign the message in your wallet...' },
  { id: 'verify', label: 'Verifying NFT ownership on-chain...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

export default function VPNConnect({ gatewayUrl = '' }) {
  const { address, isConnected } = useAccount();
  const { signMessageAsync } = useSignMessage();

  const [phase, setPhase] = useState('idle'); // idle | running | success | error
  const [currentStep, setCurrentStep] = useState(-1);
  const [completedSteps, setCompletedSteps] = useState({});
  const [errorMsg, setErrorMsg] = useState('');
  const [vpnConfig, setVpnConfig] = useState('');
  const [tierInfo, setTierInfo] = useState('');
  const [copyLabel, setCopyLabel] = useState('Copy to Clipboard');

  const markDone = (idx, text) => {
    setCompletedSteps(prev => ({ ...prev, [idx]: text }));
  };

  const startVPN = useCallback(async () => {
    setPhase('running');
    setCurrentStep(0);
    setCompletedSteps({});
    setErrorMsg('');

    try {
      // Step 0: Get challenge
      const challengeResp = await fetch(`${gatewayUrl}/auth/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address }),
      });
      if (!challengeResp.ok) {
        const err = await challengeResp.json();
        throw new Error(err.error || 'Failed to get challenge');
      }
      const challenge = await challengeResp.json();
      markDone(0, 'Challenge received');

      // Step 1: Sign message
      setCurrentStep(1);
      const signature = await signMessageAsync({ message: challenge.message });
      markDone(1, 'Message signed');

      // Step 2: Verify signature + check NFT
      setCurrentStep(2);
      const verifyResp = await fetch(`${gatewayUrl}/auth/verify`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: challenge.message, signature }),
      });
      const verifyData = await verifyResp.json();

      if (verifyData.tier === 'denied') {
        throw new Error('No Memes card found in this wallet. You need at least one card from The Memes by 6529.');
      }
      if (!verifyResp.ok) {
        throw new Error(verifyData.error || 'Verification failed');
      }
      markDone(2, `Verified: ${verifyData.tier} tier`);

      // Step 3: Generate WireGuard keys and provision
      setCurrentStep(3);
      const keys = generateKeyPair();

      const connectResp = await fetch(`${gatewayUrl}/vpn/connect`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          session_token: verifyData.address,
          public_key: keys.publicKey,
        }),
      });
      if (!connectResp.ok) {
        const err = await connectResp.json();
        throw new Error(err.error || 'Failed to provision VPN');
      }
      const vpnData = await connectResp.json();
      markDone(3, 'VPN provisioned');

      const config = [
        '[Interface]',
        `PrivateKey = ${keys.privateKey}`,
        `Address = ${vpnData.client_address}`,
        `DNS = ${vpnData.dns}`,
        '',
        '[Peer]',
        `PublicKey = ${vpnData.server_public_key}`,
        `Endpoint = ${vpnData.server_endpoint}`,
        `AllowedIPs = 0.0.0.0/0, ::/0`,
        'PersistentKeepalive = 25',
      ].join('\n');

      setVpnConfig(config);
      setTierInfo(`Access tier: ${vpnData.tier} \u2022 Expires: ${new Date(vpnData.expires_at).toLocaleString()}`);
      setTimeout(() => setPhase('success'), 400);

    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [address, signMessageAsync, gatewayUrl]);

  const downloadConfig = () => {
    const blob = new Blob([vpnConfig], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = '6529vpn.conf';
    a.click();
    URL.revokeObjectURL(url);
  };

  const copyConfig = async () => {
    await navigator.clipboard.writeText(vpnConfig);
    setCopyLabel('Copied!');
    setTimeout(() => setCopyLabel('Copy to Clipboard'), 2000);
  };

  const reset = () => {
    setPhase('idle');
    setCurrentStep(-1);
    setCompletedSteps({});
    setErrorMsg('');
    setVpnConfig('');
    setTierInfo('');
  };

  return (
    <div className="connect-section">
      <div className="connect-box">
        <h2 style={{ marginBottom: 12 }}>Connect & Get VPN Config</h2>
        <p style={{ color: 'var(--muted)', marginBottom: 24 }}>
          Sign in with your wallet to verify your Memes card and get a WireGuard config file.
        </p>

        {/* RainbowKit connect button — always visible until VPN is provisioned */}
        {phase !== 'success' && (
          <div className="wallet-connect-wrapper">
            <ConnectButton showBalance={false} />
          </div>
        )}

        {/* Idle: show sign-in button once wallet is connected */}
        {phase === 'idle' && isConnected && (
          <button className="btn-primary" onClick={startVPN} style={{ marginTop: 16 }}>
            Sign In & Get VPN Config
          </button>
        )}

        {/* Running: show step progress */}
        {phase === 'running' && (
          <div style={{ marginTop: 20 }}>
            {STEPS.map((step, i) => {
              const done = completedSteps[i];
              const active = i === currentStep && !done;
              const pending = i > currentStep;

              return (
                <div
                  key={step.id}
                  className={`flow-step${done ? ' done' : active ? ' active' : ''}`}
                  style={pending ? { opacity: 0.4 } : undefined}
                >
                  {done ? (
                    <span style={{ fontWeight: 'bold' }}>{'\u2713'}</span>
                  ) : active ? (
                    <div className="spinner" />
                  ) : (
                    <span style={{ width: 16, display: 'inline-block' }}>{'\u2022'}</span>
                  )}
                  {done || step.label}
                </div>
              );
            })}
          </div>
        )}

        {/* Success: show config */}
        {phase === 'success' && (
          <div>
            <h2 style={{ marginBottom: 8, color: 'var(--success)' }}>VPN Config Ready</h2>
            <p style={{ color: 'var(--muted)', marginBottom: 16 }}>{tierInfo}</p>
            <div className="config-display">{vpnConfig}</div>
            <div className="btn-row">
              <button className="btn-primary" onClick={downloadConfig}>Download Config</button>
              <button className="btn-secondary" onClick={copyConfig}>{copyLabel}</button>
            </div>
            <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginTop: 16 }}>
              Import this file into the{' '}
              <a href="https://www.wireguard.com/install/" target="_blank" rel="noreferrer">WireGuard app</a>{' '}
              to activate the VPN.
            </p>
            <button className="btn-secondary" onClick={reset} style={{ marginTop: 16, padding: '10px 24px', fontSize: '0.85rem' }}>
              Start Over
            </button>
          </div>
        )}

        {/* Error */}
        {phase === 'error' && (
          <div style={{ marginTop: 20 }}>
            <p style={{ color: 'var(--error)', marginBottom: 16 }}>{errorMsg}</p>
            <button className="btn-secondary" onClick={reset}>Try Again</button>
          </div>
        )}
      </div>
    </div>
  );
}
