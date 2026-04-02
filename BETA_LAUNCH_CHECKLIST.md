# Public Paid Beta Launch Checklist

Snapshot date: March 24, 2026

This is the concrete execution checklist for launching the first public paid
beta of Sovereign VPN's anonymous access path.

Important current decision: the live public beta is currently `direct wallet`
only. Use [PUBLIC_BETA_MODE.md](/home/maybe/repos/sovereign-vpn/PUBLIC_BETA_MODE.md)
as the source of truth for what is publicly enabled today. This checklist
remains the gating plan for turning on the anonymous public path.

Use this document with:

- `deploy/rehearsal/README.md` for the two-gateway rehearsal stack
- `RELEASE_GATES.md` for longer-horizon product and contract gates
- `PRIVACY.md` for logging and retention requirements
- `MAINNET_ADDRESSES.md` for verified mainnet addresses

## Launch Scope

This checklist assumes the launch surface is:

- web-first
- first-party operated gateways only
- anonymous gateway-private paid access
- issuer-backed `vpn_access_v1` entitlements
- multiple concurrent anonymous sessions
- 30-minute renewable anonymous credentials
- Redis-backed shared gateway state

This checklist does not block on:

- ownership transfer cleanup
- community operator onboarding
- `NodeRegistry` enforcement through the legacy `SessionManager`
- payout-service scale work beyond first-party beta operations
- CLI parity

## Stop-Ship Conditions

Do not launch if any of the following are true:

- production `zk-api` still exposes `dev-register` to public clients
- the public browser flow still depends on SIWE/NFT gateway auth
- gateways run without Redis-backed shared state
- wrong-node `status` or `disconnect` still fail instead of forwarding
- dead-owner reconnect fails with a fresh WireGuard key
- raw logs retain wallet addresses, auth headers, session tokens, full
  WireGuard public keys, or assigned client tunnel IPs
- the real mainnet entitled-wallet happy path is not verified end to end
- `POST /api/zk` still times out or runs close to the gateway timeout budget

## Week Plan

## Day 1: Freeze Scope And Production Config

Objective: lock the exact launch shape and confirm every service is pointed at
production-intended config rather than demo paths.

Required checks:

- `site-app/6529-zk-api/.env.local` or production env validates cleanly:

```bash
cd /home/maybe/repos/sovereign-vpn/site-app/6529-zk-api
npm run check:issuer-env -- --env-file .env.local
```

- `GET /api/meta` shows:
  - `issuer.enabled = true`
  - `devRegistration.enabled = false`
  - `credentialTtlSeconds = 1800`

```bash
curl -s http://127.0.0.1:3002/api/meta | jq '.anonymousVpn'
```

- Both gateways are configured with:
  - the same `SESSION_SIGNING_KEY`
  - the same `GATEWAY_FORWARDING_KEY`
  - unique `GATEWAY_INSTANCE_ID`
  - correct public and forwarding URLs
  - Redis enabled

Pass:

- issuer env checker reports `0 failures`
- public metadata reports issuer-first production behavior
- no public UI path depends on `dev-register`

Fail:

- any production env still enables insecure dev registration
- either gateway is still running with local in-memory-only state

## Day 2: Bring Up The Staging Rehearsal Stack

Objective: boot the exact multi-node stack used for launch rehearsal.

Run:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/rehearsal/check-prereqs.sh
```

```bash
cd /home/maybe/repos/sovereign-vpn/site-app/6529-zk-api
npm run dev
```

```bash
docker compose \
  --env-file /home/maybe/repos/sovereign-vpn/deploy/rehearsal/.env \
  -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml \
  up --build -d
```

Health checks:

```bash
curl -s http://127.0.0.1:3002/api/health | jq
curl -s http://127.0.0.1:8081/health | jq
curl -s http://127.0.0.1:8082/health | jq
```

Pass:

- `zk-api` health is `ok`
- both gateways report shared state enabled and healthy
- both gateways expose a live `wg0` interface

Fail:

- Docker, `/dev/net/tun`, or gateway health is not clean
- either gateway lacks forwarding or shared-state wiring

## Day 3: Prove The Mainnet Happy Path

Objective: verify the real customer path with a real mainnet entitled wallet.

Required path:

1. Browser loads issuer metadata.
2. Wallet signs issuer challenge.
3. Issuer activation succeeds on mainnet.
4. Browser generates `vpn_access_v1` proof.
5. Gateway accepts anonymous connect.
6. Browser can fetch status and disconnect.

Pass:

- activation succeeds without any `dev-register` fallback
- browser proof generation succeeds against live production metadata
- connect, status, and disconnect all work with a real entitled wallet

Fail:

- only throwaway or ineligible wallets have been tested
- activation requires any staging-only bypass
- connect path still leaks back to public wallet-authenticated gateway routes

Notes:

- For issuer eligibility, use the direct mainnet Memes path currently exposed in
  `site-app/6529-zk-api`.
- Use `MAINNET_ADDRESSES.md` as the source of truth for mainnet addresses.

## Day 4: Run Failure Drills

Objective: prove the multi-node behavior under real faults.

Execute the drills in `deploy/rehearsal/README.md`:

- wrong-node forwarding
- owner restart recovery
- owner death and takeover
- Redis outage

Add one more manual drill:

- reconnect after owner death using a fresh WireGuard key from the browser

Pass:

- wrong-node `status` and `disconnect` succeed through forwarding
- owner restart restores peer state cleanly
- owner death produces a recoverable unavailable state, then reconnect succeeds
- Redis outage is visible in health and recovery is clean after restore

Fail:

- any active session becomes permanently wedged after node loss
- any reconnect requires manual Redis cleanup or peer cleanup
- any gateway silently reports healthy while shared state is degraded

## Day 5: Enforce Privacy And Logging Controls

Objective: make infrastructure match the privacy claims.

Required deployment controls:

- raw operational logs retained for no more than `1 hour`
- reverse-proxy access logs disabled, or header-redacted and TTL-purged
- token-bearing headers such as `Authorization` redacted before storage
- no persistent logging of:
  - source IPs
  - wallet addresses
  - session tokens
  - SIWE signatures
  - full WireGuard public keys
  - assigned client tunnel IPs

Validation:

- inspect sample gateway logs
- inspect reverse-proxy logs
- inspect container-platform log retention settings

Pass:

- sampled logs contain only coarse operational fields
- retention is enforced by infra, not just size-based rotation

Fail:

- raw request logs persist beyond the stated limit
- sensitive headers or user identifiers are still visible in stored logs

## Day 6: Establish Alerting And Runbooks

Objective: ensure the service can be operated once it is public.

Minimum alerts:

- issuer activation failure rate spike
- `POST /api/zk` latency spike or timeout
- root publication lag
- Redis unavailable or degraded
- gateway health degraded
- WireGuard peer allocation exhaustion

Minimum runbooks:

- issuer outage
- gateway owner death
- Redis outage
- stale root publication
- emergency disable of public connect UI

Pass:

- every alert has a destination and a named responder
- every runbook is written and can be followed without repo archaeology

Fail:

- alerts exist only in someone’s head
- a new operator cannot tell how to drain traffic or disable connects

## Day 7: Go/No-Go Review

Objective: make a launch decision from evidence rather than momentum.

Required sign-off:

- product scope freeze accepted
- staging happy path passed
- failure drills passed
- privacy controls verified
- alerting and runbooks verified
- at least one real mainnet entitled-wallet session completed end to end

Launch only if all of the following are true:

- public UI uses the anonymous issuer-first path
- `dev-register` is disabled in production
- both gateways are healthy behind Redis
- a gateway can die without permanently wedging active users
- logs match `PRIVACY.md`
- operators know how to rollback or disable connects

If any launch-only blocker remains open, do not partially launch. Either fix it
or explicitly narrow the beta scope before exposing it publicly.

## Evidence To Capture

Before launch, save the following in a dated release folder or internal note:

- `api/meta` response from the launch `zk-api`
- both gateway health responses
- screenshots or logs from the real entitled-wallet happy path
- notes from each failure drill
- proof that raw-log retention and redaction are active
- the final env/secret inventory owner list

## Immediate Next Commands

If starting from this repo today, run these first:

```bash
cd /home/maybe/repos/sovereign-vpn/site-app/6529-zk-api
npm run check:issuer-env -- --env-file .env.local
```

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/rehearsal/check-prereqs.sh
```

Then follow `deploy/rehearsal/README.md` from top to bottom.
