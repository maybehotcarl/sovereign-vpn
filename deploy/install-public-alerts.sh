#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

DROPLET_HOST="${DROPLET_HOST:-root@142.93.159.175}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/sovereign-vpn}"
REMOTE_DEPLOY_DIR="${REMOTE_DEPLOY_DIR:-$REMOTE_ROOT/deploy}"
REMOTE_ENV_FILE="${REMOTE_ENV_FILE:-$REMOTE_DEPLOY_DIR/public-alerts.env}"

rsync -az \
  "$ROOT_DIR/deploy/public_stack_alerts.py" \
  "$ROOT_DIR/deploy/public-alerts.env.example" \
  "$ROOT_DIR/deploy/sovereign-public-alerts.service" \
  "$ROOT_DIR/deploy/sovereign-public-alerts.timer" \
  "$DROPLET_HOST:$REMOTE_DEPLOY_DIR/"

ssh "$DROPLET_HOST" "\
  set -euo pipefail && \
  mkdir -p '$REMOTE_DEPLOY_DIR' /var/lib/sovereign-vpn && \
  chmod 755 '$REMOTE_DEPLOY_DIR/public_stack_alerts.py' && \
  if [ ! -f '$REMOTE_ENV_FILE' ]; then \
    cp '$REMOTE_DEPLOY_DIR/public-alerts.env.example' '$REMOTE_ENV_FILE'; \
    chmod 600 '$REMOTE_ENV_FILE'; \
  fi && \
  install -m 644 '$REMOTE_DEPLOY_DIR/sovereign-public-alerts.service' /etc/systemd/system/sovereign-public-alerts.service && \
  install -m 644 '$REMOTE_DEPLOY_DIR/sovereign-public-alerts.timer' /etc/systemd/system/sovereign-public-alerts.timer && \
  systemctl daemon-reload && \
  systemctl enable --now sovereign-public-alerts.timer && \
  systemctl start sovereign-public-alerts.service || true && \
  systemctl status sovereign-public-alerts.timer --no-pager && \
  echo && \
  echo 'Configured alert env:' && \
  if grep -q '^ALERT_WEBHOOK_URL=.\\+' '$REMOTE_ENV_FILE'; then echo 'present'; else echo 'missing'; fi \
"

echo
echo "To verify from this workstation:"
echo "  PUBLIC_ALERT_REMOTE_HOST=$DROPLET_HOST python3 deploy/public_stack_alerts.py --dry-run"
