#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_ZK_API_DIR="$ROOT_DIR/site-app/6529-zk-api"
LOCAL_ZK_API_ENV_FILE="${LOCAL_ZK_API_ENV_FILE:-$LOCAL_ZK_API_DIR/.env.local}"

DROPLET_HOST="${DROPLET_HOST:-root@142.93.159.175}"
REMOTE_ROOT="${REMOTE_ROOT:-/root/sovereign-vpn}"
REMOTE_ZK_API_DIR="${REMOTE_ZK_API_DIR:-$REMOTE_ROOT/site-app/6529-zk-api}"
REMOTE_SERVICE_NAME="${REMOTE_SERVICE_NAME:-sovereign-zk-api}"
PUBLIC_SITE_URL="${PUBLIC_SITE_URL:-https://6529vpn.io}"
PUBLIC_ZK_API_URL="${PUBLIC_ZK_API_URL:-$PUBLIC_SITE_URL}"
PUBLIC_ZK_ARTIFACT_BASE_URL="${PUBLIC_ZK_ARTIFACT_BASE_URL:-$PUBLIC_SITE_URL/api/artifacts}"
GATEWAY_ZK_API_URL="${GATEWAY_ZK_API_URL:-http://127.0.0.1:3002}"
SYNC_LOCAL_ZK_API_NODE_MODULES="${SYNC_LOCAL_ZK_API_NODE_MODULES:-1}"

if [[ ! -f "$LOCAL_ZK_API_ENV_FILE" ]]; then
  echo "Local zk-api env file not found: $LOCAL_ZK_API_ENV_FILE" >&2
  exit 1
fi

cd "$LOCAL_ZK_API_DIR"
npm run check:issuer-env -- --env-file "$LOCAL_ZK_API_ENV_FILE"

TMP_DIR="$(mktemp -d)"
TMP_ENV_FILE="$TMP_DIR/.env.production.local"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cp "$LOCAL_ZK_API_ENV_FILE" "$TMP_ENV_FILE"

upsert_env() {
  local file="$1"
  local key="$2"
  local value="$3"
  python3 - "$file" "$key" "$value" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
key = sys.argv[2]
value = sys.argv[3]
line = f"{key}={value}"
raw = path.read_text() if path.exists() else ""
lines = raw.splitlines()
for idx, existing in enumerate(lines):
    if existing.startswith(f"{key}="):
        lines[idx] = line
        break
else:
    lines.append(line)
path.write_text("\n".join(lines) + "\n")
PY
}

upsert_env "$TMP_ENV_FILE" "NODE_ENV" "production"
upsert_env "$TMP_ENV_FILE" "OBS_ENV" "production"
upsert_env "$TMP_ENV_FILE" "PUBLIC_BASE_URL" "$PUBLIC_SITE_URL"
upsert_env "$TMP_ENV_FILE" "CORS_ALLOWED_ORIGINS" "$PUBLIC_SITE_URL"
upsert_env "$TMP_ENV_FILE" "ZK_VPN_ACCESS_DEV_ALLOW_INSECURE" "false"
upsert_env "$TMP_ENV_FILE" "ZK_VPN_ACCESS_ENABLE_DEV_REGISTRATION" "false"

ssh "$DROPLET_HOST" "\
  set -euo pipefail && \
  mkdir -p '$REMOTE_ZK_API_DIR' && \
  rm -rf '$REMOTE_ZK_API_DIR' && \
  mkdir -p '$REMOTE_ZK_API_DIR' \
"

rsync -az --delete \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='.next' \
  --exclude='.env.local' \
  --exclude='.env.production.local' \
  "$LOCAL_ZK_API_DIR/" "$DROPLET_HOST:$REMOTE_ZK_API_DIR/"
if [[ "$SYNC_LOCAL_ZK_API_NODE_MODULES" == "1" ]]; then
  rsync -az --delete "$LOCAL_ZK_API_DIR/node_modules/" "$DROPLET_HOST:$REMOTE_ZK_API_DIR/node_modules/"
fi
rsync -az "$ROOT_DIR/deploy/sovereign-zk-api.service" "$DROPLET_HOST:/etc/systemd/system/$REMOTE_SERVICE_NAME.service"
rsync -az "$ROOT_DIR/Caddyfile" "$DROPLET_HOST:/etc/caddy/Caddyfile"

rsync -az "$TMP_ENV_FILE" "$DROPLET_HOST:$REMOTE_ZK_API_DIR/.env.production.local"

ssh "$DROPLET_HOST" "\
  set -euo pipefail && \
  cd '$REMOTE_ZK_API_DIR' && \
  if [ '$SYNC_LOCAL_ZK_API_NODE_MODULES' != '1' ]; then npm ci; fi && \
  npm run build && \
  python3 - <<'PY' && \
from pathlib import Path
import re

path = Path('/etc/systemd/system/sovereign-gateway.service')
text = path.read_text()
line = '  --zk-api-url $GATEWAY_ZK_API_URL \\\\'
pattern = re.compile(r'^  --zk-api-url \\S+ \\\\$' , re.MULTILINE)
if pattern.search(text):
    updated = pattern.sub(line, text)
else:
    anchor = '  --cors-origin https://6529vpn.io \\\\'
    if anchor not in text:
        raise SystemExit('could not find gateway service insertion point for --zk-api-url')
    updated = text.replace(anchor, anchor + '\\n' + line, 1)
if updated != text:
    path.write_text(updated)
PY
  systemctl daemon-reload && \
  systemctl enable --now '$REMOTE_SERVICE_NAME' && \
  systemctl restart '$REMOTE_SERVICE_NAME' && \
  caddy validate --config /etc/caddy/Caddyfile && \
  systemctl reload caddy && \
  systemctl restart sovereign-gateway && \
  systemctl is-active '$REMOTE_SERVICE_NAME' >/dev/null && \
  systemctl is-active sovereign-gateway >/dev/null && \
  curl -fsS '$GATEWAY_ZK_API_URL/api/health' >/dev/null && \
  curl -fsS http://127.0.0.1:8080/health >/dev/null \
"

VITE_ENABLE_ANON_VPN=true \
VITE_ENABLE_ANON_VPN_DEV_REGISTRATION=false \
VITE_ZK_API_URL="$PUBLIC_ZK_API_URL" \
VITE_ZK_ARTIFACT_BASE_URL="$PUBLIC_ZK_ARTIFACT_BASE_URL" \
INSTALL_DEPS=0 \
"$ROOT_DIR/deploy/publish-public-frontend.sh"

curl -fsS "$PUBLIC_SITE_URL/api/health" >/dev/null
curl -fsS "$PUBLIC_SITE_URL/api/meta" >/dev/null
curl -fsS "$PUBLIC_SITE_URL/health" >/dev/null

echo "Public anonymous deploy complete."
