import { useState, useCallback, useEffect } from 'react';
import { useAccount, useSignMessage, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { formatEther } from 'viem';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { generateKeyPair } from './wgkeys';
import { SESSION_MANAGER_ADDRESS, SESSION_MANAGER_ABI } from './contracts';

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

      // If paid tier, pause for payment
      if (vData.tier === 'paid' && SESSION_MANAGER_ADDRESS) {
        setSteps(PAID_STEPS);
        setVerifyData(vData);

        // Fetch session info from gateway
        const infoResp = await fetch(`${gatewayUrl}/session/info`);
        if (!infoResp.ok) {
          throw new Error('Failed to fetch session pricing');
        }
        const info = await infoResp.json();
        const costEth = formatEther(BigInt(info.cost_wei));

        setPaymentInfo({
          node: info.node_operator,
          duration: info.duration_seconds,
          costWei: info.cost_wei,
          costEth,
          contract: info.contract,
          chainId: info.chain_id,
        });
        setCurrentStep(3); // payment step
        setPhase('payment');
        return; // pause — user clicks "Pay & Connect"
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
    if (!paymentInfo || !verifyData) return;

    try {
      setPhase('running');
      // Step 3: Send payment tx
      const hash = await writeContractAsync({
        address: paymentInfo.contract,
        abi: SESSION_MANAGER_ABI,
        functionName: 'openSession',
        args: [paymentInfo.node, BigInt(paymentInfo.duration)],
        value: BigInt(paymentInfo.costWei),
      });
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
  }, [paymentInfo, verifyData, writeContractAsync]);

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

            {/* Payment card — shown when paused for payment */}
            {phase === 'payment' && paymentInfo && (
              <div style={{
                marginTop: 20,
                padding: '20px',
                border: '1px solid var(--border, #333)',
                borderRadius: 8,
                background: 'var(--card-bg, #1a1a2e)',
              }}>
                <h3 style={{ marginBottom: 8 }}>24 Hour VPN Session</h3>
                <p style={{ fontSize: '1.5rem', fontWeight: 'bold', marginBottom: 4 }}>
                  {paymentInfo.costEth} ETH
                </p>
                <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: 16 }}>
                  Paid directly to the SessionManager contract. 80% goes to the node operator, 20% to treasury.
                </p>
                <button className="btn-primary" onClick={handlePayAndConnect}>
                  Pay & Connect
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
