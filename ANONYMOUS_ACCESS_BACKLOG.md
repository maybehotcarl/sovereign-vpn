# Anonymous Access Backlog

## Scope

This backlog turns [ANONYMOUS_ACCESS_PROTOCOL.md](ANONYMOUS_ACCESS_PROTOCOL.md) into component-level implementation work.

Concrete issue-sized work items are listed in [ANONYMOUS_ACCESS_ISSUES.md](ANONYMOUS_ACCESS_ISSUES.md).

Priority definitions:

- `P0`: required for a gateway-private anonymous free-tier path
- `P1`: required to make anonymous access the default user path
- `P2`: required for anonymous paid access or stronger issuer privacy

## New Supporting Services

These services are not optional. The anonymous path does not work by patching only the gateway.

### Policy Indexer

- [ ] `P0` Build a policy indexer that computes epoch-scoped eligibility roots from `AccessPolicy`, delegation state, subscriptions, and user bans.
- [ ] `P0` Publish `policy_epoch`, active root set IDs, grace window, and verifier metadata for clients and gateway.
- [ ] `P0` Define revocation behavior for NFT transfers, delegation changes, subscription expiry, and governance bans.
- [ ] `P1` Add operational monitoring for root publication lag and stale-root acceptance windows.
- [ ] `P2` Add emergency denylist / revocation root support for bans that must propagate faster than epoch rotation.

### Credential Issuer

- [ ] `P0` Define the issuer API that mints short-lived anonymous VPN credentials against the active `policy_epoch`.
- [ ] `P0` Separate the issuer from the access gateway so wallet identity never reaches the gateway through issuance.
- [ ] `P0` Decide credential TTL and renewal cadence (`15-60 minutes` target from the protocol spec).
- [ ] `P1` Add credential refresh and expiry handling for long-lived user sessions.
- [ ] `P2` Implement blind issuance if issuer-private privacy is required, not just gateway-private privacy.

## Gateway

### Admission And Sessions

- [ ] `P0` Add an anonymous challenge endpoint that returns `challenge_id`, `nonce`, `policy_epoch`, proof type, and expiry.
- [ ] `P0` Add an anonymous connect path that accepts `proof`, `public_signals`, and `wg_pubkey` without a wallet address.
- [ ] `P0` Add a nullifier store with atomic consume / active-session semantics.
- [ ] `P0` Add an anonymous session model keyed by session ID and nullifier, not wallet address.
- [ ] `P0` Keep the current SIWE path only as a dual-stack compatibility mode.
- [ ] `P1` Make anonymous admission the default user path and gate legacy SIWE behind an explicit fallback flag.
- [ ] `P1` Enforce fresh WireGuard key usage or reject key reuse according to the chosen concurrency policy.

### Revocation And Abuse Control

- [ ] `P0` Reject proofs against stale root IDs outside the accepted grace window.
- [ ] `P0` Define whether anonymous access allows multiple concurrent sessions or one active session per credential epoch.
- [ ] `P1` Add epoch-scoped anonymous session tags if the product chooses one-active-session enforcement.
- [ ] `P1` Add rate limiting that does not rely on wallet address on the anonymous path.
- [ ] `P2` Add RLN-style or equivalent anonymous spam resistance if connect flooding becomes an operational problem.

### Privacy And Storage

- [ ] `P0` Ensure the anonymous path never writes wallet addresses into session storage, metrics dimensions, or logs.
- [ ] `P0` Keep `/vpn/status` and `/vpn/disconnect` token-based only on the anonymous path.
- [ ] `P1` Remove any remaining user-path dependency on address-bound gateway session identity.

## Site App

### Identity And Credential Handling

- [ ] `P0` Generate and persist a local anonymous identity secret / commitment separate from the wallet address.
- [ ] `P0` Add a credential issuance and refresh flow against the new issuer.
- [ ] `P0` Surface credential expiry / refresh errors in the UI without falling back silently to SIWE.
- [ ] `P1` Make anonymous connect the default path in the browser UI.

### Proof And Session Binding

- [ ] `P0` Integrate client-side proof generation for `vpn_access_v1`.
- [ ] `P0` Bind proofs to fresh WireGuard key material and the gateway challenge.
- [ ] `P1` Rotate WireGuard keys per session by default.
- [ ] `P1` Add UX for revoked / expired credentials and stale policy roots.

## CLI Client

### Identity And Credential Handling

- [ ] `P0` Add local storage for anonymous identity material separate from the wallet key.
- [ ] `P0` Add credential issuance and renewal commands or automatic refresh.
- [ ] `P1` Add migration from SIWE-first CLI flows to anonymous-first connect.

### Proof And Transport

- [ ] `P0` Generate `vpn_access_v1` proofs and submit them on the anonymous connect path.
- [ ] `P0` Generate a fresh WireGuard keypair for each session by default.
- [ ] `P1` Add user-visible diagnostics for stale roots, revocation, and nullifier conflicts.

## ZK Service

### Proof Contract

- [ ] `P0` Define `vpn_access_v1` public signals and verifier semantics.
- [ ] `P0` Include at minimum: `policy_epoch`, `tier`, `nullifier_hash`, and `session_key_hash`.
- [ ] `P0` Enforce challenge binding so proofs cannot be replayed on a later challenge or different gateway domain.
- [ ] `P0` Enforce verifier-key versioning and root freshness rules.

### Verification And Tooling

- [ ] `P0` Add deterministic test vectors for valid proof, stale root, wrong challenge, reused nullifier, and wrong session key binding.
- [ ] `P1` Expose machine-readable verification failure reasons for gateway/client diagnostics.
- [ ] `P1` Publish prover input requirements and example payloads for both browser and CLI clients.
- [ ] `P2` Add issuer-private / blind-issuance support if that trust level is required.

## Contracts

### User Access Path

- [ ] `P0` Stop treating live on-chain user identity as part of the private connect path.
- [ ] `P0` Remove free-tier user writes to `SessionManager` from the anonymous connect flow.
- [ ] `P1` Treat `AccessPolicy` as a policy-indexer input for anonymous access, not as a live per-request gateway identity check.

### Paid Access

- [ ] `P1` Decide the anonymous paid access primitive: private prepaid note, anonymous subscription credential, or private usage voucher.
- [ ] `P2` Implement the chosen anonymous entitlement primitive without a public per-user session record.
- [ ] `P2` Ensure operator settlement no longer depends on address-bound public user sessions.

## Payout And Settlement

- [ ] `P1` Define how anonymous paid usage maps to operator earnings without reintroducing a public user trail.
- [ ] `P2` Implement aggregate or note-based settlement compatible with existing operator payout privacy goals.

## Cross-Cutting Decisions

These must be decided before the backlog can be scheduled cleanly:

- [ ] Choose whether launch target is `gateway-private` only or also `issuer-private`.
- [ ] Choose anonymous concurrency policy: multiple sessions, one active session, or plan-based limits.
- [ ] Choose the paid entitlement model for anonymous paid access.
- [ ] Choose whether proof verification lives entirely in the ZK service or partially in the gateway.

## Suggested Delivery Order

1. `P0` supporting services: policy indexer, issuer, proof schema
2. `P0` gateway anonymous connect path and nullifier store
3. `P0` site app and CLI proof generation with fresh WireGuard keys
4. `P1` default anonymous clients and revocation hardening
5. `P1/P2` contract and settlement changes for anonymous paid access
6. `P2` blind issuance if issuer-private privacy is a product requirement

## Done Criteria

- [ ] A user can connect on the private path without the gateway learning the wallet address.
- [ ] The site app and CLI both use anonymous admission by default.
- [ ] Free-tier connect no longer creates a public user-linked on-chain session write.
- [ ] Revocation propagates within the configured epoch window.
- [ ] Paid anonymous access has a defined and implemented settlement model.
