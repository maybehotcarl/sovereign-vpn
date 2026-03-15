# Anonymous Access Protocol

## Status

This document defines the target privacy-preserving access architecture for Sovereign VPN.

It is not the current implementation.

The current implementation still uses SIWE on the user path, which reveals the wallet address to the gateway and may publish session metadata on-chain. This document describes the architecture that should replace that path if protecting user wallet identity is a core product requirement.

Implementation work is tracked in [ANONYMOUS_ACCESS_BACKLOG.md](ANONYMOUS_ACCESS_BACKLOG.md).

## Why This Exists

The current access flow proves identity first and entitlement second:

1. The client signs a SIWE message.
2. The gateway recovers the wallet address.
3. The gateway checks card ownership or a ZK proof.
4. The gateway creates an address-bound session.
5. Some session flows may write user metadata on-chain.

That flow is incompatible with anonymous access. A valid ZK proof does not fix the privacy problem if the verifier already knows the wallet address.

The target architecture therefore changes the admission primitive:

- Do not prove "I am wallet `0x...`."
- Prove "I satisfy the current VPN access policy."

## Goals

- The access gateway must not learn the user's wallet address on the private access path.
- Access decisions must be based on entitlement, not identity.
- Proofs must be bound to a gateway challenge and an ephemeral session key to prevent replay.
- Sessions must be keyed to anonymous session identifiers, not wallet addresses.
- Revocation must work for NFT transfers, delegation changes, subscription expiry, and user bans within a bounded freshness window.
- The protocol must support both free and paid tiers.
- The protocol must work with ephemeral WireGuard keys so sessions are not linkable by a long-lived peer key.

## Non-Goals

- Hiding the user's source IP from the gateway.
- Hiding traffic destinations or content from the VPN node.
- Defending against a global passive network observer.
- Reusing the current on-chain per-user session model.

Those require separate network-layer changes such as multi-hop routing, blinded coordination, or mixnet-style transports.

## Threat Model

This protocol is designed so that:

- The gateway can verify entitlement without learning the wallet address.
- A compromised node still cannot infer the wallet address from the auth protocol.
- Replay of a previously valid proof does not produce another valid session.
- Transferred or revoked entitlements expire quickly enough that stale access is bounded.

This protocol does not guarantee privacy against:

- The gateway seeing the client's source IP.
- The credential issuer, unless blind issuance is implemented.
- The blockchain itself, if paid usage still relies on public-wallet transactions.

## Recommended Primitive

Use anonymous credentials with Semaphore-style nullifier proofs on the connect path.

Operationally, that means:

1. Users hold a locally generated secret and identity commitment.
2. Eligible users obtain an anonymous VPN credential for the current policy epoch.
3. The gateway verifies a zero-knowledge proof of:
   - membership in the current eligible group
   - correct tier attributes
   - proof freshness
   - binding to the requested session key
4. The gateway records only anonymous session data and proof nullifiers.

Why this design is preferred:

- It avoids forcing the gateway to learn the wallet address.
- It avoids expensive zk-secp256k1 proofs on every connect.
- It gives a clean replay-prevention model via nullifiers.
- It maps well onto the existing 6529 ZK service model.

### Explicit Non-Recommendation

Do not use SIWE on the private user access path.

SIWE may remain for:

- admin and operator dashboards
- wallet-linked governance actions
- explicit opt-in account management flows

It should not be the admission mechanism for the private VPN connection path.

## Trust Model

There are two acceptable trust levels:

### Level 1: Gateway-Private

- The gateway does not learn the wallet address.
- A separate issuer may still learn the wallet during credential issuance.

This is already a major privacy improvement over the current architecture.

### Level 2: Issuer-Private

- The gateway does not learn the wallet address.
- The issuer also cannot link the issued credential to the wallet.

This requires blind issuance or a direct anonymous entitlement proof system. It is the stronger long-term target.

The gateway-private property is mandatory. Issuer-private is strongly recommended if the project wants to market the system as meaningfully privacy-preserving rather than merely "gateway-blind."

## Components

### 1. Policy Indexer

Builds the authoritative access view for each policy epoch from:

- `AccessPolicy`
- delegate rights
- subscription state
- user ban and revocation state
- project-specific free-tier rules

Outputs:

- `policy_epoch`
- active root set IDs
- group roots by tier
- revocation root
- metadata required by the gateway and prover

### 2. Credential Issuer

Issues short-lived VPN credentials against the current policy epoch.

Inputs:

- proof that a wallet is currently eligible
- client-generated identity commitment

Outputs:

- credential for `free` or `paid` tier
- expiry bucket / epoch binding
- optional concurrency class or plan attributes

The issuer must be separated from the gateway. The gateway must never receive wallet identity from issuance.

### 3. ZK Prover

Runs client-side or in a trusted local helper and generates proofs for:

- group membership
- tier attributes
- revocation non-membership or freshness
- challenge binding
- ephemeral session key binding

### 4. ZK Verifier Service

Verifies the proof object and validates:

- proof type
- verifying key version
- root freshness
- nullifier correctness

This can extend the current ZK verification service.

### 5. Access Gateway

Issues challenges, verifies proof metadata, tracks nullifiers, and provisions anonymous sessions. It must not require a wallet address on the anonymous path.

### 6. Settlement Service

Handles operator payouts without a per-user public session trail. Paid usage settlement must be aggregate or note-based, not wallet-address based on the user path.

## Credential Model

Each client maintains:

- `identity_secret`: local secret, never shared
- `identity_commitment`: public commitment derived from `identity_secret`
- `credential`: short-lived proof of current eligibility

Recommended properties:

- Fresh credential epoch: `15-60 minutes`
- New WireGuard keypair per session
- New session token per connection

If a user transfers away the qualifying NFT, loses delegation, is banned, or loses an active plan, the next epoch should exclude them from the active root set.

## Protocol Overview

### A. Policy Publication

For each epoch:

1. The policy indexer computes the access state from chain and external sources.
2. Eligible commitments are partitioned into tier groups such as `free` and `paid`.
3. A revocation root is produced for revoked or invalidated credentials.
4. The verifier service and gateway publish the active `policy_epoch` and root set IDs.

The gateway only accepts proofs against the active epoch or a tightly bounded grace window.

### B. Credential Issuance

1. The client generates `identity_secret` and `identity_commitment`.
2. The client authenticates to the issuer to prove current eligibility.
3. The issuer checks card ownership, delegation, plan validity, and ban state.
4. The issuer issues a credential bound to:
   - `identity_commitment`
   - `policy_epoch`
   - `tier`
   - optional usage attributes
5. The credential is stored client-side.

If issuer privacy is required, this step must use blind issuance so the issuer cannot link wallet identity to the resulting anonymous credential.

### C. Anonymous Connect

1. The client requests a challenge from the gateway.
2. The gateway returns:
   - `challenge_id`
   - `nonce`
   - `policy_epoch`
   - accepted proof types
   - proof expiry
3. The client generates a fresh WireGuard public key and an ephemeral session key.
4. The client proves:
   - possession of a valid credential in the active root
   - correct tier attribute
   - non-revocation or valid epoch membership
   - binding to `challenge_id || nonce || wg_pubkey || session_pubkey`
5. The proof exposes only public signals such as:
   - `policy_epoch`
   - `tier`
   - `nullifier_hash`
   - `session_key_hash`
   - optional plan or usage class
6. The gateway verifies the proof and root set.
7. The gateway atomically marks the `nullifier_hash` as spent or active.
8. The gateway provisions the WireGuard peer and returns:
   - WireGuard config
   - opaque anonymous session token
   - session expiry

At no point on this path should the gateway receive the user's wallet address.

### D. Status And Disconnect

`/vpn/status` and `/vpn/disconnect` use only the opaque session token.

The gateway session record should contain:

- session ID
- tier
- `policy_epoch`
- `wg_pubkey_hash`
- `nullifier_hash`
- expiry
- optional anonymous concurrency tag

It must not contain the user's wallet address.

## Proof Semantics

The connect proof must bind to both a challenge and a fresh transport identity.

Minimum binding requirements:

- `external_nullifier = H(domain || challenge_id || policy_epoch || action)`
- `signal_hash = H(wg_pubkey || session_pubkey || requested_tier || expiry_bucket)`

This prevents:

- replay on a later challenge
- replay on a different gateway domain
- reuse of a proof for a different WireGuard key

## Concurrency Model

The protocol must choose one of these explicit policies:

### Option A: Allow Multiple Concurrent Sessions

- Each challenge produces a unique connect proof.
- Nullifiers prevent replay only.

### Option B: One Active Session Per Credential Epoch

- Proof also exposes an epoch-scoped anonymous session tag.
- The gateway rejects a second active session with the same tag.

If the product wants a strict anti-sharing rule, use Option B. If the product prefers better UX across devices, use plan attributes with an explicit concurrent session count.

## Revocation Model

Revocation is handled by short epochs plus root rotation.

Revocation sources:

- NFT transfer
- delegation revocation
- subscription expiry
- governance or abuse ban

Requirements:

- Root publication cadence must be fast enough to bound stale access.
- Session TTL must not exceed the maximum revocation window the product is willing to tolerate.
- The gateway must reject proofs against stale root IDs outside the allowed grace window.

For abuse response faster than an epoch, add a revocation root or emergency denylist checked during proof verification.

## Paid Access

Anonymous access is incomplete if paid usage still forces the user onto a public-wallet session path.

The correct paid design is:

1. The user acquires a private entitlement:
   - private prepaid note
   - anonymous subscription credential
   - private usage voucher
2. The connect proof attests that the entitlement is valid and unspent.
3. The gateway authorizes access without learning the wallet address.
4. Operator settlement happens in aggregate or from private note accounting, not from a public per-user session record.

### What Must Not Continue On The Anonymous Path

- Address-bound `SessionManager` sessions
- `openFreeSession(user, ...)` writes for user access
- public-wallet pay-per-session deposits tied to the connect flow

Those mechanisms reintroduce a public identity trail and defeat the privacy model.

## Bans And Reputation

The current address-based user ban check does not survive the anonymous model.

The correct replacement is:

- user bans are applied before issuance by excluding the user from the eligible policy set
- emergency bans revoke credentials through the current revocation mechanism

For anonymous abuse controls, use:

- nullifier-based replay prevention
- epoch-scoped rate limits
- optional RLN-style spam resistance if connect flooding becomes a problem

Operator reputation remains public and address-based. That does not conflict with anonymous user access.

## Delegation

Delegation must be resolved during policy computation or issuance, not during per-session gateway auth.

That means:

- delegate rights are folded into the policy indexer's eligibility view
- delegates receive the same tier-appropriate credential as direct holders
- the gateway does not query delegate registries per anonymous connect

## API Shape

The private path should converge on this flow:

### `POST /auth/challenge`

Response:

```json
{
  "challenge_id": "ch_123",
  "nonce": "base64url...",
  "policy_epoch": 412,
  "proof_type": "vpn_access_v1",
  "expires_at": "2026-03-13T21:00:00Z"
}
```

### `POST /vpn/connect`

Request:

```json
{
  "challenge_id": "ch_123",
  "proof_type": "vpn_access_v1",
  "proof": { "pi_a": [], "pi_b": [], "pi_c": [] },
  "public_signals": [
    "policy_epoch",
    "tier",
    "nullifier_hash",
    "session_key_hash"
  ],
  "wg_pubkey": "base64..."
}
```

Response:

```json
{
  "session_token": "opaque-token",
  "tier": "free",
  "expires_at": "2026-03-13T22:00:00Z",
  "server_public_key": "...",
  "server_endpoint": "vpn.example.com:51820",
  "client_address": "10.0.0.12/24",
  "dns": "1.1.1.1",
  "allowed_ips": "0.0.0.0/0, ::/0"
}
```

### `GET /vpn/status`

Use `Authorization: Bearer <session_token>`.

### `POST /vpn/disconnect`

Use the anonymous session token or a signed session key, never a wallet address.

## Data Storage Rules

The gateway may persist only:

- anonymous session IDs
- nullifier hashes
- root IDs / policy epoch
- tier
- session timestamps
- operational counters

The gateway must not persist on the anonymous path:

- user wallet address
- full WireGuard public key in logs
- SIWE message or signature
- address-derived user identifier

## Migration Plan

### Phase 1: Dual-Stack Admission

- Keep the current SIWE path for legacy clients.
- Add anonymous challenge and proof-based connect flow.
- Add nullifier store and anonymous session records.

### Phase 2: Anonymous Clients By Default

- Update CLI and site app to generate proofs and fresh WireGuard keys.
- Make anonymous connect the default user path.
- Retain SIWE only for explicit wallet-linked UX.

### Phase 3: Remove Public User Session Writes

- Remove free-tier user writes to `SessionManager` from the connect path.
- Stop treating wallet address as the gateway session identity.

### Phase 4: Anonymous Paid Access

- Replace public-wallet paid session initiation with anonymous entitlements.
- Move operator settlement to aggregate or private-note accounting.

### Phase 5: Stronger Issuer Privacy

- Add blind issuance if the project wants privacy against the issuer as well as the gateway.

## Repository Impact

### Gateway

Replace the user-facing SIWE admission path with:

- anonymous challenge endpoint
- proof verifier integration
- nullifier store
- anonymous session model

Retain SIWE only for non-private paths.

### Site App And CLI

Add:

- local identity commitment generation
- credential refresh flow
- proof generation
- per-session WireGuard key rotation

### Contracts

Keep:

- `AccessPolicy` as an input to policy computation
- `NodeRegistry` and operator staking flows
- payout and operator accounting

Deprecate on the user access path:

- address-bound free-session writes
- address-bound public paid-session admission

### Payout Service

No user-address change is needed for operator payouts, but paid user settlement must move away from per-user public sessions if anonymous paid access is required.

## Acceptance Criteria

The architecture is only complete when all of the following are true:

- A private-path user can connect without the gateway receiving their wallet address.
- A connect proof cannot be replayed on another challenge or another WireGuard key.
- Free-tier connect does not create a public on-chain record linking user and node.
- Paid-tier connect does not require a public-wallet identity on the connect path.
- NFT transfer, ban, or plan expiry revokes future access within the configured epoch window.
- Gateway logs and storage contain no wallet identifiers for anonymous sessions.

## Open Decisions

- Whether issuer privacy is mandatory at launch or deferred to a later phase.
- Whether concurrency control is one-session-per-epoch or plan-based multi-device.
- Whether paid access uses private prepaid notes, anonymous subscriptions, or another private entitlement model.
- Whether the ZK service verifies proofs directly or the gateway embeds the verifier locally.
- Whether multi-hop or blinded coordination is required for a stronger overall privacy claim.

## Bottom Line

The correct privacy-preserving user path is:

- challenge
- anonymous credential proof
- ephemeral WireGuard key binding
- anonymous session token
- no wallet address at the gateway
- no per-user public session write

Anything that starts with SIWE on the user connect path is a compatibility path, not the privacy architecture.
