# Sovereign VPN

Decentralized VPN with on-chain access control, RAILGUN private payouts, and ZK membership proofs.

## Architecture

- `contracts/` — Solidity (Foundry). NodeRegistry, SessionManager, SubscriptionManager, AccessPolicy, PayoutVault. Deployed on Ethereum mainnet.
- `gateway/` — Go. VPN gateway server. WireGuard management, SIWE auth, session management, on-chain verification.
- `client/` — Go CLI. WireGuard client, wallet-based auth, auto node selection.
- `payout-service/` — TypeScript/Node. RAILGUN private transfer engine for operator payouts.
- `site-app/` — React 19 + Vite + wagmi/RainbowKit. Web dashboard for connecting/managing VPN sessions.
- `site/` — Static landing page.
- `zk/` — Circom. ZK membership circuits (Groth16).
- `node/` — Docker. VPN node operator setup.
- `integration/` — Go. E2E tests against Sepolia.

## Build & Test

```bash
# Contracts
cd contracts && forge build && forge test

# Gateway
cd gateway && go build ./... && go test ./...

# Client
cd client && go build ./... && go test ./...

# Payout service
cd payout-service && npm install && npm test

# Site app
cd site-app && npm install && npm run build

# Integration
cd integration && go test ./...
```

## Known Issues

See `REVIEW.md` for a comprehensive code review with prioritized findings. Remaining critical items:
1. ~~Gateway session auth uses wallet address as bearer token~~ — **Fixed**: sessions now use HMAC-signed opaque tokens
2. Payout processor accounting bug across failed batches
3. SIWE validation is incomplete (missing URI, expiry checks; chain ID now enforced)

## Conventions

- Solidity: Foundry, OpenZeppelin v5.5.0
- Go: standard library + go-ethereum
- TypeScript: ESM, ethers.js v6
- Frontend: React 19, Vite, wagmi v2, RainbowKit
- Chain: Ethereum mainnet (chain ID 1)
