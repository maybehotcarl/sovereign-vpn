# Operator Enrollment Deployment Notes

Date: June 19, 2026 UTC

This note records the work done to make the operator-dashboard enrollment flow real, durable, and live on `6529vpn.io`.

## Goal

The north star was to make creating a VPN node as simple as possible:

1. An operator connects a wallet in the dashboard.
2. The dashboard asks the gateway for a SIWE challenge.
3. The operator signs the challenge.
4. The gateway creates a real enrollment token.
5. The dashboard generates a one-command Ubuntu/VPS installer.
6. The node installer reports back to the control-plane gateway.
7. Enrollment state and installer reports persist in Supabase Postgres.

## Current Production Shape

`6529vpn.io` is currently served from a DigitalOcean droplet:

- Droplet IP: `142.93.159.175`
- Hostname: `ubuntu-s-1vcpu-1gb-amd-tor1-01`
- Caddy serves HTTPS and static frontend assets.
- Go gateway runs as `sovereign-gateway.service`.
- ZK API runs as `sovereign-zk-api.service`.
- Payout service runs as `payout-service.service`.
- This deployment is not Docker-based right now; it is systemd services plus Caddy.

Important live paths:

- Gateway binary: `/usr/local/bin/sovereign-gateway`
- Gateway systemd unit: `/etc/systemd/system/sovereign-gateway.service`
- Gateway root-only env file: `/etc/sovereign-vpn/gateway.env`
- Frontend web root: `/var/www/6529vpn`
- Live Caddyfile: `/etc/caddy/Caddyfile`
- Deployment backup directory from this work: `/root/sovereign-vpn-deploy-backups/20260619050926`

The gateway process listens on `127.0.0.1:8080` / `:8080` and Caddy proxies API routes to it.

## Supabase Setup

Supabase project:

- Project name: `sovereign-vpn`
- Project ID/ref: `wqqgmzuqdubkcbufqrgn`
- Region: `ca-central-1`
- Database engine observed: Postgres 17

Created table:

- `public.operator_enrollments`

Local migration file:

- `supabase/migrations/20260619001458_operator_enrollments.sql`

The migration:

- Creates `public.operator_enrollments`
- Adds indexes on `operator`, `expires_at`, and `status`
- Enables RLS
- Revokes table access from `anon` and `authenticated` roles if those roles exist

This table is intended for server-side gateway use only. The frontend must not get direct database credentials.

## Database Connection Gotchas

The first direct Supabase connection string looked like this shape:

```text
postgresql://postgres:<password>@db.wqqgmzuqdubkcbufqrgn.supabase.co:5432/postgres
```

That failed on the DigitalOcean droplet because Supabase direct database hosts resolve to IPv6 unless the Supabase IPv4 add-on is enabled. The droplet had no IPv6 route.

Per Supabase's current connection guidance, IPv4-only servers should use the Shared Pooler session-mode connection string. The working production shape is:

```text
postgres://postgres.wqqgmzuqdubkcbufqrgn:<password>@aws-1-ca-central-1.pooler.supabase.com:5432/postgres
```

Notes:

- The actual password is only in `/etc/sovereign-vpn/gateway.env`.
- The file is `0600` and root-owned.
- Do not put this value in Vercel, `VITE_*`, frontend env vars, or browser-visible config.
- `aws-0-ca-central-1.pooler.supabase.com` was reachable but returned `tenant/user postgres.wqqgmzuqdubkcbufqrgn not found`.
- `aws-1-ca-central-1.pooler.supabase.com:5432` worked.

## Gateway Changes

The gateway now supports real operator enrollment tokens.

Key files:

- `gateway/pkg/server/operator_enrollment.go`
- `gateway/pkg/server/operator_enrollment_postgres.go`
- `gateway/pkg/server/server.go`
- `gateway/cmd/gateway/main.go`
- `gateway/pkg/server/server_test.go`

Behavior added:

- `POST /operator/enrollments`
  - Requires `operator`, `region`, `message`, and `signature`.
  - Verifies SIWE signature.
  - Requires signed wallet address to match the requested operator address.
  - Creates a real enrollment token.

- `GET /operator/enrollments/:token`
  - Returns enrollment status and installer report state.

- `POST /operator/enrollments/:token/report`
  - Called by node installer after setup.
  - Records operator, region, endpoint, gateway URL, public IP, ports, WireGuard public key, health status, installer version, and report timestamp.

Storage behavior:

- In-memory enrollment store remains the local development fallback.
- Production uses Postgres when `ENROLLMENT_DATABASE_URL` or `--enrollment-db-url` is set.
- Production log line confirming durable storage:

```text
Operator enrollment storage: postgres
```

## Dashboard Changes

Key files:

- `site-app/src/OperatorEnrollment.jsx`
- `site-app/src/index.css`
- `site-app/vite.config.js`
- `site-app/.env.example`

The dashboard now has a "Run a Node" flow that:

- Connects operator wallet.
- Calls `/auth/challenge`.
- Signs the SIWE message.
- Calls `POST /operator/enrollments`.
- Polls the enrollment token status.
- Generates an installer command with the enrollment token and operator parameters.

Relevant config:

- `VITE_CONTROL_PLANE_URL` can point the frontend at a control-plane API.
- In production on `6529vpn.io`, same-origin routing works through Caddy.

## Node Installer Changes

Key files:

- `node/install.sh`
- `node/README.md`

Installer behavior added:

- Accepts an enrollment token via `--enroll`.
- Accepts a control-plane URL via `--control-plane-url`.
- Defaults control-plane URL to `https://6529vpn.io`.
- Reports installer status back to:

```text
<CONTROL_PLANE_URL>/operator/enrollments/<ENROLLMENT_TOKEN>/report
```

The node VM does not need Supabase credentials. It only needs the enrollment token and the control-plane URL.

## Caddy / Routing

Local repo Caddyfile now includes:

```caddy
handle /operator* {
    reverse_proxy localhost:8080
}
```

Live droplet Caddyfile was patched in place to add the same route, but it was not replaced wholesale.

This matters because the live droplet Caddyfile has production-specific details that differ from the repo copy, including:

- Static root: `/var/www/6529vpn`
- ZK API route:

```caddy
handle /api/* {
    reverse_proxy 127.0.0.1:3002
}
```

Future deploys should patch the live Caddyfile deliberately, not blindly overwrite it from the repo.

## Production Deploy Performed

Built locally:

```bash
cd gateway
GOTOOLCHAIN=local mise x go@1.24.13 -- go test ./...
GOTOOLCHAIN=local mise x go@1.24.13 -- sh -c 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/sovereign-gateway-linux-amd64 ./cmd/gateway'

cd ../site-app
npm run build
```

Results:

- Gateway tests passed.
- Linux gateway binary built.
- Vite frontend build passed.
- Vite emitted existing wallet-library PURE/chunk-size warnings, but produced a valid bundle.

Copied to droplet:

- `/tmp/sovereign-gateway.new`
- `/tmp/6529vpn-dist.tgz`

Backups made on droplet:

- Old gateway binary
- Old gateway systemd unit
- Old Caddyfile
- Old frontend web root

Backup directory:

```text
/root/sovereign-vpn-deploy-backups/20260619050926
```

Frontend deployment preserved existing top-level public assets from `/var/www/6529vpn`, because the Vite build output only included `index.html` and bundled assets.

## Verification Performed

Production health:

```bash
curl -fsS https://6529vpn.io/health
```

Observed:

```json
{"active_peers":0,"active_sessions":0,"free_tier_enabled":false,"status":"ok"}
```

Operator route:

```bash
curl -i -X POST https://6529vpn.io/operator/enrollments \
  -H 'Content-Type: application/json' \
  -d '{}'
```

Observed:

```json
{"error":"operator must be a valid Ethereum address"}
```

That is the expected gateway JSON validation error. It confirms `/operator/enrollments` is routed to Go gateway, not falling through to the React SPA.

Durable storage verification:

1. Generated a throwaway wallet locally.
2. Requested a live SIWE challenge from `https://6529vpn.io/auth/challenge`.
3. Signed the message.
4. Called `POST https://6529vpn.io/operator/enrollments`.
5. Got `201 Created`.
6. Confirmed the row existed in Supabase.
7. Deleted the test row.

Deleted test token:

```text
0fcd794f8d6b7b0855a10218d8196222
```

The test row was removed from `public.operator_enrollments`.

## Temporary Access Cleanup

A temporary SSH key was generated for this deployment and added to root's `authorized_keys`.

It was removed after verification.

Confirmed:

```text
key_remaining=no
gateway_status=active
local_health=ok
```

## Operational Commands

Check gateway status:

```bash
systemctl status sovereign-gateway --no-pager
```

Restart gateway:

```bash
systemctl restart sovereign-gateway
```

Watch gateway logs without exposing secrets:

```bash
journalctl -u sovereign-gateway --no-pager --since '10 minutes ago'
```

Check whether gateway is using Postgres:

```bash
journalctl -u sovereign-gateway --no-pager --since '10 minutes ago' | grep 'Operator enrollment storage'
```

Expected:

```text
Operator enrollment storage: postgres
```

Check local gateway health from droplet:

```bash
curl -fsS http://127.0.0.1:8080/health
```

Check public gateway health:

```bash
curl -fsS https://6529vpn.io/health
```

Check configured pooler host without printing the password:

```bash
sed -n 's#^ENROLLMENT_DATABASE_URL=.*@\([^:/]*\):\([0-9]*\)/.*#host=\1 port=\2#p' /etc/sovereign-vpn/gateway.env
```

Expected:

```text
host=aws-1-ca-central-1.pooler.supabase.com port=5432
```

## Current Mental Model

There is one control-plane API/gateway for the product right now:

- `6529vpn.io`
- Go gateway on the DigitalOcean droplet
- Supabase stores enrollment state

There can be many VPN node VMs later:

- Each node runs the installer.
- Each node gets a real enrollment token from the operator dashboard.
- Each node reports installation/health metadata back to the control plane.
- Nodes do not get database passwords.

In short:

- The control plane has Supabase credentials.
- Operator/node VMs have enrollment tokens.
- Users and operators interact with the dashboard/API.

## Next Good Steps

1. Commit the local repo changes for the enrollment API, Postgres store, installer reporting, migration, and Caddy route.
2. Add an authenticated operator-facing list of created enrollments.
3. Add rate limiting around enrollment creation and installer reports.
4. Add a cleanup job or scheduled query for expired enrollment rows.
5. Consider a separate `api.6529vpn.io` later, but same-origin `6529vpn.io` is fine for the current architecture.
6. Document the production deploy process as a script so future updates are less hand-rolled.

