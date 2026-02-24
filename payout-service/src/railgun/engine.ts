import type { Config } from "../config.js";

// ---------------------------------------------------------------------------
// RAILGUN Engine Initialization (Stub)
// ---------------------------------------------------------------------------
//
// This module provides the scaffolding for initializing the RAILGUN privacy
// engine. Actual SDK integration requires:
//
//   @railgun-community/engine       - Core proving engine
//   @railgun-community/wallet       - Wallet management / key derivation
//   @railgun-community/node-services - Merkle tree sync, POI, etc.
//
// The SDK dependencies are NOT included in package.json yet because they
// require specific network artifacts (proving keys, circuit files) that
// must be downloaded separately. These stubs document the intended flow.
//
// TODO: Install RAILGUN SDK packages and replace stubs with real calls.
// ---------------------------------------------------------------------------

export interface RailgunEngineState {
  initialized: boolean;
  merkleTreeSynced: boolean;
  networkName: string;
}

const state: RailgunEngineState = {
  initialized: false,
  merkleTreeSynced: false,
  networkName: "",
};

/**
 * Initialize the RAILGUN privacy engine.
 *
 * Real implementation would:
 *   1. Call `startRailgunEngine(...)` with artifact paths and database config
 *   2. Set the network (Ethereum mainnet, Sepolia, Polygon, etc.)
 *   3. Load or create the RAILGUN wallet from the mnemonic
 *
 * Example SDK flow:
 * ```ts
 * import { startRailgunEngine } from "@railgun-community/wallet";
 * import { setOnBalanceUpdateCallback } from "@railgun-community/wallet";
 *
 * await startRailgunEngine(
 *   walletSource,          // "sovereign-vpn-payout"
 *   dbEncryptionKey,       // derived from config
 *   artifactStore,         // local file system artifact store
 *   useNativeArtifacts,    // false for Node.js
 *   skipMerkletreeScans,   // false - we need full sync
 * );
 *
 * // Load network
 * const network = NETWORK_CONFIG[chainId];
 * await loadProvider(providerConfig, network.name);
 *
 * // Create or load wallet
 * const { railgunWalletInfo } = await createRailgunWallet(
 *   dbEncryptionKey,
 *   mnemonic,
 *   creationBlockNumbers,
 * );
 * ```
 */
export async function initRailgunEngine(config: Config): Promise<void> {
  console.log("[railgun/engine] Initializing RAILGUN engine (stub)...");
  console.log(`[railgun/engine] Chain ID: ${config.chainId}`);

  if (!config.railgunMnemonic) {
    console.warn(
      "[railgun/engine] No RAILGUN_MNEMONIC configured. " +
      "Private transfers will not be available.",
    );
    return;
  }

  // TODO: Replace with actual SDK initialization
  //
  // Steps:
  //   1. Download/verify proving key artifacts
  //   2. startRailgunEngine(...)
  //   3. loadProvider(...)
  //   4. createRailgunWallet(mnemonic)
  //   5. Wait for initial balance scan

  state.initialized = true;
  state.networkName = config.chainId === 1 ? "ethereum" : `chain-${config.chainId}`;

  console.log("[railgun/engine] Engine initialized (stub) on", state.networkName);
}

/**
 * Sync the UTXO Merkle tree to the latest block.
 *
 * Real implementation would:
 *   1. Call the SDK's scan/sync utilities
 *   2. Wait for the Merkle tree to reach the latest on-chain commitment
 *   3. This is required before shielding or transferring
 *
 * Example SDK flow:
 * ```ts
 * import { refreshBalances } from "@railgun-community/wallet";
 *
 * const { chain } = NETWORK_CONFIG[chainId];
 * await refreshBalances(chain, undefined);
 * ```
 */
export async function syncMerkleTree(): Promise<void> {
  if (!state.initialized) {
    console.warn("[railgun/engine] Engine not initialized; skipping Merkle tree sync");
    return;
  }

  console.log("[railgun/engine] Syncing UTXO Merkle tree (stub)...");

  // TODO: Replace with actual SDK merkle tree sync
  // This can take several minutes on first run.

  state.merkleTreeSynced = true;
  console.log("[railgun/engine] Merkle tree synced (stub)");
}

/**
 * Get the current engine state (for health checks).
 */
export function getEngineState(): Readonly<RailgunEngineState> {
  return { ...state };
}
