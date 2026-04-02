#!/usr/bin/env bash
set -euo pipefail

DROPLET_HOST="${DROPLET_HOST:-root@142.93.159.175}"
LOOKBACK="${LOOKBACK:-1 hour ago}"

ssh "$DROPLET_HOST" "bash -s" <<EOF
set -euo pipefail

echo "== Caddyfile log directives =="
if grep -nE '^[[:space:]]*log[[:space:]]*(\{|$)' /etc/caddy/Caddyfile; then
  echo
  echo "WARN: Caddy log directive found"
else
  echo "OK: no explicit Caddy log directive found"
fi

echo
echo "== journald retention config =="
grep -nE '^(Storage|SystemMaxUse|RuntimeMaxUse|MaxRetentionSec)=' /etc/systemd/journald.conf /etc/systemd/journald.conf.d/* 2>/dev/null || echo "No explicit journald retention override found"

echo
echo "== recent sovereign-gateway logs =="
journalctl -u sovereign-gateway --since "$LOOKBACK" --no-pager || true

echo
echo "== sensitive pattern scan on recent gateway logs =="
GATEWAY_LOGS=\$(journalctl -u sovereign-gateway --since "$LOOKBACK" --no-pager || true)
EVENT_LOGS=\$(printf '%s\n' "\$GATEWAY_LOGS" | grep -E 'Session created|Access granted|VPN connected|Peer added|Peer removed|Peer recovered|dead gateway|gateway affinity|peer recovery state' || true)

check_pattern() {
  local label="\$1"
  local pattern="\$2"
  if printf '%s\n' "\$EVENT_LOGS" | grep -Eq "\$pattern"; then
    echo "LEAK: \$label"
  else
    echo "OK: \$label not found"
  fi
}

check_pattern "wallet addresses in user-event logs" '0x[a-fA-F0-9]{40}'
check_pattern "client tunnel IPs in user-event logs" '10\\.[0-9]+\\.[0-9]+\\.[0-9]+'
check_pattern "wireguard public keys in user-event logs" '[A-Za-z0-9+/]{42,}={0,2}'
check_pattern "authorization headers" 'Authorization:|authorization:'
check_pattern "session tokens" 'tok_[A-Za-z0-9_-]+'
EOF
