import fs from "node:fs";
import path from "node:path";
import { pbkdf2Sync } from "node:crypto";
import LevelDOWN from "leveldown";
import {
  startRailgunEngine,
  loadProvider,
  createRailgunWallet,
  refreshBalances,
  ArtifactStore,
} from "@railgun-community/wallet";
import {
  NetworkName,
  NETWORK_CONFIG,
} from "@railgun-community/shared-models";
import type { Config } from "../config.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface RailgunEngineState {
  initialized: boolean;
  merkleTreeSynced: boolean;
  networkName: string;
  walletId: string;
  railgunAddress: string;
}

// ---------------------------------------------------------------------------
// Module state
// ---------------------------------------------------------------------------

const state: RailgunEngineState = {
  initialized: false,
  merkleTreeSynced: false,
  networkName: "",
  walletId: "",
  railgunAddress: "",
};

let encryptionKey = "";
let networkName: NetworkName = NetworkName.EthereumSepolia;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function chainIdToNetworkName(chainId: number): NetworkName {
  switch (chainId) {
    case 1:
      return NetworkName.Ethereum;
    case 56:
      return NetworkName.BNBChain;
    case 137:
      return NetworkName.Polygon;
    case 42161:
      return NetworkName.Arbitrum;
    case 11155111:
      return NetworkName.EthereumSepolia;
    default:
      throw new Error(`Unsupported chain ID for RAILGUN: ${chainId}`);
  }
}

function deriveEncryptionKey(mnemonic: string): string {
  const salt = "railgun-payout-service";
  return pbkdf2Sync(mnemonic, salt, 100000, 32, "sha256").toString("hex");
}

function createArtifactStore(artifactsDir: string): ArtifactStore {
  return new ArtifactStore(
    async (artifactPath: string) => {
      const fullPath = path.join(artifactsDir, artifactPath);
      try {
        return await fs.promises.readFile(fullPath);
      } catch {
        return null;
      }
    },
    async (dir: string, artifactPath: string, item: string | Uint8Array) => {
      const fullDir = path.join(artifactsDir, dir);
      await fs.promises.mkdir(fullDir, { recursive: true });
      const fullPath = path.join(artifactsDir, artifactPath);
      await fs.promises.writeFile(fullPath, item);
    },
    async (artifactPath: string) => {
      const fullPath = path.join(artifactsDir, artifactPath);
      try {
        await fs.promises.access(fullPath);
        return true;
      } catch {
        return false;
      }
    },
  );
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Initialize the RAILGUN privacy engine, load the provider, and create
 * (or load) the wallet from the configured mnemonic.
 */
export async function initRailgunEngine(config: Config): Promise<void> {
  console.log("[railgun/engine] Initializing RAILGUN engine...");
  console.log(`[railgun/engine] Chain ID: ${config.chainId}`);

  if (!config.railgunMnemonic) {
    console.warn(
      "[railgun/engine] No RAILGUN_MNEMONIC configured. " +
      "Private transfers will not be available.",
    );
    return;
  }

  try {
    networkName = chainIdToNetworkName(config.chainId);
    const network = NETWORK_CONFIG[networkName];

    // 1. Create LevelDB database
    await fs.promises.mkdir(config.railgunDbPath, { recursive: true });
    const db = LevelDOWN(config.railgunDbPath);

    // 2. Create artifact store
    await fs.promises.mkdir(config.railgunArtifactsPath, { recursive: true });
    const artifactStore = createArtifactStore(config.railgunArtifactsPath);

    // 3. Start the RAILGUN engine
    await startRailgunEngine(
      "svpnpayout",        // walletSource (max 16 chars lowercase alphanumeric)
      db,
      false,               // shouldDebug
      artifactStore,
      false,               // useNativeArtifacts (false for Node.js)
      false,               // skipMerkletreeScans
      config.railgunPoiNodes.length > 0 ? config.railgunPoiNodes : undefined,
    );

    console.log("[railgun/engine] Engine started");

    // 4. Load provider
    const providerConfig = {
      chainId: config.chainId,
      providers: [
        {
          provider: config.ethRpcUrl,
          priority: 1,
          weight: 2,
        },
      ],
    };

    await loadProvider(providerConfig, networkName);
    console.log(`[railgun/engine] Provider loaded for ${networkName}`);

    // 5. Derive encryption key from mnemonic
    encryptionKey = deriveEncryptionKey(config.railgunMnemonic);

    // 6. Create or load wallet
    const creationBlockMap: Record<string, number> = {};
    creationBlockMap[networkName] = network.deploymentBlock ?? 0;

    const walletInfo = await createRailgunWallet(
      encryptionKey,
      config.railgunMnemonic,
      creationBlockMap,
    );

    state.walletId = walletInfo.id;
    state.railgunAddress = walletInfo.railgunAddress;
    state.networkName = networkName;
    state.initialized = true;

    console.log(`[railgun/engine] Wallet created: ${state.railgunAddress}`);
    console.log("[railgun/engine] Engine initialized successfully");
  } catch (err) {
    console.error("[railgun/engine] Failed to initialize RAILGUN engine:", err);
    state.initialized = false;
  }
}

/**
 * Sync the UTXO Merkle tree to the latest block.
 * Required before shielding or transferring.
 */
export async function syncMerkleTree(): Promise<void> {
  if (!state.initialized) {
    console.warn("[railgun/engine] Engine not initialized; skipping Merkle tree sync");
    return;
  }

  console.log("[railgun/engine] Syncing UTXO Merkle tree...");

  try {
    const { chain } = NETWORK_CONFIG[networkName];
    await refreshBalances(chain, [state.walletId]);
    state.merkleTreeSynced = true;
    console.log("[railgun/engine] Merkle tree synced");
  } catch (err) {
    console.error("[railgun/engine] Merkle tree sync failed:", err);
  }
}

/**
 * Get the current engine state (for health checks).
 */
export function getEngineState(): Readonly<RailgunEngineState> {
  return { ...state };
}

/**
 * Get the RAILGUN wallet ID for shield/transfer operations.
 */
export function getWalletId(): string {
  return state.walletId;
}

/**
 * Get the encryption key for shield/transfer operations.
 */
export function getEncryptionKey(): string {
  return encryptionKey;
}

/**
 * Quick check whether the RAILGUN engine is ready for operations.
 */
export function isRailgunReady(): boolean {
  return state.initialized && state.walletId !== "";
}
