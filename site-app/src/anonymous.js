const BN254_FIELD_MODULUS = BigInt(
  '21888242871839275222246405745257275088548364400416034343698204186575808495617'
);

const PROOF_TYPE = 'vpn_access_v1';

function envFlag(value) {
  return value === '1' || value === 'true';
}

function normalizeBaseUrl(value) {
  return value.replace(/\/$/, '');
}

function bytesToFieldString(bytes) {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) + BigInt(byte);
  }
  return (value % BN254_FIELD_MODULUS).toString();
}

async function sha256FieldString(input) {
  const encoded = new TextEncoder().encode(input);
  const digest = await crypto.subtle.digest('SHA-256', encoded);
  return bytesToFieldString(new Uint8Array(digest));
}

export function anonymousVPNEnabled() {
  return envFlag(import.meta.env.VITE_ENABLE_ANON_VPN || '');
}

export function anonymousVPNConfig(gatewayUrl = '') {
  const configuredZKApiUrl = (import.meta.env.VITE_ZK_API_URL || '').trim();
  const fallbackBaseUrl = gatewayUrl
    ? normalizeBaseUrl(gatewayUrl)
    : window.location.origin;
  const apiUrl = normalizeBaseUrl(configuredZKApiUrl || fallbackBaseUrl);
  const artifactBaseUrl = normalizeBaseUrl(
    (import.meta.env.VITE_ZK_ARTIFACT_BASE_URL || `${apiUrl}/api/artifacts`).trim()
  );

  return {
    enabled: anonymousVPNEnabled(),
    proofType: PROOF_TYPE,
    apiUrl,
    artifactBaseUrl,
    identitySecret: (import.meta.env.VITE_VPN_ACCESS_IDENTITY_SECRET || '').trim(),
    identitySalt: (import.meta.env.VITE_VPN_ACCESS_IDENTITY_SALT || '').trim(),
  };
}

export function validateAnonymousVPNConfig(config) {
  const problems = [];
  if (!config.identitySecret) {
    problems.push('VITE_VPN_ACCESS_IDENTITY_SECRET is required');
  }
  if (!config.identitySalt) {
    problems.push('VITE_VPN_ACCESS_IDENTITY_SALT is required');
  }
  if (!config.apiUrl) {
    problems.push('VITE_ZK_API_URL or gateway URL is required');
  }
  if (!config.artifactBaseUrl) {
    problems.push('VITE_ZK_ARTIFACT_BASE_URL is required');
  }
  return problems;
}

export async function deriveVPNAccessV1ChallengeHash(challenge) {
  const expiresUnix = Math.floor(new Date(challenge.expires_at).getTime() / 1000);
  return sha256FieldString(
    [
      PROOF_TYPE,
      challenge.challenge_id,
      challenge.nonce,
      String(challenge.policy_epoch),
      String(expiresUnix),
    ].join('|')
  );
}

export async function deriveVPNAccessV1SessionKeyHash(publicKey) {
  return sha256FieldString(publicKey.trim());
}
