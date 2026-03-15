# Anonymous Access Issue List

## Scope

This document turns [ANONYMOUS_ACCESS_BACKLOG.md](ANONYMOUS_ACCESS_BACKLOG.md) into a recommended issue ordering with rough implementation slices.

Ready-to-paste GitHub issue bodies for `AA-01` through `AA-12` are in [ANONYMOUS_ACCESS_GITHUB_ISSUES.md](ANONYMOUS_ACCESS_GITHUB_ISSUES.md).

Sizing legend:

- `S`: 1-2 focused engineering days
- `M`: 3-5 engineering days
- `L`: 1-2 engineering weeks
- `XL`: multi-week, cross-component work

## Recommended Slice Plan

### Slice 1: Foundations

Issues `AA-01` through `AA-04`

Goal:

- lock the anonymous-access contract
- define proof semantics
- publish usable roots
- issue short-lived credentials

### Slice 2: Gateway Anonymous Path

Issues `AA-05` through `AA-06`

Goal:

- admit users without wallet identity
- issue anonymous sessions
- enforce replay protection and root freshness

### Slice 3: Client Adoption

Issues `AA-07` through `AA-09`

Goal:

- browser and CLI can both generate proofs
- both clients use fresh WireGuard keys
- both clients can renew anonymous credentials

### Slice 4: Default Anonymous Mode

Issues `AA-10` through `AA-12`

Goal:

- remove free-tier public identity writes
- make anonymous access the default path
- harden revocation and observability

### Slice 5: Anonymous Paid Access

Issues `AA-13` and `AA-14`

Goal:

- replace public-wallet paid admission
- keep operator settlement compatible with privacy goals

### Slice 6: Stronger Privacy Upgrade

Issue `AA-15`

Goal:

- add issuer-private protection if product claims require it

## Issue List

### AA-01: Lock anonymous access launch decisions

- Priority: `P0`
- Size: `S`
- Components: architecture, gateway, zk service, clients
- Depends on: none
- Deliverables:
  - choose `gateway-private` launch target, with `issuer-private` explicitly deferred or required
  - choose concurrency policy: multiple sessions, one active session, or plan-based limits
  - choose verifier placement: ZK service only, gateway only, or split verification
  - record the chosen defaults in the protocol/backlog docs
- Acceptance:
  - protocol open decisions are resolved enough that implementation can start without re-litigating core assumptions

### AA-02: Define `vpn_access_v1` proof contract

- Priority: `P0`
- Size: `M`
- Components: zk service, gateway, clients
- Depends on: `AA-01`
- Deliverables:
  - fix public signals and their semantics
  - define challenge binding, `nullifier_hash`, and `session_key_hash`
  - define proof expiry and root freshness rules
  - publish example verifier inputs/outputs and failure codes
- Acceptance:
  - browser, CLI, gateway, and zk service can all code against a stable `vpn_access_v1` contract

### AA-03: Build policy indexer and root publication

- Priority: `P0`
- Size: `L`
- Components: new supporting service, contracts integration
- Depends on: `AA-01`
- Deliverables:
  - compute epoch-scoped eligibility roots from `AccessPolicy`, delegation, subscriptions, and ban state
  - publish active `policy_epoch`, root IDs, grace window, and verifier metadata
  - document revocation behavior for transfer, delegation loss, plan expiry, and ban
- Acceptance:
  - gateway and clients can consume a stable root publication API and identify the active epoch

### AA-04: Build short-lived credential issuer

- Priority: `P0`
- Size: `L`
- Components: new supporting service, clients
- Depends on: `AA-01`, `AA-03`
- Deliverables:
  - issuer API for anonymous VPN credentials against the active epoch
  - credential TTL / renewal policy
  - separation of issuer and gateway trust boundaries
  - refresh flow semantics for long-lived sessions
- Acceptance:
  - a client can obtain and renew an anonymous credential without the gateway learning wallet identity

### AA-05: Implement gateway challenge, nullifier store, and anonymous session model

- Priority: `P0`
- Size: `M`
- Components: gateway
- Depends on: `AA-02`, `AA-03`
- Deliverables:
  - anonymous challenge endpoint
  - nullifier store with atomic consume or active-session semantics
  - anonymous session record keyed by session ID / nullifier, not wallet
  - root freshness and grace-window enforcement
- Acceptance:
  - the gateway can issue a challenge and safely track anonymous sessions without using address-based state

### AA-06: Implement gateway anonymous connect path

- Priority: `P0`
- Size: `L`
- Components: gateway, zk service integration
- Depends on: `AA-02`, `AA-04`, `AA-05`
- Deliverables:
  - new connect path accepting `proof`, `public_signals`, and `wg_pubkey`
  - proof verification and session issuance
  - token-only `status` / `disconnect` behavior on the anonymous path
  - explicit legacy SIWE fallback mode retained only for compatibility
- Acceptance:
  - a user can connect without sending a wallet address to the gateway

### AA-07: Add anonymous identity and credential handling to the site app

- Priority: `P0`
- Size: `M`
- Components: `site-app`
- Depends on: `AA-04`
- Deliverables:
  - local anonymous identity secret / commitment generation
  - credential issuance and refresh flow
  - user-visible handling for expired or revoked credentials
- Acceptance:
  - the browser client can hold an anonymous identity and keep its credential fresh

### AA-08: Add proof generation and anonymous connect flow to the site app

- Priority: `P0`
- Size: `L`
- Components: `site-app`, gateway integration, zk tooling
- Depends on: `AA-02`, `AA-06`, `AA-07`
- Deliverables:
  - `vpn_access_v1` proof generation in the browser
  - proof binding to challenge and fresh WireGuard key material
  - anonymous connect submission and error handling
- Acceptance:
  - the browser client can complete an anonymous free-tier connect end to end

### AA-09: Add anonymous identity, proof flow, and fresh WireGuard keys to the CLI

- Priority: `P0`
- Size: `L`
- Components: `client`
- Depends on: `AA-02`, `AA-04`, `AA-06`
- Deliverables:
  - local storage for anonymous identity material
  - credential issuance / renewal flow
  - proof generation and submission
  - per-session WireGuard keypair rotation
- Acceptance:
  - the CLI can complete the anonymous free-tier path end to end

### AA-10: Remove free-tier public user session writes from the private path

- Priority: `P1`
- Size: `M`
- Components: gateway, contracts, session manager integration
- Depends on: `AA-06`
- Deliverables:
  - stop calling `openFreeSession(user, ...)` from the anonymous connect path
  - document the replacement behavior for free-tier accounting / telemetry
  - ensure no public user-linked free-tier session record is emitted
- Acceptance:
  - anonymous free-tier connect creates no public user-linked on-chain session write

### AA-11: Make anonymous access the default user path

- Priority: `P1`
- Size: `M`
- Components: gateway, `site-app`, `client`
- Depends on: `AA-08`, `AA-09`, `AA-10`
- Deliverables:
  - anonymous connect is the default in browser and CLI
  - SIWE path is hidden behind an explicit compatibility switch
  - reused WireGuard keys are rejected or warned according to the selected policy
- Acceptance:
  - the normal user path is anonymous-first, not SIWE-first

### AA-12: Harden revocation, diagnostics, and anonymous-path observability

- Priority: `P1`
- Size: `M`
- Components: policy indexer, gateway, site app, client, zk service
- Depends on: `AA-03`, `AA-05`, `AA-06`, `AA-08`, `AA-09`
- Deliverables:
  - stale-root rejection is enforced consistently
  - revocation lag and root publication lag are observable
  - clients receive actionable errors for revocation, nullifier conflicts, and stale policy state
- Acceptance:
  - the anonymous path is operationally supportable without falling back to address-based debugging

### AA-13: Choose and spec the anonymous paid entitlement model

- Priority: `P1`
- Size: `M`
- Components: contracts, payout/settlement, gateway, architecture
- Depends on: `AA-01`
- Deliverables:
  - choose between private prepaid note, anonymous subscription credential, or private usage voucher
  - document how operator earnings derive from anonymous paid usage
  - document how revocation and double-spend prevention work for paid access
- Acceptance:
  - paid anonymous access has a concrete technical model rather than an open design question

### AA-14: Implement anonymous paid access and privacy-compatible settlement

- Priority: `P2`
- Size: `XL`
- Components: contracts, gateway, zk service, payout/settlement, clients
- Depends on: `AA-13`
- Deliverables:
  - anonymous paid entitlement verification on connect
  - no public-wallet `SessionManager` / subscription identity on the private connect path
  - aggregate or note-based settlement that preserves operator payout requirements
- Acceptance:
  - paid users can connect without reintroducing a public user identity trail

### AA-15: Add issuer-private blind issuance

- Priority: `P2`
- Size: `L`
- Components: issuer, zk service, clients
- Depends on: `AA-04`
- Deliverables:
  - blind issuance or equivalent issuer-private mechanism
  - issuer cannot link wallet identity to the resulting anonymous credential
  - updated protocol and product claims reflecting the stronger trust model
- Acceptance:
  - privacy claims can extend beyond gateway-private to issuer-private

## Suggested Initial Milestone

If you want the first milestone to be narrow and real, stop after:

- `AA-01`
- `AA-02`
- `AA-03`
- `AA-04`
- `AA-05`
- `AA-06`
- `AA-08`
- `AA-09`

That gives you:

- anonymous free-tier connect
- gateway-private privacy
- dual-stack compatibility
- both browser and CLI coverage

It does not yet give you:

- anonymous paid access
- issuer-private privacy
- removal of every legacy SIWE path

## Mapping Back To Release Gates

- `AA-05`, `AA-06`, `AA-10`, and `AA-11` are the main path to the anonymous session / no-SIWE gates.
- `AA-03`, `AA-04`, and `AA-12` are the main path to the issuer/root/revocation gates.
- `AA-08` and `AA-09` are the main path to the client adoption gate.
- `AA-13` and `AA-14` are the main path to the anonymous paid-access gate.
- `AA-15` is the main path to the issuer-private gate.
