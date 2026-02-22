import { useState, useCallback, useEffect } from 'react';
import { useAccount, useSignMessage, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { formatEther } from 'viem';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { generateKeyPair } from './wgkeys';
import { SESSION_MANAGER_ADDRESS, SESSION_MANAGER_ABI, SUBSCRIPTION_MANAGER_ABI } from './contracts';

const FREE_STEPS = [
  { id: 'challenge', label: 'Requesting challenge...' },
  { id: 'sign', label: 'Sign the message in your wallet...' },
  { id: 'verify', label: 'Verifying NFT ownership on-chain...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

const PAID_STEPS = [
  { id: 'challenge', label: 'Requesting challenge...' },
  { id: 'sign', label: 'Sign the message in your wallet...' },
  { id: 'verify', label: 'Verifying NFT ownership on-chain...' },
  { id: 'payment', label: 'Confirm payment in your wallet...' },
  { id: 'confirm', label: 'Waiting for transaction confirmation...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

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

export default function VPNConnect({ gatewayUrl = '', onSessionCreated }) {
  const { address, isConnected } = useAccount();
  const { signMessageAsync } = useSignMessage();
  const { writeContractAsync } = useWriteContract();

  const [phase, setPhase] = useState('idle'); // idle | running | payment | success | error
  const [steps, setSteps] = useState(FREE_STEPS);
  const [currentStep, setCurrentStep] = useState(-1);
  const [completedSteps, setCompletedSteps] = useState({});
  const [errorMsg, setErrorMsg] = useState('');
  const [vpnConfig, setVpnConfig] = useState('');
  const [tierInfo, setTierInfo] = useState('');
  const [copyLabel, setCopyLabel] = useState('Copy to Clipboard');

  // Paid tier state
  const [paymentInfo, setPaymentInfo] = useState(null); // { node, duration, costWei, costEth }
  const [verifyData, setVerifyData] = useState(null);
  const [paymentTxHash, setPaymentTxHash] = useState(null);

  // Tier picker state
  const [selectedTier, setSelectedTier] = useState(null); // '24h' or tier object from API
  const [tierOptions, setTierOptions] = useState(null); // { session: {...}, subscriptions: [...] }

  // Watch for tx confirmation
  const { isSuccess: txConfirmed, isError: txFailed } = useWaitForTransactionReceipt({
    hash: paymentTxHash,
  });

  // When tx confirms, continue the flow
  useEffect(() => {
    if (txConfirmed && phase === 'payment' && verifyData) {
      continueAfterPayment();
    }
    if (txFailed && phase === 'payment') {
      setErrorMsg('Transaction failed on-chain');
      setPhase('error');
    }
  }, [txConfirmed, txFailed]);

  const markDone = (idx, text) => {
    setCompletedSteps(prev => ({ ...prev, [idx]: text }));
  };

  const startVPN = useCallback(async () => {
    setPhase('running');
    setSteps(FREE_STEPS);
    setCurrentStep(0);
    setCompletedSteps({});
    setErrorMsg('');
    setPaymentInfo(null);
    setVerifyData(null);
    setPaymentTxHash(null);
    setSelectedTier(null);
    setTierOptions(null);

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
      const vData = await verifyResp.json();

      if (vData.tier === 'denied') {
        if (vData.error && vData.error.includes('banned')) {
          throw new Error('This wallet has been banned by the community.');
        }
        throw new Error('No Memes card found in this wallet. You need at least one card from The Memes by 6529.');
      }
      if (!verifyResp.ok) {
        throw new Error(vData.error || 'Verification failed');
      }
      markDone(2, `Verified: ${vData.tier} tier`);

      // If paid tier, fetch pricing and show tier picker
      if (vData.tier === 'paid' && SESSION_MANAGER_ADDRESS) {
        setSteps(PAID_STEPS);
        setVerifyData(vData);

        // Fetch session info and subscription tiers in parallel
        const [sessionResp, subResp] = await Promise.all([
          fetch(`${gatewayUrl}/session/info`),
          fetch(`${gatewayUrl}/subscription/tiers`).catch(() => null),
        ]);

        if (!sessionResp.ok) {
          throw new Error('Failed to fetch session pricing');
        }
        const sessionInfo = await sessionResp.json();

        let subTiers = null;
        if (subResp && subResp.ok) {
          subTiers = await subResp.json();
        }

        setTierOptions({
          session: {
            node: sessionInfo.node_operator,
            duration: sessionInfo.duration_seconds,
            costWei: sessionInfo.cost_wei,
            costEth: formatEther(BigInt(sessionInfo.cost_wei)),
            contract: sessionInfo.contract,
            chainId: sessionInfo.chain_id,
          },
          subscriptions: subTiers ? subTiers.tiers.map(t => ({
            id: t.id,
            durationKey: formatDuration(t.duration_seconds),
            duration: t.duration_seconds,
            costWei: t.price_wei,
            costEth: formatEther(BigInt(t.price_wei)),
            contract: subTiers.contract,
            chainId: subTiers.chain_id,
          })) : [],
        });

        setCurrentStep(3); // payment step
        setPhase('payment');
        return; // pause — user picks a tier and clicks "Pay & Connect"
      }

      // Free tier: continue directly to VPN provisioning
      await provisionVPN(vData, 3);

    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [address, signMessageAsync, gatewayUrl]);

  const handlePayAndConnect = useCallback(async () => {
    if (!selectedTier || !verifyData || !tierOptions) return;

    try {
      setPhase('running');

      let hash;
      if (selectedTier === '24h') {
        // 24h session via SessionManager
        const info = tierOptions.session;
        hash = await writeContractAsync({
          address: info.contract,
          abi: SESSION_MANAGER_ABI,
          functionName: 'openSession',
          args: [info.node, BigInt(info.duration)],
          value: BigInt(info.costWei),
        });
      } else {
        // Subscription via SubscriptionManager
        const tier = tierOptions.subscriptions.find(t => t.durationKey === selectedTier);
        hash = await writeContractAsync({
          address: tier.contract,
          abi: SUBSCRIPTION_MANAGER_ABI,
          functionName: 'subscribe',
          args: [tierOptions.session.node, tier.id],
          value: BigInt(tier.costWei),
        });
      }

      markDone(3, 'Payment sent');

      // Step 4: Wait for confirmation
      setCurrentStep(4);
      setPaymentTxHash(hash);
      // useWaitForTransactionReceipt will trigger continueAfterPayment via useEffect
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Payment failed');
      setPhase('error');
    }
  }, [selectedTier, tierOptions, verifyData, writeContractAsync]);

  const continueAfterPayment = useCallback(async () => {
    try {
      markDone(4, 'Transaction confirmed');
      // Step 5: Provision VPN
      await provisionVPN(verifyData, 5);
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [verifyData, gatewayUrl]);

  const provisionVPN = async (vData, stepIdx) => {
    setCurrentStep(stepIdx);
    const keys = generateKeyPair();

    const connectResp = await fetch(`${gatewayUrl}/vpn/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_token: vData.address,
        public_key: keys.publicKey,
      }),
    });
    if (!connectResp.ok) {
      const err = await connectResp.json();
      throw new Error(err.error || 'Failed to provision VPN');
    }
    const vpnData = await connectResp.json();
    markDone(stepIdx, 'VPN provisioned');

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

    // Save session for the dashboard
    if (onSessionCreated) {
      onSessionCreated({
        address: vData.address,
        tier: vpnData.tier,
        expiresAt: vpnData.expires_at,
        serverEndpoint: vpnData.server_endpoint,
        clientAddress: vpnData.client_address,
        serverPublicKey: vpnData.server_public_key,
        gatewayUrl,
        vpnConfig: config,
        connectedAt: new Date().toISOString(),
        publicKey: keys.publicKey,
      });
      return; // parent will switch to dashboard view
    }

    // Fallback: show config inline (if no dashboard handler)
    setVpnConfig(config);
    setTierInfo(`Access tier: ${vpnData.tier} \u2022 Expires: ${new Date(vpnData.expires_at).toLocaleString()}`);
    setTimeout(() => setPhase('success'), 400);
  };

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
    setSteps(FREE_STEPS);
    setCurrentStep(-1);
    setCompletedSteps({});
    setErrorMsg('');
    setVpnConfig('');
    setTierInfo('');
    setPaymentInfo(null);
    setVerifyData(null);
    setPaymentTxHash(null);
    setSelectedTier(null);
    setTierOptions(null);
  };

  // Build tier options for the picker
  const allTierOptions = tierOptions ? [
    { key: '24h', label: '24 Hours', price: tierOptions.session.costEth },
    ...tierOptions.subscriptions.map(t => ({
      key: t.durationKey,
      label: TIER_LABELS[t.durationKey] || `${Math.floor(t.duration / 86400)} Days`,
      price: t.costEth,
    })),
  ] : [];

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
        {(phase === 'running' || phase === 'payment') && (
          <div style={{ marginTop: 20 }}>
            {steps.map((step, i) => {
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
                    phase === 'payment' && step.id === 'payment' ? (
                      <span style={{ width: 16, display: 'inline-block' }}>{'\u2022'}</span>
                    ) : (
                      <div className="spinner" />
                    )
                  ) : (
                    <span style={{ width: 16, display: 'inline-block' }}>{'\u2022'}</span>
                  )}
                  {done || step.label}
                </div>
              );
            })}

            {/* Tier picker — shown when paused for payment */}
            {phase === 'payment' && tierOptions && (
              <div style={{
                marginTop: 20,
                padding: '20px',
                border: '1px solid var(--border, #333)',
                borderRadius: 8,
                background: 'var(--card-bg, #1a1a2e)',
              }}>
                <h3 style={{ marginBottom: 12 }}>Choose Your Plan</h3>
                <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: 16 }}>
                  Paid directly to the smart contract. 80% goes to the node operator, 20% to treasury.
                </p>

                <div className="tier-picker">
                  {allTierOptions.map(opt => (
                    <button
                      key={opt.key}
                      className={`tier-option${selectedTier === opt.key ? ' selected' : ''}`}
                      onClick={() => setSelectedTier(opt.key)}
                    >
                      <span className="tier-option-label">{opt.label}</span>
                      <span className="tier-option-price">{opt.price} ETH</span>
                      {opt.key !== '24h' && (
                        <span className="tier-savings">Save vs daily</span>
                      )}
                    </button>
                  ))}
                </div>

                <button
                  className="btn-primary"
                  onClick={handlePayAndConnect}
                  disabled={!selectedTier}
                  style={{ marginTop: 16, width: '100%' }}
                >
                  {selectedTier ? `Pay & Connect` : 'Select a plan'}
                </button>
              </div>
            )}
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
