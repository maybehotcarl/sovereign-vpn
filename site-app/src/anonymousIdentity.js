const STORAGE_KEY = 'svpn_vpn_access_identity_v1';
const BN254_FIELD_MODULUS = BigInt(
  '21888242871839275222246405745257275088548364400416034343698204186575808495617'
);

function bytesToFieldString(bytes) {
  let value = 0n;
  for (const byte of bytes) {
    value = (value << 8n) + BigInt(byte);
  }
  const fieldValue = value % BN254_FIELD_MODULUS;
  return (fieldValue === 0n ? 1n : fieldValue).toString();
}

function randomFieldString() {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return bytesToFieldString(bytes);
}

function parseIdentity(raw) {
  if (!raw) return null;

  try {
    const parsed = JSON.parse(raw);
    if (
      parsed &&
      typeof parsed.identitySecret === 'string' &&
      typeof parsed.identitySalt === 'string'
    ) {
      return {
        identitySecret: parsed.identitySecret,
        identitySalt: parsed.identitySalt,
        createdAt: typeof parsed.createdAt === 'string' ? parsed.createdAt : new Date().toISOString(),
      };
    }
  } catch {
    // ignore malformed local state
  }

  return null;
}

export function loadAnonymousVPNIdentity() {
  try {
    return parseIdentity(localStorage.getItem(STORAGE_KEY));
  } catch {
    return null;
  }
}

export function getOrCreateAnonymousVPNIdentity() {
  const existing = loadAnonymousVPNIdentity();
  if (existing) {
    return existing;
  }

  const identity = {
    identitySecret: randomFieldString(),
    identitySalt: randomFieldString(),
    createdAt: new Date().toISOString(),
  };

  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(identity));
  } catch {
    // fall through and keep the in-memory identity for this session
  }

  return identity;
}

export function clearAnonymousVPNIdentity() {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // no-op
  }
}
