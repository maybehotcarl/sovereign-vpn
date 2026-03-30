# Sovereign VPN Multi-Node Rehearsal

This stack is the shortest path to a production-like local rehearsal for:

- real `zk-api` issuer env
- Redis-backed shared gateway state
- two gateway instances
- real WireGuard interfaces inside the gateway containers
- failure drills for owner-node restart, owner-node death, wrong-node forwarding, and Redis loss

## 1. Host prerequisites

Run:

```bash
./deploy/rehearsal/check-prereqs.sh
```

Hard requirements:

- local access to the Docker daemon
- `/dev/net/tun`
- Node/NPM for the host-run `zk-api`

## 2. Validate the issuer env

Run from the zk-api repo:

```bash
cd /home/maybe/repos/sovereign-vpn/site-app/6529-zk-api
npm run check:issuer-env -- --env-file .env.local
```

The script validates the required production issuer inputs without printing secrets.

## 3. Start the real issuer

Keep `zk-api` on the host so it can use your existing `.env.local` directly:

```bash
cd /home/maybe/repos/sovereign-vpn/site-app/6529-zk-api
npm run dev
```

Health checks:

```bash
curl -s http://127.0.0.1:3002/api/health | jq
curl -s http://127.0.0.1:3002/api/meta | jq '.anonymousVpn'
```

If `anonymousVpn.activeRoot` is `null`, there is no live anonymous entitlement published right now.
Run a real anonymous issuer activation from a subscribed wallet before expecting scripted
`vpn_access_v1` proof generation to work.

## 4. Start Redis + two gateways

Create the rehearsal env file:

```bash
cp /home/maybe/repos/sovereign-vpn/deploy/rehearsal/.env.example \
  /home/maybe/repos/sovereign-vpn/deploy/rehearsal/.env
```

Set at least:

- `SESSION_SIGNING_KEY`
- `GATEWAY_FORWARDING_KEY`
- `ETH_RPC_URL`
- `SUBSCRIPTION_MANAGER` if you want legacy subscription-aware gateway paths available

Then boot the stack:

```bash
docker compose \
  --env-file /home/maybe/repos/sovereign-vpn/deploy/rehearsal/.env \
  -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml \
  up --build -d
```

Gateway health checks:

```bash
curl -s http://127.0.0.1:8081/health | jq
curl -s http://127.0.0.1:8082/health | jq
```

Expected health fields:

- `gateway_instance_id`
- `gateway_public_url`
- `shared_state.enabled`
- `shared_state.status`
- `forwarding.enabled`
- `forwarding.forward_target_configured`

## 5. WireGuard sanity check

After the gateways are up, inspect the interfaces inside the containers:

```bash
docker compose -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml exec gateway-a wg show
docker compose -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml exec gateway-b wg show
```

Both containers should have a live `wg0` interface and a generated server public key.

## 6. Failure drills

### Wrong-node forwarding

1. Connect a browser session through `http://127.0.0.1:8081`.
2. Call `GET /vpn/status` or `POST /vpn/disconnect` against `http://127.0.0.1:8082`.
3. Expect success, not `409`, because gateway B should forward to the owner using `GATEWAY_FORWARD_URL`.

### Owner restart recovery

1. Connect through gateway A.
2. Restart only gateway A:

```bash
docker compose -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml restart gateway-a
```

3. Retry `GET /vpn/status` against either gateway.
4. Expect the owner session to recover because peer state is persisted and replayed on startup.

### Owner death and takeover

1. Connect through gateway A.
2. Kill gateway A:

```bash
docker compose -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml kill gateway-a
```

3. Query `GET /vpn/status` through gateway B.
4. Expect `code: "gateway_owner_unavailable"` until the client reconnects with a fresh WireGuard key.
5. Reconnect through gateway B and confirm the session rebinds there.

### Redis outage

1. Stop Redis:

```bash
docker compose -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml stop redis
```

2. Check both gateway health endpoints.
3. Expect `shared_state.status` to flip away from `ok`.
4. Restore Redis and confirm health returns to normal.

## 7. Shutdown

```bash
docker compose \
  --env-file /home/maybe/repos/sovereign-vpn/deploy/rehearsal/.env \
  -f /home/maybe/repos/sovereign-vpn/deploy/rehearsal/docker-compose.yml \
  down
```
