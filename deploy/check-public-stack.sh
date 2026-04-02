#!/usr/bin/env bash
set -euo pipefail

PUBLIC_SITE_URL="${PUBLIC_SITE_URL:-https://6529vpn.io}"

echo "Frontend asset:"
curl -fsS "$PUBLIC_SITE_URL" | grep -oP 'src="\K/assets/[^"]+\.js' | head -n1

echo
echo "Health:"
curl -fsS "$PUBLIC_SITE_URL/health"

echo
echo
echo "Session info:"
curl -fsS "$PUBLIC_SITE_URL/session/info"

echo
echo
echo "Subscription tiers:"
curl -fsS "$PUBLIC_SITE_URL/subscription/tiers"
