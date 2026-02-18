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

The network builds on the [Sentinel dVPN](https://github.com/sentinel-official/dvpn-node) open-source protocol and adds three layers: Memes-gated access, reputation scoring, and TDH governance.

## Why?

Existing dVPNs (Sentinel, Mysterium, Orchid) solve the networking layer but have:
- Weak identity models (anyone can connect)
- No meaningful reputation systems (trust is assumed)
- Centralized or foundation-led governance

The 6529 ecosystem's thesis -- that open NFTs can replace accounts and communities can self-govern infrastructure -- has not been applied to privacy infrastructure. Until now.

## Read the Spec

**[TECHNICAL-SPEC.md](./TECHNICAL-SPEC.md)** -- Full technical specification including architecture, smart contracts, reputation system, governance model, and a sprint-level build plan.

## Project Status

**Phase 0: Proof of Concept** -- Not started. Looking for contributors.

See the [Sprint 1 issues](../../issues) to find tasks you can pick up.

## Architecture Overview

```
User Device (Wallet + VPN Client)
        |
        v
Access Gateway (SIWE auth + Memes card check)
        |
        v
Sentinel dVPN Layer (WireGuard tunnels + bandwidth tracking)
        |
        v
Ethereum Smart Contracts (AccessPolicy + Reputation + Parameters)
```

## How to Contribute

This project needs:

| Role | What You'd Work On |
|------|-------------------|
| **Go developer** | Sentinel node middleware, gateway, liveness monitor (heaviest workload) |
| **Solidity developer** | AccessPolicy, NodeRegistry, SessionManager, PaymentSplitter |
| **Frontend developer** | Desktop/mobile client: wallet connect + VPN in one app |
| **DevOps** | Node deployment, monitoring, infrastructure |

If you're in the 6529 community and want to help build this, open an issue or reach out.

### Getting Started (Development)

The project is in early planning. To get oriented:

1. Read [TECHNICAL-SPEC.md](./TECHNICAL-SPEC.md)
2. Look at the open issues for Sprint 1 tasks
3. Check out the Sentinel node codebase: [sentinel-official/dvpn-node](https://github.com/sentinel-official/dvpn-node)

## Key Links

- [Technical Specification](./TECHNICAL-SPEC.md)
- [The Memes by 6529](https://6529.io/the-memes)
- [6529 Network / seize.io](https://seize.io)
- [Sentinel dVPN Node](https://github.com/sentinel-official/dvpn-node)
- [EIP-4361: Sign-In with Ethereum](https://eips.ethereum.org/EIPS/eip-4361)
- [delegate.xyz](https://docs.delegate.xyz/)

## License

MIT
