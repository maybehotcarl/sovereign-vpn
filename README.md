# 6529 VPN

**An NFT-Gated, Reputation-Based, Community-Governed Decentralized VPN Network**

Built for and governed by the [6529 community](https://6529.io/).

---

## What Is This?

Sovereign VPN is a decentralized VPN where:

- **Access = holding a Memes card.** Any card from [The Memes by 6529](https://6529.io/the-memes) gets you in. No accounts, no emails, no KYC.
- **This project's Meme card = free VPN.** The card representing this idea is your free pass. All other Memes holders pay a fee.
- **Node operators stake ETH and earn community rep.** Quality is enforced by the [6529 reputation system](https://6529.io/) — operators need 6,529 "VPN Operator" rep (given by TDH holders) to run a node. On-chain staking + slashing backs it up.
- **All governance is TDH-weighted.** No new token. No new voting system. Decisions happen on [6529.io](https://6529.io/) using existing 6529 network infrastructure.

## Project Status

**Phase 0: In Active Development**

| Component | Status | Tests |
|-----------|--------|-------|
| Smart Contracts (AccessPolicy, TestMemes) | Deployed (Sepolia) | 38 passing |
| NodeRegistry (staking, heartbeat, slashing, RAILGUN addr) | Deployed (Sepolia) | 42 passing |
| SessionManager (payments, revenue split, vault routing) | Deployed (Sepolia) | 32 passing |
| SubscriptionManager (7/30/90/365-day plans, vault routing) | Built | 48 passing |
| PayoutVault (operator earnings aggregation) | Built | 26 passing |
| RAILGUN Private Payout Service | Built | 17 passing |
| SIWE Authentication Gateway | Built | 9 passing |
| NFT Gate Middleware + Sessions | Built | 11 passing |
| WireGuard Peer Manager | Built | 11 passing |
| Delegation Support (delegate.xyz + 6529) | Built | 5 passing |
| Transfer Event Revocation | Built | 5 passing |
| 6529 Rep Integration (api.6529.io) | Built | 10 passing |
| Node Registry Go Client + Discovery API | Built | - |
| CLI Client (`svpn`) | Built | 19 passing |
| End-to-End Integration Tests (mock + Sepolia) | Built | 9 passing |
| GitHub Actions CI (5 jobs) | Configured | - |
| Docker Support | Configured | - |

**Total: 177 Solidity + Go tests, 17 payout-service tests**

### Sepolia Testnet Contracts

| Contract | Address |
|----------|---------|
| TestMemes (ERC-1155) | `0x98C361b7C385b9589E60B36B880501D66123B294` |
| AccessPolicy | `0xF1AfCFD8eF6a869987D50e173e22F6fc99431712` |
| NodeRegistry | `0x35E5DB4132EB20E1Fab24Bb016BD87f382645018` |
| SessionManager | `0x2F13DE263b2Ceec57355833bDcC63b2a99853537` |

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   User Device                    │
│  svpn CLI: keygen → connect → status             │
│  Wallet (ERC-191 signing) + WireGuard client     │
└──────────────────────┬──────────────────────────┘
                       │
              HTTPS (SIWE auth)
                       │
┌──────────────────────▼──────────────────────────┐
│               Access Gateway                     │
│  POST /auth/challenge  → SIWE nonce              │
│  POST /auth/verify     → NFT check → session     │
│  POST /vpn/connect     → WireGuard peer config   │
│  POST /vpn/disconnect  → peer removal            │
│  GET  /vpn/status      → session info (Bearer)   │
│  GET  /nodes           → node discovery           │
│  GET  /health          → gateway status           │
│                                                  │
│  ┌─────────────┐ ┌──────────────┐ ┌───────────┐ │
│  │ NFT Checker  │ │  Delegation  │ │ Revocation│ │
│  │ AccessPolicy │ │ delegate.xyz │ │ Transfer  │ │
│  │ on-chain call│ │ 6529 registry│ │ events WS │ │
│  └─────────────┘ └──────────────┘ └───────────┘ │
│  ┌──────────────────┐ ┌────────────────────────┐ │
│  │ 6529 Rep Checker  │ │ Node Registry Client   │ │
│  │ api.6529.io       │ │ NodeRegistry on-chain  │ │
│  │ "VPN Operator" rep│ │ heartbeat + discovery  │ │
│  └──────────────────┘ └────────────────────────┘ │
└──────────────────────┬──────────────────────────┘
                       │
              WireGuard (UDP :51820)
                       │
┌──────────────────────▼──────────────────────────┐
│            Ethereum Smart Contracts              │
│  AccessPolicy.sol           - NFT ownership → tier      │
│  NodeRegistry.sol           - node staking + heartbeat  │
│  SessionManager.sol         - payments + revenue split  │
│  SubscriptionManager.sol    - subscription plans        │
│  PayoutVault.sol            - operator earnings vault   │
│  TestMemes.sol              - Sepolia test ERC-1155     │
└──────────────────────────────────────────────────────────┘
                       │
              ┌────────▼────────┐
              │  Payout Service  │
              │  (TypeScript)    │
              │  PayoutVault →   │
              │  RAILGUN shield  │
              │  → 0zk transfer  │
              └─────────────────┘
```

## Quick Start

### Build

```bash
make build           # builds bin/sovereign-gateway and bin/svpn
make test            # runs all tests (Solidity + Go + integration)
make docker-build    # builds Docker images
```

### Generate a wallet

```bash
./bin/svpn keygen --out wallet.key
# Address: 0x...
# Private key saved to: wallet.key
```

### Connect to a gateway

```bash
./bin/svpn connect \
  --gateway http://your-gateway:8080 \
  --key wallet.key

# Writes sovereign-vpn.conf — activate with:
sudo wg-quick up ./sovereign-vpn.conf
```

### Run a gateway node

```bash
./bin/sovereign-gateway \
  --listen :8080 \
  --eth-rpc 'https://eth-sepolia.g.alchemy.com/v2/YOUR_KEY' \
  --policy-contract '0xF1AfCFD8eF6a869987D50e173e22F6fc99431712' \
  --memes-contract '0x98C361b7C385b9589E60B36B880501D66123B294' \
  --node-registry '0x35E5DB4132EB20E1Fab24Bb016BD87f382645018' \
  --wg-interface wg0 \
  --wg-pubkey 'YOUR_WG_PUBLIC_KEY' \
  --wg-endpoint 'your-server:51820' \
  --delegation \
  --rep-min 6529 \
  --rep-category 'VPN Operator'
```

See [deploy/setup-node.sh](deploy/setup-node.sh) for full VPS setup.

## Repository Structure

```
sovereign-vpn/
├── contracts/          # Solidity (Foundry)
│   ├── src/
│   │   ├── AccessPolicy.sol         # NFT ownership → VPN access tiers
│   │   ├── NodeRegistry.sol         # Node staking, reputation, RAILGUN address
│   │   ├── SessionManager.sol       # Payment routing + revenue split
│   │   ├── SubscriptionManager.sol  # Subscription plans (7/30/90/365-day)
│   │   ├── PayoutVault.sol          # Operator earnings aggregation vault
│   │   └── TestMemes.sol            # Test ERC-1155 for Sepolia
│   ├── test/                        # 160 Foundry tests
│   └── script/                      # Deploy scripts
├── gateway/            # Go gateway server
│   ├── cmd/gateway/            # Entry point with graceful shutdown
│   ├── pkg/
│   │   ├── config/             # JSON config loader
│   │   ├── siwe/               # EIP-4361 challenge/verify
│   │   ├── nftcheck/           # AccessPolicy on-chain checker + delegation
│   │   ├── nftgate/            # Session management + HTTP middleware
│   │   ├── wireguard/          # Peer lifecycle + IP pool
│   │   ├── delegation/         # delegate.xyz v2 + 6529 registry
│   │   ├── revocation/         # ERC-1155 transfer event watcher
│   │   ├── noderegistry/       # On-chain node registry + RAILGUN address
│   │   ├── payoutvault/        # PayoutVault read-only client
│   │   ├── rep6529/            # 6529 community rep checker (api.6529.io)
│   │   └── server/             # HTTP handlers + node discovery + payout API
│   └── Dockerfile
├── client/             # Go CLI client
│   ├── cmd/svpn/               # CLI: connect, nodes, status, keygen, health
│   ├── pkg/
│   │   ├── api/                # Gateway HTTP client (auth + VPN + nodes)
│   │   ├── wallet/             # Ethereum key management + ERC-191 signing
│   │   └── wgconf/             # WireGuard config generation
│   └── Dockerfile
├── payout-service/     # TypeScript RAILGUN payout processor
│   ├── src/
│   │   ├── vault.ts            # PayoutVault contract bindings
│   │   ├── registry.ts         # NodeRegistry 0zk address reader
│   │   ├── railgun/            # RAILGUN SDK: shield + private transfer
│   │   ├── payout/             # Batch processor + retry logic
│   │   ├── receipts/           # SQLite payout receipt storage
│   │   └── monitoring/         # Health check endpoint
│   ├── test/                   # 17 vitest tests
│   └── Dockerfile
├── site-app/           # React frontend (wagmi + viem)
│   └── src/
│       ├── RailgunAddress.jsx  # Operator 0zk address registration
│       ├── SessionDashboard.jsx # VPN dashboard + payout status
│       └── contracts.js        # Contract ABIs + addresses
├── integration/        # End-to-end tests (mock + Sepolia live)
├── deploy/             # VPS setup scripts
├── research/           # Sentinel handshake analysis
└── .github/workflows/  # CI pipeline (5 jobs)
```

## RAILGUN Private Payouts

Operator earnings are private by default. Instead of withdrawing ETH to a public wallet (exposing payment amounts and operator identities on-chain), earnings route through RAILGUN's shielded transfer system.

**How it works:**

1. Users pay for VPN via SessionManager or SubscriptionManager contracts
2. The operator's 80% share gets forwarded to the **PayoutVault** contract
3. A **payout service** (weekly cron) pulls funds from the vault, shields them into RAILGUN, and sends private 0zk-to-0zk transfers to each operator's registered address
4. Operators register their RAILGUN `0zk` address on-chain via NodeRegistry (or through the frontend UI)

**Privacy trade-off:** Operators receive ~99.25–99.5% of earnings (RAILGUN shielding fee ~0.25%, relayer ~0.25%) in exchange for complete payment privacy.

The system is backward-compatible — if no vault is configured, operator earnings accumulate as withdrawable balances (legacy behavior). The payout service defaults to `DRY_RUN=true` for safe initial deployment.

## Privacy And Logging

Raw operational logs should not persist for more than `1 hour`, and they should never include source IPs, session tokens, full user wallet addresses, assigned client tunnel IPs, or traffic metadata. The codebase now redacts sensitive gateway runtime logs, but retention is still enforced by deployment infrastructure.

See [PRIVACY.md](PRIVACY.md) for the logging policy and deployment requirements. This project should be described as using minimal, short-lived off-chain logs, not as a literal "no logs" VPN.

If protecting user wallet identity on the connect path is a hard requirement, see [ANONYMOUS_ACCESS_PROTOCOL.md](ANONYMOUS_ACCESS_PROTOCOL.md) for the target architecture that replaces SIWE-based user admission.

## How to Contribute

| Role | What You'd Work On |
|------|-------------------|
| **Go developer** | Sentinel node middleware, gateway, liveness monitor |
| **Solidity developer** | NodeRegistry, SessionManager, PaymentSplitter |
| **Frontend developer** | Desktop/mobile client: wallet connect + VPN in one app |
| **DevOps** | Node deployment, monitoring, multi-region infrastructure |

If you're in the 6529 community and want to help build this, open an issue or reach out.

## Key Links

- [Technical Specification](./TECHNICAL-SPEC.md)
- [The Memes by 6529](https://6529.io/the-memes)
- [6529 Network / 6529.io](https://6529.io/)
- [Sentinel dVPN Node](https://github.com/sentinel-official/dvpn-node)
- [EIP-4361: Sign-In with Ethereum](https://eips.ethereum.org/EIPS/eip-4361)
- [delegate.xyz](https://docs.delegate.xyz/)

## License

MIT
