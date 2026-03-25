import { webcrypto } from 'node:crypto';
import { Buffer } from 'node:buffer';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname } from 'node:path';
import { ZKClient } from '/home/maybe/repos/sovereign-vpn/site-app/6529-zk-service/dist/browser/index.js';
import { generateKeyPair } from '/home/maybe/repos/sovereign-vpn/site-app/src/wgkeys.js';

globalThis.crypto ??= webcrypto;
globalThis.btoa ??= (value) => Buffer.from(value, 'binary').toString('base64');

const BN254_FIELD_MODULUS = BigInt(
  '21888242871839275222246405745257275088548364400416034343698204186575808495617'
);

const gatewayUrl = (process.env.GATEWAY_URL || 'http://127.0.0.1:8081').replace(
  /\/$/,
  ''
);
const zkApiUrl = (process.env.ZK_API_URL || 'http://127.0.0.1:3002').replace(
  /\/$/,
  ''
);
const artifactBaseUrl = (
  process.env.ZK_ARTIFACT_BASE_URL ||
  '/home/maybe/repos/sovereign-vpn/site-app/6529-zk-service/build'
).replace(/\/$/, '');
const identitySecret = process.env.VPN_ACCESS_IDENTITY_SECRET || '111';
const identitySalt = process.env.VPN_ACCESS_IDENTITY_SALT || '222';
const outputPath =
  process.env.OUTPUT_PATH || '/tmp/rehearsal-anon-session.json';

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

async function deriveChallengeHash(challenge) {
  const expiresUnix = Math.floor(new Date(challenge.expires_at).getTime() / 1000);
  return sha256FieldString(
    [
      'vpn_access_v1',
      challenge.challenge_id,
      challenge.nonce,
      String(challenge.policy_epoch),
      String(expiresUnix),
    ].join('|')
  );
}

async function deriveSessionKeyHash(publicKey) {
  return sha256FieldString(publicKey.trim());
}

async function parseJson(response, label) {
  const text = await response.text();
  let payload;
  try {
    payload = text ? JSON.parse(text) : null;
  } catch {
    throw new Error(`${label} returned non-JSON response: ${text}`);
  }
  if (!response.ok) {
    throw new Error(`${label} failed: ${response.status} ${JSON.stringify(payload)}`);
  }
  return payload;
}

async function main() {
  const zkClient = new ZKClient({
    apiUrl: zkApiUrl,
    artifactBaseUrl,
  });

  const challengeResp = await fetch(`${gatewayUrl}/auth/anonymous/challenge`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  });
  const challenge = await parseJson(challengeResp, 'challenge');

  const keys = generateKeyPair();
  const challengeHash = await deriveChallengeHash(challenge);
  const sessionKeyHash = await deriveSessionKeyHash(keys.publicKey);
  const identityCommitment = await zkClient.deriveVPNAccessIdentityCommitment({
    identitySecret,
    identitySalt,
  });

  const proofResult = await zkClient.proveVPNAccessV1(
    {
      identitySecret,
      identitySalt,
      challengeHash,
      sessionKeyHash,
    },
    (stage, detail) => {
      const suffix = detail ? ` ${detail}` : '';
      console.log(`[prove:${stage}]${suffix}`);
    }
  );

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
  const session = await parseJson(connectResp, 'anonymous connect');

  const output = {
    createdAt: new Date().toISOString(),
    gatewayUrl,
    requestedGatewayUrl: gatewayUrl,
    zkApiUrl,
    identityCommitment,
    challenge,
    sessionToken: session.session_token,
    publicKey: keys.publicKey,
    privateKey: keys.privateKey,
    gatewayInstanceId: session.gateway_instance_id || null,
    ownerGatewayUrl: session.gateway_url || gatewayUrl,
    clientAddress: session.client_address,
    serverEndpoint: session.server_endpoint,
    serverPublicKey: session.server_public_key,
    expiresAt: session.expires_at,
    tier: session.tier,
  };

  await mkdir(dirname(outputPath), { recursive: true });
  await writeFile(outputPath, JSON.stringify(output, null, 2));

  console.log(JSON.stringify(output, null, 2));
  console.log(`\nSaved session to ${outputPath}`);
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack || error.message : String(error));
  process.exitCode = 1;
});
