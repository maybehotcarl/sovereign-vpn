# Sovereign VPN Comprehensive Code Review

Scope reviewed: `contracts/`, `gateway/`, `client/`, `payout-service/`, `site/`, `site-app/`, `node/`, `deploy/`, `zk/`, `integration/`.

Method: deep source read of all files in-scope (not just listings), plus targeted consistency and security tracing across components.

## Severity Legend
- **Critical**: exploitable or production-breaking with immediate impact.
- **High**: serious security/correctness risk likely to cause incidents.
- **Medium**: important defects, reliability gaps, or major maintainability risks.
- **Low**: quality/docs/consistency issues with lower immediate impact.

---

## contracts/

### Critical
- None found in Solidity logic itself.

### High
- `SessionManager.openSession` does not validate that `node` is a registered/active node in `NodeRegistry`, so users can pay arbitrary addresses as “node operators” (`contracts/src/SessionManager.sol:133`).
  - Category: Bugs/logic, Security, Config consistency.

### Medium
- `AccessPolicy.setThisCardTokenId` allows `tokenId == 0`, while `0` is also the “not set” sentinel used elsewhere (`contracts/src/AccessPolicy.sol:102`, `contracts/src/AccessPolicy.sol:116`). This can silently misconfigure free-tier behavior.
  - Category: Bugs/logic, Input validation.
- `NodeRegistry` constructor does not reject zero `memesContract` address, causing permanent eligibility-check failures if misconfigured (`contracts/src/NodeRegistry.sol:110`).
  - Category: Config/deployment, Input validation.
- `NodeRegistry.setRailgunAddress` only checks prefix `0zk` and allows trivially invalid payloads like `"0zk"` (`contracts/src/NodeRegistry.sol:222`).
  - Category: Input validation.

### Low
- Contract script/docs inconsistency: deployment guidance references non-existent `registerNode` function (`contracts/script/DeployNodeRegistry.s.sol:48`), actual function is `register`.
  - Category: Documentation consistency.

### Testing gaps
- Missing tests for `setThisCardTokenId(0)` misconfiguration path.
- Missing tests that `SessionManager.openSession` rejects non-registered nodes (currently no such guard).
- Missing `NodeRegistry` constructor zero-address test.

### Dependencies/config
- OZ usage is modern (`v5.5.0`) and generally good.
- Foundry RPC defaults are hardcoded to public nodes (`contracts/foundry.toml`), no fallback strategy documented.

---

## gateway/

### Critical
- **Session hijack/auth bypass**: session token is just wallet address and not a secret; `/vpn/connect` trusts `session_token` directly (`gateway/pkg/server/server.go:429`, `gateway/pkg/server/server.go:441`). Any party knowing an active wallet address can connect with their own WG key.
  - Category: Security (auth bypass).

### High
- `parseAddress` is permissive and maps invalid hex chars to `0` nibble, enabling malformed-token ambiguity/spoofing (`gateway/pkg/server/server.go:777`, `gateway/pkg/server/server.go:795`).
  - Category: Security, input validation.
- `/vpn/disconnect` has no session ownership validation and removes peers by public key only (`gateway/pkg/server/server.go:535`, `gateway/pkg/server/server.go:542`). Enables easy DoS if key leaks or is guessed.
  - Category: Security.
- Revocation watcher revokes both sender and receiver sessions on transfer (`gateway/pkg/revocation/watcher.go:181`, `gateway/pkg/revocation/watcher.go:188`). Receiver should usually only get cache invalidation; current behavior can disconnect newly eligible users.
  - Category: Bugs/logic.
- SIWE verification checks only domain + nonce; it does not enforce URI, chain ID, issued-at/expiration fields from EIP-4361 (`gateway/pkg/siwe/siwe.go:141`, `gateway/pkg/siwe/siwe.go:153`, `gateway/pkg/siwe/siwe.go:181`).
  - Category: Security.

### Medium
- Config declares `rate_limit_per_minute` but no request path uses it (`gateway/pkg/config/config.go:32`; only references are config declarations/examples). Effective brute-force/rate limiting is missing.
  - Category: Security, config.
- `SessionManager` integration derives `node` from tx signer key (`gateway/pkg/sessionmgr/sessionmgr.go:147`, `gateway/pkg/sessionmgr/sessionmgr.go:161`). If owner key is not the node operator, on-chain free-session attribution is wrong.
  - Category: Logic consistency.
- `Server.ListenAndServe()` bypasses CORS wrapper by serving `s.mux` directly instead of `s.Handler()` (`gateway/pkg/server/server.go:154`, `gateway/pkg/server/server.go:157`).
  - Category: Config/deployment consistency.

### Low
- Background cleanup goroutines in nonce/session/checker stores are never stopped cleanly (resource leak risk in long-lived test/process churn).

### Testing gaps
- No tests for auth bypass/session hijack scenario.
- No endpoint-level tests covering `/vpn/disconnect` authorization.
- `server_test.go` mostly tests helpers, not real handler security behavior.

### Dependencies/config
- `go.mod` targets `go 1.24.0`; ensure build infrastructure supports it (`gateway/go.mod`).

---

## client/

### Critical
- None.

### High
- Auto-node selection logic depends on `rep` fields that server no longer returns (`client/pkg/api/client.go:192`, `gateway/pkg/server/server.go:593`), so selection degenerates to first node and displays misleading rep values (`client/cmd/svpn/main.go:109`).
  - Category: Logic consistency.

### Medium
- Host parsing strips by last `:` and forces `https://`, which is unsafe for IPv6 endpoints and non-TLS local/test deployments (`client/cmd/svpn/main.go:116`, `client/cmd/svpn/main.go:126`).
  - Category: Bugs/config.
- `Status()` does not check HTTP status codes before decode (`client/pkg/api/client.go:151`), reducing error transparency.
  - Category: Error handling.

### Low
- CLI disconnect requires manual `--wg-pubkey`; no local session state helper.

### Testing gaps
- No tests for malformed `/nodes` payloads or missing `rep` fields.
- No integration tests for auto-node URL parsing edge cases (IPv6, ports, protocol).

### Dependencies/config
- Same `go 1.24.0` requirement (`client/go.mod`).

---

## payout-service/

### Critical
- **Incorrect payout accounting across failed batches**: shielding/transfers use `result.totalAmount` and iterate all `eligible` operators even if some `processBatchPayout` calls failed (`payout-service/src/payout/processor.ts:126`, `payout-service/src/payout/processor.ts:186`, `payout-service/src/payout/processor.ts:215`). This can over-shield or attempt transfers for funds never withdrawn.
  - Category: Bugs/logic.

### High
- Receipt duplication/incorrect update model: successful vault batch inserts receipt, then successful private transfer inserts another row instead of updating first record (`payout-service/src/payout/processor.ts:177`, `payout-service/src/payout/processor.ts:233`, `payout-service/src/receipts/store.ts:79`).
  - Category: Data integrity.
- `SUM(CAST(amount AS INTEGER))` in SQLite stats can overflow/lose precision for wei-scale values (`payout-service/src/receipts/store.ts:126`).
  - Category: Logic/data correctness.

### Medium
- No explicit runtime chain-ID sanity check against provider despite config contract claiming it (`payout-service/src/config.ts`, `payout-service/src/index.ts`).
  - Category: Config/deployment.
- Service proceeds without RAILGUN mnemonic (by design), but this silently degrades privacy guarantees and centralizes funds in executor wallet.
  - Category: Security/operational risk.

### Low
- Health `nextRunAt` is not an actual timestamp (`payout-service/src/index.ts`).

### Testing gaps
- Tests are mostly unit/spec-like and do not exercise `PayoutProcessor.runPayoutCycle()` end-to-end with mocked contract outcomes.
- No tests for partial batch failure accounting (the critical bug above).

### Dependencies/config
- Could not run `npm audit` in current environment (no installed toolchain/deps).

---

## site/

### Critical
- None.

### High
- Session token is wallet address in connect flow, mirroring backend weakness (`site/index.html:253` equivalent in JS flow), so frontend participates in insecure session model.
  - Category: Security architecture.

### Medium
- `copyConfig()` relies on global `event` object (`site/index.html:671`, `site/index.html:673`), which is non-portable and fragile.
  - Category: Bugs/code quality.
- Contains a second full WireGuard keygen implementation duplicated from `site-app`, increasing drift risk.
  - Category: Consistency/maintainability.

### Testing gaps
- No tests at all for UI flow or browser compatibility.

### Documentation/config
- None specific.

---

## site-app/

### High
- Stores full VPN config (including WireGuard private key) in `localStorage` via persisted session (`site-app/src/VPNConnect.jsx:264`, `site-app/src/VPNConnect.jsx:287`, `site-app/src/useSession.js:19`). Any XSS/browser compromise leaks long-lived VPN credentials.
  - Category: Security.
- Node UI expects/prints `rep` even though gateway response no longer includes it (`site-app/src/NodeSelector.jsx:80`, backend `gateway/pkg/server/server.go:593`).
  - Category: Consistency/logic.

### Medium
- Dev proxy missing `/subscription` and `/payout`, but app calls those endpoints (`site-app/vite.config.js:10`, `site-app/src/VPNConnect.jsx:143`, `site-app/src/SessionDashboard.jsx:121`). Local development flow breaks.
  - Category: Config/deployment.
- Dashboard expects `session.nodeOperator`, but `VPNConnect` never stores it (`site-app/src/SessionDashboard.jsx:66`, `site-app/src/VPNConnect.jsx:279`). Payout status panel is effectively dead code.
  - Category: Logic/dead code.
- Uses `Number(...)` on wei strings for ETH formatting, causing precision loss (`site-app/src/SessionDashboard.jsx:225`).
  - Category: Logic correctness.

### Low
- Hardcoded WalletConnect project ID in source instead of env/config (`site-app/src/wagmi.js:9`).
- `App.css` is unused (`site-app/src/App.css`).

### Testing gaps
- No frontend tests (unit/integration/e2e).

### Dependencies/config
- Could not run `npm audit` in current environment.

---

## node/

### High
- Sensitive private keys are passed as process args (`--session-key`, `--heartbeat-key`), exposing them via process listings and some runtime telemetry (`node/entrypoint.sh:95`, `node/entrypoint.sh:103`).
  - Category: Security.

### Medium
- No shutdown trap to run `wg-quick down` and clean iptables on container stop; network rules can be left dirty.
  - Category: Config/ops reliability.
- Entry point hardcodes assumptions like `eth0` in iptables NAT rule (`node/entrypoint.sh` generated config), fragile across hosts.

### Low
- `.env.example` and docs lag optional flags present in entrypoint (e.g. `SUBSCRIPTION_MANAGER`, `ZK_API_URL`) causing operator confusion.

### Testing gaps
- No tests for shell entrypoint behavior or compose runtime validation.

---

## deploy/

### High
- Setup instructions are stale/inconsistent with current gateway flags: uses `--policy-contract` in sample command (`deploy/setup-node.sh:99`), while modern node flow emphasizes `--direct-mode` mainnet checks.
  - Category: Documentation/config.

### Medium
- Script mutates host networking/firewall directly without rollback/validation checkpoints.

### Testing gaps
- No validation/dry-run mode for setup script.

---

## zk/

### High
- Trusted setup warning is present but build script still creates local single-party PTau for convenience (`zk/scripts/build.sh`), which is unsafe for production if reused.
  - Category: Security/process.

### Medium
- No automated integration tests verifying generated `Groth16Verifier.sol` against contract-side verification in this repo.
- Production/test circuit split is manual and error-prone (`memes_membership` vs `memes_membership_test`).

### Low
- No README in `zk/` explaining safe ceremony requirements and deployment constraints.

---

## integration/

### Medium
- E2E tests explicitly allow `/vpn/connect` failure and return early (`integration/e2e_test.go:232`, `integration/e2e_test.go:240`), so key network provisioning behavior is not truly validated.
  - Category: Testing gap.
- Live Sepolia addresses are hardcoded; long-term drift risk when contracts are redeployed.
  - Category: Config consistency.

### Low
- `go 1.24.0` requirement may constrain CI/engineer environments (`integration/go.mod`).

---

## Cross-Component Consistency Findings

### High
- Session/auth model mismatch: backend, CLI, and frontend all treat wallet address as bearer session token rather than opaque secret/JWT, creating a systemic auth weakness (`gateway/pkg/server/server.go:441`, `client/pkg/api/client.go`, `site-app/src/VPNConnect.jsx:253`, `site/index.html` connect flow).

### Medium
- Rep-based node selection still appears in client/frontend, while gateway now returns card-eligibility fields instead of rep. This causes misleading UX and non-deterministic node selection policy.

### Medium
- Multiple duplicated implementations (WireGuard keygen in `site` and `site-app`) increase maintenance drift risk.

---

## Dependency / Vulnerability Review Status

- Static dependency manifests were reviewed.
- Runtime vulnerability scans could not be executed in this environment:
  - `go` not available (`go test` failed in `client/`, `gateway/`, `integration/`).
  - JS toolchain deps not installed (`vitest`/`vite` unavailable).
  - `forge` not available for Solidity tests.

Recommend running in CI or a dev machine with toolchains installed:
1. `cd contracts && forge test`
2. `cd gateway && go test ./...`
3. `cd client && go test ./...`
4. `cd integration && go test ./...`
5. `cd payout-service && npm ci && npm test && npm audit`
6. `cd site-app && npm ci && npm run build && npm audit`

---

## Prioritized Remediation Plan

1. Replace wallet-address session tokens with opaque, signed, short-lived session IDs (or JWTs) and enforce ownership checks on `/vpn/connect` and `/vpn/disconnect`.
2. Fix payout processor accounting to track only successfully-withdrawn batches for shielding/private transfers.
3. Tighten SIWE validation (chain ID, URI, issued-at/expiration) and strict address parsing (`common.IsHexAddress`).
4. Remove private keys from process args in node runtime; use file descriptors/secrets mounts/env with minimal exposure.
5. Remove/replace stale rep fields in client/site-app and align node selection with actual gateway schema.
6. Add targeted tests for all high/critical paths listed above.
