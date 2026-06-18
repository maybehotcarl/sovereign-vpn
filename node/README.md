# Sovereign VPN Node — One-Command Setup

Run a 6529 VPN node with Docker. Community members can connect to your node using the 6529vpn.io site or the `svpn` CLI.

## North-Star Quick Start

Start from a fresh Ubuntu 22.04+ VPS with a public IPv4 address and ports `8080/tcp` and `51820/udp` open.

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh | sudo bash
```

The installer:

- installs Docker when missing;
- clones or updates this repo at `/opt/sovereign-vpn`;
- detects the public IP;
- writes `node/.env` with mainnet defaults;
- opens UFW ports when UFW is installed;
- starts the node with Docker Compose;
- waits for `http://127.0.0.1:8080/health`.

First run builds the local Docker image, so it can take a few minutes.

## Common Installer Options

Use your own RPC endpoint:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- --eth-rpc "https://eth-mainnet.g.alchemy.com/v2/YOUR_KEY"
```

Set a public DNS name or override IP detection:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- --public-ip "vpn.example.com"
```

Enable delegated wallet checks:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- --enable-delegation
```

Use a specific repo ref while testing:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- --repo-ref "my-branch"
```

The operator dashboard generates the longer form with enrollment metadata:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- \
      --enroll "generated-token" \
      --control-plane-url "https://6529vpn.io" \
      --operator "0xYourWallet" \
      --region "us-east"
```

The dashboard issues the enrollment token after a wallet signature, then polls until the installer reports back. Those values are stored in `node/.env` for dashboard/control-plane follow-up. They are intentionally safe metadata, not private keys.

## Registration And Heartbeats

Registration is still evolving toward the target operator-dashboard flow.

Current optional path:

```bash
curl -fsSL https://raw.githubusercontent.com/maybehotcarl/sovereign-vpn/main/node/install.sh \
  | sudo bash -s -- \
      --node-registry "0x..." \
      --heartbeat-key "LOW_PRIVILEGE_HEARTBEAT_KEY"
```

Do not use a main wallet private key as a heartbeat key. The target design is browser-wallet registration plus a low-privilege delegated heartbeat signer.

## Manual Docker Path

Use this if you are developing locally or changing the node image.

```bash
git clone https://github.com/maybehotcarl/sovereign-vpn.git
cd sovereign-vpn/node
cp .env.example .env
```

Edit `.env` if needed, then launch:

```bash
docker compose up -d --build
```

Verify it is running:

```bash
curl http://localhost:8080/health
```

## TLS

The current node gateway listens on HTTP by default. For browser-based production use, put HTTPS in front of the gateway with Caddy, a managed node subdomain, or the upcoming operator-dashboard flow.

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
