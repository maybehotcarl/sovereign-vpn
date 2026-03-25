# Sovereign VPN Current State

Snapshot date: March 17, 2026

This file exists because the top-level `README.md` still mostly describes the
older NFT-gated VPN architecture. The active implementation focus has moved to
the anonymous paid-access path built around `vpn_access_v1`.

For the current verified mainnet contract surface, see `MAINNET_ADDRESSES.md`.
For the concrete beta launch execution plan, see
`BETA_LAUNCH_CHECKLIST.md`.

## What Is Actually Working

- `site-app/6529-zk-api` starts locally with the current `.env.local`.
- The local ZK API health endpoint returns `status=ok`.
- The `vpn_access_v1` verification key and circuit artifacts are already staged
  in `site-app/6529-zk-api`.
- The ZK API is serving the latest `vpn_access_v1` root.
- The ZK API is serving proof lookup data for a registered identity
  commitment.
- The dev entitlement registration route is live and returning `200`.

## Where The Project Really Is

The current implementation line is:

1. A browser client keeps a local anonymous identity.
2. The client gets an anonymous challenge from the VPN gateway.
3. The client generates a `vpn_access_v1` proof locally.
4. The gateway verifies the proof against the hosted ZK API.
5. The gateway provisions a WireGuard session without learning the wallet
   address.

Relevant code paths:

- Frontend anonymous flow: `site-app/src/VPNConnect.jsx`
- Frontend anonymous config helpers: `site-app/src/anonymous.js`
- Frontend local identity storage: `site-app/src/anonymousIdentity.js`
- Hosted ZK API: `site-app/6529-zk-api`
- Reusable ZK SDK and circuits: `site-app/6529-zk-service`
- Gateway anonymous challenge and connect routes: `gateway/pkg/server/server.go`

## What Is Not Finished

- The final entitlement issuer/indexer architecture is not built yet.
  `POST /api/vpn-access/dev-register` is still the local/staging stand-in.
- The top-level docs are not fully aligned with the anonymous launch path.
- A full browser-to-gateway-to-WireGuard demo has not been re-verified in this
  snapshot.
- The dev entitlement registration path is noticeably slow in local testing
  (roughly 45-50 seconds), which is likely the main live performance issue in
  the current path.

## What Help Is Most Useful Right Now

- Decide the immediate goal:
  - local ZK smoke-test only
  - full anonymous browser + gateway demo
  - final issuer/indexer implementation
- If the goal is the full demo, provide or confirm the gateway and WireGuard
  environment.
- If the goal is productization, prioritize replacing the dev registration flow
  with the real entitlement issuer/indexer.

## Recommended Next Step

If the goal is clarity and momentum, the best next checkpoint is:

1. Re-verify the full anonymous browser + gateway demo end to end.
2. Document the exact run sequence that works.
3. Then replace the dev registration path with the real issuer flow.
