# Sovereign VPN: Technical Specification

### An NFT-Gated, Reputation-Based, Community-Governed Decentralized VPN Network

**Version:** 0.2 (Draft)
**Date:** 2026-02-18
**Status:** Pre-Development

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Design Principles](#2-design-principles)
3. [System Architecture Overview](#3-system-architecture-overview)
4. [Layer 1: Network Transport (Sentinel dVPN Fork)](#4-layer-1-network-transport)
5. [Layer 2: NFT-Gated Access Control](#5-layer-2-nft-gated-access-control)
6. [Layer 3: On-Chain Reputation System](#6-layer-3-on-chain-reputation-system)
7. [Layer 4: Community Governance](#7-layer-4-dao-governance)
8. [Smart Contract Architecture](#8-smart-contract-architecture)
9. [Client Architecture](#9-client-architecture)
10. [Node Operator Architecture](#10-node-operator-architecture)
11. [Security Model](#11-security-model)
12. [Phased Rollout Plan](#12-phased-rollout-plan)
13. [Open Questions](#13-open-questions)

---

## 1. Executive Summary

Sovereign VPN is a decentralized VPN network built for and governed by the 6529 community:

- **Access is gated by Memes card ownership.** Hold any card from The Memes by 6529 collection, and you can access the network. No accounts, no emails, no KYC.
- **This project's own Meme card grants free VPN access.** The card representing this whitepaper/idea is your free pass. All other Memes holders can access the network for a fee.
- **Node operator quality is enforced by an on-chain reputation system** -- operators stake ETH, build reputation through uptime and quality, and face slashing for misbehavior.
- **All governance is TDH-weighted, using existing 6529 network tooling.** No new governance token. No new voting system. The community already has the infrastructure -- we use it.

The network builds on the **Sentinel dVPN open-source protocol** for the underlying VPN transport layer (WireGuard tunnels, peer discovery, bandwidth tracking), and adds three new layers on top: Memes-gated access control, reputation scoring, and TDH-weighted governance.

### Why This Doesn't Exist Yet

Existing dVPNs (Sentinel, Mysterium, Orchid) solve the networking layer but have weak identity/access models (anyone can connect), no meaningful reputation systems (trust is assumed), and centralized or foundation-led governance. The 6529 ecosystem's thesis -- that open NFTs can replace accounts and that communities can self-govern infrastructure -- has not yet been applied to privacy infrastructure.

### The Meme Card

This specification is being submitted as a Meme card. The art IS this whitepaper. Holders of this card get free access to the VPN network. This aligns incentives perfectly: the people who back this idea by holding the card are the ones who use the network for free.

---

## 2. Design Principles

1. **No accounts, only wallets.** Identity is a wallet address. Access is a Memes card. Nothing else.
2. **No new tokens, no new governance.** TDH is the governance weight. ETH is the currency at launch. The 6529 network is the governance platform. We don't reinvent what already works. When the 6529 network token ($6529) launches, it is the natural second currency for this network -- payment interfaces are designed to support it via a governance vote, not a contract rewrite.
3. **No company, no foundation.** The community owns everything. There is no legal entity that can be compelled to shut it down.
4. **Node operators are untrusted by default.** Reputation is earned through measurable behavior, not self-assertion.
5. **Zero on-chain transactions for normal VPN usage.** Authentication uses wallet signatures (free). NFT verification uses RPC reads (free). The only on-chain costs are for node operators (staking, session settlement).
6. **Cold storage compatible.** Users can keep their Memes in cold wallets and use the 6529 delegation contract or delegate.xyz to authorize a hot wallet for VPN access.
7. **Forkable and composable.** Every component is open source. Any community can fork the contracts, deploy their own NFT gate, and run their own network.

---

## 3. System Architecture Overview

```
+------------------------------------------------------------------+
|                        USER DEVICE                                |
|                                                                   |
|  +------------------+    +-------------------+                    |
|  | Wallet           |    | VPN Client        |                    |
|  | (MetaMask, etc.) |    | (WireGuard-based) |                    |
|  +--------+---------+    +--------+----------+                    |
|           |                       |                               |
+-----------|------SIWE Auth--------|------WireGuard Tunnel---------+
            |                       |
            v                       v
+------------------------------------------------------------------+
|                     ACCESS GATEWAY                                |
|                                                                   |
|  1. Verify SIWE signature (recover wallet address)                |
|  2. Check Memes card ownership (RPC read to Ethereum)             |
|  3. Check 6529 delegation / delegate.xyz (if using delegation)    |
|  4. Check if holder has THIS card -> free tier vs paid tier        |
|  5. Issue time-bounded WireGuard peer credentials                 |
|                                                                   |
+------------------------------------------------------------------+
            |                       |
            v                       v
+------------------------------------------------------------------+
|                     SENTINEL dVPN LAYER                           |
|                                                                   |
|  - Node discovery (on-chain registry)                             |
|  - WireGuard / V2Ray tunnel management                            |
|  - Bandwidth tracking & proof of bandwidth                        |
|  - Session lifecycle management                                   |
|  - Payment settlement (deposit/escrow model)                      |
|                                                                   |
+------------------------------------------------------------------+
            |
            v
+------------------------------------------------------------------+
|                     ETHEREUM SMART CONTRACTS                      |
|                                                                   |
|  +----------------+  +------------------+  +------------------+   |
|  | Access Policy  |  | Reputation       |  | Parameter        |   |
|  | (reads Memes   |  | Registry         |  | Registry         |   |
|  |  contract)     |  |                  |  |                  |   |
|  | - Card check   |  | - Score calc     |  | - Governed by    |   |
|  | - Free vs paid |  | - Staking/slash  |  |   TDH votes on   |   |
|  | - Delegation   |  | - Node registry  |  |   6529 network   |   |
|  +----------------+  +------------------+  +------------------+   |
|                                                                   |
+------------------------------------------------------------------+
```

---

## 4. Layer 1: Network Transport

### 4.1 Building on Sentinel

Rather than building VPN infrastructure from scratch, this project builds on top of Sentinel's open-source dVPN node software and protocol. The key components we use:

| Component | What It Does | Source |
|-----------|-------------|--------|
| `dvpn-node` | Node software: WireGuard/V2Ray tunnel management, peer handling, bandwidth tracking | [sentinel-official/dvpn-node](https://github.com/sentinel-official/dvpn-node) |
| `sentinel-go-sdk` | VPN service abstractions (AddPeer, RemovePeer, PeerStatistics) | [sentinel-official/sentinel-go-sdk](https://github.com/sentinel-official/sentinel-go-sdk) |
| Sentinel Hub | Cosmos SDK blockchain for node registry, session management, deposits | [sentinel-official/hub](https://github.com/sentinel-official/hub) |

### 4.2 What We Modify

**The handshake endpoint is the integration point.** Sentinel's node software exposes a `POST /` API endpoint for client connections. Currently it:

1. Receives session ID + client WireGuard public key + signature
2. Queries the Sentinel blockchain to verify the session
3. Calls `Service.AddPeer()` to configure the WireGuard tunnel
4. Returns the VPN connection configuration

**Our modification:** We insert a Memes card verification step between steps 1 and 2. Before checking the Sentinel blockchain for a valid session, the node checks:

- Does the client's wallet hold any Memes card? (Ethereum RPC read to the Memes ERC-1155 contract)
- Does the wallet hold THIS card (the whitepaper card)? (Determines free vs. paid tier)
- Is the client using a delegated wallet? (Check 6529 delegation contract, then delegate.xyz)

This is implemented as **middleware wrapping the existing handshake handler**, not a fork of the handler itself. This keeps us compatible with upstream Sentinel updates.

### 4.3 Supported VPN Protocols

Inherited from Sentinel's pluggable `ServerService` interface:

| Protocol | Transport | Status |
|----------|-----------|--------|
| WireGuard | Kernel-level, fastest | Primary (recommended) |
| V2Ray (VMess/VLESS) | TCP, WebSocket, QUIC, gRPC | Secondary (for censored networks) |
| OpenVPN | TCP/UDP | Legacy support |

### 4.4 Dual-Chain Architecture

The system operates across two chains:

- **Ethereum (L1 or L2):** NFT membership contracts, reputation registry, Community governance, staking/slashing. This is where economic security lives.
- **Sentinel Hub (Cosmos):** Node registry, session management, bandwidth proofs, payment settlement. This is where network operations live.

A **bridge oracle** synchronizes critical state between the two chains (e.g., "is this node registered and in good standing on Ethereum?" verified on Sentinel Hub before accepting sessions).

**Alternative (simpler):** Deploy everything on an Ethereum L2 (Base, Arbitrum, or Optimism) and run a custom node registry + session management on the same chain, eliminating the need for Sentinel Hub entirely. This trades the maturity of Sentinel's protocol for architectural simplicity. This is a key design decision (see [Open Questions](#13-open-questions)).

---

## 5. Layer 2: Memes-Gated Access Control

### 5.1 No New NFT -- Use The Memes

There is no new membership NFT. The existing Memes by 6529 collection IS the access credential.

**Contract:** `0x33fd426905f149f8376e227d0c9d3340aad17af1` (ERC-1155)

**Access tiers:**

| Condition | Tier | Cost | Bandwidth | Devices |
|-----------|------|------|-----------|---------|
| Holds THIS Meme card (the whitepaper card) | Free | None | Unlimited | 3 |
| Holds any other Meme card | Paid | Per-GB or subscription (ETH) | Based on plan | 1-3 |
| Holds no Meme card | Denied | N/A | N/A | N/A |

**Why this works:**
- Thousands of Memes holders already exist -- massive user base from day one.
- No cold-start problem. No new contract to deploy for access.
- Holding THIS card = backing this idea. Free VPN is the reward.
- The Memes collection is open and accessible by design (large edition sizes, affordable). This aligns with the mission.

**Bandwidth and device limits** for paid users are configurable via TDH-weighted governance on the 6529 network.

### 5.2 Authentication Flow

```
                                         Ethereum Node
                                        (Infura/Alchemy)
                                              |
                                         RPC reads (free)
                                              |
 +--------+     1. SIWE Challenge    +---------+---------+
 | Client | <----------------------- | Access Gateway    |
 | Wallet |                          |                   |
 |        | ---- 2. Signed Msg ----> | 3. Verify sig     |
 |        |                          |    (ecrecover)    |
 |        |                          |                   |
 |        |                          | 4. Check Memes:   |
 |        |                          |    balanceOf(addr, |
 |        |                          |    tokenId) for   |
 |        |                          |    all token IDs  |
 |        |                          |                   |
 |        |                          | 5. Check 6529     |
 |        |                          |    delegation or  |
 |        |                          |    delegate.xyz   |
 |        |                          |                   |
 |        |                          | 6. THIS card?     |
 |        |                          |    -> free tier   |
 |        |                          |    Other card?    |
 |        |                          |    -> paid tier   |
 |        |                          |                   |
 |        | <-- 7. WG Config ------  | 8. Issue time-    |
 |        |    (24h validity)        |    bounded creds  |
 +--------+                          +-------------------+
```

**Step-by-step:**

1. **Client requests access.** The gateway returns a SIWE challenge (EIP-4361 message) containing: domain, nonce (random, >= 8 chars), URI, chain ID, issued-at timestamp.

2. **Client signs the SIWE message** using their wallet (MetaMask, WalletConnect, etc.). This uses ERC-191 (`personal_sign`). The user sees a human-readable message: "sovereignvpn.network wants you to sign in with your Ethereum account: 0x..."

3. **Gateway verifies the signature** using `ecrecover` to recover the wallet address. Checks: nonce matches (prevents replay), domain matches (prevents phishing), timestamp is recent.

4. **Gateway checks Memes ownership** via read-only RPC calls to the Memes contract (`0x33fd...`). It calls `balanceOf(recoveredAddress, tokenId)` across relevant token IDs. The gateway needs to check: does this wallet hold ANY Memes card? And specifically, does it hold THIS card (the whitepaper card)?

5. **If no direct ownership, check delegation.** First, check the 6529 Collections delegation contract ([6529-Collections/nftdelegation](https://github.com/6529-Collections/nftdelegation)) for consolidations. Then fall back to delegate.xyz. This supports cold storage users who keep their Memes in a vault wallet.

6. **Determine tier.** If the wallet (directly or via delegation) holds THIS card: free tier. If it holds any other Memes card: paid tier. If it holds nothing: access denied.

7. **Gateway issues a time-bounded WireGuard configuration** containing: server public key, endpoint address, allowed IPs, client IP assignment, and a credential expiry timestamp.

8. **Credential expiry and renewal.** Credentials are valid for 24 hours (configurable via governance). On expiry, the client automatically re-authenticates (re-signs SIWE + re-checks Memes ownership). If the card has been transferred, access is denied.

### 5.3 Real-Time Revocation

To prevent continued access after a Memes card is transferred:

- **Primary:** Event monitoring. The gateway subscribes to `TransferSingle` and `TransferBatch` events on the Memes contract via WebSocket. When a transfer is detected, any active VPN sessions for the sending address are re-evaluated (if they no longer hold any qualifying card, their WireGuard peer is removed).

- **Fallback:** Credential TTL. Even if event monitoring fails, the 24-hour credential expiry ensures stale access expires naturally.

- **Node-level enforcement:** Each VPN node independently verifies Memes ownership at session establishment. Even if the gateway is compromised, nodes will not serve unauthenticated clients.

### 5.4 Future Access Expansion

The `AccessPolicy` contract can be extended via TDH governance to accept additional 6529 ecosystem NFTs (e.g., NextGen collections, Gradient, or future collections). The community decides what grants access and at what tier.

---

## 6. Layer 3: On-Chain Reputation System

### 6.1 Design Goals

The reputation system solves the core trust problem in dVPNs: **your traffic exits through someone else's node, and you have no way to know if they're logging it.** While no system can cryptographically guarantee a node isn't logging (this is a fundamental limitation), reputation creates economic incentives to behave honestly and allows the network to isolate bad actors.

### 6.2 Reputation Score Components

The reputation score is a composite value in the range [0, 1000], computed from four weighted categories:

```
ReputationScore = (w1 * UptimeScore) + (w2 * QualityScore) + (w3 * StakeScore) + (w4 * HistoryScore)

Default weights (Community-governable):
  w1 = 300 (Uptime/Availability)
  w2 = 300 (Service Quality)
  w3 = 200 (Stake & Skin-in-the-Game)
  w4 = 200 (On-Chain History)
```

#### Category 1: Uptime Score (0-300)

**Measurement:** The network runs periodic liveness probes from a decentralized set of monitor nodes. Each epoch (1 hour), every active node is probed. The uptime score uses an exponential moving average:

```
UptimeScore_t = alpha * was_online_this_epoch + (1 - alpha) * UptimeScore_{t-1}
alpha = 0.05 (recent performance weighted, but history matters)
```

Normalized to [0, 300].

**Why EMA:** A node that had 99.9% uptime for 6 months but went down for 1 hour shouldn't be punished severely. Conversely, a new node with 100% uptime for 1 day shouldn't be rated equal to the 6-month veteran.

#### Category 2: Quality Score (0-300)

**Measurement:** After each VPN session, the client software automatically reports (off-chain, signed):
- Average throughput (Mbps)
- Latency to first byte (ms)
- Connection stability (% of session without drops)
- Whether the session completed normally or was interrupted

Quality measurements are aggregated per-node per-epoch. An oracle committee (see 6.5) attests the aggregated scores on-chain.

```
QualityScore = min(300, (ThroughputUtil * 100) + (LatencyUtil * 100) + (StabilityUtil * 100))

ThroughputUtil  = min(1, measuredMbps / targetMbps)
LatencyUtil     = max(0, 1 - (measuredLatency / maxAcceptableLatency))
StabilityUtil   = sessionCompletionRate
```

#### Category 3: Stake Score (0-200)

**Measurement:** Direct function of the node operator's staked amount relative to the minimum stake requirement.

```
StakeScore = min(200, 200 * (stakedAmount / (3 * minimumStake)))
```

Staking 3x the minimum gets the full 200 points. This incentivizes operators to over-collateralize as a trust signal, following the Cardano pledge model where larger self-stake indicates higher commitment.

#### Category 4: History Score (0-200)

**Measurement:** Derived from on-chain records:

```
HistoryScore = AgeScore + VolumeScore + CleanRecord

AgeScore     = min(80, 80 * (monthsActive / 12))     // Max at 1 year
VolumeScore  = min(80, 80 * (totalGBServed / 10000))  // Max at 10TB
CleanRecord  = 40 if never slashed, 0 if slashed in last 6 months,
               20 if slashed but >6 months ago
```

### 6.3 Staking Mechanism

**Minimum Stake:** Governed by TDH vote (initial suggestion: 0.5 ETH).

**Staking Contract:**
- Operators lock ETH in the `NodeStaking` contract.
- Staked ETH is subject to a **21-day unbonding period** (prevents slash evasion).
- Slashing reduces the staked amount directly and irreversibly.
- Staked ETH earns a share of network fees as rewards (proportional to stake * reputation).

**Slashing Conditions:**

| Offense | Detection | Penalty |
|---------|-----------|---------|
| Extended downtime (>24h without notice) | Automated (missed liveness probes) | 1% of stake |
| Consistently poor quality (QualityScore < 100 for 7 days) | Automated (aggregated reports) | 2% of stake |
| Provable traffic logging/manipulation | Dispute + evidence submission | 25% of stake |
| Confirmed malicious behavior (MITM, DNS hijacking) | Dispute + evidence submission | 100% of stake (full slash) |

**Dispute Process for Manual Slashing:**

1. Anyone can submit a dispute with a bond (e.g., 0.01 ETH).
2. The dispute includes evidence (packet captures, DNS logs, cryptographic proofs).
3. A dispute resolution committee (elected by the Community) reviews the evidence.
4. If valid: operator is slashed, disputer's bond is returned + rewarded from the slashed amount.
5. If invalid: disputer's bond is forfeited.

### 6.4 Sybil Resistance

To prevent one entity from running many low-quality nodes and gaming the system:

1. **Minimum stake per node:** Each node requires its own independent stake deposit. Running 10 nodes costs 10x the minimum stake.

2. **Diminishing returns:** A single operator running multiple nodes sees diminishing reputation benefits. The second node starts with a lower base HistoryScore than the first.

3. **Geographic diversity bonus:** Nodes in underserved regions receive a reputation boost. Running 5 nodes in the same datacenter provides minimal additional benefit.

4. **IP fingerprinting:** Nodes sharing the same ASN (autonomous system number) or /24 IP range are flagged and grouped. Their combined reputation is capped.

### 6.5 Oracle Committee

Quality measurements happen off-chain (client reports) but reputation scores are stored on-chain. An **oracle committee** bridges this gap:

- A set of 7-15 members elected by the Community.
- Each epoch, the committee aggregates off-chain quality reports.
- A 2/3 majority must sign the aggregated scores before they are posted on-chain.
- Committee members are rotated periodically and can be replaced via governance.
- Committee members are bonded (staked) and can be slashed for provably false attestations.

**Long-term goal:** Replace the oracle committee with a fully decentralized measurement protocol using commit-reveal schemes and cross-node verification. The oracle committee is a pragmatic starting point.

---

## 7. Layer 4: TDH-Weighted Governance via 6529 Network

### 7.1 No New Governance System

This project does not create a new governance token, a new voting contract, or a new governance platform. **All governance decisions are made on the existing 6529 network (seize.io), weighted by TDH (Total Days Held).**

TDH is the 6529 network's core metric. It measures how long you've held your Memes cards. The longer you hold, the more TDH you accumulate, the more governance weight you carry. This is already trusted across the 6529 community for meme card curation votes and network decisions.

### 7.2 Why TDH Is the Right Governance Weight

| Property | Benefit for VPN Governance |
|----------|---------------------------|
| **Commitment-based, not capital-based** | You can't buy TDH -- you earn it by holding over time. This prevents governance capture by wealthy newcomers. |
| **Already battle-tested** | TDH voting is used for Meme card selection. The community trusts the mechanics. |
| **Sybil-resistant** | Splitting cards across wallets doesn't increase total TDH -- consolidation exists to combine TDH across wallets a user owns. |
| **Anti-flash-loan** | NFTs can't be flash-loaned, and TDH requires holding duration, making it immune to flash governance attacks. |
| **Aligned incentives** | Long-term Memes holders care about the ecosystem's health. They're the right people to govern infrastructure. |

### 7.3 Governance Flow

```
1. Proposal is created on the 6529 network (seize.io)
   - Anyone with sufficient TDH can propose

2. Community discusses and votes, weighted by TDH
   - Time-weighted voting: votes cast earlier carry more weight
     (prevents last-minute manipulation, per existing 6529 mechanics)

3. If passed, a designated executor multisig enacts the change on-chain
   - The multisig is bound to follow TDH vote outcomes
   - Execution goes through a TimelockController for safety

4. Parameter change takes effect after the timelock delay
```

### 7.4 Governed Parameters

All network parameters are governable via TDH votes. Categories:

**Network Economics:**
- Fee rates for paid users (per-GB and subscription pricing)
- Fee distribution split (operators / treasury / stakers)
- Minimum node operator stake (ETH)
- Credential TTL duration

**Reputation & Quality:**
- Reputation score weights (uptime, quality, stake, history)
- Slashing percentages for each offense category
- Oracle committee membership
- Minimum reputation threshold for node activation

**Access Policy:**
- Which additional NFT collections (beyond Memes) grant access
- Bandwidth allocations per tier
- Max concurrent device limits

**Emergency:**
- Contract pause
- Emergency node blacklisting
- Circuit breaker activation

### 7.5 Parameter Bounds (On-Chain Safety)

While governance decisions happen via TDH on the 6529 network, the smart contracts that hold network parameters enforce hard bounds that no governance vote can exceed:

```solidity
uint256 public constant MIN_STAKE_LOWER_BOUND = 0.1 ether;   // Cannot set min stake below 0.1 ETH
uint256 public constant MIN_STAKE_UPPER_BOUND = 100 ether;    // Cannot set min stake above 100 ETH
uint256 public constant MAX_SLASH_PERCENT = 100;               // Cannot slash more than 100%
uint256 public constant MIN_TIMELOCK_DELAY = 1 days;           // Cannot reduce timelock below 1 day
uint256 public constant MAX_CREDENTIAL_TTL = 7 days;           // Cannot set credential TTL above 7 days
uint256 public constant MIN_CREDENTIAL_TTL = 1 hours;          // Cannot set credential TTL below 1 hour
```

This prevents destructive parameter changes even if a TDH vote is somehow manipulated.

### 7.6 Executor Multisig

A multisig (initially 4/7, community-elected via TDH vote) bridges TDH governance decisions to on-chain execution:

- The multisig signers are public, known community members.
- They are **obligated to execute** the outcome of TDH votes. They do not exercise independent judgment.
- They can refuse to execute only if a vote outcome would trigger a parameter bound violation (i.e., the smart contract would reject it anyway).
- Multisig membership is itself governed by TDH votes.
- The multisig holds the `EXECUTOR_ROLE` on the `TimelockController` that owns the `ParameterRegistry` and other governed contracts.

**Long-term goal:** Replace the executor multisig with a trustless bridge that reads TDH vote outcomes programmatically and submits them on-chain. This requires API integration with the 6529 network's governance data.

### 7.7 Treasury

The treasury receives:
- A percentage of network fees from paid users (configurable, default 10%)
- Slashed funds from misbehaving node operators

The treasury is held in a Gnosis Safe controlled by the executor multisig. Spending requires a TDH governance vote.

Treasury funds are used for:
- Node operator incentives (bootstrap rewards for early operators)
- Security audits
- Development grants
- Subsidizing free-tier bandwidth for THIS card holders

### 7.8 Progressive Decentralization

| Phase | Governance Model |
|-------|-----------------|
| Phase 0 (Launch) | Core team multisig controls all parameters. TDH signaling votes for direction. |
| Phase 1 (3-6 months) | Executor multisig follows binding TDH votes for major parameters. Core team retains emergency powers. |
| Phase 2 (6-12 months) | All parameters governed by TDH votes via executor multisig + timelock. Emergency powers move to community-elected Security Council. |
| Phase 3 (12+ months) | Trustless bridge from 6529 network governance to on-chain execution. Security Council limited to pause-only emergency powers. |

---

## 8. Smart Contract Architecture

### 8.1 Contract Overview

```
contracts/
  access/
    AccessPolicy.sol              # Reads Memes contract, determines free vs paid tier
  reputation/
    NodeRegistry.sol              # Node registration with ETH stake deposits
    ReputationOracle.sol          # Oracle committee score attestation
    SlashingManager.sol           # Slashing logic and dispute resolution
  governance/
    ParameterRegistry.sol         # Stores all governed parameters with bounds
    TimelockController.sol        # OZ Timelock, owned by executor multisig
  gateway/
    SessionManager.sol            # Session credential issuance records
    PaymentSplitter.sol           # Fee distribution (operators / treasury / stakers); token-agnostic (ETH + ERC-20)
```

Note: No membership NFT contract (uses existing Memes contract). No Governor contract (governance via TDH on 6529 network). This is a significantly smaller contract surface than the v0.1 spec.

### 8.2 Key Contract: AccessPolicy.sol

```
State:
  address public constant MEMES_CONTRACT = 0x33fd426905f149f8376e227d0c9d3340aad17af1
  uint256 public thisCardTokenId                        // The whitepaper card token ID
  mapping(address => mapping(uint256 => bool)) public additionalCollections  // Future expansions

Functions:
  hasAccess(address user) -> bool                       // Holds any Memes card?
  hasFreeTier(address user) -> bool                     // Holds THIS card?
  checkWithDelegation(address user) -> (bool access, bool free)  // Checks 6529 delegation + delegate.xyz
  setThisCardTokenId(uint256 tokenId)                   // Set by governance (one-time after card mint)
  addCollection(address collection, uint256 tokenId)    // Governed: add new qualifying collections
```

### 8.3 Key Contract: NodeRegistry.sol

```
State:
  mapping(address => Node) public nodes
    Node: { operator, stakeAmount, registeredAt, reputationScore, status, endpoint }
  uint256 public minimumStake                          // Community-governed

Functions:
  registerNode(string endpoint) payable                 // Stake ETH + register
  deregisterNode()                                      // Begin 21-day unbonding
  withdrawStake()                                       // After unbonding completes
  updateReputationScore(address node, uint256 score)    // Oracle committee only
  slash(address node, uint256 percent, string reason)   // SlashingManager only
  getActiveNodes() -> Node[]                            // For client node discovery
  getNodesByReputation(uint256 minScore) -> Node[]      // Filtered discovery
```

### 8.4 Key Contract: ParameterRegistry.sol

```
Owner: TimelockController (executor multisig -> timelock -> registry)

State:
  mapping(bytes32 => uint256) public params
  mapping(bytes32 => ParamBounds) public bounds
    ParamBounds: { minValue, maxValue, timelockTier }

Functions:
  setParam(bytes32 key, uint256 value)                 // Only callable by timelock
  getParam(bytes32 key) -> uint256
  setBounds(bytes32 key, ParamBounds bounds)            // Only Tier 1 governance

Predefined parameter keys:
  MIN_STAKE, SLASH_DOWNTIME_PERCENT, SLASH_POOR_QUALITY_PERCENT,
  SLASH_LOGGING_PERCENT, SLASH_MALICIOUS_PERCENT, CREDENTIAL_TTL,
  FEE_OPERATOR_SHARE, FEE_TREASURY_SHARE, FEE_STAKER_SHARE,
  REPUTATION_WEIGHT_UPTIME, REPUTATION_WEIGHT_QUALITY,
  REPUTATION_WEIGHT_STAKE, REPUTATION_WEIGHT_HISTORY,
  UNBONDING_PERIOD, LIVENESS_PROBE_INTERVAL, ORACLE_COMMITTEE_SIZE,
  ACCEPTED_PAYMENT_TOKENS                     // Governed whitelist; ETH at launch, $6529 when ready
```

---

## 9. Client Architecture

### 9.1 Client Application

A cross-platform application (desktop + mobile) that combines:

1. **Wallet connection** (WalletConnect / injected provider)
2. **SIWE authentication** (sign-in with Ethereum)
3. **VPN client** (WireGuard tunnel management)
4. **Quality reporter** (sends session quality metrics to oracle)

### 9.2 Connection Flow (User Perspective)

```
1. Open app -> Connect wallet (one-time setup)
2. App checks NFT ownership -> Shows available tier
3. Tap "Connect" -> App performs SIWE auth in background
4. Gateway verifies NFT -> Issues WireGuard config
5. WireGuard tunnel established -> User is connected
6. On disconnect -> Session quality metrics submitted (signed)
```

After initial wallet connection, subsequent connections require only a single signature approval (can be automated with session keys for lower-friction UX).

### 9.3 Node Selection

The client selects nodes based on:
1. **Reputation score** (higher is better, minimum threshold configurable)
2. **Geographic proximity** (lower latency)
3. **Current load** (prefer less loaded nodes)
4. **User tier** (Free-tier THIS card holders and paid-tier users have equal node access; paid users may access higher-capacity nodes if operators offer tiered pricing)
5. **Random component** (prevents traffic concentration and enables privacy)

---

## 10. Node Operator Architecture

### 10.1 Running a Node

A node operator runs:
1. **Modified Sentinel dvpn-node** with NFT verification middleware
2. **Ethereum RPC connection** for NFT ownership checks
3. **Staking transaction** on the NodeRegistry contract

### 10.2 Node Software Components

```
sovereign-node/
  cmd/
    sovereign-node          # Main binary
  middleware/
    nft_gate.go             # NFT verification middleware for handshake endpoint
    reputation.go           # Reputation score reporting
  config/
    config.toml             # Node configuration
  workers/
    quality_reporter.go     # Periodically reports node metrics to oracle
    ethereum_sync.go        # Monitors Ethereum for NFT transfers, governance changes
```

### 10.3 Node Operator Economics

Node operators earn revenue from:
- **Session fees:** Paid-tier users pay per-GB or per-hour. Denominated in ETH at launch; the payment interface is token-agnostic (accepts any governance-whitelisted ERC-20), so $6529 can be added as a payment option via a TDH vote when the 6529 network token launches.
- **Staking rewards:** A share of network fees distributed to stakers proportional to (stake * reputation).
- **Free-tier subsidy:** THIS card holders use the network for free. Their bandwidth costs are covered by the Community treasury (funded by paid-tier fees and slashed funds).

Node operators pay for:
- **Staking deposit:** Minimum ETH stake locked in NodeRegistry.
- **Infrastructure costs:** Server, bandwidth, electricity.
- **Gas costs:** Registration, session settlement, and bandwidth proof transactions.

---

## 11. Security Model

### 11.1 Threat Model

| Threat | Mitigation |
|--------|-----------|
| **Node logging traffic** | Cannot be prevented cryptographically. Reputation system creates economic disincentive (slashing + lost future revenue). Multi-hop routing (future feature) limits single-node visibility. |
| **Node serving as MITM** | Client verifies TLS certificates end-to-end. DNS-over-HTTPS prevents DNS manipulation. Provable MITM evidence triggers full stake slash. |
| **NFT theft granting access** | Stolen NFTs grant VPN access, but this is equivalent to any stolen credential. delegate.xyz + cold storage mitigates. Transfer event monitoring revokes access within minutes. |
| **Flash loan governance attack** | NFTs cannot be flash-loaned. TDH requires holding duration, making it impossible to flash-borrow governance power. |
| **Sybil nodes (many low-quality)** | Minimum stake per node, IP fingerprinting, diminishing returns, geographic diversity requirements. |
| **51% governance attack** | Parameter bounds prevent destructive values. Tiered timelocks give honest participants time to react. Security Council can pause. |
| **Oracle committee collusion** | 2/3 threshold, bonded committee members, Community can replace, long-term goal is full decentralization of measurement. |

### 11.2 Privacy Properties

- **The Community and contracts never see traffic content.** Only bandwidth amounts and session metadata are recorded on-chain.
- **Node operators see traffic** (as with any VPN exit node). This is a fundamental property of VPNs, not unique to decentralized VPNs.
- **The Access Gateway sees wallet addresses.** This is the most significant privacy trade-off: VPN usage is linkable to a wallet address. Mitigations: use a dedicated wallet for VPN, use delegate.xyz from a fresh wallet.
- **On-chain session records are public.** Anyone can see that address X connected to node Y at time Z. The content of the session is not on-chain, but the metadata is. This is a meaningful privacy limitation compared to centralized VPNs that claim "no logs."

### 11.3 Audit Requirements

Before mainnet launch:
- [ ] Smart contract audit (all contracts)
- [ ] Node software security review (focus on handshake endpoint, credential issuance)
- [ ] Cryptographic review of SIWE implementation
- [ ] Penetration testing of gateway and node API surfaces

---

## 12. Phased Rollout Plan

### Phase 0: Proof of Concept (Months 1-3)

**Goal:** Demonstrate that Memes-gated VPN access works.

**Deliverables:**
- [ ] AccessPolicy.sol deployed on testnet (Sepolia), pointing at a test ERC-1155 contract that mimics Memes
- [ ] Modified dvpn-node with Memes verification middleware (single node)
- [ ] Basic client: wallet connect -> SIWE -> Memes check -> WireGuard config -> connect
- [ ] Free-tier (THIS card) vs. paid-tier (other Memes card) vs. denied (no card) logic working
- [ ] 6529 delegation contract + delegate.xyz integration
- [ ] No reputation system, no governance, no staking
- [ ] 3-5 test nodes operated by the core team

**Success criteria:** A Memes holder can connect to a VPN node. THIS card holders connect for free. Non-holders are rejected.

#### Sprint Breakdown (6 x 2-week sprints)

```
DEPENDENCY GRAPH

  Sprint 1A              Sprint 1B
  [Sentinel Node]        [Test ERC-1155]
       |                      |
       v                      v
  Sprint 2A              Sprint 2B
  [SIWE Gateway]         [AccessPolicy.sol]
       |                      |
       +----------+-----------+
                  |
                  v
             Sprint 3
         [NFT Middleware]
         (wire gateway +
          contract into
          node handshake)
                  |
                  v
             Sprint 4
         [WireGuard Creds]
         (auth -> tunnel)
                  |
                  v
             Sprint 5
         [Client App +
          Delegation]
                  |
                  v
             Sprint 6
         [Multi-Node +
          Community Test]
```

**Sprint 1 -- Foundations (Weeks 1-2)**

Two independent workstreams that can run in parallel:

*1A: Get Sentinel running*
- [ ] Clone `sentinel-official/dvpn-node`, build from source
- [ ] Run a single vanilla Sentinel node on a test server (WireGuard mode)
- [ ] Manually connect to it with a WireGuard client -- confirm traffic flows
- [ ] Document the handshake endpoint (`POST /`): request format, response format, auth model
- [ ] Identify the exact code path where we insert middleware (the handler function, the file, the line)

*1B: Deploy test NFTs on Sepolia*
- [ ] Write a minimal ERC-1155 contract that mimics The Memes (same interface, test tokens)
- [ ] Deploy to Sepolia
- [ ] Mint token ID 1 = "THIS card" (the whitepaper card) to 3-5 dev wallets
- [ ] Mint token ID 2 = "other Memes card" to a different set of dev wallets
- [ ] Keep some dev wallets empty (for testing denial)
- [ ] Verify `balanceOf(address, tokenId)` returns correct values via Etherscan / cast

**Done when:** Sentinel node is running and accepting WireGuard connections. Test ERC-1155 is deployed with tokens distributed across dev wallets.

---

**Sprint 2 -- Auth + Access Contract (Weeks 3-4)**

Two independent workstreams, again in parallel:

*2A: SIWE Authentication Gateway*
- [ ] Stand up a gateway service (Go or TypeScript -- match Sentinel's language for middleware, Go recommended)
- [ ] Implement SIWE challenge generation (EIP-4361): domain, nonce, URI, chain ID, issued-at
- [ ] Implement SIWE signature verification: `ecrecover` to recover wallet address
- [ ] Implement replay protection: nonce store (in-memory for now, Redis later)
- [ ] Implement timestamp validation (reject challenges older than 5 minutes)
- [ ] Test: sign with MetaMask, hit gateway, confirm wallet address recovery

*2B: AccessPolicy.sol*
- [ ] Write `AccessPolicy.sol` (see Section 8.2 for interface)
- [ ] `hasAccess(address)` -- calls `balanceOf` across a configurable list of token IDs, returns true if any > 0
- [ ] `hasFreeTier(address)` -- calls `balanceOf` for `thisCardTokenId`, returns true if > 0
- [ ] `setThisCardTokenId(uint256)` -- owner-only setter
- [ ] Write comprehensive tests: has THIS card, has other card, has no card, has both, edge cases
- [ ] Deploy to Sepolia, point at the test ERC-1155 from Sprint 1B
- [ ] Set `thisCardTokenId = 1`

**Done when:** Gateway can authenticate a wallet via SIWE. AccessPolicy correctly identifies free/paid/denied for test wallets on Sepolia.

---

**Sprint 3 -- NFT Verification Middleware (Weeks 5-6)**

This is where the two Sprint 2 workstreams converge:

- [ ] Write `nft_gate.go` middleware that wraps Sentinel's handshake handler
- [ ] Middleware flow: intercept `POST /` -> extract wallet address from SIWE session -> call AccessPolicy on Sepolia via RPC -> allow/deny/set tier -> pass to Sentinel handler or reject
- [ ] Implement RPC read caching (cache `balanceOf` results for 5 minutes to reduce RPC calls)
- [ ] Implement tier response: free-tier clients get no bandwidth cap flag; paid-tier clients get a "payment required" response (payment not implemented yet -- stub it)
- [ ] Integration test: connect wallet with THIS card -> middleware allows -> Sentinel handshake proceeds
- [ ] Integration test: connect wallet with other card -> middleware allows (paid stub) -> Sentinel handshake proceeds
- [ ] Integration test: connect wallet with no card -> middleware rejects -> connection refused
- [ ] Load test: 50 concurrent auth requests, confirm RPC caching holds

**Done when:** A wallet with a test Memes card can pass through the NFT gate and reach the Sentinel handshake. A wallet without is blocked.

---

**Sprint 4 -- WireGuard Credential Issuance (Weeks 7-8)**

- [ ] After successful NFT gate + Sentinel handshake, generate a time-bounded WireGuard peer configuration
- [ ] Credential contains: server public key, endpoint, allowed IPs, client IP assignment, expiry timestamp (24h)
- [ ] Implement credential expiry check: node rejects traffic from expired credentials
- [ ] Implement re-authentication flow: on expiry, client must re-sign SIWE + re-verify NFT
- [ ] **End-to-end test (the big one):** wallet signs SIWE -> gateway verifies -> NFT gate checks Memes -> WireGuard config issued -> tunnel established -> browse the internet through the VPN -> confirm IP is the node's IP
- [ ] Test credential expiry: wait for TTL, confirm tunnel drops, re-auth works
- [ ] Test free vs paid: both tiers can connect (paid payment is still stubbed)

**Done when:** A person with a test Memes card can connect to the VPN and browse the internet. This is the first time the full pipeline works end-to-end.

---

**Sprint 5 -- Client App + Delegation (Weeks 9-10)**

*5A: Minimal client application*
- [ ] Desktop app (Electron or native -- start with whatever is fastest to prototype)
- [ ] Wallet connection via WalletConnect and/or injected provider (MetaMask)
- [ ] "Connect" button: triggers SIWE sign -> sends to gateway -> receives WireGuard config -> activates tunnel
- [ ] Status display: connected/disconnected, current node, tier (free/paid), credential expiry countdown
- [ ] "Disconnect" button: tears down WireGuard tunnel
- [ ] Auto-reconnect on credential expiry (re-sign SIWE in background if wallet is unlocked)

*5B: Delegation support*
- [ ] Integrate 6529 Collections delegation contract ([nftdelegation](https://github.com/6529-Collections/nftdelegation)): if wallet has no Memes directly, check if it's a delegate for a wallet that does
- [ ] Integrate delegate.xyz as fallback
- [ ] Update AccessPolicy.sol with `checkWithDelegation(address)` (or implement delegation checks off-chain in the gateway -- cheaper and simpler for Phase 0)
- [ ] Test: cold wallet holds Memes, hot wallet is registered as delegate, hot wallet connects to VPN successfully
- [ ] Test: hot wallet that is NOT a delegate is still rejected

**Done when:** A user can open the app, connect their wallet, tap one button, and be on the VPN. Delegation works for cold storage users.

---

**Sprint 6 -- Multi-Node + Community Testing (Weeks 11-12)**

- [ ] Deploy 3-5 nodes across different regions (US East, US West, Europe, Asia -- use whatever is available)
- [ ] Basic node discovery: hardcoded node list in the client (no on-chain registry yet -- that's Phase 1)
- [ ] Client can select a node from the list (or auto-select by ping)
- [ ] Implement real-time revocation: gateway subscribes to `TransferSingle` / `TransferBatch` events on the test ERC-1155 via WebSocket
- [ ] Test: user connects -> transfer their test NFT to another wallet -> VPN session is terminated within minutes
- [ ] Credential TTL fallback test: disable event monitoring, confirm session still expires at 24h
- [ ] **Community testing round:** invite 10-20 6529 community members to test with Sepolia test NFTs
- [ ] Collect feedback: connection success rate, speed, UX friction points
- [ ] Write Phase 0 retrospective: what worked, what didn't, what changes for Phase 1

**Done when:** Multiple nodes in multiple regions, community members successfully connecting, real-time revocation working. Phase 0 is complete.

---

#### Phase 0 Team Requirements

| Role | Needed | Notes |
|------|--------|-------|
| Solidity developer | 1 | AccessPolicy.sol, test ERC-1155 (light -- ~2 weeks of work total) |
| Go developer | 1-2 | Sentinel middleware, gateway, node operations (heaviest workload) |
| Frontend developer | 1 | Client app (Sprints 5-6 only, can overlap with Go work early) |
| DevOps / infra | 1 (part-time) | Node deployment, Sepolia RPC, server provisioning |

Minimum viable team: **2 developers** (1 full-stack who can write Solidity + Go, 1 who handles client app + infra). Comfortable team: **3-4**.

### Phase 1: Alpha Network (Months 3-6)

**Goal:** Multi-node network with basic reputation and staking.

**Deliverables:**
- [ ] NodeRegistry.sol with ETH staking (testnet)
- [ ] Basic reputation scoring (uptime only)
- [ ] Node discovery (client can see and choose from multiple nodes)
- [ ] SessionManager.sol and PaymentSplitter.sol (testnet)
- [ ] 10-20 nodes operated by community volunteers
- [ ] Core team multisig for parameter management, TDH signaling votes on seize.io for direction

**Success criteria:** Community members can run nodes, stake ETH, and serve VPN connections to Memes holders. Nodes with poor uptime are visibly ranked lower.

#### Sprint Breakdown (6 x 2-week sprints)

**Context:** Phase 0 ended with 3-5 core-team nodes, a working NFT gate, a basic client, and a hardcoded node list. Phase 1 replaces the hardcoded list with an on-chain registry, adds staking so anyone can run a node, adds payments so paid-tier users can actually pay, and adds basic reputation so bad nodes are visible. By the end, this is a real network -- just not fully governed yet.

```
DEPENDENCY GRAPH

  Sprint 7A               Sprint 7B
  [NodeRegistry.sol]       [Liveness Monitor]
  (staking + register)     (ping service)
       |                        |
       v                        |
  Sprint 8                      |
  [Node Operator Toolkit]       |
  (register, stake, run)        |
       |                        |
       +----------+-------------+
                  |
                  v
             Sprint 9
         [Uptime Reputation]
         (EMA scoring from
          liveness probes)
                  |
                  v
             Sprint 10A             Sprint 10B
         [On-Chain Node Discovery]  [SessionManager +
          (client reads registry)    PaymentSplitter]
                  |                       |
                  +----------+------------+
                             |
                             v
                        Sprint 11
                    [Paid Tier Payments]
                    (end-to-end: connect,
                     use, pay, settle)
                             |
                             v
                        Sprint 12
                    [Community Operator
                     Onboarding + Multisig]
```

**Sprint 7 -- Staking Contract + Liveness Monitor (Weeks 13-14)**

Two parallel workstreams:

*7A: NodeRegistry.sol (Solidity)*
- [ ] Write `NodeRegistry.sol` (see Section 8.3 for interface)
- [ ] `registerNode(endpoint)` payable -- operator sends ETH >= `minimumStake`, node is registered with status `Active`
- [ ] `deregisterNode()` -- begins 21-day unbonding, sets status to `Unbonding`, records unbonding start timestamp
- [ ] `withdrawStake()` -- callable after unbonding period, returns ETH, removes node
- [ ] `minimumStake` hardcoded for now (0.5 ETH on testnet) -- governance sets it later in Phase 2
- [ ] `getActiveNodes()` -- returns all nodes with status `Active` (endpoint, operator address, stake amount, registration date)
- [ ] Events: `NodeRegistered`, `NodeDeregistered`, `StakeWithdrawn`
- [ ] Write tests: register, try to register below minimum (revert), deregister, try to withdraw early (revert), withdraw after unbonding
- [ ] Deploy to Sepolia

*7B: Liveness monitor service (Go)*
- [ ] Build a standalone service that pings every registered node's endpoint once per epoch (1 hour)
- [ ] Ping = attempt a TCP connection to the node's WireGuard port + an HTTP health check endpoint on the node's API
- [ ] Record results: `{ nodeAddress, epoch, wasOnline, latencyMs, timestamp }`
- [ ] Store results in a simple database (SQLite or Postgres -- this is off-chain)
- [ ] Expose a REST API: `GET /node/{address}/uptime` returns uptime history
- [ ] Run 3 monitor instances in different regions to prevent single-point-of-failure false negatives (node is "online" if 2/3 monitors can reach it)

**Done when:** Operators can stake ETH and register nodes on Sepolia. Liveness monitor is pinging registered nodes every hour and recording results.

---

**Sprint 8 -- Node Operator Toolkit (Weeks 15-16)**

- [ ] Write a node operator setup guide: system requirements, ports, firewall rules, how to get Sepolia ETH
- [ ] Build `sovereign-node register` CLI command: reads operator's private key (or connects to a wallet), calls `NodeRegistry.registerNode()` with stake deposit
- [ ] Build `sovereign-node deregister` CLI command: calls `deregisterNode()`, shows unbonding countdown
- [ ] Build `sovereign-node status` CLI command: shows stake amount, registration date, current uptime from liveness monitor API, earnings (placeholder for now)
- [ ] Add a health check endpoint to the node software (`GET /health`) that the liveness monitor hits
- [ ] Node auto-announces its endpoint to the registry on startup (reads from config, verifies it matches on-chain record)
- [ ] Test the full operator flow: install software -> configure -> stake -> register -> node appears in `getActiveNodes()` -> liveness monitor starts pinging it
- [ ] Recruit 3-5 community volunteers to run through this flow on testnet and report friction

**Done when:** A community member can follow the guide, stake testnet ETH, register a node, and see it appear in the registry and liveness monitor.

---

**Sprint 9 -- Uptime Reputation Scoring (Weeks 17-18)**

- [ ] Implement the uptime EMA calculation (Section 6.2, Category 1):
  ```
  UptimeScore_t = alpha * was_online_this_epoch + (1 - alpha) * UptimeScore_{t-1}
  alpha = 0.05
  ```
- [ ] Run the calculation off-chain in the liveness monitor service, per node, per epoch
- [ ] Add `updateReputationScore(address node, uint256 score)` to `NodeRegistry.sol` -- callable only by a designated `REPORTER_ROLE` (core team address for now, oracle committee in Phase 2)
- [ ] Liveness monitor service posts uptime scores on-chain once per day (not every epoch -- gas optimization; the hourly data is kept off-chain, the daily rollup goes on-chain)
- [ ] Add `reputationScore` to the `Node` struct in `NodeRegistry` -- updated by the reporter
- [ ] `getNodesByReputation(uint256 minScore)` -- returns nodes above a threshold
- [ ] Client-visible reputation: update client to show each node's uptime score in the node selection list
- [ ] Add a "minimum reputation" threshold in the client: nodes below score 200 are shown with a warning, nodes below 100 are hidden by default
- [ ] **Stake score (Section 6.2, Category 3):** Implement the simple formula `StakeScore = min(200, 200 * (stakedAmount / (3 * minimumStake)))` -- this can be computed on-chain directly from existing state, no off-chain measurement needed
- [ ] Combine into a Phase 1 simplified reputation: `ReputationScore = UptimeScore (0-300) + StakeScore (0-200)` -- max 500. Quality and history scores deferred to Phase 2.

**Done when:** Nodes have visible reputation scores driven by real uptime data. Nodes that go offline see their score drop. Nodes that stake more have higher scores. The client prefers higher-reputation nodes.

---

**Sprint 10 -- On-Chain Discovery + Payment Contracts (Weeks 19-20)**

Two parallel workstreams:

*10A: Replace hardcoded node list with on-chain discovery*
- [ ] Client reads `getActiveNodes()` from `NodeRegistry` on Sepolia at startup and on a refresh interval (every 5 minutes)
- [ ] Client caches the node list locally (so it works if RPC is temporarily unavailable)
- [ ] Node selection algorithm (Section 9.3): sort by reputation score, filter by minimum threshold, weight by geographic proximity (use IP geolocation of node endpoints), add random component
- [ ] Client UI: show node list with location flags, reputation scores, and ping latency
- [ ] Remove the hardcoded node list from Phase 0 -- the registry is the source of truth now

*10B: SessionManager.sol + PaymentSplitter.sol (Solidity)*
- [ ] `SessionManager.sol`: records session start/end on-chain. `startSession(nodeAddress)` creates a session record with timestamp. `endSession(sessionId, bandwidthUsedBytes)` closes it.
- [ ] Sessions are linked to the user's wallet and the node's address
- [ ] `PaymentSplitter.sol`: receives ETH payments and splits them according to configured ratios:
  - Operator share (default 80%)
  - Treasury share (default 10%)
  - Staker rewards pool (default 10%)
- [ ] `PaymentSplitter` is token-agnostic: accepts ETH via `receive()` and ERC-20 via `payWithToken(address token, uint256 amount)` -- but only ETH is whitelisted for now ($6529 opening preserved)
- [ ] Treasury address = core team multisig for now (Gnosis Safe on Sepolia)
- [ ] Write tests: start session, end session, payment splits correctly, only whitelisted tokens accepted
- [ ] Deploy both to Sepolia

**Done when:** Client discovers nodes from the on-chain registry. Session and payment contracts are deployed and tested on Sepolia.

---

**Sprint 11 -- Paid Tier End-to-End (Weeks 21-22)**

This is the sprint where money actually flows:

- [ ] Define the initial fee model: per-GB pricing (simplest to implement). Node operators set their own price per GB in their registry entry (add `pricePerGB` field to Node struct, or keep it off-chain in node API metadata -- off-chain is simpler to iterate on)
- [ ] Client shows price before connecting: "This node charges X ETH per GB. Estimated cost for 1 hour: ~Y ETH"
- [ ] Implement deposit/escrow flow:
  1. User deposits ETH into `SessionManager` when starting a session (pre-pay for estimated bandwidth)
  2. On session end, actual bandwidth is calculated, unused deposit is refunded
  3. Used portion is sent to `PaymentSplitter`
- [ ] Node middleware enforces payment: checks that the session has a valid deposit before allowing traffic
- [ ] Implement bandwidth tracking in the node: counts bytes per session, reports to `SessionManager` on session end
- [ ] Free-tier bypass: if user has THIS card (free tier), skip payment entirely -- no deposit, no settlement. Node serves them and logs bandwidth for future treasury reimbursement tracking.
- [ ] **End-to-end test:** paid-tier user connects -> deposits ETH -> uses VPN -> disconnects -> bandwidth settled -> operator receives 80%, treasury 10%, staker pool 10% -> user gets refund of unused deposit
- [ ] **End-to-end test:** free-tier user connects -> no deposit -> uses VPN -> disconnects -> bandwidth logged but no payment
- [ ] Gas optimization: batch session settlements (settle every N sessions or every M minutes, not per-session) -- or settle off-chain with periodic on-chain checkpoints. Decide approach based on testnet gas costs.

**Done when:** Paid users pay ETH to use the VPN. Operators receive revenue. Free-tier users connect without payment. The economic loop is closed.

---

**Sprint 12 -- Community Operators + Governance Prep (Weeks 23-24)**

- [ ] **Operator onboarding push:** publish the setup guide, announce in 6529 community channels, target 10-20 community-operated nodes
- [ ] Provide operator support: dedicated channel (Telegram/Discord) for troubleshooting
- [ ] Monitor network health: uptime dashboard showing all registered nodes, their reputation scores, session counts, revenue earned
- [ ] Deploy core team multisig (Gnosis Safe) on Sepolia with 3/5 threshold for parameter management
- [ ] Multisig controls: `minimumStake` value, `REPORTER_ROLE` assignment, treasury withdrawals, emergency node blacklisting
- [ ] Create first TDH signaling vote on seize.io: "Should Sovereign VPN move to mainnet?" -- non-binding, but establishes the governance pattern and gets community buy-in
- [ ] **Staker rewards distribution (first run):** calculate rewards from the staker pool proportional to (stake * reputation), distribute to node operators. This can be manual (multisig sends) for Phase 1 -- automated in Phase 2.
- [ ] Client polish based on Phase 0 + Phase 1 feedback: connection reliability, UX improvements, error messages
- [ ] Write Phase 1 retrospective: node count, uptime stats, revenue generated, community feedback, what changes for Phase 2
- [ ] **Document what's needed for mainnet:** remaining contracts (SlashingManager, ParameterRegistry, TimelockController), full reputation system, audit scope, and estimated costs

**Done when:** 10-20 community nodes running. Operators earning revenue. Multisig managing parameters. First TDH signaling vote completed. Network is ready for Phase 2 hardening.

---

#### Phase 1 Team Requirements

| Role | Needed | Notes |
|------|--------|-------|
| Solidity developer | 1 | NodeRegistry, SessionManager, PaymentSplitter (~6 weeks of work) |
| Go developer | 1-2 | Liveness monitor, operator CLI, node middleware updates, bandwidth tracking |
| Frontend developer | 1 | Client: on-chain discovery, reputation display, payment UX, deposit flow |
| DevOps / infra | 1 | Multi-region node deployment, monitoring dashboard, Gnosis Safe setup |
| Community manager | 1 (part-time) | Operator onboarding, support channel, signaling vote coordination |

Minimum viable team: **3 developers** + community support. Comfortable team: **4-5**.

#### Phase 0 -> Phase 1 Transition Checklist

Before starting Phase 1, confirm from Phase 0:
- [ ] NFT gate is stable (no auth failures in community testing)
- [ ] WireGuard tunnels are reliable (no unexplained drops)
- [ ] Delegation works for cold storage users
- [ ] At least 5 community members have successfully connected
- [ ] Phase 0 retrospective is written and reviewed

### Phase 2: Beta Network (Months 6-12)

**Goal:** Full reputation system and TDH governance, mainnet launch preparation.

**Deliverables:**
- [ ] Full reputation system (all 4 categories)
- [ ] Oracle committee for quality attestation
- [ ] SlashingManager with dispute resolution
- [ ] ParameterRegistry + TimelockController governed by executor multisig following TDH votes
- [ ] Executor multisig (4/7) elected via TDH vote on seize.io
- [ ] Treasury Gnosis Safe controlled by executor multisig
- [ ] Client apps (desktop + mobile)
- [ ] Smart contract audit
- [ ] Mainnet deployment of contracts
- [ ] AccessPolicy.sol pointed at real Memes contract on mainnet, thisCardTokenId set after card mint

**Success criteria:** The community can govern network parameters via TDH votes on seize.io. Node operators are meaningfully differentiated by reputation. Slashing works for provable misbehavior.

### Phase 3: Production Network (Month 12+)

**Goal:** Fully operational, TDH-governed, community-owned VPN network.

**Deliverables:**
- [ ] Full progressive decentralization (Phase 3 governance -- trustless bridge from 6529 governance to on-chain execution)
- [ ] 100+ community-operated nodes
- [ ] Integration with additional 6529 ecosystem NFTs (NextGen, Gradient, etc.) via governance votes
- [ ] Multi-hop routing (privacy enhancement)
- [ ] Geographic diversity incentives
- [ ] Self-sustaining economics (fees cover costs + free-tier subsidy + growth)

---

## 13. Open Questions

These are design decisions that need community input before implementation. Questions resolved through earlier discussion are marked as such.

### Architecture

1. **Ethereum L2 vs. Sentinel Hub?** Do we build the node registry and session management on an Ethereum L2 (simpler, single-chain) or integrate with Sentinel Hub (more mature VPN protocol, but dual-chain complexity)?

2. **Single-chain vs. dual-chain?** If we go Ethereum-only, we need to build session management and bandwidth proofs from scratch. If dual-chain, we need a bridge oracle. What's the better trade-off?

3. **Which L2?** If Ethereum L2, which one? Base (Coinbase ecosystem, low fees), Arbitrum (largest TVL), Optimism (governance-focused)?

### Economics

4. ~~**Native token or ETH-denominated?**~~ **RESOLVED: ETH-only.** No new token. All staking, fees, and payments in ETH.

5. **Fee model?** Per-GB, per-hour, flat monthly subscription, or hybrid? Node operators set their own pricing; the governance question is whether to set floor/ceiling parameters.

6. ~~**NFT pricing?**~~ **RESOLVED: No new NFT.** Access is gated by existing Memes cards. No new membership NFT to price.

### Governance

7. ~~**Voting power model?**~~ **RESOLVED: TDH-weighted.** All governance decisions weighted by TDH via existing 6529 network tooling on seize.io.

8. ~~**Should node operators get governance power from stake?**~~ **RESOLVED: No.** Governance power comes from TDH only, like everything else on the 6529 network. Node operators participate in governance via their TDH, not their stake.

### Privacy

9. **How much on-chain metadata is acceptable?** Session start/end times and bandwidth amounts are currently on-chain (inherited from Sentinel). Should we minimize this with zero-knowledge proofs or accept the trade-off?

10. **Multi-hop routing?** This significantly improves privacy (no single node sees both who you are and what you access) but increases latency and complexity. Phase 3 feature or core requirement?

### Implementation

11. **THIS card's token ID.** The `thisCardTokenId` in AccessPolicy.sol can only be set after the Meme card is minted and assigned a token ID. Who sets it and via what process? (Likely: core team multisig sets it once, then renounces the setter role.)

12. **Free-tier economics.** How is bandwidth for free-tier (THIS card) holders funded long-term? Initial plan is treasury subsidy from paid-tier fees, but is this sustainable at scale? What's the fallback if free-tier demand far exceeds treasury reserves?

### $6529 Token Integration

13. **$6529 payment timing.** The 6529 network token ($6529) is a natural payment currency for this network. Payment contracts are designed to be token-agnostic so $6529 can be enabled via governance vote. When the token launches and stabilizes, the community should vote on: adding $6529 to `ACCEPTED_PAYMENT_TOKENS`, fee pricing in $6529 terms, and whether to offer a discount for $6529 payment (aligning incentives with the broader 6529 ecosystem).

14. **$6529 for staking?** Should node operators be able to stake $6529 in addition to (or instead of) ETH? This could tighten the alignment between VPN node operators and the 6529 network, but introduces token price volatility into the security model. Decision deferred until $6529 tokenomics are known.

---

## References

### Sentinel dVPN Protocol
- [Sentinel Official Website](https://sentinel.co)
- [Sentinel dVPN Node (GitHub)](https://github.com/sentinel-official/dvpn-node)
- [Sentinel Hub (GitHub)](https://github.com/sentinel-official/hub)
- [Sentinel Go SDK (GitHub)](https://github.com/sentinel-official/sentinel-go-sdk)
- [Sentinel Whitepaper](https://docs.sentinel.co/assets/files/whitepaper-513665f81a5d6c4b462e111926d26f57.pdf)

### NFT Authentication & Token Gating
- [EIP-4361: Sign-In with Ethereum](https://eips.ethereum.org/EIPS/eip-4361)
- [delegate.xyz Documentation](https://docs.delegate.xyz/)
- [NiftyGate (Rust reverse proxy for NFT gating)](https://github.com/colstrom/niftygate)
- [Alchemy NFT API](https://www.alchemy.com/overviews/nft-token-gating)

### Reputation & Staking
- [EigenLayer Slashing Architecture](https://github.com/Layr-Labs/eigenlayer-contracts/blob/main/docs/core/AllocationManager.md)
- [Filecoin Reputation Systems](https://filecoin.io/blog/posts/reputation-systems-in-filecoin/)
- [Nym Delegated Staking](https://nym.com/blog/nym-delegated-staking-reputation-rewards-and-community-selection)
- [Helium HIP-66 Trust Score](https://github.com/helium/HIP/blob/main/0066-trust-score-and-denylist-convenience.md)
- [Chainlink Super-Linear Staking](https://blog.chain.link/explicit-staking-in-chainlink-2-0/)
- [EigenTrust Algorithm (Stanford)](https://nlp.stanford.edu/pubs/eigentrust.pdf)

### Community Governance
- [OpenZeppelin TimelockController](https://docs.openzeppelin.com/contracts/5.x/api/governance#TimelockController)
- [Gnosis Safe](https://safe.global/)
- [Nouns Community (1 NFT = 1 Vote)](https://nouns.wtf)

### 6529 Ecosystem
- [The Memes by 6529](https://6529.io/the-memes) -- ERC-1155 collection at `0x33fd426905f149f8376e227d0c9d3340aad17af1`
- [6529 Network / seize.io](https://seize.io) -- Governance platform, TDH-weighted voting
- [TDH (Total Days Held)](https://seize.io/tdh) -- Core governance metric: measures holding duration of Memes cards
- [6529 Collections NFT Delegation](https://github.com/6529-Collections/nftdelegation) -- Consolidation and delegation contract for cold storage support
- [6529 FAQ](https://6529.io/about/faq)
