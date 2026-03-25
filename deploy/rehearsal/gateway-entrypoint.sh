#!/bin/sh

set -eu

WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_STATE_DIR="${WG_STATE_DIR:-/var/lib/sovereign-vpn/wireguard}"
WG_LISTEN_PORT="${WG_LISTEN_PORT:-51820}"
WG_ENDPOINT_HOST="${WG_ENDPOINT_HOST:-127.0.0.1}"
WG_ENDPOINT_PORT="${WG_ENDPOINT_PORT:-$WG_LISTEN_PORT}"
WG_SERVER_IP="${WG_SERVER_IP:-10.80.1.1/24}"
WG_SUBNET="${WG_SUBNET:-10.80.1.0/24}"
WG_DNS="${WG_DNS:-1.1.1.1}"
WG_MTU="${WG_MTU:-1420}"
UPLINK_INTERFACE="${UPLINK_INTERFACE:-}"

if [ -z "${ETH_RPC_URL:-}" ]; then
  echo "ETH_RPC_URL is required" >&2
  exit 1
fi

if [ -z "${MEMES_CONTRACT:-}" ]; then
  echo "MEMES_CONTRACT is required" >&2
  exit 1
fi

if [ -z "${ZK_API_URL:-}" ]; then
  echo "ZK_API_URL is required" >&2
  exit 1
fi

mkdir -p "$WG_STATE_DIR"
PRIVATE_KEY_FILE="$WG_STATE_DIR/privatekey"
PUBLIC_KEY_FILE="$WG_STATE_DIR/publickey"

if [ ! -s "$PRIVATE_KEY_FILE" ]; then
  wg genkey | tee "$PRIVATE_KEY_FILE" | wg pubkey > "$PUBLIC_KEY_FILE"
  chmod 600 "$PRIVATE_KEY_FILE" "$PUBLIC_KEY_FILE"
fi

if [ -z "$UPLINK_INTERFACE" ]; then
  UPLINK_INTERFACE=$(ip route | awk '/default/ { print $5; exit }')
fi

SERVER_PUBLIC_KEY=$(cat "$PUBLIC_KEY_FILE")
WG_ENDPOINT="${WG_ENDPOINT_HOST}:${WG_ENDPOINT_PORT}"

if ip link show "$WG_INTERFACE" >/dev/null 2>&1; then
  ip link delete "$WG_INTERFACE"
fi

ip link add dev "$WG_INTERFACE" type wireguard
ip address add "$WG_SERVER_IP" dev "$WG_INTERFACE"
wg set "$WG_INTERFACE" listen-port "$WG_LISTEN_PORT" private-key "$PRIVATE_KEY_FILE"
ip link set mtu "$WG_MTU" up dev "$WG_INTERFACE"

# Docker Compose already requests the needed sysctls for this container. On
# some hosts `/proc/sys` is mounted read-only inside the container, so an
# in-container `sysctl -w` would fail even though forwarding is already enabled
# by the runtime. Treat that as non-fatal for the rehearsal stack.
if [ "$(cat /proc/sys/net/ipv4/ip_forward 2>/dev/null || echo 0)" != "1" ]; then
  if ! sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1; then
    echo "warning: unable to set net.ipv4.ip_forward inside container; relying on container runtime sysctls" >&2
  fi
fi

if [ -n "$UPLINK_INTERFACE" ]; then
  iptables -C FORWARD -i "$WG_INTERFACE" -j ACCEPT 2>/dev/null || iptables -A FORWARD -i "$WG_INTERFACE" -j ACCEPT
  iptables -C FORWARD -o "$WG_INTERFACE" -j ACCEPT 2>/dev/null || iptables -A FORWARD -o "$WG_INTERFACE" -j ACCEPT
  iptables -t nat -C POSTROUTING -o "$UPLINK_INTERFACE" -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -o "$UPLINK_INTERFACE" -j MASQUERADE
fi

set -- /usr/local/bin/gateway \
  --direct-mode \
  --listen :8080 \
  --eth-rpc "$ETH_RPC_URL" \
  --memes-contract "$MEMES_CONTRACT" \
  --wg-interface "$WG_INTERFACE" \
  --wg-pubkey "$SERVER_PUBLIC_KEY" \
  --wg-endpoint "$WG_ENDPOINT" \
  --wg-subnet "$WG_SUBNET" \
  --wg-dns "$WG_DNS" \
  --zk-api-url "$ZK_API_URL"

if [ -n "${ZK_API_KEY:-}" ]; then
  set -- "$@" --zk-api-key "$ZK_API_KEY"
fi

if [ -n "${CORS_ORIGIN:-}" ]; then
  set -- "$@" --cors-origin "$CORS_ORIGIN"
fi

if [ -n "${SUBSCRIPTION_MANAGER:-}" ]; then
  set -- "$@" --subscription-manager "$SUBSCRIPTION_MANAGER"
fi

if [ -n "${REDIS_URL:-}" ]; then
  set -- "$@" --redis-url "$REDIS_URL"
fi

if [ -n "${REDIS_PREFIX:-}" ]; then
  set -- "$@" --redis-prefix "$REDIS_PREFIX"
fi

if [ -n "${SESSION_SIGNING_KEY:-}" ]; then
  set -- "$@" --session-signing-key "$SESSION_SIGNING_KEY"
fi

if [ -n "${GATEWAY_INSTANCE_ID:-}" ]; then
  set -- "$@" --gateway-instance-id "$GATEWAY_INSTANCE_ID"
fi

if [ -n "${GATEWAY_PUBLIC_URL:-}" ]; then
  set -- "$@" --gateway-public-url "$GATEWAY_PUBLIC_URL"
fi

if [ -n "${GATEWAY_FORWARD_URL:-}" ]; then
  set -- "$@" --gateway-forward-url "$GATEWAY_FORWARD_URL"
fi

if [ -n "${GATEWAY_FORWARDING_KEY:-}" ]; then
  set -- "$@" --gateway-forwarding-key "$GATEWAY_FORWARDING_KEY"
fi

if [ "${ENABLE_FREE_TIER:-false}" = "true" ]; then
  set -- "$@" --enable-free-tier
fi

exec "$@"
