# 6529 VPN Desktop

First desktop companion slice for Sovereign VPN.

Current goals:

- accept a localhost handoff from the website
- write the WireGuard config locally
- bring the tunnel up with platform elevation
- auto-disconnect the tunnel when the lease expires

This app is intentionally focused on desktop tunnel lifecycle first. Wallet auth and purchase still happen in the browser.

## Development

```bash
cd desktop-app
npm install
npm run start
```

## Packaging

```bash
cd desktop-app
npm install
npm run package:dir
```

Platform-specific builds:

```bash
npm run package:linux
npm run package:mac
npm run package:win
```

Artifacts are written to `desktop-app/release/`.

Notes:

- Linux packaging works best on Linux
- macOS packaging should be run on macOS
- Windows packaging should be run on Windows
- icons still need real branded replacements in `build-resources/`

## Current handoff contract

The website POSTs the live session to:

```http
POST http://127.0.0.1:9469/handoff
```

Payload shape:

```json
{
  "vpnConfig": "[Interface] ...",
  "expiresAt": "2026-04-04T12:34:56.000Z",
  "accessMode": "direct",
  "serverEndpoint": "6529vpn.io:51820",
  "sessionLabel": "Direct Wallet Session"
}
```

The app also registers `sovereignvpn://open` so the site can wake/focus the app before retrying the localhost handoff.

## Limitations

- Linux currently expects `pkexec` and `wg-quick`
- macOS currently expects `wireguard-tools` plus an admin prompt
- Windows currently expects the official WireGuard client
- Browser auth handoff is implemented, but packaging/signing is not done yet
