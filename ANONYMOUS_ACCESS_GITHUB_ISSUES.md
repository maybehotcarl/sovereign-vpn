# Anonymous Access GitHub Issues

## Scope

This document contains ready-to-paste GitHub issue bodies for `AA-01` through `AA-12` from [ANONYMOUS_ACCESS_ISSUES.md](ANONYMOUS_ACCESS_ISSUES.md).

## Epic Recommendation

Recommended classification for the full anonymous-access track:

- `Epic`: `AA-03`, `AA-04`, `AA-06`, `AA-08`, `AA-09`, `AA-14`
- `Normal issue`: `AA-01`, `AA-02`, `AA-05`, `AA-07`, `AA-10`, `AA-11`, `AA-12`, `AA-13`, `AA-15`

Recommended classification for the first implementation wave:

- `AA-01`: normal issue
- `AA-02`: normal issue
- `AA-03`: epic
- `AA-04`: epic
- `AA-05`: normal issue
- `AA-06`: epic

Rationale:

- `AA-03`, `AA-04`, and `AA-06` are multi-step, cross-component efforts that will likely need child tasks.
- `AA-01`, `AA-02`, and `AA-05` are still important, but they are better framed as single deliverables with clear closure conditions.

## Suggested Labels

Base labels:

- `privacy`
- `anonymous-access`

Optional labels by type:

- `type/epic`
- `type/issue`

Optional labels by priority:

- `priority/P0`
- `priority/P1`
- `priority/P2`

Optional labels by area:

- `area/architecture`
- `area/gateway`
- `area/zk`
- `area/site-app`
- `area/client`
- `area/contracts`
- `area/settlement`
- `area/platform`

## AA-01

Title:

```text
AA-01: Lock anonymous access launch decisions
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P0, privacy, anonymous-access, area/architecture, area/gateway, area/zk
```

Body:

```markdown
## Summary

Lock the launch-critical decisions for anonymous access so engineering work can start without re-opening the core architecture on every task.

## Why

The protocol and backlog are defined, but implementation cannot start cleanly until we choose the launch trust model, concurrency behavior, and verifier placement. These decisions affect the gateway, clients, ZK service, and future release gates.

## Decisions Required

- Choose launch target:
  - `gateway-private` only
  - `gateway-private` + `issuer-private`
- Choose concurrency policy:
  - multiple concurrent sessions
  - one active session per credential epoch
  - plan-based limits
- Choose verifier placement:
  - ZK service only
  - gateway only
  - split verification

## Deliverables

- Written decision record for the three items above
- Updated anonymous-access docs reflecting the chosen defaults
- Any resulting changes to release-gate language if needed

## Acceptance Criteria

- The launch privacy target is explicitly chosen
- The anonymous session concurrency model is explicitly chosen
- The verifier placement model is explicitly chosen
- The protocol/backlog docs reflect those choices
- Follow-on issues can reference these decisions without ambiguity

## Out Of Scope

- Implementing the anonymous connect path
- Building the policy indexer or issuer
- Building blind issuance

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-07

Title:

```text
AA-07: Add anonymous identity and credential handling to the site app
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P0, privacy, anonymous-access, area/site-app
```

Body:

```markdown
## Summary

Add anonymous identity storage and credential lifecycle handling to the site app so the browser client can participate in the anonymous-access flow.

## Why

The browser client cannot use anonymous admission until it can manage a local anonymous identity and keep short-lived credentials fresh without silently falling back to SIWE.

## Scope

Implement in the site app:

- local anonymous identity secret / commitment generation
- credential issuance integration with the issuer
- credential refresh / renewal handling
- user-visible handling for expired or revoked credentials

## Deliverables

- Browser-side anonymous identity storage model
- Credential issuance flow
- Credential renewal flow
- UI behavior for expiry / revocation / refresh failures

## Acceptance Criteria

- The browser client can generate and keep a local anonymous identity
- The browser client can obtain a credential for the active policy epoch
- The browser client can renew credentials according to issuer policy
- Expired or revoked credentials do not silently fall back to SIWE

## Out Of Scope

- Browser proof generation
- Gateway anonymous connect endpoint
- Making anonymous access the default browser path

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
```

## AA-08

Title:

```text
AA-08: Add proof generation and anonymous connect flow to the site app
```

Type:

```text
Epic
```

Suggested labels:

```text
type/epic, priority/P0, privacy, anonymous-access, area/site-app, area/gateway, area/zk
```

Body:

```markdown
## Summary

Add the full anonymous-connect path to the site app so the browser client can generate a proof, bind it to fresh WireGuard key material, and connect without disclosing wallet identity to the gateway.

## Why

Anonymous access is not real for browser users until the site app can drive the new challenge -> proof -> connect flow end to end.

## Scope

Implement in the site app:

- `vpn_access_v1` proof generation
- challenge fetching and proof binding
- fresh WireGuard key binding for the session
- anonymous connect submission
- user-visible handling for proof and revocation failures

## Deliverables

- Browser proof generation for `vpn_access_v1`
- Challenge-aware connect flow
- Session-bound key handling
- Anonymous connect UI and error states

## Acceptance Criteria

- The browser client can complete an anonymous free-tier connect end to end
- The proof is bound to the gateway challenge and fresh session key material
- Anonymous connect does not require the browser to submit wallet identity to the gateway
- Failures are actionable enough for support and debugging

## Suggested Child Tasks

- wire up challenge request flow
- integrate browser proof generation
- bind proof to fresh WireGuard key material
- submit anonymous connect request
- handle stale root / nullifier / revocation failures in the UI

## Out Of Scope

- Making anonymous access the default browser path
- Anonymous paid entitlement flow
- Issuer-private blind issuance

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-09

Title:

```text
AA-09: Add anonymous identity, proof flow, and fresh WireGuard keys to the CLI
```

Type:

```text
Epic
```

Suggested labels:

```text
type/epic, priority/P0, privacy, anonymous-access, area/client, area/gateway, area/zk
```

Body:

```markdown
## Summary

Add the anonymous-access flow to the CLI so command-line users can obtain credentials, generate proofs, rotate WireGuard keys per session, and connect without disclosing wallet identity to the gateway.

## Why

The first anonymous-access milestone is incomplete unless both supported clients can use it. The CLI needs the same privacy guarantees as the browser path.

## Scope

Implement in the CLI:

- local storage for anonymous identity material
- credential issuance / renewal flow
- `vpn_access_v1` proof generation
- anonymous connect submission
- fresh WireGuard keypair rotation per session

## Deliverables

- CLI identity storage model
- Credential issuance / refresh commands or automation
- Proof generation and submission path
- Per-session WireGuard key rotation
- CLI-visible diagnostics for proof / revocation failures

## Acceptance Criteria

- The CLI can complete the anonymous free-tier connect path end to end
- The CLI rotates WireGuard keys per session by default
- The CLI can renew credentials without exposing wallet identity to the gateway
- Errors for stale roots, revocation, and nullifier conflicts are actionable

## Suggested Child Tasks

- add anonymous identity storage
- add issuer integration
- add proof generation path
- rotate WireGuard keys per session
- add error and status diagnostics for anonymous connect

## Out Of Scope

- Anonymous paid access
- Blind issuance
- Making anonymous mode the default for all legacy CLI entry points

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-10

Title:

```text
AA-10: Remove free-tier public user session writes from the private path
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P1, privacy, anonymous-access, area/gateway, area/contracts
```

Body:

```markdown
## Summary

Remove free-tier public user-linked session writes from the anonymous private path so a successful anonymous connect no longer emits a public on-chain user record.

## Why

Anonymous gateway admission is not enough if the free-tier path still writes `openFreeSession(user, ...)` or an equivalent public user-linked session record on-chain.

## Scope

Change the anonymous private path so that:

- it does not call `openFreeSession(user, ...)`
- it does not emit an equivalent public user-linked free-tier record
- replacement telemetry or accounting behavior is documented

## Deliverables

- Anonymous free-tier path without public user-linked session writes
- Updated documentation for free-tier accounting / telemetry behavior
- Validation that anonymous free-tier connect no longer creates a public user-node link

## Acceptance Criteria

- Anonymous free-tier connect produces no public user-linked on-chain session record
- Gateway private-path logic no longer depends on `openFreeSession(user, ...)`
- Replacement observability / accounting behavior is documented

## Out Of Scope

- Anonymous paid entitlement model
- Making anonymous access the default path
- Blind issuance

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-11

Title:

```text
AA-11: Make anonymous access the default user path
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P1, privacy, anonymous-access, area/gateway, area/site-app, area/client
```

Body:

```markdown
## Summary

Make anonymous access the default user path in the gateway, site app, and CLI, leaving SIWE only as an explicit compatibility fallback.

## Why

Anonymous access only meaningfully changes product behavior once it becomes the normal path for users rather than an alternative flow hidden behind legacy defaults.

## Scope

Update the user path so that:

- browser and CLI default to anonymous connect
- SIWE is retained only as an explicit compatibility mode
- key-reuse behavior matches the chosen concurrency policy

## Deliverables

- Anonymous-first browser flow
- Anonymous-first CLI flow
- Explicit legacy SIWE fallback switch
- Key-reuse handling consistent with concurrency policy

## Acceptance Criteria

- Normal user connect behavior is anonymous-first
- SIWE is no longer the default private-path admission mechanism
- Browser and CLI behavior match the selected concurrency policy
- Legacy compatibility remains available but clearly separated

## Out Of Scope

- Anonymous paid entitlement implementation
- Blind issuance
- Revocation observability hardening beyond the core path change

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-12

Title:

```text
AA-12: Harden revocation, diagnostics, and anonymous-path observability
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P1, privacy, anonymous-access, area/platform, area/gateway, area/site-app, area/client, area/zk
```

Body:

```markdown
## Summary

Harden revocation handling and operational diagnostics for the anonymous path so it is supportable in production without falling back to address-based debugging.

## Why

Anonymous access removes wallet-based debugging shortcuts. Root publication lag, revocation timing, stale roots, and nullifier conflicts need first-class diagnostics or the system will be hard to operate.

## Scope

Implement:

- consistent stale-root rejection
- observability for revocation lag and root publication lag
- actionable client and gateway errors for:
  - revocation
  - stale policy state
  - nullifier conflicts

## Deliverables

- Root freshness enforcement across the anonymous path
- Revocation / publication lag observability
- Actionable diagnostics for browser, CLI, and gateway operators

## Acceptance Criteria

- Stale roots are rejected consistently
- Operators can observe revocation lag and publication lag
- Browser and CLI receive actionable errors instead of opaque failures
- Support does not need wallet identity to reason about common anonymous-path failures

## Out Of Scope

- Anonymous paid entitlement model
- Blind issuance
- Major changes to the underlying proof contract unless required for diagnostics

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-02

Title:

```text
AA-02: Define vpn_access_v1 proof contract
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P0, privacy, anonymous-access, area/zk, area/gateway, area/client, area/site-app
```

Body:

```markdown
## Summary

Define the `vpn_access_v1` proof contract so the gateway, site app, CLI, and ZK service can all implement against a stable interface.

## Why

The anonymous-access path depends on a shared definition of proof semantics. Without a fixed proof contract, client and gateway work will drift and rework will be expensive.

## Scope

Define and document:

- proof type name and versioning rules
- public signals
- challenge binding rules
- `nullifier_hash` semantics
- `session_key_hash` semantics
- root freshness rules
- proof expiry rules
- verifier failure codes / machine-readable errors

At minimum, the public contract should include:

- `policy_epoch`
- `tier`
- `nullifier_hash`
- `session_key_hash`

## Deliverables

- A written `vpn_access_v1` proof contract
- Example payloads for prover and verifier
- Example success/failure responses
- Stable verifier semantics that downstream work can target

## Acceptance Criteria

- Gateway, browser client, CLI, and ZK service can all code against the same proof contract
- Challenge-binding and replay-prevention semantics are explicit
- Root freshness behavior is explicit
- Failure reasons are specific enough for client diagnostics

## Out Of Scope

- Policy indexer implementation
- Credential issuer implementation
- Anonymous session storage

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
```

## AA-03

Title:

```text
AA-03: Build policy indexer and root publication for anonymous access
```

Type:

```text
Epic
```

Suggested labels:

```text
type/epic, priority/P0, privacy, anonymous-access, area/platform, area/contracts, area/gateway
```

Body:

```markdown
## Summary

Build the policy indexer that produces epoch-scoped anonymous-access roots and publishes the active policy metadata consumed by the gateway and clients.

## Why

Anonymous access requires a root publication layer that replaces live wallet-based gateway checks. The gateway cannot verify anonymous proofs without a stable source of truth for the active policy epoch and eligible roots.

## Scope

Build a service that:

- computes eligibility roots from:
  - `AccessPolicy`
  - delegation state
  - subscription state
  - user ban / revocation state
- publishes:
  - `policy_epoch`
  - active root IDs
  - grace window
  - verifier metadata
- defines revocation behavior for:
  - NFT transfer
  - delegation revocation
  - subscription expiry
  - governance bans

## Deliverables

- Policy indexer service or module
- Root publication API / artifact format
- Epoch and grace-window semantics
- Revocation behavior documentation
- Operational visibility for stale-root / publication lag in a follow-on task if not included here

## Acceptance Criteria

- Gateway and clients can discover the active `policy_epoch` and current root set
- Eligibility roots reflect the required inputs from contracts and off-chain state
- Revocation behavior is defined and testable
- The indexer can be treated as the source of truth for anonymous admission

## Suggested Child Tasks

- ingest `AccessPolicy` eligibility inputs
- ingest delegation state
- ingest subscription state
- ingest ban / revocation state
- build root computation pipeline
- publish active-root metadata
- document epoch and revocation semantics

## Out Of Scope

- Gateway anonymous connect path
- Browser or CLI proof generation
- Blind issuance

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-04

Title:

```text
AA-04: Build short-lived anonymous credential issuer
```

Type:

```text
Epic
```

Suggested labels:

```text
type/epic, priority/P0, privacy, anonymous-access, area/platform, area/client, area/site-app
```

Body:

```markdown
## Summary

Build the short-lived credential issuer used by anonymous-access clients to obtain credentials for the active policy epoch without sending wallet identity to the gateway.

## Why

The anonymous-connect path depends on a separate issuer trust boundary. The gateway should verify proofs, not learn the user's wallet during issuance.

## Scope

Build an issuer that:

- issues anonymous VPN credentials for the active `policy_epoch`
- is operationally separate from the gateway
- defines credential TTL and renewal behavior
- supports both browser and CLI clients
- documents refresh behavior for long-lived sessions

## Deliverables

- Issuer API
- Credential issuance flow
- Credential renewal / expiry model
- Explicit gateway/issuer trust boundary
- Client-consumable response format

## Acceptance Criteria

- A client can obtain a credential for the active epoch
- A client can renew a credential before or after expiry according to the policy
- Gateway does not receive wallet identity through issuer integration
- Browser and CLI clients have a stable contract to integrate against

## Suggested Child Tasks

- define issuer API
- implement issuance against active policy epoch
- define TTL / renewal cadence
- implement client auth / eligibility handoff to issuer
- document operational separation from gateway

## Out Of Scope

- Blind issuance / issuer-private privacy
- Gateway anonymous connect path
- Anonymous paid entitlement issuance

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```

## AA-05

Title:

```text
AA-05: Implement gateway challenge, nullifier store, and anonymous session model
```

Type:

```text
Issue
```

Suggested labels:

```text
type/issue, priority/P0, privacy, anonymous-access, area/gateway
```

Body:

```markdown
## Summary

Add the gateway-side primitives needed for anonymous sessions: challenge issuance, nullifier storage, and an anonymous session record that is not keyed by wallet address.

## Why

Even with a valid anonymous proof contract, the gateway cannot support anonymous access until it can issue challenges, prevent replay, and track sessions without address-based identity.

## Scope

Implement:

- anonymous challenge endpoint
- nullifier store with atomic consume or active-session semantics
- anonymous session model keyed by session ID / nullifier
- root freshness and grace-window enforcement hooks

## Deliverables

- challenge API
- nullifier persistence model
- anonymous session storage model
- gateway-side root freshness enforcement

## Acceptance Criteria

- The gateway can issue a challenge usable by the anonymous proof flow
- Nullifier handling prevents trivial replay on the gateway
- Anonymous sessions are not keyed by wallet address
- Root freshness checks are available to the anonymous connect path

## Out Of Scope

- Full anonymous connect endpoint
- Browser or CLI proof generation
- Removal of legacy SIWE compatibility paths

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
```

## AA-06

Title:

```text
AA-06: Implement gateway anonymous connect path
```

Type:

```text
Epic
```

Suggested labels:

```text
type/epic, priority/P0, privacy, anonymous-access, area/gateway, area/zk
```

Body:

```markdown
## Summary

Implement the gateway's anonymous connect path so users can connect by presenting a valid proof and fresh WireGuard key without disclosing wallet identity to the gateway.

## Why

This is the core gateway deliverable for anonymous access. Without it, the project still relies on SIWE-first admission and remains wallet-visible at the gateway.

## Scope

Implement a connect path that:

- accepts `proof`, `public_signals`, and `wg_pubkey`
- verifies the proof against the active root and proof contract
- consumes or activates the nullifier
- provisions the anonymous session
- returns an opaque session token
- keeps `status` and `disconnect` token-based on the anonymous path
- retains SIWE only as an explicit compatibility fallback

## Deliverables

- anonymous connect endpoint
- proof verification integration
- nullifier consume / active-session enforcement
- anonymous session issuance
- token-only anonymous `status` / `disconnect`
- explicit dual-stack compatibility behavior for legacy clients

## Acceptance Criteria

- A valid anonymous proof can produce a VPN session without sending a wallet address to the gateway
- Replayed or stale proofs are rejected
- Anonymous session issuance is fully token-based after connect
- Legacy SIWE behavior remains isolated as a compatibility path, not the primary private path

## Suggested Child Tasks

- implement anonymous connect handler
- integrate proof verification
- enforce nullifier state transitions
- issue anonymous session token
- adapt `status` and `disconnect` behavior for anonymous sessions
- document legacy fallback behavior

## Out Of Scope

- Browser proof generation
- CLI proof generation
- Removing free-tier on-chain public session writes
- Anonymous paid entitlement model

## References

- `ANONYMOUS_ACCESS_PROTOCOL.md`
- `ANONYMOUS_ACCESS_BACKLOG.md`
- `ANONYMOUS_ACCESS_ISSUES.md`
- `RELEASE_GATES.md`
```
