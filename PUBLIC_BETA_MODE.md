# Public Beta Mode

Snapshot date: April 2, 2026

## Current Decision

The current public beta mode for `https://6529vpn.io` is:

- `Direct Wallet Session` is public and supported.
- `Anonymous Session` is public and supported.

This is the current launch posture because both paths are now wired on the live
stack:

- public site deploy is working
- wallet sign-in works
- subscription purchase works
- WireGuard config generation works
- live WireGuard tunnel establishment has been verified against the production
  gateway
- public `zk-api` is deployed at the same origin under `/api/*`
- the public gateway is configured with `--zk-api-url http://127.0.0.1:3002`
- the public browser build is pinned to `https://6529vpn.io/api/*`

The two public access modes now serve different purposes:

- `Direct Wallet Session` is the better default for regular daily VPN use.
- `Anonymous Session` is the privacy-focused path with a 30-minute renewable
  lease layered on top of an active subscription.

## What Is Still True About Anonymous

Even though anonymous mode is now public, the UX tradeoff is still real:

- the anonymous lease is still `30 minutes`
- the user must reconnect or refresh to obtain a new anonymous session after
  the lease expires
- direct wallet mode remains the simpler recommendation for uninterrupted VPN use

## What "Public Beta" Means Right Now

Public beta currently means:

- web-first
- first-party operated
- direct wallet-bound access on the public site
- anonymous paid access on the public site
- on-chain subscription purchase on mainnet
- WireGuard config download/import by the customer

## Remaining Validation To Keep Watching

Public anonymous is live, but these are still the checks to keep exercising:

- a public `zk-api` is deployed and healthy
- the public gateway is configured with `--zk-api-url`
- CORS is correct for `https://6529vpn.io`
- the public frontend is built with the real public ZK API URL
- a real entitled wallet completes the full anonymous path on the public stack:
  issuer activation, proof generation, anonymous connect, status, disconnect
- public logs and retention controls are verified against the anonymous path

## Operator Rule

- keep the public `zk-api` on the same origin under `/api/*`
- keep `sovereign-zk-api.service` bound to `127.0.0.1:3002`
- keep the public gateway pointed at `http://127.0.0.1:3002`
