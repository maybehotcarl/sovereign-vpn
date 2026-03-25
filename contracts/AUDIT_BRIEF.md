# Sovereign VPN — Smart Contract Audit Brief

## Overview

Sovereign VPN is an NFT-gated, decentralized VPN service built on Ethereum. Users holding Memes by 6529 ERC-1155 tokens gain VPN access. Node operators stake ETH and must hold a designated operator card. All session/subscription revenue flows to a community treasury, and governance distributes rewards to operators.

## Contracts in Scope

| Contract | Lines | Dependencies |
|----------|-------|--------------|
| AccessPolicy.sol | 201 | OZ Ownable2Step |
| NodeRegistry.sol | 407 | OZ Ownable2Step, ReentrancyGuard |
| SessionManager.sol | 339 | OZ Ownable2Step, ReentrancyGuard |
| SubscriptionManager.sol | 339 | OZ Ownable2Step, ReentrancyGuard |
| PayoutVault.sol | 242 | OZ Ownable2Step, ReentrancyGuard, SafeERC20 |
| **Total** | **1,528** | |

TestMemes.sol (34 lines) is a test-only ERC-1155 mock and is NOT in scope.

Solidity version: `^0.8.24`
OpenZeppelin version: `v5.x` (Ownable2Step, ReentrancyGuard, SafeERC20)
Build system: Foundry (forge)

## Architecture

```
                    ┌──────────────┐
                    │ AccessPolicy │  Reads Memes ERC-1155
                    │  (view-only) │  balanceOf to determine
                    └──────────────┘  Free / Paid / Denied tier
                           │
          ┌────────────────┼────────────────┐
          ▼                ▼                ▼
   ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
   │ NodeRegistry │ │SessionManager│ │Subscription  │
   │              │ │              │ │   Manager    │
   │ Card-gated   │ │ Per-session  │ │ Tiered subs  │
   │ staking +    │ │ payments     │ │ (7/30/90/365 │
   │ heartbeat    │ │              │ │  days)       │
   └──────────────┘ └──────┬───────┘ └──────┬───────┘
                           │                │
                           ▼                ▼
                    ┌──────────────────────────┐
                    │       PayoutVault        │
                    │                          │
                    │  Aggregates operator     │
                    │  earnings for RAILGUN    │
                    │  private payouts         │
                    └──────────────────────────┘
```

## Key Invariants

1. **Card gating**: Only holders of `operatorCardId` on the Memes ERC-1155 can call `NodeRegistry.register()` and `reactivate()`.
2. **Revenue routing (operatorShareBps = 0)**: When operator share is 0%, all payment goes to `treasuryBalance`. The vault should never receive 0-value calls. (Bug found and fixed during integration testing — see "Known Issues" below.)
3. **PayoutVault crediting**: Only `authorizedSources` (SessionManager, SubscriptionManager) can call `creditOperator()`.
4. **Payout execution**: Only `payoutExecutor` can call `processPayout()` / `processBatchPayout()`.
5. **Ownership**: All admin functions use `Ownable2Step` (two-step transfer). Intent is to transfer to a Gnosis Safe multisig post-deployment.
6. **Staking**: Operators must stake >= `minStake` ETH. Slashing deducts from stake and accumulates in `slashedFunds`. Slashed operators cannot reactivate.
7. **Session lifecycle**: One active session per user. Session payment is locked on open and distributed on close.
8. **Subscription lifecycle**: One active subscription per user. Payment is distributed immediately on subscribe/renew.

## ETH Flow

```
User pays for session/subscription
        │
        ▼
operatorPayout = payment * operatorShareBps / 10000
treasuryPayout = payment - operatorPayout

if operatorPayout > 0:
    if payoutVault configured:
        → PayoutVault.creditOperator{value: operatorPayout}(node)
    else:
        → operatorBalance[node] += operatorPayout  (legacy mode)

treasuryBalance += treasuryPayout

Owner calls distributeRewards(operators, amounts):
    → treasuryBalance -= sum(amounts)
    → operatorBalance[each] += amounts[each]

Operators call withdrawOperatorEarnings():
    → ETH sent to msg.sender
```

## Known Issues (Found & Fixed)

### PayoutVault ZeroAmount Revert (Fixed)

**Severity**: High (broke session close entirely when operatorShareBps=0 and PayoutVault was configured)

When `operatorShareBps = 0`, `closeSession()` and `_distributePayment()` computed `operatorPayout = 0` but still called `payoutVault.creditOperator{value: 0}()`, which reverted with `ZeroAmount()`. Fixed by guarding with `if (operatorPayout > 0)` before the vault call.

**Affected files**: SessionManager.sol (line 210), SubscriptionManager.sol (line 330)

## Areas of Concern for Auditors

1. **Reentrancy in withdrawals**: `withdrawOperatorEarnings()` and `withdrawTreasury()` use the checks-effects-interactions pattern with `nonReentrant`, but verify no edge cases exist with the `payoutVault.creditOperator()` external call during `closeSession()`.

2. **`distributeRewards()` trust model**: Owner can distribute any amount up to `treasuryBalance` to any addresses. No timelock, vesting, or on-chain governance. This is by design (off-chain TDH-weighted governance), but should be documented as a centralization risk.

3. **`_removeFromList()` in NodeRegistry**: Swap-and-pop pattern for unbounded arrays. Verify no index corruption if called in unexpected states.

4. **`operatorCardId` mutability**: Recently changed from `immutable` to mutable with `setOperatorCardId()` onlyOwner setter. Allows governance to change which card is required without redeployment. Verify this doesn't create a window where existing operators become ineligible unexpectedly (note: card check only on `register()` and `reactivate()`, not on `heartbeat()` or other operations).

5. **PayoutVault `emergencyWithdrawETH()`**: Allows owner to sweep all ETH including pending operator payouts. This is an intentional safety valve but represents a rug risk if ownership is compromised.

6. **Price calculation rounding**: `(pricePerHour * duration) / 3600` in SessionManager rounds down, potentially allowing slightly underpaid sessions for non-hour-aligned durations.

7. **`slash()` percentage math**: `(stakedAmount * slashPercent) / 100` rounds down. A 1-wei stake with 50% slash would slash 0 wei.

8. **No `receive()` or `fallback()` on SessionManager/SubscriptionManager**: These contracts hold ETH but can only receive it through `payable` functions. Verify no scenario where ETH can be locked.

## Test Coverage

- **180 Foundry tests** covering all contracts
- All tests pass with `forge test -vvv`
- Integration tested on Sepolia (deployed and exercised full lifecycle)

## Build & Test

```bash
cd contracts
forge install
forge build
forge test -vvv
```

## Historical Address Snapshot

This block is historical context only. It is not the current source of truth
for mainnet addresses.

For the currently verified mainnet contract surface, use
`/home/maybe/repos/sovereign-vpn/MAINNET_ADDRESSES.md`.

In particular:

- do not treat the `NodeRegistry` address below as a verified current mainnet deployment
- do not treat the `PayoutVault` address below as a verified current mainnet deployment
- do not treat the `AccessPolicy` address below as safe for current mainnet issuer checks

## Deployed Addresses (Historical / Sepolia-era reference)

```
TestMemes:           0x98C361b7C385b9589E60B36B880501D66123B294
AccessPolicy:        0xF1AfCFD8eF6a869987D50e173e22F6fc99431712
NodeRegistry:        0xC34cAfE3370224d4a4Ee7ada6BF58d2c99230CF2
SessionManager:      0x071725366D3aB388be6Fa3b442d89e62B09C8597
SubscriptionManager: 0x99ad395f0318ddb155d42a973b6fE7E054CD3B92
PayoutVault:         0x1F6BbB06952d53F0A87Fdb6F17e34d89206B32Da
```

## Contact

Repository: `github.com/maybehotcarl/sovereign-vpn` (contracts/ directory)
