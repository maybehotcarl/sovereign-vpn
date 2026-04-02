# Public Beta Mode

Snapshot date: April 2, 2026

## Current Decision

The current public beta mode for `https://6529vpn.io` is:

- `Direct Wallet Session` is public and supported.
- `Anonymous Session` is not part of the public beta yet.

This is the current launch posture because the direct wallet path is now proven
on the live stack:

- public site deploy is working
- wallet sign-in works
- subscription purchase works
- WireGuard config generation works
- live WireGuard tunnel establishment has been verified against the production
  gateway

The anonymous path is still an internal/staging track until the public stack is
ready for it end to end.

## Why Anonymous Is Not Public Yet

The public stack is not ready to expose anonymous access safely and cleanly.

Current blockers:

- the live public gateway is still running without a public `--zk-api-url`
- there is no public `zk-api` deployment currently wired into `6529vpn.io`
- the public browser build is not yet pinned to a public ZK API contract
- the full anonymous public flow has not been re-verified end to end on the
  public stack with a real entitled wallet

## What "Public Beta" Means Right Now

Public beta currently means:

- web-first
- first-party operated
- direct wallet-bound access on the public site
- on-chain subscription purchase on mainnet
- WireGuard config download/import by the customer

Anonymous access remains a staged follow-on feature, not a launch blocker for
the current public beta.

## Exit Criteria To Turn On Public Anonymous Mode

Do not enable anonymous mode publicly until all of the following are true:

- a public `zk-api` is deployed and healthy
- the public gateway is configured with `--zk-api-url`
- CORS is correct for `https://6529vpn.io`
- the public frontend is built with the real public ZK API URL
- a real entitled wallet completes the full anonymous path on the public stack:
  issuer activation, proof generation, anonymous connect, status, disconnect
- public logs and retention controls are verified against the anonymous path

## Operator Rule

Until those gates are met:

- keep `VITE_ENABLE_ANON_VPN=false` in the public build
- treat anonymous access as staging/internal only
- do not describe public anonymous mode as live
