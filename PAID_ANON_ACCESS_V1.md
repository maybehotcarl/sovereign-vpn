# Paid Anonymous Access V1

## Status

This document defines the paid-anonymous launch variant for Sovereign VPN.

It is a design target, not the current implementation.

It narrows the broader target described in [ANONYMOUS_ACCESS_PROTOCOL.md](ANONYMOUS_ACCESS_PROTOCOL.md) to a paid-first user path where free tier remains supported by policy but is disabled by default.

## Why This Exists

The generic anonymous-access documents describe a system that can support both free and paid tiers. The immediate product direction is narrower:

- keep free-tier logic in the system
- do not offer free-tier admission by default
- move toward anonymous paid access first
- route user payment to the treasury, not directly to operators

That changes the most important design question from:

- "Does this user hold THIS card?"

to:

- "Does this user currently have anonymous paid access?"

## Launch Assumptions

These assumptions are fixed for `v1` unless explicitly changed:

1. Launch privacy target is `gateway-private`, not `issuer-private`.
2. Users pay the treasury, not the operator directly.
3. Operators are compensated from treasury-side aggregate settlement, not per-user on-chain session records.
4. Access still requires Memes-card eligibility.
5. Free tier remains part of the long-term policy model, but is disabled by default for launch.
6. Anonymous paid access is modeled as a short-lived active entitlement, not a one-time per-session spend note.
7. The anonymous path does not create public per-user session records on-chain.

## Core Design

`v1` should not ask the verifier to perform a live query like:

- "Did wallet `0x...` pay the treasury?"

That would reintroduce address linkage into the user path.

Instead, `v1` uses this split:

1. An issuer/indexer checks whether a wallet is both card-eligible and currently paid.
2. If yes, the issuer/indexer publishes or issues a short-lived anonymous paid-access credential bound to the user's hidden identity commitment.
3. The client later proves anonymous membership in the active paid-access root.
4. The gateway verifies the proof and never learns the wallet address.

In other words, the zk statement is not:

- "I am wallet X and I paid"

It is:

- "I hold a currently valid anonymous paid-access credential in the active root."

## Architectural Split

### 1. Treasury Payment Layer

Responsible for:

- receiving user payment
- pricing plans such as `24h`, `7d`, or `30d`
- exposing the payment events or subscription state that drive entitlement activation

Not responsible for:

- proving payment directly to the gateway
- linking a wallet address to a VPN session on the user path

### 2. Entitlement Indexer / Issuer

Responsible for:

- checking Memes-card eligibility
- consuming the treasury payment source of truth, such as subscription state, paid-plan events, or a dedicated access ledger
- converting that payment state into an active entitlement record
- applying bans or revocations
- mapping a user to a short-lived anonymous paid-access credential
- assigning `entitlement_class` and `expiry_bucket`
- publishing the active `paid_access_root` for the current `policy_epoch`
- publishing the grace-window metadata and root schema version the prover/verifier need

This component may learn the wallet in `gateway-private` mode.

It must not pass that wallet identity to the gateway.

The issuer/indexer is the only layer that should answer questions like:

- does this wallet currently satisfy the Memes-card gate?
- does this wallet currently have paid access?
- when does that paid access expire?

The gateway and verifier should only consume the derived anonymous entitlement state.

### 3. ZK Service

Responsible for:

- verifying `vpn_access_v1`
- validating active root freshness
- validating challenge binding
- validating session-key binding
- rejecting reused nullifiers

It should verify proofs against an active paid-access root.

It should not perform live chain lookups for payment or card ownership at verify time.

### 4. Gateway

Responsible for:

- issuing anonymous challenges
- verifying proof metadata and challenge/session binding
- issuing anonymous session tokens
- provisioning WireGuard peers
- keeping anonymous session state only

The gateway should learn:

- challenge ID
- proof validity
- entitlement class
- session token
- WireGuard public key

The gateway should not learn:

- wallet address
- payment transaction source address
- Memes-card identity

### 5. Settlement Layer

Responsible for:

- attributing operator earnings from treasury-side accounting
- paying operators without reintroducing a public user trail

It should use aggregate usage or aggregate session accounting, not per-user public paid-session writes.

## Identity Model

Each client holds:

- `identity_secret`: local secret, never shared
- `identity_commitment`: public commitment derived from `identity_secret`

The issuer/indexer binds paid access to `identity_commitment`, not to the gateway session.

That allows:

- issuer-side eligibility checks
- gateway-side anonymous proof verification

without requiring the gateway to learn the wallet.

## Entitlement Model

`v1` uses a short-lived active entitlement rather than a spend-once note.

An active entitlement means:

- the user currently satisfies the Memes-card gate
- the user currently has a paid plan or treasury-paid access window
- the entitlement expires at a defined time bucket

Recommended initial shapes:

- `24h`
- `7d`
- `30d`

The entitlement should be renewed or republished as needed. It does not need to be consumed on every connect.

### Why This Is Better Than A Per-Session Spend Note For V1

- it matches the existing product direction of time-based access
- it simplifies reconnects inside the paid window
- it removes the need to design per-session anonymous micro-settlement first
- it lets replay prevention be challenge-scoped instead of payment-note-spend-scoped

## Protocol Flow

### Phase 1: Payment

1. The user pays the treasury for a supported plan.
2. The payment layer records plan class and expiry.
3. The issuer/indexer observes the paid status.

This payment may still be public on-chain in `v1`.

That is acceptable for `gateway-private` privacy, but not sufficient for full chain-private anonymity.

## Phase 2: Entitlement Activation

1. The user authenticates to the issuer.
2. The issuer checks:
   - Memes-card eligibility
   - paid status from the treasury payment source of truth
   - ban / revocation state
3. The issuer normalizes that state into a deterministic entitlement record:
   - `identity_commitment`
   - `entitlement_class`
   - `expiry_bucket`
   - `policy_epoch`
   - schema version
4. The issuer or indexer publishes:
   - the active root for the current `policy_epoch`
   - the accepted grace root, if any
   - the expiry-bucket rules
   - proof lookup material for the prover
5. The client uses that published state to construct a proof against the active paid-access root.

The issuer may issue a local credential artifact as a convenience, but the gateway path should rely on the active root and proof semantics, not on a bearer token from the issuer.

### Phase 3: Anonymous Challenge

1. The gateway returns:
   - `challenge_id`
   - `nonce`
   - `policy_epoch`
   - `proof_type = vpn_access_v1`
   - `expires_at`
2. The client generates a fresh WireGuard keypair.
3. The client computes `session_key_hash` from the WireGuard public key.

### Phase 4: Proof Generation

The client generates a `vpn_access_v1` proof that demonstrates:

- membership in the active paid-access root
- current `policy_epoch`
- valid expiry bucket
- binding to the gateway challenge
- binding to `session_key_hash`
- a valid challenge-scoped nullifier

For circuit compatibility, `challenge_hash` and `session_key_hash` should be
represented as field-safe decimal strings on the wire, not raw hex digests.
Hashing may still use SHA-256 upstream, but the final public-signal value must
be reduced into the proving field before it reaches the gateway/verifier.

### Phase 5: Gateway Verification

The gateway forwards the proof to the ZK service and requires all of the following:

- proof is cryptographically valid
- root is active or within the accepted grace window
- `policy_epoch` matches the active policy window
- challenge hash matches the issued challenge
- `session_key_hash` matches the submitted WireGuard public key
- nullifier has not been used for this proof scope
- entitlement expiry has not passed

If valid:

- the gateway creates an anonymous session
- the gateway provisions a WireGuard peer
- the gateway returns a session token

### Phase 6: Reconnect And Refresh

If the user reconnects while the entitlement remains active:

- the client requests a new challenge
- the client generates a new proof
- the client uses a fresh WireGuard keypair

If the entitlement has expired:

- the issuer must refresh or reissue active paid access after a new treasury payment or renewal

## Verifier Semantics

For `v1`, the proof answers this exact question:

- "Does this hidden identity currently belong to the active paid-access set for the current policy epoch, and is this proof bound to this challenge and this session key?"

It does not answer:

- which Memes card the user holds
- which wallet paid
- which treasury transaction funded access

Those are issuer/indexer concerns, not gateway-verifier concerns.

## Replay And Concurrency

`v1` only requires challenge-scoped replay prevention.

That means:

- a proof cannot be replayed for the same challenge
- a proof cannot be copied to a different session key
- a proof cannot be replayed after the challenge expires

`v1` does not require the proof itself to enforce one-active-session semantics.

If product policy later needs that, add a separate anonymous session tag or concurrency class at the gateway layer.

## Contract And Settlement Impact

This design implies:

- the anonymous paid path should not depend on `SessionManager.openSession(...)`
- the anonymous paid path should not create public per-user paid session records
- operator compensation should come from treasury-side aggregate accounting

That means the existing on-chain paid-session flow can remain as a legacy path, but it is not the target anonymous paid path.

## What This Repo Must Implement

In `sovereign-vpn`:

- anonymous paid challenge/connect endpoints
- anonymous session model
- gateway verification of `vpn_access_v1`
- treasury-oriented paid-plan UX
- removal of user-linked paid admission assumptions from the anonymous path
- aggregate operator payout accounting for anonymous sessions

## What The ZK Project Must Implement

In the ZK service:

- `vpn_access_v1` proof type
- active paid-access root handling
- public-signal contract
- verifier failure codes
- nullifier semantics for the paid-access proof

## Explicit Non-Goals For V1

- issuer-private / blind issuance
- chain-private payment
- direct user-to-operator anonymous payment
- per-byte private metering
- one-proof support for both paid and free launch variants

## Immediate Next Steps

1. Freeze the `vpn_access_v1` public-signal contract in the ZK project.
2. Define the entitlement issuer/indexer API.
3. Decide the initial paid entitlement classes (`24h`, `7d`, `30d`).
4. Replace the current anonymous free-path placeholder assumptions in the gateway with the paid-access model.
5. Define aggregate operator settlement for anonymous paid sessions.
