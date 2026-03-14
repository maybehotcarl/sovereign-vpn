# Release Gates

## Production Scale Gates

- [ ] Node discovery and monitoring no longer depend on unbounded full-list reads from `NodeRegistry` (`getActiveNodes`, `getActiveNodesByRegion`, `getOverdueNodes`, and payout-service `getNodeList` path).
- [ ] Gateway and payout-service use paginated/indexed reads (or an event-driven indexer) so discovery remains reliable as node count grows.
- [ ] A migration plan exists for current contracts: either deploy a paginated `NodeRegistry` version or run an indexer-backed discovery API and switch consumers.

## PayoutVault Accounting Gates

- [ ] `PayoutVault` payout path includes an explicit vault-balance guard (`amount <= address(this).balance`) with a dedicated insolvency error for clear operational diagnostics.
- [ ] `pendingPayouts` remains the source of truth for per-operator entitlement; payout logic does not switch to distributing raw `address(this).balance`.
- [ ] Emergency ETH withdrawal flow has a documented and tested reconciliation path for stale `pendingPayouts` (batch clear, migration, or equivalent).
