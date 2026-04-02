# Release Gates

For the execution-order checklist tied to the current anonymous paid beta path,
see `BETA_LAUNCH_CHECKLIST.md`. For the current public launch posture, see
`PUBLIC_BETA_MODE.md`.

## Production Scale Gates

- [ ] Node discovery and monitoring no longer depend on unbounded full-list reads from `NodeRegistry` (`getActiveNodes`, `getActiveNodesByRegion`, `getOverdueNodes`, and payout-service `getNodeList` path).
- [ ] Gateway and payout-service use paginated/indexed reads (or an event-driven indexer) so discovery remains reliable as node count grows.
- [ ] A migration plan exists for current contracts: either deploy a paginated `NodeRegistry` version or run an indexer-backed discovery API and switch consumers.

## PayoutVault Accounting Gates

- [ ] `PayoutVault` payout path includes an explicit vault-balance guard (`amount <= address(this).balance`) with a dedicated insolvency error for clear operational diagnostics.
- [ ] `pendingPayouts` remains the source of truth for per-operator entitlement; payout logic does not switch to distributing raw `address(this).balance`.
- [ ] Emergency ETH withdrawal flow has a documented and tested reconciliation path for stale `pendingPayouts` (batch clear, migration, or equivalent).

## Anonymous Access Privacy Gates

- [ ] The private user connect path no longer starts with SIWE or any other gateway-visible wallet authentication step.
- [ ] Gateway anonymous sessions are keyed only by anonymous session IDs / nullifiers; user wallet addresses are not stored, logged, or used as the session identity on the private path.
- [ ] A policy indexer, credential issuer, and active root publication flow exist for anonymous access; revocation is bounded by a documented epoch/grace window.
- [ ] Site app and CLI both generate anonymous proofs and fresh WireGuard keys by default on the private path.
- [ ] Free-tier user access no longer writes `openFreeSession(user, ...)` or any equivalent public user-linked session record on-chain.
- [ ] Paid-tier user access no longer depends on public-wallet `SessionManager` / subscription identity on the connect path.
- [ ] User bans and revocations are enforced through issuance / root rotation / revocation, not by live wallet-address checks at the gateway.
- [ ] If product claims include privacy from the issuer as well as the gateway, blind issuance or an equivalent issuer-private mechanism is implemented.
