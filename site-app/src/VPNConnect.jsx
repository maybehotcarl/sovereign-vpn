import { useState, useCallback, useEffect } from 'react';
import { useAccount, useSignMessage, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
import { formatEther } from 'viem';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { generateKeyPair } from './wgkeys';
import { SESSION_MANAGER_ADDRESS, SESSION_MANAGER_ABI, SUBSCRIPTION_MANAGER_ABI } from './contracts';
import {
  anonymousVPNConfig,
  anonymousVPNEnabled,
  deriveVPNAccessV1ChallengeHash,
  deriveVPNAccessV1SessionKeyHash,
  validateAnonymousVPNConfig,
} from './anonymous';
import {
  AnonymousIssuerRequestError,
  ensureAnonymousVPNIssuerEntitlement,
  getAnonymousVPNMeta,
} from './anonymousIssuer';
import { getOrCreateAnonymousVPNIdentity } from './anonymousIdentity';

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

const ANON_STEPS = [
  { id: 'challenge', label: 'Requesting anonymous challenge...' },
  { id: 'entitlement', label: 'Refreshing anonymous credential...' },
  { id: 'prove', label: 'Generating anonymous access proof...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

const ANON_PAYMENT_STEPS = [
  { id: 'challenge', label: 'Requesting anonymous challenge...' },
  { id: 'entitlement', label: 'Checking anonymous entitlement...' },
  { id: 'payment', label: 'Confirm subscription in your wallet...' },
  { id: 'confirm', label: 'Waiting for transaction confirmation...' },
  { id: 'prove', label: 'Generating anonymous access proof...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

const TIER_LABELS = {
  '24h': '24 Hours',
  '7d': '7 Days',
  '30d': '30 Days',
  '90d': '90 Days',
  '365d': '365 Days',
};

async function readResponsePayload(response) {
  const text = await response.text();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function responseErrorMessage(payload, fallback) {
  if (payload && typeof payload === 'object') {
    if (typeof payload.error === 'string' && payload.error.trim()) {
      return payload.error;
    }
    if (typeof payload.message === 'string' && payload.message.trim()) {
      return payload.message;
    }
  }
  if (typeof payload === 'string' && payload.trim()) {
    return payload.trim();
  }
  return fallback;
}

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
  const anonMode = anonymousVPNEnabled();

  const [phase, setPhase] = useState('idle'); // idle | running | payment | success | error
  const [steps, setSteps] = useState(FREE_STEPS);
  const [currentStep, setCurrentStep] = useState(-1);
  const [completedSteps, setCompletedSteps] = useState({});
  const [errorMsg, setErrorMsg] = useState('');
  const [vpnConfig, setVpnConfig] = useState('');
  const [tierInfo, setTierInfo] = useState('');
  const [copyLabel, setCopyLabel] = useState('Copy to Clipboard');

  // Paid tier state
  const [verifyData, setVerifyData] = useState(null);
  const [paymentTxHash, setPaymentTxHash] = useState(null);
  const [paymentIntent, setPaymentIntent] = useState(null); // null | { kind: 'legacy-connect' | 'anon-subscription' }

  // Tier picker state
  const [selectedTier, setSelectedTier] = useState(null); // '24h' or tier object from API
  const [tierOptions, setTierOptions] = useState(null); // { mode, session, subscriptions }

  // Watch for tx confirmation
  const { isSuccess: txConfirmed, isError: txFailed } = useWaitForTransactionReceipt({
    hash: paymentTxHash,
  });

  const markDone = (idx, text) => {
    setCompletedSteps(prev => ({ ...prev, [idx]: text }));
  };

  const loadGatewayPaymentOptions = useCallback(async ({
    includeSessionOption,
    requireSubscriptionOptions,
  }) => {
    const [sessionResp, subResp] = await Promise.all([
      fetch(`${gatewayUrl}/session/info`).catch(() => null),
      fetch(`${gatewayUrl}/subscription/tiers`).catch(() => null),
    ]);

    const sessionInfo = sessionResp && sessionResp.ok ? await sessionResp.json() : null;
    const subTiers = subResp && subResp.ok ? await subResp.json() : null;

    if (!sessionInfo?.node_operator) {
      throw new Error('Gateway does not expose node purchase info for subscriptions.');
    }
    if (requireSubscriptionOptions && (!subTiers?.tiers || subTiers.tiers.length === 0)) {
      throw new Error('Gateway does not expose active subscription tiers.');
    }
    if (includeSessionOption && !sessionInfo?.contract) {
      throw new Error('Gateway does not expose paid session pricing.');
    }

    return {
      session: {
        node: sessionInfo.node_operator,
        duration: sessionInfo.duration_seconds,
        costWei: sessionInfo.cost_wei,
        costEth:
          sessionInfo.cost_wei != null
            ? formatEther(BigInt(sessionInfo.cost_wei))
            : null,
        contract: sessionInfo.contract || '',
        chainId: sessionInfo.chain_id,
      },
      subscriptions: subTiers
        ? subTiers.tiers.map(t => ({
            id: t.id,
            durationKey: formatDuration(t.duration_seconds),
            duration: t.duration_seconds,
            costWei: t.price_wei,
            costEth: formatEther(BigInt(t.price_wei)),
            contract: subTiers.contract,
            chainId: subTiers.chain_id,
          }))
        : [],
    };
  }, [gatewayUrl]);

  const finalizeVPNProvision = useCallback(async (vpnData, keys, sessionMeta) => {
    const resolvedGatewayUrl = vpnData.gateway_url || gatewayUrl;
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

    // Resolve node operator for dashboard payout panel.
    // Paid flow already has it in tierOptions; free flow fetches /session/info.
    let nodeOperator = tierOptions?.session?.node || '';
    if (!nodeOperator) {
      try {
        const infoResp = await fetch(`${resolvedGatewayUrl}/session/info`);
        if (infoResp.ok) {
          const info = await infoResp.json();
          nodeOperator = info.node_operator || '';
        }
      } catch {
        // non-critical — payout panel just won't render
      }
    }

    // Save session for the dashboard
    if (onSessionCreated) {
      onSessionCreated({
        address: sessionMeta.address,
        sessionToken: sessionMeta.sessionToken,
        tier: vpnData.tier,
        expiresAt: vpnData.expires_at,
        serverEndpoint: vpnData.server_endpoint,
        clientAddress: vpnData.client_address,
        serverPublicKey: vpnData.server_public_key,
        gatewayUrl: resolvedGatewayUrl,
        nodeOperator,
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
  }, [gatewayUrl, onSessionCreated, tierOptions]);

  const provisionVPN = useCallback(async (vData, stepIdx) => {
    setCurrentStep(stepIdx);
    const keys = generateKeyPair();

    const connectResp = await fetch(`${gatewayUrl}/vpn/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        session_token: vData.session_token,
        public_key: keys.publicKey,
      }),
    });
    if (!connectResp.ok) {
      const err = await readResponsePayload(connectResp);
      throw new Error(responseErrorMessage(err, 'Failed to provision VPN'));
    }
    const vpnData = await readResponsePayload(connectResp);
    if (!vpnData || typeof vpnData !== 'object') {
      throw new Error('Gateway returned an invalid VPN response');
    }
    markDone(stepIdx, 'VPN provisioned');

    await finalizeVPNProvision(vpnData, keys, {
      address: vData.address,
      sessionToken: vData.session_token,
    });
  }, [finalizeVPNProvision, gatewayUrl]);

  const startAnonymousVPN = useCallback(async () => {
    const config = anonymousVPNConfig(gatewayUrl);
    const configProblems = validateAnonymousVPNConfig(config);
    if (configProblems.length > 0) {
      throw new Error(`Anonymous access is not configured: ${configProblems.join('; ')}`);
    }

    const challengeResp = await fetch(`${gatewayUrl}/auth/anonymous/challenge`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    if (!challengeResp.ok) {
      const err = await readResponsePayload(challengeResp);
      if (challengeResp.status === 404) {
        throw new Error(
          'Anonymous access is not available on the selected gateway.'
        );
      }
      throw new Error(responseErrorMessage(err, 'Failed to get anonymous challenge'));
    }
    const challenge = await readResponsePayload(challengeResp);
    if (!challenge || typeof challenge !== 'object') {
      throw new Error('Gateway returned an invalid anonymous challenge response');
    }
    if (challenge.proof_type !== 'vpn_access_v1') {
      throw new Error(`Unsupported anonymous proof type: ${challenge.proof_type}`);
    }
    markDone(0, `Challenge received for epoch ${challenge.policy_epoch}`);

    setCurrentStep(1);
    const keys = generateKeyPair();
    const challengeHash = await deriveVPNAccessV1ChallengeHash(challenge);
    const sessionKeyHash = await deriveVPNAccessV1SessionKeyHash(keys.publicKey);

    const { ZKClient } = await import('../6529-zk-service/dist/browser/index.js');
    const zkClient = new ZKClient({
      apiUrl: config.apiUrl,
      artifactBaseUrl: config.artifactBaseUrl,
    });
    const serviceMeta = await zkClient.getServiceMeta();
    const anonymousMeta = getAnonymousVPNMeta(serviceMeta);
    if (!anonymousMeta) {
      throw new Error('Anonymous VPN service metadata is unavailable.');
    }
    const identity = getOrCreateAnonymousVPNIdentity();
    const identityCommitment = await zkClient.deriveVPNAccessIdentityCommitment(identity);
    const allowDevRegistrationFallback =
      config.allowDevRegistrationFallback &&
      anonymousMeta?.devRegistration?.enabled;

    let issuerEntitlement;
    try {
      issuerEntitlement = await ensureAnonymousVPNIssuerEntitlement({
        apiUrl: config.apiUrl,
        anonymousMeta,
        address,
        allowDevRegistrationFallback,
        isConnected,
        signMessageAsync,
        identityCommitment,
        policyEpoch: challenge.policy_epoch,
      });
    } catch (error) {
      if (
        error instanceof AnonymousIssuerRequestError &&
        error.code === 'subscription_inactive'
      ) {
        const paymentOptions = await loadGatewayPaymentOptions({
          includeSessionOption: false,
          requireSubscriptionOptions: true,
        });
        setSteps(ANON_PAYMENT_STEPS);
        markDone(0, `Challenge received for epoch ${challenge.policy_epoch}`);
        markDone(1, 'Supported card verified; subscription required');
        setTierOptions({
          mode: 'anon-subscription',
          ...paymentOptions,
        });
        setSelectedTier(null);
        setPaymentIntent({ kind: 'anon-subscription' });
        setCurrentStep(2);
        setPhase('payment');
        return;
      }
      throw error;
    }
    if (issuerEntitlement) {
      markDone(
        1,
        issuerEntitlement.mode === 'refreshed'
          ? 'Anonymous credential refreshed'
          : 'Anonymous credential activated'
      );
    } else if (allowDevRegistrationFallback) {
      const registrationClient =
        config.devRegistrationUrl === config.apiUrl
          ? zkClient
          : new ZKClient({
              apiUrl: config.devRegistrationUrl,
              artifactBaseUrl: config.artifactBaseUrl,
          });
      await registrationClient.registerVPNAccessDevEntitlement({
        identityCommitment,
        policyEpoch: challenge.policy_epoch,
        registrationToken: config.devRegistrationToken,
        metadata: {
          source: 'site-app',
        },
      });
      markDone(1, 'Anonymous entitlement published');
    } else {
      throw new Error('Anonymous issuer did not return an entitlement result.');
    }

    setCurrentStep(2);
    let proofResult;
    try {
      proofResult = await zkClient.proveVPNAccessV1(
        {
          identitySecret: identity.identitySecret,
          identitySalt: identity.identitySalt,
          challengeHash,
          sessionKeyHash,
        },
        (stage) => {
          if (stage === 'done') {
            markDone(2, 'Anonymous proof generated');
          }
        }
      );
    } catch (err) {
      if (err instanceof Error && err.message.includes('Proof not found')) {
        throw new Error(
          allowDevRegistrationFallback
            ? 'Anonymous entitlement was not published for this device identity.'
            : 'No anonymous entitlement was found for this device identity.'
        );
      }
      throw err;
    }

    setCurrentStep(3);
    const connectResp = await fetch(`${gatewayUrl}/vpn/anonymous/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        challenge_id: challenge.challenge_id,
        proof_type: proofResult.proofType,
        proof: proofResult.proof,
        public_signals: proofResult.publicSignals,
        nullifier_hash: proofResult.publicSignals[4],
        session_key_hash: sessionKeyHash,
        public_key: keys.publicKey,
      }),
    });
    if (!connectResp.ok) {
      const err = await readResponsePayload(connectResp);
      throw new Error(responseErrorMessage(err, 'Failed to provision anonymous VPN'));
    }
    const vpnData = await readResponsePayload(connectResp);
    if (!vpnData || typeof vpnData !== 'object') {
      throw new Error('Gateway returned an invalid anonymous VPN response');
    }
    markDone(3, 'Anonymous VPN provisioned');

    await finalizeVPNProvision(vpnData, keys, {
      address: 'anonymous',
      sessionToken: vpnData.session_token,
    });
  }, [
    address,
    finalizeVPNProvision,
    gatewayUrl,
    isConnected,
    loadGatewayPaymentOptions,
    signMessageAsync,
  ]);

  const startVPN = useCallback(async () => {
    setPhase('running');
    setSteps(anonMode ? ANON_STEPS : FREE_STEPS);
    setCurrentStep(0);
    setCompletedSteps({});
    setErrorMsg('');

    setVerifyData(null);
    setPaymentTxHash(null);
    setPaymentIntent(null);
    setSelectedTier(null);
    setTierOptions(null);

    try {
      if (anonMode) {
        await startAnonymousVPN();
        return;
      }

      // Step 0: Get challenge
      const challengeResp = await fetch(`${gatewayUrl}/auth/challenge`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ address }),
      });
      if (!challengeResp.ok) {
        const err = await readResponsePayload(challengeResp);
        throw new Error(responseErrorMessage(err, 'Failed to get challenge'));
      }
      const challenge = await readResponsePayload(challengeResp);
      if (!challenge || typeof challenge !== 'object') {
        throw new Error('Gateway returned an invalid challenge response');
      }
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
      const vData = await readResponsePayload(verifyResp);
      if (!vData || typeof vData !== 'object') {
        throw new Error('Gateway returned an invalid verification response');
      }

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
        setPaymentIntent({ kind: 'legacy-connect' });

        const paymentOptions = await loadGatewayPaymentOptions({
          includeSessionOption: true,
          requireSubscriptionOptions: false,
        });

        setTierOptions({
          mode: 'legacy',
          ...paymentOptions,
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
  }, [address, anonMode, gatewayUrl, loadGatewayPaymentOptions, provisionVPN, signMessageAsync, startAnonymousVPN]);

  const handlePayAndConnect = useCallback(async () => {
    if (!selectedTier || !tierOptions) return;

    try {
      let hash;
      const paymentStepIndex =
        paymentIntent?.kind === 'anon-subscription' ? 2 : 3;
      const confirmStepIndex =
        paymentIntent?.kind === 'anon-subscription' ? 3 : 4;

      if (tierOptions.mode === 'legacy' && selectedTier === '24h') {
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
        if (!tier) {
          throw new Error('Select a valid subscription plan.');
        }
        hash = await writeContractAsync({
          address: tier.contract,
          abi: SUBSCRIPTION_MANAGER_ABI,
          functionName: 'subscribe',
          args: [tierOptions.session.node, tier.id],
          value: BigInt(tier.costWei),
        });
      }

      markDone(
        paymentStepIndex,
        paymentIntent?.kind === 'anon-subscription'
          ? 'Subscription transaction sent'
          : 'Payment sent'
      );
      setCurrentStep(confirmStepIndex);
      setPaymentTxHash(hash);
      // useWaitForTransactionReceipt will trigger continueAfterPayment via useEffect
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Payment failed');
      setPhase('error');
    }
  }, [paymentIntent, selectedTier, tierOptions, writeContractAsync]);

  const continueAfterPayment = useCallback(async () => {
    try {
      setPhase('running');
      markDone(4, 'Transaction confirmed');
      // Step 5: Provision VPN
      await provisionVPN(verifyData, 5);
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [provisionVPN, verifyData]);

  const continueAnonymousAfterSubscription = useCallback(async () => {
    try {
      markDone(3, 'Subscription confirmed');
      await startVPN();
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Subscription confirmed but activation retry failed');
      setPhase('error');
    }
  }, [startVPN]);

  // When tx confirms, continue the flow
  useEffect(() => {
    if (txConfirmed && phase === 'payment' && paymentIntent?.kind === 'anon-subscription') {
      continueAnonymousAfterSubscription();
      return;
    }
    if (txConfirmed && phase === 'payment' && verifyData) {
      continueAfterPayment();
    }
    if (txFailed && phase === 'payment') {
      setErrorMsg('Transaction failed on-chain');
      setPhase('error');
    }
  }, [continueAfterPayment, continueAnonymousAfterSubscription, paymentIntent, phase, txConfirmed, txFailed, verifyData]);

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

    setVerifyData(null);
    setPaymentTxHash(null);
    setPaymentIntent(null);
    setSelectedTier(null);
    setTierOptions(null);
  };

  // Build tier options for the picker
  const allTierOptions = tierOptions ? [
    ...(tierOptions.mode === 'legacy' && tierOptions.session?.costEth ? [
      { key: '24h', label: '24 Hours', price: tierOptions.session.costEth },
    ] : []),
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
          {anonMode
            ? 'Anonymous paid access beta: connect your wallet only to activate or refresh this browser entitlement, then the browser proves access locally and connects to the gateway anonymously.'
            : 'Sign in with your wallet to verify your Memes card and get a WireGuard config file.'}
        </p>

        {/* Wallet connect is required for legacy flow and issuer-backed anonymous activation */}
        {phase === 'idle' && (
          <div className="wallet-connect-wrapper">
            <ConnectButton showBalance={false} />
          </div>
        )}

        {phase === 'idle' && anonMode && (
          <button className="btn-primary" onClick={startVPN} style={{ marginTop: 16 }}>
            Activate Anonymous Access & Get VPN Config
          </button>
        )}

        {/* Idle: show sign-in button once wallet is connected */}
        {phase === 'idle' && !anonMode && isConnected && (
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
                <h3 style={{ marginBottom: 12 }}>
                  {paymentIntent?.kind === 'anon-subscription'
                    ? 'Choose a Subscription Plan'
                    : 'Choose Your Plan'}
                </h3>
                <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginBottom: 16 }}>
                  {paymentIntent?.kind === 'anon-subscription'
                    ? 'Anonymous access requires an active paid subscription on this wallet. After confirmation, the site will retry anonymous activation automatically.'
                    : 'Paid directly to the smart contract. 80% goes to the node operator, 20% to treasury.'}
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
                      {paymentIntent?.kind !== 'anon-subscription' && opt.key !== '24h' && (
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
                  {selectedTier
                    ? paymentIntent?.kind === 'anon-subscription'
                      ? 'Buy Subscription & Continue'
                      : 'Pay & Connect'
                    : 'Select a plan'}
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
