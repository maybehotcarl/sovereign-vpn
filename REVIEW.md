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
- **[FIXED]** `SessionManager.openSession` does not validate that `node` is a registered/active node in `NodeRegistry`, so users can pay arbitrary addresses as “node operators” (`contracts/src/SessionManager.sol:133`). *Fixed in 4c0272b.*
  - Category: Bugs/logic, Security, Config consistency.

### Medium
- `AccessPolicy.setThisCardTokenId` allows `tokenId == 0`, while `0` is also the “not set” sentinel used elsewhere (`contracts/src/AccessPolicy.sol:102`, `contracts/src/AccessPolicy.sol:116`). This can silently misconfigure free-tier behavior.
  - Category: Bugs/logic, Input validation.
- `NodeRegistry` constructor does not reject zero `memesContract` address, causing permanent eligibility-check failures if misconfigured (`contracts/src/NodeRegistry.sol:110`).
  - Category: Config/deployment, Input validation.
- `NodeRegistry.setRailgunAddress` only checks prefix `0zk` and allows trivially invalid payloads like `"0zk"` (`contracts/src/NodeRegistry.sol:222`).
  - Category: Input validation.

### Low
- **[FIXED]** Contract script/docs inconsistency: deployment guidance references non-existent `registerNode` function (`contracts/script/DeployNodeRegistry.s.sol:48`), actual function is `register`. *Fixed in 91e71f1.*
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
- **[FIXED]** ~~**Session hijack/auth bypass**: session token is just wallet address and not a secret.~~ Sessions now use HMAC-signed opaque tokens; `/vpn/connect` validates via `GetSessionByToken`. *Fixed in 0d52584.*
  - Category: Security (auth bypass).

### High
- **[FIXED]** `parseAddress` is permissive and maps invalid hex chars to `0` nibble. Now uses `common.IsHexAddress` for strict validation. *Fixed in f256f2c.*
  - Category: Security, input validation.
- `/vpn/disconnect` has no session ownership validation and removes peers by public key only (`gateway/pkg/server/server.go:535`, `gateway/pkg/server/server.go:542`). Enables easy DoS if key leaks or is guessed.
  - Category: Security.
- Revocation watcher revokes both sender and receiver sessions on transfer (`gateway/pkg/revocation/watcher.go:181`, `gateway/pkg/revocation/watcher.go:188`). Receiver should usually only get cache invalidation; current behavior can disconnect newly eligible users.
  - Category: Bugs/logic.
- **[PARTIAL]** SIWE verification checks only domain + nonce; it does not enforce URI, chain ID, issued-at/expiration fields from EIP-4361. *Chain ID enforcement added in f256f2c; URI and expiry checks remain open.*
  - Category: Security.

### Medium
- **[FIXED]** Config declares `rate_limit_per_minute` but no request path uses it. Per-IP rate limiter middleware now wraps all requests. *Fixed in 0d52584.*
  - Category: Security, config.
- **[FIXED]** `SessionManager` integration derives `node` from tx signer key. Now uses separate `nodeAddr` field with `SetNodeOperator()`. *Fixed in 0d52584.*
  - Category: Logic consistency.
- **[FIXED]** `Server.ListenAndServe()` bypasses CORS wrapper by serving `s.mux` directly instead of `s.Handler()`. Now uses `s.Handler()`.
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
- **[FIXED]** Auto-node selection logic depends on `rep` fields that server no longer returns. Client now uses card-eligibility fields. *Fixed in 5f4c0e8.*
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
- **[FIXED]** No explicit runtime chain-ID sanity check against provider. Now verifies `provider.getNetwork().chainId` against config at startup. *Fixed in 9907bea.*
  - Category: Config/deployment.
- Service proceeds without RAILGUN mnemonic (by design), but this silently degrades privacy guarantees and centralizes funds in executor wallet.
  - Category: Security/operational risk.

### Low
- **[FIXED]** Health `nextRunAt` is not an actual timestamp. Now computed as ISO 8601 via cron-parser. *Fixed in 9907bea.*

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
- **[FIXED]** ~~Session token is wallet address in connect flow.~~ Backend now returns opaque tokens; frontend uses them accordingly. *Fixed in 09e7e63.*
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
- **[FIXED]** Dev proxy missing `/subscription` and `/payout`. Both routes now added to `vite.config.js`. *Fixed in 09e7e63.*
  - Category: Config/deployment.
- **[FIXED]** Dashboard expects `session.nodeOperator`, but `VPNConnect` never stores it. `nodeOperator` now resolved and passed to session. *Fixed in 09e7e63.*
  - Category: Logic/dead code.
- **[FIXED]** Uses `Number(...)` on wei strings for ETH formatting, causing precision loss. Now uses `formatEther(BigInt(...))`. *Fixed in 09e7e63.*
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
- **[FIXED]** Sensitive private keys are passed as process args (`--session-key`, `--heartbeat-key`). Now read from environment/files instead. *Fixed in db37046.*
  - Category: Security.

### Medium
- **[FIXED]** No shutdown trap to run `wg-quick down` and clean iptables on container stop. Trap added for SIGTERM/SIGINT. *Fixed in 91e71f1.*
  - Category: Config/ops reliability.
- **[FIXED]** Entry point hardcodes `eth0` in iptables NAT rule. Now auto-detects default interface via `ip route`, with `NAT_INTERFACE` override. *Fixed in 91e71f1.*

### Low
- `.env.example` and docs lag optional flags present in entrypoint (e.g. `SUBSCRIPTION_MANAGER`, `ZK_API_URL`) causing operator confusion.

### Testing gaps
- No tests for shell entrypoint behavior or compose runtime validation.

---

## deploy/

### High
- **[FIXED]** Setup instructions are stale/inconsistent: used `--policy-contract` in sample command. Updated to `--direct-mode` workflow. *Fixed in 91e71f1.*
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
- **[FIXED]** E2E tests explicitly allow `/vpn/connect` failure and return early. Now fail hard on 401/403 and assert connected status. *Fixed in 9907bea.*
  - Category: Testing gap.
- Live Sepolia addresses are hardcoded; long-term drift risk when contracts are redeployed.
  - Category: Config consistency.

### Low
- `go 1.24.0` requirement may constrain CI/engineer environments (`integration/go.mod`).

---

## Cross-Component Consistency Findings

### High
- **[FIXED]** ~~Session/auth model mismatch: all components treat wallet address as bearer session token.~~ Backend now issues HMAC-signed opaque tokens; CLI and frontend updated to use them. *Fixed across 0d52584, 8ea36fa, 09e7e63.*

### Medium
- **[PARTIAL]** Rep-based node selection still appears in client/frontend, while gateway now returns card-eligibility fields instead of rep. *Client fixed in 5f4c0e8; site-app `NodeSelector.jsx` may still reference rep.*

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

1. ~~Replace wallet-address session tokens with opaque, signed, short-lived session IDs~~ — **Done** (HMAC opaque tokens, ownership checks on connect/disconnect).
2. Fix payout processor accounting to track only successfully-withdrawn batches for shielding/private transfers.
3. ~~Tighten SIWE validation and strict address parsing~~ — **Partial** (chain ID + `common.IsHexAddress` done; URI/expiry checks remain).
4. ~~Remove private keys from process args in node runtime~~ — **Done** (env/file-based secrets).
5. ~~Remove/replace stale rep fields in client/site-app~~ — **Done** (client uses card-eligibility fields).
6. Add targeted tests for all high/critical paths listed above.
