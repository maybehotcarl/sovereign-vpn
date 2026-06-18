#!/usr/bin/env bash
# Sovereign VPN node one-command installer.
#
# Intended fresh-host usage:
#   curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh | sudo bash
#
# Optional flags:
#   --eth-rpc URL
#   --public-ip IP_OR_HOST
#   --install-dir PATH
#   --repo-url URL
#   --repo-ref REF
#   --enroll TOKEN
#   --operator ADDRESS
#   --region REGION
#   --gateway-port PORT
#   --wg-port PORT
#   --node-registry ADDRESS
#   --heartbeat-key HEX
#   --subscription-manager ADDRESS
#   --session-manager ADDRESS
#   --payout-vault ADDRESS
#   --enable-delegation

set -euo pipefail

DEFAULT_REPO_URL="https://github.com/maybehotcarl/sovereign-vpn.git"
DEFAULT_REPO_REF="main"
DEFAULT_INSTALL_DIR="/opt/sovereign-vpn"
DEFAULT_ETH_RPC_URL="https://ethereum-rpc.publicnode.com"
DEFAULT_MEMES_CONTRACT="0x33FD426905F149f8376e227d0C9D3340AaD17aF1"
DEFAULT_CHAIN_ID="1"
DEFAULT_GATEWAY_PORT="8080"
DEFAULT_WG_PORT="51820"
DEFAULT_SIWE_DOMAIN="6529vpn.io"
DEFAULT_CORS_ORIGIN="https://6529vpn.io"

REPO_URL="${REPO_URL:-$DEFAULT_REPO_URL}"
REPO_REF="${REPO_REF:-$DEFAULT_REPO_REF}"
INSTALL_DIR="${INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
ETH_RPC_URL="${ETH_RPC_URL:-$DEFAULT_ETH_RPC_URL}"
MEMES_CONTRACT="${MEMES_CONTRACT:-$DEFAULT_MEMES_CONTRACT}"
CHAIN_ID="${CHAIN_ID:-$DEFAULT_CHAIN_ID}"
GATEWAY_PORT="${GATEWAY_PORT:-$DEFAULT_GATEWAY_PORT}"
WG_PORT="${WG_PORT:-$DEFAULT_WG_PORT}"
SIWE_DOMAIN="${SIWE_DOMAIN:-$DEFAULT_SIWE_DOMAIN}"
CORS_ORIGIN="${CORS_ORIGIN:-$DEFAULT_CORS_ORIGIN}"
ENROLLMENT_TOKEN="${ENROLLMENT_TOKEN:-}"
OPERATOR_ADDRESS="${OPERATOR_ADDRESS:-}"
NODE_REGION="${NODE_REGION:-}"
PUBLIC_IP="${PUBLIC_IP:-}"
NODE_REGISTRY="${NODE_REGISTRY:-}"
HEARTBEAT_KEY="${HEARTBEAT_KEY:-}"
SUBSCRIPTION_MANAGER="${SUBSCRIPTION_MANAGER:-}"
SESSION_MANAGER="${SESSION_MANAGER:-}"
SESSION_KEY="${SESSION_KEY:-}"
PAYOUT_VAULT="${PAYOUT_VAULT:-}"
ENABLE_DELEGATION="${ENABLE_DELEGATION:-false}"
THIS_CARD_ID="${THIS_CARD_ID:-0}"
MAX_TOKEN_ID="${MAX_TOKEN_ID:-350}"
WG_DNS="${WG_DNS:-1.1.1.1}"

usage() {
  cat <<EOF
Usage: sudo bash install.sh [options]

Options:
  --eth-rpc URL                  Ethereum RPC endpoint.
  --public-ip IP_OR_HOST         Public IP or DNS name for this node.
  --install-dir PATH             Install directory. Default: $DEFAULT_INSTALL_DIR
  --repo-url URL                 Git repository URL. Default: $DEFAULT_REPO_URL
  --repo-ref REF                 Git branch/tag/commit. Default: $DEFAULT_REPO_REF
  --enroll TOKEN                 Dashboard enrollment token.
  --operator ADDRESS             Operator wallet address.
  --region REGION                Node region label, such as us-east.
  --gateway-port PORT            Gateway HTTP port. Default: $DEFAULT_GATEWAY_PORT
  --wg-port PORT                 WireGuard UDP port. Default: $DEFAULT_WG_PORT
  --node-registry ADDRESS        Optional NodeRegistry contract address.
  --heartbeat-key HEX            Optional low-privilege heartbeat private key.
  --subscription-manager ADDRESS Optional SubscriptionManager contract address.
  --session-manager ADDRESS      Optional SessionManager contract address.
  --session-key HEX              Optional SessionManager signer private key.
  --payout-vault ADDRESS         Optional PayoutVault contract address.
  --this-card-id ID              THIS-card token id. Default: 0
  --max-token-id ID              Highest Memes token id to scan. Default: 350
  --enable-delegation            Enable delegated wallet checks.
  -h, --help                     Show this help.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --eth-rpc) ETH_RPC_URL="${2:?missing value for --eth-rpc}"; shift 2 ;;
    --public-ip) PUBLIC_IP="${2:?missing value for --public-ip}"; shift 2 ;;
    --install-dir) INSTALL_DIR="${2:?missing value for --install-dir}"; shift 2 ;;
    --repo-url) REPO_URL="${2:?missing value for --repo-url}"; shift 2 ;;
    --repo-ref) REPO_REF="${2:?missing value for --repo-ref}"; shift 2 ;;
    --enroll) ENROLLMENT_TOKEN="${2:?missing value for --enroll}"; shift 2 ;;
    --operator) OPERATOR_ADDRESS="${2:?missing value for --operator}"; shift 2 ;;
    --region) NODE_REGION="${2:?missing value for --region}"; shift 2 ;;
    --gateway-port) GATEWAY_PORT="${2:?missing value for --gateway-port}"; shift 2 ;;
    --wg-port) WG_PORT="${2:?missing value for --wg-port}"; shift 2 ;;
    --node-registry) NODE_REGISTRY="${2:?missing value for --node-registry}"; shift 2 ;;
    --heartbeat-key) HEARTBEAT_KEY="${2:?missing value for --heartbeat-key}"; shift 2 ;;
    --subscription-manager) SUBSCRIPTION_MANAGER="${2:?missing value for --subscription-manager}"; shift 2 ;;
    --session-manager) SESSION_MANAGER="${2:?missing value for --session-manager}"; shift 2 ;;
    --session-key) SESSION_KEY="${2:?missing value for --session-key}"; shift 2 ;;
    --payout-vault) PAYOUT_VAULT="${2:?missing value for --payout-vault}"; shift 2 ;;
    --this-card-id) THIS_CARD_ID="${2:?missing value for --this-card-id}"; shift 2 ;;
    --max-token-id) MAX_TOKEN_ID="${2:?missing value for --max-token-id}"; shift 2 ;;
    --enable-delegation) ENABLE_DELEGATION="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

log() {
  printf '\n==> %s\n' "$*"
}

warn() {
  printf '\nWARNING: %s\n' "$*" >&2
}

need_root() {
  if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    echo "Please run as root: curl ... | sudo bash" >&2
    exit 1
  fi
}

command_exists() {
  command -v "$1" >/dev/null 2>&1
}

apt_install() {
  DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
}

install_base_packages() {
  if ! command_exists apt-get; then
    echo "This installer currently supports Ubuntu/Debian hosts with apt-get." >&2
    exit 1
  fi

  log "Installing base packages"
  apt-get update -qq
  apt_install ca-certificates curl git jq
}

install_docker() {
  if command_exists docker && docker compose version >/dev/null 2>&1; then
    log "Docker is already installed"
    return
  fi

  log "Installing Docker"
  curl -fsSL https://get.docker.com | sh

  if ! docker compose version >/dev/null 2>&1; then
    echo "Docker Compose plugin was not found after Docker install." >&2
    exit 1
  fi
}

detect_public_ip() {
  if [ -n "$PUBLIC_IP" ]; then
    return
  fi

  log "Detecting public IP"
  PUBLIC_IP="$(curl -fsS --max-time 5 https://api.ipify.org || true)"
  if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP="$(curl -fsS --max-time 5 https://ifconfig.me || true)"
  fi
  if [ -z "$PUBLIC_IP" ]; then
    echo "Could not detect public IP. Re-run with --public-ip <ip-or-host>." >&2
    exit 1
  fi
}

sync_repo() {
  log "Installing Sovereign VPN repo at $INSTALL_DIR"
  if [ -d "$INSTALL_DIR/.git" ]; then
    git -C "$INSTALL_DIR" fetch --all --tags
    git -C "$INSTALL_DIR" checkout "$REPO_REF"
    git -C "$INSTALL_DIR" pull --ff-only || true
  else
    mkdir -p "$(dirname "$INSTALL_DIR")"
    git clone "$REPO_URL" "$INSTALL_DIR"
    git -C "$INSTALL_DIR" checkout "$REPO_REF"
  fi
}

write_env() {
  local env_file="$INSTALL_DIR/node/.env"
  log "Writing node configuration"

  if [ -f "$env_file" ]; then
    cp "$env_file" "$env_file.bak.$(date +%Y%m%d%H%M%S)"
  fi

  cat > "$env_file" <<EOF
# Generated by node/install.sh on $(date -u +"%Y-%m-%dT%H:%M:%SZ")
ETH_RPC_URL=$ETH_RPC_URL
MEMES_CONTRACT=$MEMES_CONTRACT
PUBLIC_IP=$PUBLIC_IP

WG_PORT=$WG_PORT
GATEWAY_PORT=$GATEWAY_PORT
CHAIN_ID=$CHAIN_ID
SIWE_DOMAIN=$SIWE_DOMAIN
CORS_ORIGIN=$CORS_ORIGIN
THIS_CARD_ID=$THIS_CARD_ID
MAX_TOKEN_ID=$MAX_TOKEN_ID
WG_DNS=$WG_DNS
DELEGATION=$ENABLE_DELEGATION
EOF

  if [ -n "$ENROLLMENT_TOKEN" ]; then
    echo "ENROLLMENT_TOKEN=$ENROLLMENT_TOKEN" >> "$env_file"
  fi

  if [ -n "$OPERATOR_ADDRESS" ]; then
    echo "OPERATOR_ADDRESS=$OPERATOR_ADDRESS" >> "$env_file"
  fi

  if [ -n "$NODE_REGION" ]; then
    echo "NODE_REGION=$NODE_REGION" >> "$env_file"
  fi

  if [ -n "$NODE_REGISTRY" ]; then
    {
      echo "NODE_REGISTRY=$NODE_REGISTRY"
      echo "HEARTBEAT_INTERVAL=${HEARTBEAT_INTERVAL:-30m}"
    } >> "$env_file"
  fi

  if [ -n "$HEARTBEAT_KEY" ]; then
    warn "A heartbeat private key was written to $env_file. Use a low-privilege hot key, not a main wallet key."
    echo "HEARTBEAT_KEY=$HEARTBEAT_KEY" >> "$env_file"
  fi

  if [ -n "$SUBSCRIPTION_MANAGER" ]; then
    echo "SUBSCRIPTION_MANAGER=$SUBSCRIPTION_MANAGER" >> "$env_file"
  fi

  if [ -n "$SESSION_MANAGER" ]; then
    echo "SESSION_MANAGER=$SESSION_MANAGER" >> "$env_file"
  fi

  if [ -n "$SESSION_KEY" ]; then
    warn "A session private key was written to $env_file. This should be avoided for normal operators."
    echo "SESSION_KEY=$SESSION_KEY" >> "$env_file"
  fi

  if [ -n "$PAYOUT_VAULT" ]; then
    echo "PAYOUT_VAULT=$PAYOUT_VAULT" >> "$env_file"
  fi

  chmod 600 "$env_file"
}

open_firewall_ports() {
  if ! command_exists ufw; then
    return
  fi

  log "Opening UFW ports when UFW is active"
  ufw allow "$GATEWAY_PORT/tcp" || true
  ufw allow "$WG_PORT/udp" || true
}

start_node() {
  log "Starting node"
  docker compose -f "$INSTALL_DIR/node/docker-compose.yml" --env-file "$INSTALL_DIR/node/.env" up -d --build
}

wait_for_health() {
  log "Waiting for gateway health"
  local url="http://127.0.0.1:$GATEWAY_PORT/health"
  local attempt

  for attempt in $(seq 1 30); do
    if curl -fsS "$url" >/tmp/sovereign-vpn-health.json 2>/dev/null; then
      cat /tmp/sovereign-vpn-health.json
      rm -f /tmp/sovereign-vpn-health.json
      return
    fi
    sleep 2
  done

  rm -f /tmp/sovereign-vpn-health.json
  warn "Gateway did not become healthy within 60 seconds. Check logs with:"
  echo "  docker compose -f $INSTALL_DIR/node/docker-compose.yml --env-file $INSTALL_DIR/node/.env logs -f"
}

print_summary() {
  cat <<EOF

Sovereign VPN node install complete.

Public endpoint:
  WireGuard: $PUBLIC_IP:$WG_PORT
  Gateway:   http://$PUBLIC_IP:$GATEWAY_PORT

Useful commands:
  cd $INSTALL_DIR/node
  docker compose --env-file .env ps
  docker compose --env-file .env logs -f
  docker compose --env-file .env up -d --build

Next north-star steps:
  1. Register this node from the operator dashboard.
  2. Use a low-privilege heartbeat signer once delegated signer support lands.
  3. Set a RAILGUN payout address before reward/payout activation.
EOF
}

need_root
install_base_packages
install_docker
detect_public_ip
sync_repo
write_env
open_firewall_ports
start_node
wait_for_health
print_summary
