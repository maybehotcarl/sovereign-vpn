import { useState, useCallback, useEffect } from 'react';
import { useAccount, usePublicClient, useSignMessage, useWriteContract, useWaitForTransactionReceipt } from 'wagmi';
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
  { id: 'entitlement', label: 'Refreshing anonymous credential...' },
  { id: 'challenge', label: 'Requesting anonymous challenge...' },
  { id: 'prove', label: 'Generating anonymous access proof...' },
  { id: 'vpn', label: 'Provisioning VPN connection...' },
];

const ANON_PAYMENT_STEPS = [
  { id: 'entitlement', label: 'Checking anonymous entitlement...' },
  { id: 'payment', label: 'Confirm subscription in your wallet...' },
  { id: 'confirm', label: 'Waiting for transaction confirmation...' },
  { id: 'challenge', label: 'Requesting anonymous challenge...' },
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

const ZERO_ADDRESS = '0x0000000000000000000000000000000000000000';

function subscriptionOptionKey(tierId) {
  return `subscription:${tierId}`;
}

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

function endpointCandidates(baseUrl, path, { preferSameOrigin = false } = {}) {
  const candidates = [];
  if (preferSameOrigin) {
    candidates.push(path);
  }
  if (baseUrl) {
    candidates.push(`${baseUrl}${path}`);
  }
  if (!preferSameOrigin) {
    candidates.push(path);
  }
  return [...new Set(candidates)];
}

async function fetchFirstOkJson(urls, timeoutMs = 3000) {
  for (const url of urls) {
    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), timeoutMs);
      const response = await fetch(url, { signal: controller.signal });
      clearTimeout(timeout);
      if (!response.ok) {
        continue;
      }
      const payload = await readResponsePayload(response);
      if (payload && typeof payload === 'object') {
        return payload;
      }
    } catch {
      // try the next candidate
    }
  }
  return null;
}

function formatDuration(seconds) {
  const days = Math.floor(seconds / 86400);
  if (days >= 365) return '365d';
  if (days >= 90) return '90d';
  if (days >= 30) return '30d';
  if (days >= 7) return '7d';
  return '24h';
}

function formatSubscriptionOptionLabel(durationSeconds) {
  const days = Math.floor(durationSeconds / 86400);
  if (days >= 365) return '365 Days';
  if (days >= 90) return '90 Days';
  if (days >= 30) return '30 Days';
  if (days >= 7) return '7 Days';
  return '24 Hours';
}

function shortenAddress(address) {
  if (!address || address.length < 10) {
    return address;
  }
  return `${address.slice(0, 6)}...${address.slice(-4)}`;
}

function normalizeDateValue(value) {
  if (value == null || value === '') {
    return null;
  }
  if (typeof value === 'string' && value.includes('T')) {
    return value;
  }
  try {
    return new Date(Number(value) * 1000).toISOString();
  } catch {
    return null;
  }
}

async function loadManagerSettlement(publicClient, contractAddress, abi) {
  if (!publicClient || !contractAddress) {
    return null;
  }

  try {
    const [operatorShareBps, treasury, payoutVault] = await Promise.all([
      publicClient.readContract({
        address: contractAddress,
        abi,
        functionName: 'operatorShareBps',
      }),
      publicClient.readContract({
        address: contractAddress,
        abi,
        functionName: 'treasury',
      }),
      publicClient.readContract({
        address: contractAddress,
        abi,
        functionName: 'payoutVault',
      }),
    ]);

    return {
      contract: contractAddress,
      operatorShareBps: Number(operatorShareBps),
      treasuryShareBps: 10000 - Number(operatorShareBps),
      treasury,
      payoutVault,
    };
  } catch {
    return null;
  }
}

export default function VPNConnect({ gatewayUrl = '', onSessionCreated }) {
  const { address, isConnected } = useAccount();
  const publicClient = usePublicClient();
  const { signMessageAsync } = useSignMessage();
  const { writeContractAsync } = useWriteContract();
  const anonymousAvailable = anonymousVPNEnabled();
  const [accessMode, setAccessMode] = useState(() =>
    anonymousAvailable ? 'anonymous' : 'direct'
  );
  const usingAnonymousMode =
    anonymousAvailable && accessMode === 'anonymous';

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
  const [selectedTier, setSelectedTier] = useState(null); // '24h' or `subscription:<tierId>`
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
    const [sessionInfo, subTiers] = await Promise.all([
      fetchFirstOkJson(
        endpointCandidates(gatewayUrl, '/session/info', { preferSameOrigin: true })
      ),
      fetchFirstOkJson(
        endpointCandidates(gatewayUrl, '/subscription/tiers', { preferSameOrigin: true })
      ),
    ]);

    const [sessionSettlement, subscriptionSettlement] = await Promise.all([
      loadManagerSettlement(publicClient, sessionInfo?.contract || '', SESSION_MANAGER_ABI),
      loadManagerSettlement(publicClient, subTiers?.contract || '', SUBSCRIPTION_MANAGER_ABI),
    ]);

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
        settlement: sessionSettlement,
      },
      subscription: {
        contract: subTiers?.contract || '',
        chainId: subTiers?.chain_id,
        settlement: subscriptionSettlement,
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
  }, [gatewayUrl, publicClient]);

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
        accessMode: sessionMeta.accessMode || 'wallet',
        sessionToken: sessionMeta.sessionToken,
        tier: vpnData.tier,
        expiresAt: vpnData.expires_at,
        subscriptionExpiresAt: sessionMeta.subscriptionExpiresAt || null,
        entitlementExpiresAt: sessionMeta.entitlementExpiresAt || null,
        refreshAt: sessionMeta.refreshAt || null,
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

  const provisionVPN = useCallback(async (vData, stepIdx, sessionMeta = {}) => {
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
      ...sessionMeta,
    });
  }, [finalizeVPNProvision, gatewayUrl]);

  const startAnonymousVPN = useCallback(async () => {
    const config = anonymousVPNConfig(gatewayUrl);
    const configProblems = validateAnonymousVPNConfig(config);
    if (configProblems.length > 0) {
      throw new Error(`Anonymous access is not configured: ${configProblems.join('; ')}`);
    }

    const { ZKClient } = await import('@6529/zk-service/browser');
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
        policyEpoch: anonymousMeta?.policy?.policyEpoch,
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
        markDone(0, 'Supported card verified; subscription required');
        setTierOptions({
          mode: 'anon-subscription',
          ...paymentOptions,
        });
        setSelectedTier(null);
        setPaymentIntent({ kind: 'anon-subscription' });
        setCurrentStep(1);
        setPhase('payment');
        return;
      }
      throw error;
    }
    const issuerResult = issuerEntitlement?.result ?? null;
    if (issuerEntitlement) {
      markDone(
        0,
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
        policyEpoch: anonymousMeta?.policy?.policyEpoch,
        registrationToken: config.devRegistrationToken,
        metadata: {
          source: 'site-app',
        },
      });
      markDone(0, 'Anonymous entitlement published');
    } else {
      throw new Error('Anonymous issuer did not return an entitlement result.');
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
      const message = responseErrorMessage(err, 'Failed to get anonymous challenge');
      if (message.includes('anonymous policy metadata unavailable')) {
        throw new Error(
          'Anonymous access is still publishing on the gateway. Retry in a few seconds.'
        );
      }
      throw new Error(message);
    }
    const challenge = await readResponsePayload(challengeResp);
    if (!challenge || typeof challenge !== 'object') {
      throw new Error('Gateway returned an invalid anonymous challenge response');
    }
    if (challenge.proof_type !== 'vpn_access_v1') {
      throw new Error(`Unsupported anonymous proof type: ${challenge.proof_type}`);
    }
    markDone(1, `Challenge received for epoch ${challenge.policy_epoch}`);

    const keys = generateKeyPair();
    const challengeHash = await deriveVPNAccessV1ChallengeHash(challenge);
    const sessionKeyHash = await deriveVPNAccessV1SessionKeyHash(keys.publicKey);

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
      accessMode: 'anonymous',
      subscriptionExpiresAt: normalizeDateValue(
        issuerResult?.entitlement?.metadata?.subscriptionExpiresAt
      ),
      entitlementExpiresAt: normalizeDateValue(
        issuerResult?.entitlementExpiresAt
      ),
      refreshAt: normalizeDateValue(issuerResult?.refreshAt),
    });
  }, [
    address,
    finalizeVPNProvision,
    gatewayUrl,
    isConnected,
    loadGatewayPaymentOptions,
    signMessageAsync,
  ]);

  const startDirectWalletVPN = useCallback(async function runDirectWalletVPN(allowRetry = true) {
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
        try {
          await provisionVPN(vData, 3, { accessMode: 'wallet' });
          return;
        } catch (err) {
          const message = err instanceof Error ? err.message : String(err);
          if (
            allowRetry &&
            (
              message.includes('session_token and public_key are required') ||
              message.includes('session expired or not found')
            )
          ) {
            markDone(2, 'Verified: paid tier; refreshing sign-in');
            await runDirectWalletVPN(false);
            return;
          }
          if (!message.includes('on-chain payment required for paid tier')) {
            throw err;
          }
        }

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
  }, [address, gatewayUrl, loadGatewayPaymentOptions, provisionVPN, signMessageAsync]);

  const startVPN = useCallback(async () => {
    setPhase('running');
    setSteps(usingAnonymousMode ? ANON_STEPS : FREE_STEPS);
    setCurrentStep(0);
    setCompletedSteps({});
    setErrorMsg('');

    setVerifyData(null);
    setPaymentTxHash(null);
    setPaymentIntent(null);
    setSelectedTier(null);
    setTierOptions(null);

    try {
      if (usingAnonymousMode) {
        await startAnonymousVPN();
        return;
      }

      await startDirectWalletVPN();
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [startAnonymousVPN, startDirectWalletVPN, usingAnonymousMode]);

  const handlePayAndConnect = useCallback(async () => {
    if (!selectedTier || !tierOptions) return;

    try {
      let hash;
      const paymentStepIndex =
        paymentIntent?.kind === 'anon-subscription' ? 1 : 3;
      const confirmStepIndex =
        paymentIntent?.kind === 'anon-subscription' ? 2 : 4;

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
        const tier = tierOptions.subscriptions.find(
          t => subscriptionOptionKey(t.id) === selectedTier
        );
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

      const needsFreshSignIn = !verifyData?.session_token;
      if (needsFreshSignIn) {
        markDone(4, 'Transaction confirmed; refreshing sign-in');
        await startVPN();
        return;
      }

      // Step 5: Provision VPN
      try {
        await provisionVPN(verifyData, 5);
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        if (
          message.includes('session_token and public_key are required') ||
          message.includes('session expired or not found')
        ) {
          markDone(4, 'Transaction confirmed; refreshing sign-in');
          await startVPN();
          return;
        }
        throw err;
      }
    } catch (err) {
      console.error(err);
      setErrorMsg(err.message || 'Something went wrong');
      setPhase('error');
    }
  }, [provisionVPN, startVPN, verifyData]);

  const continueAnonymousAfterSubscription = useCallback(async () => {
    try {
      markDone(2, 'Subscription confirmed');
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
    setSteps(usingAnonymousMode ? ANON_STEPS : FREE_STEPS);
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
  const subscriptionLabelCounts = tierOptions
    ? tierOptions.subscriptions.reduce((counts, tier) => {
        const label = formatSubscriptionOptionLabel(tier.duration);
        counts[label] = (counts[label] || 0) + 1;
        return counts;
      }, {})
    : {};

  const allTierOptions = tierOptions ? [
    ...(tierOptions.mode === 'legacy' && tierOptions.session?.costEth ? [
      {
        key: '24h',
        label: '24 Hours',
        price: tierOptions.session.costEth,
        description: 'Single paid session',
      },
    ] : []),
    ...tierOptions.subscriptions.map(t => {
      const baseLabel =
        TIER_LABELS[t.durationKey] || `${Math.floor(t.duration / 86400)} Days`;
      const isFreeTier = BigInt(t.costWei) === 0n;
      const hasDuplicateDuration = (subscriptionLabelCounts[baseLabel] || 0) > 1;

      const label = hasDuplicateDuration
        ? isFreeTier
          ? `${baseLabel} (Free)`
          : `${baseLabel} (${t.costEth} ETH)`
        : baseLabel;

      let description = null;
      if (isFreeTier) {
        description = hasDuplicateDuration ? 'Temporary free tier' : 'Free tier';
      } else if (hasDuplicateDuration) {
        description = `Tier ${t.id}`;
      }

      return {
        key: subscriptionOptionKey(t.id),
        label,
        price: t.costEth,
        tierId: t.id,
        description,
      };
    }),
  ] : [];

  const selectedTierOption = allTierOptions.find(option => option.key === selectedTier) || null;

  const selectedSettlement = (() => {
    if (!tierOptions) return null;
    if (paymentIntent?.kind === 'anon-subscription') {
      return tierOptions.subscription?.settlement || null;
    }
    if (selectedTier === '24h') {
      return tierOptions.session?.settlement || null;
    }
    if (selectedTier && selectedTier !== '24h') {
      return tierOptions.subscription?.settlement || null;
    }
    return tierOptions.session?.settlement || tierOptions.subscription?.settlement || null;
  })();

  const selectedContractAddress = (() => {
    if (!tierOptions) return '';
    if (paymentIntent?.kind === 'anon-subscription') {
      return tierOptions.subscription?.contract || '';
    }
    if (selectedTier === '24h') {
      return tierOptions.session?.contract || '';
    }
    if (selectedTier && selectedTier !== '24h') {
      return tierOptions.subscription?.contract || '';
    }
    return tierOptions.subscription?.contract || tierOptions.session?.contract || '';
  })();

  return (
    <div className="connect-section">
      <div className="connect-box">
        <h2 style={{ marginBottom: 12 }}>Connect & Get VPN Config</h2>
        <p style={{ color: 'var(--muted)', marginBottom: 24 }}>
          {usingAnonymousMode
            ? 'Anonymous paid access beta: buying a subscription pays for wallet-level access, but each VPN config is issued as a short-lived anonymous session. Your wallet is used only to activate or refresh this browser entitlement; the browser then proves access locally and connects to the gateway anonymously.'
            : 'Direct wallet mode gives you a normal wallet-bound VPN session without the short anonymous lease. Use this if you want a more typical VPN experience.'}
        </p>

        {phase === 'idle' && anonymousAvailable && (
          <div style={{ marginBottom: 20 }}>
            <div style={{ marginBottom: 10, fontWeight: 600 }}>Choose access mode</div>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
                gap: 12,
              }}
            >
              <button
                type="button"
                onClick={() => setAccessMode('direct')}
                style={{
                  textAlign: 'left',
                  padding: '14px 16px',
                  borderRadius: 10,
                  border: accessMode === 'direct'
                    ? '1px solid var(--accent, #4ecdc4)'
                    : '1px solid var(--border, #333)',
                  background: accessMode === 'direct'
                    ? 'rgba(78,205,196,0.08)'
                    : 'rgba(255,255,255,0.03)',
                  color: 'var(--text)',
                  cursor: 'pointer',
                }}
              >
                <div style={{ fontWeight: 600, marginBottom: 6 }}>Direct Wallet Session</div>
                <div style={{ color: 'var(--muted)', fontSize: '0.85rem', lineHeight: 1.5 }}>
                  Regular wallet-bound VPN use. Better for everyday use because you are not reconnecting on a 30-minute anonymous lease.
                </div>
              </button>
              <button
                type="button"
                onClick={() => setAccessMode('anonymous')}
                style={{
                  textAlign: 'left',
                  padding: '14px 16px',
                  borderRadius: 10,
                  border: accessMode === 'anonymous'
                    ? '1px solid var(--accent, #4ecdc4)'
                    : '1px solid var(--border, #333)',
                  background: accessMode === 'anonymous'
                    ? 'rgba(78,205,196,0.08)'
                    : 'rgba(255,255,255,0.03)',
                  color: 'var(--text)',
                  cursor: 'pointer',
                }}
              >
                <div style={{ fontWeight: 600, marginBottom: 6 }}>Anonymous Session</div>
                <div style={{ color: 'var(--muted)', fontSize: '0.85rem', lineHeight: 1.5 }}>
                  Better privacy. The browser proves access locally, but the resulting VPN session is intentionally short-lived and renewable.
                </div>
              </button>
            </div>
          </div>
        )}

        {/* Wallet connect is required for legacy flow and issuer-backed anonymous activation */}
        {phase === 'idle' && (
          <div className="wallet-connect-wrapper">
            <ConnectButton showBalance={false} />
          </div>
        )}

        {phase === 'idle' && usingAnonymousMode && (
          <button className="btn-primary" onClick={startVPN} style={{ marginTop: 16 }}>
            Activate Anonymous Access & Get VPN Config
          </button>
        )}

        {/* Idle: show sign-in button once wallet is connected */}
        {phase === 'idle' && !usingAnonymousMode && isConnected && (
          <button className="btn-primary" onClick={startVPN} style={{ marginTop: 16 }}>
            Sign In & Get VPN Config
          </button>
        )}

        {phase === 'idle' && !usingAnonymousMode && !isConnected && (
          <p style={{ color: 'var(--muted)', marginTop: 16, fontSize: '0.85rem' }}>
            Connect your wallet to start a direct wallet-bound VPN session.
          </p>
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
                    : 'Paid directly to the live mainnet smart contract. Funds are routed by the current on-chain payout configuration.'}
                </p>

                <div style={{
                  marginBottom: 16,
                  padding: '12px',
                  border: '1px solid var(--border, #333)',
                  borderRadius: 8,
                  background: 'rgba(255,255,255,0.03)',
                  fontSize: '0.85rem',
                  color: 'var(--muted)',
                }}>
                  <div style={{ marginBottom: 6 }}>
                    Payment contract: <code>{selectedContractAddress || 'Unavailable'}</code>
                  </div>
                  {selectedSettlement ? (
                    <>
                      <div style={{ marginBottom: 6 }}>
                        Current on-chain split: {selectedSettlement.operatorShareBps / 100}% operator, {selectedSettlement.treasuryShareBps / 100}% treasury
                      </div>
                      <div style={{ marginBottom: 6 }}>
                        Treasury: <code title={selectedSettlement.treasury}>{shortenAddress(selectedSettlement.treasury)}</code>
                      </div>
                      {selectedSettlement.operatorShareBps > 0 && selectedSettlement.payoutVault && selectedSettlement.payoutVault !== ZERO_ADDRESS && (
                        <div>
                          Operator payouts route via vault: <code title={selectedSettlement.payoutVault}>{shortenAddress(selectedSettlement.payoutVault)}</code>
                        </div>
                      )}
                    </>
                  ) : (
                    <div>Current treasury and split could not be read from the contract.</div>
                  )}
                </div>

                <div className="tier-picker">
                  {allTierOptions.map(opt => (
                    <button
                      key={opt.key}
                      type="button"
                      className={`tier-option${selectedTier === opt.key ? ' selected' : ''}`}
                      aria-pressed={selectedTier === opt.key}
                      onClick={() => setSelectedTier(opt.key)}
                    >
                      <span className="tier-option-label">{opt.label}</span>
                      <span className="tier-option-price">{opt.price} ETH</span>
                      {opt.description && (
                        <span className="tier-option-meta">{opt.description}</span>
                      )}
                      {paymentIntent?.kind !== 'anon-subscription' && opt.key !== '24h' && (
                        <span className="tier-savings">Save vs daily</span>
                      )}
                    </button>
                  ))}
                </div>

                {selectedTierOption && (
                  <p style={{ color: 'var(--muted)', fontSize: '0.85rem', marginTop: 12 }}>
                    Selected plan: {selectedTierOption.label} • {selectedTierOption.price} ETH
                    {selectedTierOption.description ? ` • ${selectedTierOption.description}` : ''}
                  </p>
                )}

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
