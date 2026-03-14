# Sovereign VPN Node — One-Command Setup

Run a 6529 VPN node with Docker. Community members can connect to your node using the 6529vpn.io site or the `svpn` CLI.

## Quick Start

### 1. Get a VPS

Any provider works (DigitalOcean, Hetzner, Vultr, etc.). Requirements:
- Ubuntu 22.04+ (or any Linux with Docker)
- 1 vCPU / 1 GB RAM minimum
- A public IPv4 address
- Ports open: 8080/tcp (gateway), 51820/udp (WireGuard)

### 2. Install Docker

```bash
curl -fsSL https://get.docker.com | sh
```

### 3. Clone & Configure

```bash
git clone https://github.com/maybehotcarl/sovereign-vpn.git
cd sovereign-vpn/node
cp .env.example .env
```

Edit `.env` and fill in:
- `ETH_RPC_URL` — your Ethereum RPC endpoint (Alchemy, Infura, etc.)
- `PUBLIC_IP` — your server's public IP address
- `MEMES_CONTRACT` — The Memes ERC-1155 contract address

### 4. Launch

```bash
docker compose up -d
```

Verify it's running:

```bash
curl http://localhost:8080/health
```

### 5. Set Up TLS with Caddy

Point a domain at your server's IP, then install Caddy:

```bash
apt install -y caddy
```

Example `/etc/caddy/Caddyfile`:

```
your-node.example.com {
    reverse_proxy localhost:8080
}
```

```bash
systemctl restart caddy
```

Caddy automatically provisions Let's Encrypt certificates.

### 6. (Optional) Register on NodeRegistry

To appear in the node list on 6529vpn.io, register your node on-chain and enable heartbeats. Add to your `.env`:

```env
NODE_REGISTRY=0x...
HEARTBEAT_KEY=your_operator_private_key_hex
```

Then restart: `docker compose up -d`

## Configuration Reference

See `.env.example` for all available environment variables.

## Updating

```bash
git pull
docker compose up -d --build
```

## Logs

```bash
docker compose logs -f
```

Treat this as live troubleshooting output only.

- Raw operational logs must be purged within `1 hour`.
- Do not enable Caddy access logs unless they are redacted and TTL-deleted within `1 hour`.
- Docker log rotation reduces disk persistence but does not guarantee time-based deletion. To meet the policy, use a log pipeline with TTL-based deletion or disable persistent raw container logs entirely.

See [PRIVACY.md](../PRIVACY.md) for the full policy.
