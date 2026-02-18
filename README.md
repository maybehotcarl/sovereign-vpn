# Sovereign VPN

**An NFT-Gated, Reputation-Based, DAO-Governed Decentralized VPN Network**

Built for and governed by the [6529 community](https://seize.io).

---

## What Is This?

Sovereign VPN is a decentralized VPN where:

- **Access = holding a Memes card.** Any card from [The Memes by 6529](https://6529.io/the-memes) gets you in. No accounts, no emails, no KYC.
- **This project's Meme card = free VPN.** The card representing this idea is your free pass. All other Memes holders pay a fee.
- **Node operators stake ETH and earn reputation.** Quality is enforced by an on-chain reputation system with slashing for misbehavior.
- **All governance is TDH-weighted.** No new token. No new voting system. Decisions happen on [seize.io](https://seize.io) using existing 6529 network infrastructure.

## Project Status

**Phase 0: In Active Development**

| Component | Status | Tests |
|-----------|--------|-------|
| Smart Contracts (AccessPolicy, TestMemes) | Built | 38 passing |
| SIWE Authentication Gateway | Built | 9 passing |
| NFT Gate Middleware + Sessions | Built | 11 passing |
| WireGuard Peer Manager | Built | 11 passing |
| Delegation Support (delegate.xyz + 6529) | Built | 5 passing |
| Transfer Event Revocation | Built | 5 passing |
| CLI Client (`svpn`) | Built | 19 passing |
| End-to-End Integration Tests | Built | 7 passing |
| GitHub Actions CI | Configured | - |
| Docker Support | Configured | - |

**Total: 105 tests across Solidity + Go**

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
│  GET  /vpn/status      → session info            │
│  GET  /health          → gateway status           │
│                                                  │
│  ┌─────────────┐ ┌──────────────┐ ┌───────────┐ │
│  │ NFT Checker  │ │  Delegation  │ │ Revocation│ │
│  │ AccessPolicy │ │ delegate.xyz │ │ Transfer  │ │
│  │ on-chain call│ │ 6529 registry│ │ events WS │ │
│  └─────────────┘ └──────────────┘ └───────────┘ │
└──────────────────────┬──────────────────────────┘
                       │
              WireGuard (UDP :51820)
                       │
┌──────────────────────▼──────────────────────────┐
│            Ethereum Smart Contracts              │
│  AccessPolicy.sol   - NFT ownership → tier       │
│  TestMemes.sol      - Sepolia test ERC-1155      │
└─────────────────────────────────────────────────┘
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
  --policy-contract '0xYOUR_ACCESS_POLICY' \
  --memes-contract '0x33fd426905f149f8376e227d0c9d3340aad17af1' \
  --wg-interface wg0 \
  --wg-pubkey 'YOUR_WG_PUBLIC_KEY' \
  --wg-endpoint 'your-server:51820' \
  --delegation
```

See [deploy/setup-node.sh](deploy/setup-node.sh) for full VPS setup.

## Repository Structure

```
sovereign-vpn/
├── contracts/          # Solidity (Foundry)
│   ├── src/
│   │   ├── AccessPolicy.sol    # NFT ownership → VPN access tiers
│   │   └── TestMemes.sol       # Test ERC-1155 for Sepolia
│   └── test/                   # 38 Foundry tests
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
│   │   └── server/             # HTTP handlers + revoker
│   └── Dockerfile
├── client/             # Go CLI client
│   ├── cmd/svpn/               # CLI entry point
│   ├── pkg/
│   │   ├── api/                # Gateway HTTP client
│   │   ├── wallet/             # Ethereum key management + ERC-191 signing
│   │   └── wgconf/             # WireGuard config generation
│   └── Dockerfile
├── integration/        # End-to-end tests with mock Ethereum RPC
├── deploy/             # VPS setup scripts
├── research/           # Sentinel handshake analysis
└── .github/workflows/  # CI pipeline
```

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
- [6529 Network / seize.io](https://seize.io)
- [Sentinel dVPN Node](https://github.com/sentinel-official/dvpn-node)
- [EIP-4361: Sign-In with Ethereum](https://eips.ethereum.org/EIPS/eip-4361)
- [delegate.xyz](https://docs.delegate.xyz/)

## License

MIT
