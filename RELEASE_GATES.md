# Release Gates

## Production Scale Gates

- [ ] Node discovery and monitoring no longer depend on unbounded full-list reads from `NodeRegistry` (`getActiveNodes`, `getActiveNodesByRegion`, `getOverdueNodes`, and payout-service `getNodeList` path).
- [ ] Gateway and payout-service use paginated/indexed reads (or an event-driven indexer) so discovery remains reliable as node count grows.
- [ ] A migration plan exists for current contracts: either deploy a paginated `NodeRegistry` version or run an indexer-backed discovery API and switch consumers.
