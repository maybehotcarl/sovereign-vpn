# Public Operations Runbook

Snapshot date: April 2, 2026

This runbook is for the current public `6529vpn.io` stack:

- static frontend at `/var/www/6529vpn`
- gateway service `sovereign-gateway.service`
- public ZK API service `sovereign-zk-api.service`
- WireGuard interface `wg0`

## Basic Health

From your workstation:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/check-public-stack.sh
```

From the droplet:

```bash
curl -sS http://127.0.0.1:8080/health
curl -sS http://127.0.0.1:3002/api/health
systemctl is-active sovereign-gateway
systemctl is-active sovereign-zk-api
sudo wg show wg0
```

## Privacy Audit

Run:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/check-live-privacy.sh
```

Current expectations:

- no Caddy access-log directive
- journald retention override present with `MaxRetentionSec=1h`
- no wallet addresses in recent user-event logs
- no client tunnel IPs in recent user-event logs
- no full WireGuard public keys in recent user-event logs
- no session tokens or `Authorization` headers in recent logs

## Gateway Restart

```bash
ssh root@142.93.159.175 \
  "systemctl restart sovereign-gateway && systemctl status sovereign-gateway --no-pager"
```

## zk-api Restart

```bash
ssh root@142.93.159.175 \
  "systemctl restart sovereign-zk-api && systemctl status sovereign-zk-api --no-pager"
```

## Frontend Publish

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/publish-public-frontend.sh
```

## Public Anonymous Deploy

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/deploy-public-anon.sh
```

## Verify A Live User Tunnel

From the user device:

```bash
sudo wg
curl -s https://ifconfig.co/json
```

From the droplet:

```bash
ssh root@142.93.159.175 "sudo wg show wg0"
```

Success means:

- the client shows a recent handshake
- the server shows the same peer with recent handshake and traffic

## Roll Back The Gateway Binary

List backups:

```bash
ssh root@142.93.159.175 "ls -1t /usr/local/bin/sovereign-gateway.backup-* | head"
```

Restore one:

```bash
ssh root@142.93.159.175 "\
  install -m 755 /usr/local/bin/sovereign-gateway.backup-<timestamp> /usr/local/bin/sovereign-gateway && \
  systemctl restart sovereign-gateway && \
  systemctl status sovereign-gateway --no-pager"
```

## Current Manual Alert Checks

Actual external alert routing is still not wired. Until that exists, the manual checks to run are:

- `./deploy/check-public-stack.sh`
- `./deploy/check-live-privacy.sh`
- `ssh root@142.93.159.175 "systemctl is-active sovereign-gateway"`
- `ssh root@142.93.159.175 "systemctl is-active sovereign-zk-api"`
- `ssh root@142.93.159.175 "sudo wg show wg0"`

## Open Gap

The current stack now has:

- live privacy audit commands
- reduced gateway log sensitivity
- a `1h` journald retention override

It does not yet have:

- hosted alert routing
- paging/integration with a real alert destination
