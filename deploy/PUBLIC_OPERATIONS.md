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

## Alerting

Install the public-stack alert runner on the droplet:

```bash
cd /home/maybe/repos/sovereign-vpn
./deploy/install-public-alerts.sh
```

The timer runs [public_stack_alerts.py](/home/maybe/repos/sovereign-vpn/deploy/public_stack_alerts.py) every minute through:

- [sovereign-public-alerts.service](/home/maybe/repos/sovereign-vpn/deploy/sovereign-public-alerts.service)
- [sovereign-public-alerts.timer](/home/maybe/repos/sovereign-vpn/deploy/sovereign-public-alerts.timer)

Alert env lives on the droplet at:

```bash
/root/sovereign-vpn/deploy/public-alerts.env
```

If `ALERT_WEBHOOK_URL` is already set in `/root/sovereign-vpn/site-app/6529-zk-api/.env.production.local`,
the runner will reuse it automatically. Otherwise set it in `public-alerts.env`.

Telegram is also supported directly by the alert runner. Set these in
`public-alerts.env` if you want alerts sent to a bot instead of, or in addition
to, a generic webhook:

```bash
ALERT_TELEGRAM_BOT_TOKEN=<bot-token>
ALERT_TELEGRAM_CHAT_ID=<chat-id>
ALERT_TELEGRAM_MESSAGE_THREAD_ID=<topic-id>   # optional
```

Verify alert runner state:

```bash
ssh root@142.93.159.175 "systemctl status sovereign-public-alerts.timer --no-pager"
ssh root@142.93.159.175 "journalctl -u sovereign-public-alerts.service -n 50 --no-pager"
PUBLIC_ALERT_REMOTE_HOST=root@142.93.159.175 python3 deploy/public_stack_alerts.py --dry-run
```

Current automated checks:

- public frontend reachability
- gateway `/health`
- `zk-api` `/api/health`
- `zk-api` `/api/meta`, including anonymous enablement and client API URL
- `/session/info`
- `/subscription/tiers`
- `sovereign-gateway.service`
- `sovereign-zk-api.service`
- `wg0`

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

## Manual Backup Checks

If the alert timer itself is suspected, the manual checks to run are still:

- `./deploy/check-public-stack.sh`
- `./deploy/check-live-privacy.sh`
- `ssh root@142.93.159.175 "systemctl is-active sovereign-gateway"`
- `ssh root@142.93.159.175 "systemctl is-active sovereign-zk-api"`
- `ssh root@142.93.159.175 "sudo wg show wg0"`

## Remaining Input

The alert runner is now deployable and stateful. The remaining operator input is
a real external delivery destination:

- `ALERT_WEBHOOK_URL` for a generic JSON webhook
- or `ALERT_TELEGRAM_BOT_TOKEN` plus `ALERT_TELEGRAM_CHAT_ID` for Telegram
