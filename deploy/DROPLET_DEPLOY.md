# Public Droplet Deploy

This runbook updates the public `6529vpn.io` droplet that serves:

- static frontend assets from `site-app/dist`
- gateway API routes through Caddy to `localhost:8080`

It assumes the current top-level [Caddyfile](/home/maybe/repos/sovereign-vpn/Caddyfile) pattern:

- `/var/www/6529vpn` is the static web root
- `/auth`, `/vpn`, `/session`, `/subscription`, `/nodes`, `/health`, and `/payout` proxy to the gateway

## Scope

Use this when you want the public site to reflect the current repo state and the current gateway behavior.

The current droplet layout is:

- repo checkout: `/root/sovereign-vpn`
- static web root: `/var/www/6529vpn`
- public gateway service: `sovereign-gateway.service`

`site-app` now consumes `@6529/zk-service` as a normal package dependency. You do not need a sibling `site-app/6529-zk-service` checkout on the droplet just to build the frontend.

For the anonymous path, the frontend also needs a public `zk-api` URL and the public gateway must be started with `--zk-api-url`. The checked-in helper [deploy-public-anon.sh](/home/maybe/repos/sovereign-vpn/deploy/deploy-public-anon.sh) now handles that same-origin deployment path.

## Preflight

1. Make sure the code you want to deploy exists on a remote branch for:
   - `sovereign-vpn`
   - `site-app/6529-zk-api` if the public `zk-api` is also deployed from this machine
2. Confirm the public ZK API URL you want the browser to use.
3. Confirm CORS on that `zk-api` allows `https://6529vpn.io`.

## Production Frontend Env

Create `site-app/.env.production.local` on the droplet from [site-app/.env.production.example](/home/maybe/repos/sovereign-vpn/site-app/.env.production.example).

At minimum, set:

```env
VITE_CHAIN=mainnet
VITE_SESSION_MANAGER=0xb644c990c884911670adc422719243D9F76Df0d6
VITE_SUBSCRIPTION_MANAGER=0xEb54c8604b7EEADE804d121BD8f158A006827882
VITE_NODE_REGISTRY=0x1Fd64c16c745e373428068eB52AA73525576B594
VITE_ENABLE_ANON_VPN=false
VITE_ENABLE_ANON_VPN_DEV_REGISTRATION=false
```

Enable anonymous mode only after the public `zk-api` exists and the live gateway is configured to talk to it. The current public deployment uses:

- browser-facing `zk-api`: `https://6529vpn.io/api/*`
- upstream `zk-api`: `127.0.0.1:3002`
- gateway `--zk-api-url`: `http://127.0.0.1:3002`

Do not copy local dev values like `127.0.0.1:3002` or `127.0.0.1:8081` into the public build.

## Fast Path: Publish The Frontend From Your Workstation

The checked-in script [publish-public-frontend.sh](/home/maybe/repos/sovereign-vpn/deploy/publish-public-frontend.sh) builds `site-app` with production env, syncs `dist/` to the droplet, and prints the live asset hash.

For the current direct-wallet public beta:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/publish-public-frontend.sh
```

If you need anonymous mode in the public build, export the public ZK API URL first:

```bash
export VITE_ENABLE_ANON_VPN=true
export VITE_ZK_API_URL=https://<public-zk-api-domain>
export VITE_ZK_ARTIFACT_BASE_URL=https://<public-zk-api-domain>/api/artifacts
./deploy/publish-public-frontend.sh
```

To update the full public anonymous stack in one pass:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/deploy-public-anon.sh
```

For a quick public verification after deploy:

```bash
./deploy/check-public-stack.sh
```

For ongoing operations on the public stack, see
[PUBLIC_OPERATIONS.md](/home/maybe/repos/sovereign-vpn/deploy/PUBLIC_OPERATIONS.md).

## Frontend + Gateway Update On The Droplet

On the droplet:

```bash
cd /root/sovereign-vpn

export ROOT_BRANCH=checkpoint/launch-hardening-2026-03-25

git fetch origin
git switch "$ROOT_BRANCH"
git pull --ff-only origin "$ROOT_BRANCH"
```

Build the public frontend:

```bash
cd /root/sovereign-vpn/site-app
npm ci
npm run build
```

Publish the built frontend:

```bash
rm -rf /var/www/6529vpn/*
cp -R dist/* /var/www/6529vpn/
```

If the gateway binary or config changed:

```bash
systemctl restart sovereign-gateway
systemctl status sovereign-gateway --no-pager
```

If the Caddy config changed:

```bash
caddy validate --config /etc/caddy/Caddyfile
systemctl reload caddy
```

If only `site-app/dist` changed, Caddy does not need a reload.

## Optional: Update Public zk-api On The Same Droplet

Only do this if the public `zk-api` is actually hosted on the droplet.

If the sibling `6529-zk-api` repo is missing:

```bash
git clone https://github.com/maybehotcarl/6529-zk-api.git \
  /root/sovereign-vpn/site-app/6529-zk-api
```

Then:

```bash
cd /root/sovereign-vpn/site-app/6529-zk-api

export ZK_API_BRANCH=checkpoint/launch-hardening-2026-03-25

git fetch origin
git switch "$ZK_API_BRANCH"
git pull --ff-only origin "$ZK_API_BRANCH"
npm ci
npm run build
```

Restart it using whatever is already supervising the process. Common patterns:

```bash
systemctl list-units | rg 6529-zk-api
pm2 ls
ps -ef | rg "next start -p 3002"
```

Then restart the matching service/process.

## Verification

Frontend:

```bash
curl -I https://6529vpn.io
curl -s https://6529vpn.io/health | jq
```

Check that the static HTML changed:

```bash
curl -s https://6529vpn.io | rg "Sovereign VPN|og-card.svg|Anonymous Session|Direct Wallet Session"
```

Anonymous backend:

```bash
curl -s https://<public-zk-api-domain>/api/health | jq
curl -s https://<public-zk-api-domain>/api/meta | jq '.anonymousVpn'
```

## Rollback

If the new deploy is bad:

```bash
cd /root/sovereign-vpn
git log --oneline -n 5
git switch <last-known-good-branch-or-tag>
git pull --ff-only

cd /root/sovereign-vpn/site-app
npm ci
npm run build

rm -rf /var/www/6529vpn/*
cp -R dist/* /var/www/6529vpn/
```

If the public `zk-api` was updated on the same droplet, roll that branch/service back separately too.
