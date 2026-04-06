# Public Beta Test Matrix

Snapshot date: April 6, 2026

This document is the concrete beta test matrix for the live public product at
`https://6529vpn.io`.

It separates:

- checks Codex can verify from this environment
- checks that require a real wallet or a real user device
- platform coverage that can only be closed on macOS or Windows

Use this with:

- [BETA_LAUNCH_CHECKLIST.md](/home/maybe/repos/sovereign-vpn/BETA_LAUNCH_CHECKLIST.md)
- [PUBLIC_BETA_MODE.md](/home/maybe/repos/sovereign-vpn/PUBLIC_BETA_MODE.md)
- [deploy/PUBLIC_OPERATIONS.md](/home/maybe/repos/sovereign-vpn/deploy/PUBLIC_OPERATIONS.md)
- [desktop-app/README.md](/home/maybe/repos/sovereign-vpn/desktop-app/README.md)

## Status Legend

- `verified-codex`: verified directly from this shell or the live droplet
- `pending-human`: needs a real wallet or a real user device
- `blocked-platform`: needs a Mac or Windows machine
- `failed`: test was run and failed

## Codex-Verified Prerequisites

These are the live checks that were verified from this environment on April 6,
2026.

| Check | Status | Evidence |
| --- | --- | --- |
| Public frontend reachable | `verified-codex` | `./deploy/check-public-stack.sh` |
| Public gateway `/health` returns `status: ok` | `verified-codex` | `./deploy/check-public-stack.sh` |
| Public `zk-api` `/api/health` returns `status: ok` | `verified-codex` | `./deploy/check-public-stack.sh` |
| Public `zk-api` `/api/meta` advertises anonymous mode enabled and `dev-register` disabled | `verified-codex` | `./deploy/check-public-stack.sh` |
| Public `/session/info` returns mainnet purchase metadata | `verified-codex` | `./deploy/check-public-stack.sh` |
| Public `/subscription/tiers` returns active mainnet tiers | `verified-codex` | `./deploy/check-public-stack.sh` |
| Droplet `sovereign-gateway` service active | `verified-codex` | `PUBLIC_ALERT_REMOTE_HOST=root@142.93.159.175 python3 deploy/public_stack_alerts.py --dry-run --json` |
| Droplet `sovereign-zk-api` service active | `verified-codex` | `PUBLIC_ALERT_REMOTE_HOST=root@142.93.159.175 python3 deploy/public_stack_alerts.py --dry-run --json` |
| Droplet `wg0` interface healthy | `verified-codex` | `PUBLIC_ALERT_REMOTE_HOST=root@142.93.159.175 python3 deploy/public_stack_alerts.py --dry-run --json` |
| Linux desktop app source smoke | `verified-codex` | `cd desktop-app && npm run smoke` |
| Linux desktop artifacts exist | `verified-codex` | `ls -lh desktop-app/release` |

## Human Wallet Test Matrix

Each row below should be run against the live public site. The six platform
columns correspond to:

- `L-Import`: Linux + raw WireGuard config import
- `L-App`: Linux + desktop app handoff
- `Mac-Import`: macOS + raw WireGuard config import
- `Mac-App`: macOS + desktop app handoff
- `Win-Import`: Windows + raw WireGuard config import
- `Win-App`: Windows + desktop app handoff

Default current state for all human rows is `pending-human` unless otherwise
noted.

| ID | Wallet State | Access Mode | Expected Result | L-Import | L-App | Mac-Import | Mac-App | Win-Import | Win-App | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| D1 | Clean wallet, supported card, no subscription | Direct | Purchase flow appears, subscription purchase succeeds, connect succeeds, WireGuard session comes up | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | Must not require a second purchase after the first successful buy |
| A1 | Clean wallet, supported card, no subscription | Anonymous | Purchase flow appears, issuer activation succeeds after purchase, anonymous connect succeeds | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | Confirms wallet -> issuer -> prove -> connect bootstrap from zero |
| D2 | Wallet with active subscription | Direct | Connect succeeds immediately with no purchase prompt | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | Use a wallet that already has an active subscription before loading the site |
| D3 | Wallet with active subscription | Direct | Disconnect succeeds cleanly, reconnect succeeds cleanly, no new purchase required | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | Confirm session dashboard state updates correctly between each step |
| A2 | Wallet with active subscription | Anonymous | Issuer activation succeeds, proof generation succeeds, anonymous connect succeeds | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | This is the main public anonymous happy path |
| A3 | Wallet with active subscription | Anonymous | Disconnect succeeds cleanly, reconnect succeeds and returns a fresh anonymous session | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | Confirm a fresh config/session is issued on reconnect |
| A4 | Wallet with active subscription | Anonymous | Let the 30-minute lease expire, then reconnect successfully | `pending-human` | `pending-human` | `pending-human` | `blocked-platform` | `pending-human` | `blocked-platform` | For raw import, explicitly record whether stale full-tunnel config blackholes traffic until manual disconnect |

## Platform-Specific Expectations

These expectations are important while running the matrix:

- Linux imported config:
  - expected to work with `wireguard-tools`
  - raw import remains the least friendly path
- Linux desktop app:
  - should receive localhost handoff from the website
  - should write config and manage disconnect at lease expiry
- macOS imported config:
  - expected to rely on the official WireGuard app or equivalent
- macOS desktop app:
  - currently requires a real Mac build and runtime validation
- Windows imported config:
  - expected to rely on the official WireGuard client
- Windows desktop app:
  - currently requires a real Windows build and runtime validation

## How To Record Each Manual Result

For every human-run row, capture:

- timestamp in UTC
- wallet state used:
  - card only, no subscription
  - active subscription
- access mode:
  - direct
  - anonymous
- delivery method:
  - imported config
  - desktop app handoff
- whether purchase UI appeared
- whether purchase was required or skipped
- whether connect succeeded
- whether disconnect succeeded
- whether reconnect succeeded
- before/after public IP
- any on-screen error text

## Codex Follow-Up Commands

After a human run, these are the commands Codex can use to validate the live
stack side:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/check-public-stack.sh
```

```bash
PUBLIC_ALERT_REMOTE_HOST=root@142.93.159.175 \
python3 deploy/public_stack_alerts.py --dry-run --json
```

```bash
ssh root@142.93.159.175 "sudo wg show wg0"
```

```bash
ssh root@142.93.159.175 \
  "journalctl -u sovereign-gateway --since '-10 min' --no-pager | tail -n 200"
```

## What Is Actually Closed Today

As of this snapshot, the matrix is only partially closed:

- live public stack health is green
- live anonymous metadata is green
- Linux desktop packaging and source smoke are green

What is not closed yet:

- clean-wallet purchase flows on the public site
- formal direct disconnect/reconnect coverage on all platforms
- formal anonymous expiry/reconnect coverage on all platforms
- macOS desktop handoff
- Windows desktop handoff
