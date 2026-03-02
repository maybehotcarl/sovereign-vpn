import cron from "node-cron";
import type { Server } from "node:http";
import type { ScheduledTask } from "node-cron";

import { loadConfig, verifyChainId, type Config } from "./config.js";
import { createVaultContract } from "./vault.js";
import { createRegistryContract } from "./registry.js";
import { createExecutorWallet } from "./wallet.js";
import { initRailgunEngine } from "./railgun/engine.js";
import { PayoutProcessor } from "./payout/processor.js";
import { ReceiptStore } from "./receipts/store.js";
import { startHealthServer, type HealthStatus } from "./monitoring/health.js";

// ---------------------------------------------------------------------------
// Service state
// ---------------------------------------------------------------------------

let config: Config;
let processor: PayoutProcessor;
let receiptStore: ReceiptStore;
let healthServer: Server;
let cronTask: ScheduledTask;
let isRunning = false;
let lastRunAt: string | null = null;
let nextRunAt: string | null = null;
let pendingOperatorCount = 0;

// ---------------------------------------------------------------------------
// Graceful shutdown
// ---------------------------------------------------------------------------

function shutdown(signal: string): void {
  console.log(`\n[main] Received ${signal}. Shutting down gracefully...`);

  if (cronTask) {
    cronTask.stop();
    console.log("[main] Cron task stopped");
  }

  if (healthServer) {
    healthServer.close(() => {
      console.log("[main] Health server closed");
    });
  }

  if (receiptStore) {
    receiptStore.close();
    console.log("[main] Receipt store closed");
  }

  console.log("[main] Shutdown complete");
  process.exit(0);
}

// ---------------------------------------------------------------------------
// Payout cycle wrapper
// ---------------------------------------------------------------------------

async function runCycle(): Promise<void> {
  if (isRunning) {
    console.warn("[main] Previous payout cycle still running. Skipping this invocation.");
    return;
  }

  isRunning = true;

  try {
    const result = await processor.runPayoutCycle();
    lastRunAt = result.completedAt.toISOString();
    pendingOperatorCount = result.operatorCount;

    console.log(
      `[main] Cycle finished: ${result.successCount} succeeded, ` +
      `${result.failureCount} failed`,
    );
  } catch (err) {
    console.error("[main] Payout cycle threw an unhandled error:", err);
  } finally {
    isRunning = false;
  }
}

// ---------------------------------------------------------------------------
// Health status provider
// ---------------------------------------------------------------------------

function getHealthStatus(): HealthStatus {
  return {
    running: true,
    lastRunAt,
    nextRunAt,
    pendingOperatorCount,
    dryRun: config?.dryRun ?? true,
    uptimeSeconds: 0, // populated by the health server
  };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main(): Promise<void> {
  console.log("=".repeat(70));
  console.log("  Sovereign VPN - Payout Service");
  console.log("=".repeat(70));
  console.log();

  // 1. Load configuration
  try {
    config = loadConfig();
  } catch (err) {
    console.error("[main] Configuration error:", err);
    process.exit(1);
  }

  console.log(`[main] Chain ID:       ${config.chainId}`);
  console.log(`[main] Vault:          ${config.payoutVaultAddress}`);
  console.log(`[main] Registry:       ${config.nodeRegistryAddress}`);
  console.log(`[main] Min payout:     ${config.minPayoutWei} wei`);
  console.log(`[main] Cron schedule:  ${config.payoutCron}`);
  console.log(`[main] Dry run:        ${config.dryRun}`);
  console.log();

  // 1b. Verify provider chain ID matches configuration
  try {
    await verifyChainId(config);
    console.log(`[main] Provider chain ID verified: ${config.chainId}`);
  } catch (err) {
    console.error("[main] Chain ID verification failed:", err);
    process.exit(1);
  }

  // 1c. Warn if RAILGUN mnemonic is absent
  if (!config.railgunMnemonic) {
    console.warn(
      "[main] WARNING: RAILGUN_MNEMONIC is not set. " +
      "Private transfers are disabled — all payouts will remain in the executor wallet " +
      "and be sent as regular on-chain transactions.",
    );
  }

  // 2. Initialize contracts
  const vaultContract = createVaultContract(
    config.ethRpcUrl,
    config.payoutVaultAddress,
    config.executorPrivateKey,
  );

  const registryContract = createRegistryContract(
    config.ethRpcUrl,
    config.nodeRegistryAddress,
  );

  // 3. Create executor wallet (shared between vault and RAILGUN operations)
  const executorWallet = createExecutorWallet(config.ethRpcUrl, config.executorPrivateKey);
  console.log(`[main] Executor wallet: ${executorWallet.address}`);

  // 4. Initialize RAILGUN engine
  await initRailgunEngine(config);

  // 5. Initialize receipt store
  const dbPath = process.env["RECEIPT_DB_PATH"] ?? "./data/receipts.db";
  try {
    receiptStore = new ReceiptStore(dbPath);
    console.log(`[main] Receipt store opened at ${dbPath}`);
  } catch (err) {
    console.error(`[main] Failed to open receipt store at ${dbPath}:`, err);
    process.exit(1);
  }

  // 6. Create payout processor
  processor = new PayoutProcessor(config, vaultContract, registryContract, receiptStore, executorWallet);

  // 7. Start health server
  healthServer = startHealthServer(config.healthPort, getHealthStatus);

  // 8. Schedule cron job
  if (!cron.validate(config.payoutCron)) {
    console.error(`[main] Invalid cron expression: ${config.payoutCron}`);
    process.exit(1);
  }

  cronTask = cron.schedule(config.payoutCron, () => {
    console.log(`[main] Cron triggered at ${new Date().toISOString()}`);
    void runCycle();
  });

  // Compute the next approximate run time for health reporting
  nextRunAt = `Scheduled: ${config.payoutCron}`;

  console.log(`[main] Cron job scheduled: ${config.payoutCron}`);
  console.log("[main] Payout service is running. Press Ctrl+C to stop.\n");

  // 9. Register signal handlers
  process.on("SIGINT", () => shutdown("SIGINT"));
  process.on("SIGTERM", () => shutdown("SIGTERM"));

  // 10. Optionally run an immediate cycle on startup
  if (process.env["RUN_ON_STARTUP"] === "true") {
    console.log("[main] RUN_ON_STARTUP=true, executing immediate payout cycle...");
    await runCycle();
  }
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

main().catch((err) => {
  console.error("[main] Fatal error:", err);
  process.exit(1);
});
