#!/bin/bash
# Sovereign VPN Node — Docker entrypoint
# Sets up WireGuard and launches the gateway binary.
set -euo pipefail

# ---- Required env ----
: "${ETH_RPC_URL:?ETH_RPC_URL is required}"
: "${MEMES_CONTRACT:?MEMES_CONTRACT is required}"
: "${PUBLIC_IP:?PUBLIC_IP is required}"

# ---- Defaults ----
WG_PORT="${WG_PORT:-51820}"
WG_SUBNET="${WG_SUBNET:-10.8.0.0/24}"
WG_SERVER_IP="${WG_SERVER_IP:-10.8.0.1/24}"
GATEWAY_PORT="${GATEWAY_PORT:-8080}"
SIWE_DOMAIN="${SIWE_DOMAIN:-6529vpn.io}"
CORS_ORIGIN="${CORS_ORIGIN:-https://6529vpn.io}"
CHAIN_ID="${CHAIN_ID:-1}"
THIS_CARD_ID="${THIS_CARD_ID:-0}"
MAX_TOKEN_ID="${MAX_TOKEN_ID:-350}"

WG_DIR="/etc/wireguard"
WG_INTERFACE="wg0"

# ---- WireGuard key management ----
if [ -n "${WG_PRIVATE_KEY:-}" ]; then
    echo "$WG_PRIVATE_KEY" > "$WG_DIR/privatekey"
    chmod 600 "$WG_DIR/privatekey"
    echo "$WG_PRIVATE_KEY" | wg pubkey > "$WG_DIR/publickey"
elif [ ! -f "$WG_DIR/privatekey" ]; then
    echo "Generating WireGuard keys..."
    wg genkey | tee "$WG_DIR/privatekey" | wg pubkey > "$WG_DIR/publickey"
    chmod 600 "$WG_DIR/privatekey"
fi

PRIV_KEY=$(cat "$WG_DIR/privatekey")
PUB_KEY=$(cat "$WG_DIR/publickey")

echo "=== Sovereign VPN Node ==="
echo "  Public IP:   $PUBLIC_IP"
echo "  WG PubKey:   $PUB_KEY"
echo "  WG Port:     $WG_PORT"
echo "  Gateway:     :$GATEWAY_PORT"

# ---- WireGuard config ----
cat > "$WG_DIR/$WG_INTERFACE.conf" <<EOF
[Interface]
Address = $WG_SERVER_IP
PrivateKey = $PRIV_KEY
ListenPort = $WG_PORT
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -A FORWARD -o %i -j ACCEPT; iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -D FORWARD -o %i -j ACCEPT; iptables -t nat -D POSTROUTING -o eth0 -j MASQUERADE
EOF

chmod 600 "$WG_DIR/$WG_INTERFACE.conf"

# ---- Bring up WireGuard ----
wg-quick up "$WG_INTERFACE"

# ---- Build gateway arguments ----
ARGS=(
    --direct-mode
    --listen ":$GATEWAY_PORT"
    --eth-rpc "$ETH_RPC_URL"
    --memes-contract "$MEMES_CONTRACT"
    --chain-id "$CHAIN_ID"
    --siwe-domain "$SIWE_DOMAIN"
    --cors-origin "$CORS_ORIGIN"
    --this-card-id "$THIS_CARD_ID"
    --max-token-id "$MAX_TOKEN_ID"
    --wg-interface "$WG_INTERFACE"
    --wg-pubkey "$PUB_KEY"
    --wg-endpoint "$PUBLIC_IP:$WG_PORT"
    --wg-subnet "$WG_SUBNET"
    --wg-dns "${WG_DNS:-1.1.1.1}"
)

# Optional: delegation
if [ "${DELEGATION:-false}" = "true" ]; then
    ARGS+=(--delegation)
fi

# Optional: user ban check
if [ "${USER_BAN_CHECK:-false}" = "true" ]; then
    ARGS+=(--user-ban-check)
    if [ -n "${USER_BAN_CATEGORY:-}" ]; then
        ARGS+=(--user-ban-category "$USER_BAN_CATEGORY")
    fi
fi

# Optional: node registry + heartbeat
if [ -n "${NODE_REGISTRY:-}" ]; then
    ARGS+=(--node-registry "$NODE_REGISTRY")
    if [ -n "${HEARTBEAT_KEY:-}" ]; then
        ARGS+=(--heartbeat-key "$HEARTBEAT_KEY")
    fi
    if [ -n "${HEARTBEAT_INTERVAL:-}" ]; then
        ARGS+=(--heartbeat-interval "$HEARTBEAT_INTERVAL")
    fi
fi

echo "  Starting gateway..."
exec gateway "${ARGS[@]}"
