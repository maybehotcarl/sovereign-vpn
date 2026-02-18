#!/bin/bash
# Sovereign VPN Node Setup Script
# Run on a fresh Ubuntu 22.04+ VPS
#
# Usage: sudo ./setup-node.sh
#
# Prerequisites: Docker installed, or run with --install-docker

set -euo pipefail

# Configuration (override via environment variables)
WG_INTERFACE="${WG_INTERFACE:-wg0}"
WG_PORT="${WG_PORT:-51820}"
WG_SUBNET="${WG_SUBNET:-10.8.0.0/24}"
WG_SERVER_IP="${WG_SERVER_IP:-10.8.0.1/24}"
GATEWAY_PORT="${GATEWAY_PORT:-8080}"

echo "=== Sovereign VPN Node Setup ==="
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo)"
    exit 1
fi

# Install WireGuard
echo "Installing WireGuard..."
apt-get update -qq
apt-get install -y wireguard wireguard-tools iptables

# Generate WireGuard keys if they don't exist
WG_DIR="/etc/wireguard"
if [ ! -f "$WG_DIR/privatekey" ]; then
    echo "Generating WireGuard keys..."
    wg genkey | tee "$WG_DIR/privatekey" | wg pubkey > "$WG_DIR/publickey"
    chmod 600 "$WG_DIR/privatekey"
fi

WG_PRIVATE_KEY=$(cat "$WG_DIR/privatekey")
WG_PUBLIC_KEY=$(cat "$WG_DIR/publickey")

# Detect public IP
PUBLIC_IP=$(curl -s https://api.ipify.org || curl -s https://ifconfig.me || echo "UNKNOWN")
NETWORK_INTERFACE=$(ip route | grep default | awk '{print $5}' | head -1)

echo "Public IP:     $PUBLIC_IP"
echo "WG Public Key: $WG_PUBLIC_KEY"
echo "Network:       $NETWORK_INTERFACE"

# Create WireGuard config
cat > "$WG_DIR/$WG_INTERFACE.conf" <<WGEOF
[Interface]
Address = $WG_SERVER_IP
PrivateKey = $WG_PRIVATE_KEY
ListenPort = $WG_PORT
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -A FORWARD -o %i -j ACCEPT; iptables -t nat -A POSTROUTING -o $NETWORK_INTERFACE -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -D FORWARD -o %i -j ACCEPT; iptables -t nat -D POSTROUTING -o $NETWORK_INTERFACE -j MASQUERADE
WGEOF

chmod 600 "$WG_DIR/$WG_INTERFACE.conf"

# Enable IP forwarding
echo "Enabling IP forwarding..."
sysctl -w net.ipv4.ip_forward=1
if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf; then
    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
fi

# Start WireGuard
echo "Starting WireGuard..."
systemctl enable wg-quick@$WG_INTERFACE
systemctl start wg-quick@$WG_INTERFACE || wg-quick up $WG_INTERFACE

# Open firewall ports
echo "Configuring firewall..."
if command -v ufw &>/dev/null; then
    ufw allow $WG_PORT/udp
    ufw allow $GATEWAY_PORT/tcp
fi

echo ""
echo "=== Setup Complete ==="
echo ""
echo "WireGuard interface: $WG_INTERFACE"
echo "WireGuard port:      $WG_PORT"
echo "Server public key:   $WG_PUBLIC_KEY"
echo "VPN subnet:          $WG_SUBNET"
echo "Public IP:           $PUBLIC_IP"
echo ""
echo "Next steps:"
echo "  1. Deploy the AccessPolicy contract to Sepolia/mainnet"
echo "  2. Build the gateway: cd gateway && go build -o sovereign-gateway ./cmd/gateway"
echo "  3. Run the gateway:"
echo ""
echo "     ./sovereign-gateway \\"
echo "       --listen :$GATEWAY_PORT \\"
echo "       --eth-rpc 'https://eth-sepolia.g.alchemy.com/v2/YOUR_KEY' \\"
echo "       --policy-contract '0xYOUR_ACCESS_POLICY' \\"
echo "       --memes-contract '0xYOUR_MEMES_CONTRACT' \\"
echo "       --wg-interface $WG_INTERFACE \\"
echo "       --wg-pubkey '$WG_PUBLIC_KEY' \\"
echo "       --wg-endpoint '$PUBLIC_IP:$WG_PORT' \\"
echo "       --wg-subnet '$WG_SUBNET' \\"
echo "       --delegation"
echo ""
echo "  4. Test with the client:"
echo "     svpn keygen --out wallet.key"
echo "     svpn connect --gateway http://$PUBLIC_IP:$GATEWAY_PORT --key wallet.key"
