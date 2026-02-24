import "dotenv/config";

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

export interface Config {
  /** Ethereum JSON-RPC URL */
  ethRpcUrl: string;
  /** Target chain ID (used for sanity-checking the provider) */
  chainId: number;

  /** Deployed PayoutVault contract address */
  payoutVaultAddress: string;
  /** Deployed NodeRegistry contract address */
  nodeRegistryAddress: string;

  /** Private key for the executor wallet that processes payouts from the vault */
  executorPrivateKey: string;

  /** BIP-39 mnemonic for deriving the RAILGUN wallet */
  railgunMnemonic: string;

  /** Cron expression controlling when payout cycles run (default: weekly on Sunday midnight) */
  payoutCron: string;
  /** Minimum pending payout (in wei) before an operator is included in a cycle */
  minPayoutWei: bigint;

  /** When true the service logs intended payouts but does not execute any on-chain transactions */
  dryRun: boolean;

  /** TCP port for the /health HTTP endpoint */
  healthPort: number;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function requireEnv(name: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

function optionalEnv(name: string, fallback: string): string {
  const value = process.env[name];
  if (value === undefined || value === "") {
    return fallback;
  }
  return value;
}

function isValidAddress(address: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(address);
}

// ---------------------------------------------------------------------------
// Loader
// ---------------------------------------------------------------------------

export function loadConfig(): Config {
  const ethRpcUrl = requireEnv("ETH_RPC_URL");
  const chainId = Number(requireEnv("CHAIN_ID"));
  if (Number.isNaN(chainId) || chainId <= 0) {
    throw new Error("CHAIN_ID must be a positive integer");
  }

  const payoutVaultAddress = requireEnv("PAYOUT_VAULT_ADDRESS");
  if (!isValidAddress(payoutVaultAddress)) {
    throw new Error(`PAYOUT_VAULT_ADDRESS is not a valid Ethereum address: ${payoutVaultAddress}`);
  }

  const nodeRegistryAddress = requireEnv("NODE_REGISTRY_ADDRESS");
  if (!isValidAddress(nodeRegistryAddress)) {
    throw new Error(`NODE_REGISTRY_ADDRESS is not a valid Ethereum address: ${nodeRegistryAddress}`);
  }

  const executorPrivateKey = requireEnv("EXECUTOR_PRIVATE_KEY");
  if (!executorPrivateKey.startsWith("0x") || executorPrivateKey.length !== 66) {
    throw new Error("EXECUTOR_PRIVATE_KEY must be a 0x-prefixed 64-hex-char private key");
  }

  const railgunMnemonic = optionalEnv("RAILGUN_MNEMONIC", "");

  const payoutCron = optionalEnv("PAYOUT_CRON", "0 0 * * 0");

  const minPayoutWeiRaw = optionalEnv("MIN_PAYOUT_WEI", "10000000000000000"); // 0.01 ETH
  let minPayoutWei: bigint;
  try {
    minPayoutWei = BigInt(minPayoutWeiRaw);
  } catch {
    throw new Error(`MIN_PAYOUT_WEI is not a valid integer: ${minPayoutWeiRaw}`);
  }
  if (minPayoutWei <= 0n) {
    throw new Error("MIN_PAYOUT_WEI must be positive");
  }

  const dryRunRaw = optionalEnv("DRY_RUN", "true").toLowerCase();
  const dryRun = dryRunRaw === "true" || dryRunRaw === "1";

  const healthPortRaw = optionalEnv("HEALTH_PORT", "3001");
  const healthPort = Number(healthPortRaw);
  if (Number.isNaN(healthPort) || healthPort <= 0 || healthPort > 65535) {
    throw new Error(`HEALTH_PORT must be a valid port number: ${healthPortRaw}`);
  }

  const config: Config = {
    ethRpcUrl,
    chainId,
    payoutVaultAddress,
    nodeRegistryAddress,
    executorPrivateKey,
    railgunMnemonic,
    payoutCron,
    minPayoutWei,
    dryRun,
    healthPort,
  };

  return config;
}
