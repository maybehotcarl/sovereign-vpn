import type { Contract } from "ethers";
import type { Config } from "../config.js";
import {
  getPendingPayouts,
  processBatchPayout,
  isPaused,
} from "../vault.js";
import { getOperatorsWithRailgun } from "../registry.js";
import { shieldETH } from "../railgun/shield.js";
import { sendPrivateTransfer } from "../railgun/transfer.js";
import { batchOperators, MAX_BATCH_SIZE } from "./batch.js";
import { withRetry } from "./retry.js";
import type { ReceiptStore } from "../receipts/store.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PayoutCycleResult {
  /** Total number of operators processed */
  operatorCount: number;
  /** Total ETH distributed (in wei) */
  totalAmount: bigint;
  /** Number of successful payouts */
  successCount: number;
  /** Number of failed payouts */
  failureCount: number;
  /** Timestamp when the cycle started */
  startedAt: Date;
  /** Timestamp when the cycle completed */
  completedAt: Date;
}

export interface EligibleOperator {
  /** Ethereum address of the operator */
  address: string;
  /** RAILGUN 0zk address (empty string if not set) */
  railgunAddress: string;
  /** Pending payout amount in wei */
  pendingAmount: bigint;
}

// ---------------------------------------------------------------------------
// PayoutProcessor
// ---------------------------------------------------------------------------

export class PayoutProcessor {
  private readonly config: Config;
  private readonly vaultContract: Contract;
  private readonly registryContract: Contract;
  private readonly receiptStore: ReceiptStore | null;
  private lastCycleResult: PayoutCycleResult | null = null;

  constructor(
    config: Config,
    vaultContract: Contract,
    registryContract: Contract,
    receiptStore: ReceiptStore | null = null,
  ) {
    this.config = config;
    this.vaultContract = vaultContract;
    this.registryContract = registryContract;
    this.receiptStore = receiptStore;
  }

  /**
   * Get the result of the last completed payout cycle (for health checks).
   */
  getLastCycleResult(): PayoutCycleResult | null {
    return this.lastCycleResult;
  }

  /**
   * Run a full payout cycle:
   *   1. Query pending payouts above threshold
   *   2. Match operators to RAILGUN addresses
   *   3. Process batch payout on-chain
   *   4. Shield and privately transfer (TODO: RAILGUN integration)
   *   5. Record receipts
   */
  async runPayoutCycle(): Promise<PayoutCycleResult> {
    const startedAt = new Date();
    console.log(`\n${"=".repeat(70)}`);
    console.log(`[processor] Starting payout cycle at ${startedAt.toISOString()}`);
    console.log(`${"=".repeat(70)}\n`);

    const result: PayoutCycleResult = {
      operatorCount: 0,
      totalAmount: 0n,
      successCount: 0,
      failureCount: 0,
      startedAt,
      completedAt: startedAt,
    };

    try {
      // --- Pre-flight checks ---
      const vaultPaused = await isPaused(this.vaultContract);
      if (vaultPaused) {
        console.warn("[processor] PayoutVault is paused. Skipping cycle.");
        result.completedAt = new Date();
        this.lastCycleResult = result;
        return result;
      }

      // --- Step 1: Get eligible operators ---
      const eligible = await this.getEligibleOperators();
      if (eligible.length === 0) {
        console.log("[processor] No eligible operators found. Skipping cycle.");
        result.completedAt = new Date();
        this.lastCycleResult = result;
        return result;
      }

      result.operatorCount = eligible.length;
      result.totalAmount = eligible.reduce((sum, op) => sum + op.pendingAmount, 0n);

      console.log(
        `[processor] Found ${eligible.length} eligible operators, ` +
        `total pending: ${result.totalAmount} wei`,
      );

      // --- Dry run: log and exit ---
      if (this.config.dryRun) {
        console.log("\n[processor] *** DRY RUN MODE - No transactions will be sent ***\n");
        for (const op of eligible) {
          console.log(
            `  [dry-run] ${op.address} => ${op.pendingAmount} wei ` +
            `(0zk: ${op.railgunAddress || "NOT SET"})`,
          );
        }
        console.log(`\n[processor] Dry run complete. Would process ${eligible.length} payouts.\n`);
        result.successCount = eligible.length;
        result.completedAt = new Date();
        this.lastCycleResult = result;
        return result;
      }

      // --- Step 2: Process batch payout on-chain ---
      const batches = batchOperators(eligible, MAX_BATCH_SIZE);
      console.log(
        `[processor] Split into ${batches.length} batch(es) of max ${MAX_BATCH_SIZE}`,
      );

      for (let i = 0; i < batches.length; i++) {
        const batch = batches[i];
        console.log(
          `[processor] Processing batch ${i + 1}/${batches.length} (${batch.length} operators)`,
        );

        const operators = batch.map((op) => op.address);
        const amounts = batch.map((op) => op.pendingAmount);

        try {
          const tx = await withRetry(
            () => processBatchPayout(this.vaultContract, operators, amounts),
            3,
            2000,
          );

          const receipt = await tx.wait();
          const txHash = receipt?.hash ?? tx.hash;
          console.log(`[processor] Batch ${i + 1} confirmed: ${txHash}`);

          // Record receipts for each operator in this batch
          for (const op of batch) {
            this.recordReceipt(op, txHash);
            result.successCount++;
          }
        } catch (err) {
          console.error(`[processor] Batch ${i + 1} failed:`, err);
          result.failureCount += batch.length;
        }
      }

      // --- Step 3: Shield received ETH via RAILGUN (TODO) ---
      //
      // After processBatchPayout, the executor wallet holds the sum of all
      // payout amounts as ETH. We need to shield it into RAILGUN:
      //
      //   const shieldResult = await shieldETH(result.totalAmount, railgunWalletId);
      //
      // For now we log a placeholder:
      if (result.successCount > 0) {
        console.log(
          `[processor] TODO: Shield ${result.totalAmount} wei into RAILGUN`,
        );
        const _shieldResult = await shieldETH(result.totalAmount, "TODO_WALLET_ID");
        if (!_shieldResult.success) {
          console.warn(
            `[processor] Shield stub returned: ${_shieldResult.error}`,
          );
        }
      }

      // --- Step 4: Send private transfers to each operator (TODO) ---
      //
      // After shielding, send individual private transfers:
      //
      //   for (const op of eligible) {
      //     await sendPrivateTransfer(walletId, op.railgunAddress, op.pendingAmount, WETH);
      //   }
      //
      if (result.successCount > 0) {
        console.log(
          `[processor] TODO: Send ${eligible.length} private transfers via RAILGUN`,
        );
        for (const op of eligible) {
          if (op.railgunAddress) {
            const _transferResult = await sendPrivateTransfer(
              "TODO_WALLET_ID",
              op.railgunAddress,
              op.pendingAmount,
              "TODO_WETH_ADDRESS",
            );
            if (!_transferResult.success) {
              console.warn(
                `[processor] Transfer stub for ${op.address}: ${_transferResult.error}`,
              );
            }
          }
        }
      }
    } catch (err) {
      console.error("[processor] Payout cycle failed with unhandled error:", err);
    }

    result.completedAt = new Date();
    this.lastCycleResult = result;

    console.log(`\n${"=".repeat(70)}`);
    console.log(
      `[processor] Cycle complete. ` +
      `Success: ${result.successCount}, Failed: ${result.failureCount}, ` +
      `Total: ${result.totalAmount} wei`,
    );
    console.log(`${"=".repeat(70)}\n`);

    return result;
  }

  /**
   * Get all operators eligible for payout:
   *   - Pending balance >= minPayoutWei
   *   - Has a RAILGUN 0zk address registered on-chain
   */
  async getEligibleOperators(): Promise<EligibleOperator[]> {
    // Fetch RAILGUN addresses for all registered operators
    const railgunMap = await getOperatorsWithRailgun(this.registryContract);
    const operatorAddresses = Array.from(railgunMap.keys());

    if (operatorAddresses.length === 0) {
      console.log("[processor] No operators have RAILGUN addresses set");
      return [];
    }

    // Fetch pending payouts from the vault
    const pendingPayouts = await getPendingPayouts(
      this.vaultContract,
      operatorAddresses,
    );

    // Filter by minimum payout threshold
    const eligible: EligibleOperator[] = [];
    for (const payout of pendingPayouts) {
      if (payout.amount >= this.config.minPayoutWei) {
        const railgunAddress = railgunMap.get(payout.operator) ?? "";
        eligible.push({
          address: payout.operator,
          railgunAddress,
          pendingAmount: payout.amount,
        });
      }
    }

    // Sort by pending amount descending (largest payouts first)
    eligible.sort((a, b) => {
      if (a.pendingAmount > b.pendingAmount) return -1;
      if (a.pendingAmount < b.pendingAmount) return 1;
      return 0;
    });

    return eligible;
  }

  /**
   * Record a payout receipt to the SQLite store.
   */
  private recordReceipt(
    operator: EligibleOperator,
    txHash: string,
    railgunTxId?: string,
  ): void {
    if (!this.receiptStore) return;

    try {
      this.receiptStore.recordPayout(
        operator.address,
        operator.pendingAmount.toString(),
        txHash,
        railgunTxId ?? null,
      );
    } catch (err) {
      console.error(
        `[processor] Failed to record receipt for ${operator.address}:`,
        err,
      );
    }
  }
}
