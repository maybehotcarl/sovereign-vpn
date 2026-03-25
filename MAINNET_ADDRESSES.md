# Sovereign VPN Mainnet Address Sheet

Snapshot date: March 19, 2026

This file is the canonical checked-in summary of the mainnet contract surface
that is currently relevant to Sovereign VPN.

Verification basis:

- direct mainnet RPC reads against `https://ethereum-rpc.publicnode.com`
- repo deployment scripts in `contracts/script/DeployMainnet.s.sol` and
  `contracts/script/CompleteMainnet.s.sol`

## Verified Live Contracts

| Component | Address | Status | Notes |
| --- | --- | --- | --- |
| Memes ERC-1155 | `0x33FD426905F149f8376e227d0C9D3340AaD17aF1` | Verified live | Mainnet Memes contract used by the direct issuer path |
| AccessPolicy | `0x5b651A5a92e21c5E91996Ab439D559Fd84E5b6d0` | Verified live | Original February 24, 2026 mainnet deployment; `checkAccess()` returns normally |
| NodeRegistry | `0x8545C0a44a738fA5bb70cB8f3f949F0Bcd298c65` | Verified live | Original February 24, 2026 mainnet deployment |
| NodeRegistry | `0x1Fd64c16c745e373428068eB52AA73525576B594` | Verified live | Fresh mainnet deployment on March 19, 2026 |
| SubscriptionManager | `0xEb54c8604b7EEADE804d121BD8f158A006827882` | Verified live | `hasActiveSubscription()` and `getSubscription()` return normally |
| SessionManager | `0xb644c990c884911670adc422719243D9F76Df0d6` | Verified live | `owner()`, `pendingOwner()`, `nextSessionId()`, and `payoutVault()` return normally |
| PayoutVault | `0xb2C1f2282d09c61F4043B2fcDe00e61361De199f` | Verified live | Discovered from `SubscriptionManager.payoutVault()` and `SessionManager.payoutVault()` |

## Live Wiring

The following was verified on mainnet on March 19, 2026:

- `AccessPolicy.owner()` is `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `AccessPolicy.pendingOwner()` is `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `AccessPolicy.checkAccess(0x000...dEaD)` returns `(false, false)`
- `NodeRegistry.owner()` at `0x8545...` is `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `NodeRegistry.pendingOwner()` at `0x8545...` is `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `NodeRegistry.minStake()` at `0x8545...` is `0.1 ETH`
- `NodeRegistry.heartbeatInterval()` at `0x8545...` is `3600`
- `NodeRegistry.memesContract()` at `0x8545...` returns the mainnet Memes contract
- `NodeRegistry.operatorCardId()` at `0x8545...` is `1`
- `NodeRegistry.owner()` at `0x1Fd64...` is `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `NodeRegistry.pendingOwner()` at `0x1Fd64...` is `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `NodeRegistry.minStake()` at `0x1Fd64...` is `0.1 ETH`
- `NodeRegistry.heartbeatInterval()` at `0x1Fd64...` is `3600`
- `NodeRegistry.memesContract()` at `0x1Fd64...` returns the mainnet Memes contract
- `NodeRegistry.operatorCardId()` at `0x1Fd64...` is `1`
- `SubscriptionManager.payoutVault()` returns `0xb2C1f2282d09c61F4043B2fcDe00e61361De199f`
- `SessionManager.payoutVault()` returns `0xb2C1f2282d09c61F4043B2fcDe00e61361De199f`
- `PayoutVault.authorizedSources(SessionManager)` is `true`
- `PayoutVault.authorizedSources(SubscriptionManager)` is `true`
- `PayoutVault.paused()` is `false`
- `PayoutVault.totalPending()` is `0`

Current limitation:

- the live `SessionManager.nodeRegistry()` call still reverts, so the fresh
  `NodeRegistry` is not yet confirmed as the registry enforced by the current
  mainnet `SessionManager`

## Verified Product State

These values were also verified on mainnet:

- `SubscriptionManager.tiers(1)` = `0.006 ETH`, `7 days`, active
- `SubscriptionManager.tiers(2)` = `0.02 ETH`, `30 days`, active
- `SubscriptionManager.tiers(3)` = `0.05 ETH`, `90 days`, active
- `SubscriptionManager.tiers(4)` = `0.15 ETH`, `365 days`, active
- `SessionManager.nextSessionId()` = `1`

## Control Addresses Observed On Chain

These are not user-facing service addresses, but they are useful context:

- `SessionManager.owner()` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `SubscriptionManager.owner()` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `AccessPolicy.owner()` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `AccessPolicy.pendingOwner()` = `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `NodeRegistry.owner()` at `0x8545...` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `NodeRegistry.pendingOwner()` at `0x8545...` = `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `NodeRegistry.owner()` at `0x1Fd64...` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `PayoutVault.owner()` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`
- `SessionManager.pendingOwner()` = `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `SubscriptionManager.pendingOwner()` = `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `NodeRegistry.pendingOwner()` at `0x1Fd64...` = `0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337`
- `PayoutVault.payoutExecutor()` = `0x002443462a23A5A8297e3657F0e7e96371c9B57B`

## Unresolved Or Unsafe

### SessionManager Registry Wiring

- Repo / historical `NodeRegistry` address:
  `0xC34cAfE3370224d4a4Ee7ada6BF58d2c99230CF2`
- Current state for that historical address: no code on mainnet
- Original verified mainnet `NodeRegistry` deployment:
  `0x8545C0a44a738fA5bb70cB8f3f949F0Bcd298c65`
- Fresh verified `NodeRegistry` deployment:
  `0x1Fd64c16c745e373428068eB52AA73525576B594`
- Remaining issue: `SessionManager.nodeRegistry()` currently reverts on the
  live manager address, so registry enforcement in the old mainnet
  `SessionManager` is still unresolved

Conclusion: mainnet has two trustworthy standalone `NodeRegistry`
deployments, but the legacy `SessionManager` integration remains unresolved.

## Stale Repo References

These addresses should not be treated as current mainnet source of truth:

| Component | Stale Address | Problem |
| --- | --- | --- |
| NodeRegistry | `0xC34cAfE3370224d4a4Ee7ada6BF58d2c99230CF2` | No code on mainnet |
| PayoutVault | `0x1F6BbB06952d53F0A87Fdb6F17e34d89206B32Da` | No code on mainnet |
| AccessPolicy | `0xF1AfCFD8eF6a869987D50e173e22F6fc99431712` | Historical non-current address; mainnet issuer should use `0x5b651A5a92e21c5E91996Ab439D559Fd84E5b6d0` if using AccessPolicy |

## Practical Guidance

- For mainnet anonymous issuer testing, use the direct Memes path.
- For subscription checks, use `SubscriptionManager` at
  `0xEb54c8604b7EEADE804d121BD8f158A006827882`.
- For session-related payment plumbing, use `SessionManager` at
  `0xb644c990c884911670adc422719243D9F76Df0d6`.
- For vault reconciliation, use `PayoutVault` at
  `0xb2C1f2282d09c61F4043B2fcDe00e61361De199f`.
- For AccessPolicy-based checks, use `AccessPolicy` at
  `0x5b651A5a92e21c5E91996Ab439D559Fd84E5b6d0`.
- For node registration, the original verified mainnet registry is
  `0x8545C0a44a738fA5bb70cB8f3f949F0Bcd298c65`.
- The fresh redeployed registry is
  `0x1Fd64c16c745e373428068eB52AA73525576B594`.
- For a fresh mainnet `NodeRegistry` deployment, use
  `contracts/script/DeployNodeRegistryMainnet.s.sol`.
- Do not rely on the historical stale `0xC34c...` `NodeRegistry` entry or the
  stale `0xF1Af...` `AccessPolicy` entry.
- Do not assume the old mainnet `SessionManager` is enforcing the new registry
  until that integration is verified or replaced.
