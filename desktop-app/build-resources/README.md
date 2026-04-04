Packaging resources for 6529 VPN Desktop.

Current state:

- macOS entitlements stub is present
- protocol registration is configured in `package.json`
- platform icons are still using Electron defaults

Before public distribution, replace the default Electron branding with:

- `icon.icns` for macOS
- `icon.ico` for Windows
- `icon.png` for Linux
