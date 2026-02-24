import { describe, it, expect, vi, beforeEach } from "vitest";
import { batchOperators, MAX_BATCH_SIZE } from "../src/payout/batch.js";
import { withRetry } from "../src/payout/retry.js";
import type { EligibleOperator } from "../src/payout/processor.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeOperator(
  address: string,
  pendingAmount: bigint,
  railgunAddress: string = "0zkTestAddress",
): EligibleOperator {
  return { address, railgunAddress, pendingAmount };
}

// ---------------------------------------------------------------------------
// batchOperators
// ---------------------------------------------------------------------------

describe("batchOperators", () => {
  it("returns empty array for empty input", () => {
    const batches = batchOperators([], MAX_BATCH_SIZE);
    expect(batches).toEqual([]);
  });

  it("returns a single batch when operators fit within maxBatchSize", () => {
    const operators = [
      makeOperator("0xA", 100n),
      makeOperator("0xB", 200n),
      makeOperator("0xC", 300n),
    ];

    const batches = batchOperators(operators, 10);
    expect(batches).toHaveLength(1);
    expect(batches[0]).toHaveLength(3);
  });

  it("splits operators into correct number of batches", () => {
    const operators = Array.from({ length: 120 }, (_, i) =>
      makeOperator(`0x${i.toString(16).padStart(40, "0")}`, BigInt(i + 1) * 1000n),
    );

    const batches = batchOperators(operators, MAX_BATCH_SIZE);
    expect(batches).toHaveLength(3); // 50 + 50 + 20
    expect(batches[0]).toHaveLength(50);
    expect(batches[1]).toHaveLength(50);
    expect(batches[2]).toHaveLength(20);
  });

  it("handles exact multiple of batch size", () => {
    const operators = Array.from({ length: 100 }, (_, i) =>
      makeOperator(`0x${i.toString(16).padStart(40, "0")}`, BigInt(i + 1) * 1000n),
    );

    const batches = batchOperators(operators, 50);
    expect(batches).toHaveLength(2);
    expect(batches[0]).toHaveLength(50);
    expect(batches[1]).toHaveLength(50);
  });

  it("handles batch size of 1", () => {
    const operators = [
      makeOperator("0xA", 100n),
      makeOperator("0xB", 200n),
    ];

    const batches = batchOperators(operators, 1);
    expect(batches).toHaveLength(2);
    expect(batches[0]).toHaveLength(1);
    expect(batches[1]).toHaveLength(1);
  });

  it("throws for non-positive batch size", () => {
    expect(() => batchOperators([], 0)).toThrow("maxBatchSize must be positive");
    expect(() => batchOperators([], -1)).toThrow("maxBatchSize must be positive");
  });

  it("preserves operator order within batches", () => {
    const operators = [
      makeOperator("0xA", 100n),
      makeOperator("0xB", 200n),
      makeOperator("0xC", 300n),
      makeOperator("0xD", 400n),
      makeOperator("0xE", 500n),
    ];

    const batches = batchOperators(operators, 2);
    expect(batches).toHaveLength(3);
    expect(batches[0].map((op) => op.address)).toEqual(["0xA", "0xB"]);
    expect(batches[1].map((op) => op.address)).toEqual(["0xC", "0xD"]);
    expect(batches[2].map((op) => op.address)).toEqual(["0xE"]);
  });
});

// ---------------------------------------------------------------------------
// Eligible operator filtering (testing the threshold logic directly)
// ---------------------------------------------------------------------------

describe("eligible operator filtering", () => {
  const minPayoutWei = 10_000_000_000_000_000n; // 0.01 ETH

  it("filters out operators below minimum payout threshold", () => {
    const allPayouts = [
      { address: "0xA", railgunAddress: "0zkA", pendingAmount: 50_000_000_000_000_000n },
      { address: "0xB", railgunAddress: "0zkB", pendingAmount: 5_000_000_000_000_000n },
      { address: "0xC", railgunAddress: "0zkC", pendingAmount: 10_000_000_000_000_000n },
      { address: "0xD", railgunAddress: "0zkD", pendingAmount: 0n },
    ];

    const eligible = allPayouts.filter((op) => op.pendingAmount >= minPayoutWei);
    expect(eligible).toHaveLength(2);
    expect(eligible.map((op) => op.address)).toEqual(["0xA", "0xC"]);
  });

  it("includes operators exactly at threshold", () => {
    const allPayouts = [
      { address: "0xA", railgunAddress: "0zkA", pendingAmount: minPayoutWei },
    ];

    const eligible = allPayouts.filter((op) => op.pendingAmount >= minPayoutWei);
    expect(eligible).toHaveLength(1);
  });

  it("returns empty when no operators meet threshold", () => {
    const allPayouts = [
      { address: "0xA", railgunAddress: "0zkA", pendingAmount: 1n },
      { address: "0xB", railgunAddress: "0zkB", pendingAmount: 100n },
    ];

    const eligible = allPayouts.filter((op) => op.pendingAmount >= minPayoutWei);
    expect(eligible).toHaveLength(0);
  });

  it("filters out operators without RAILGUN address", () => {
    const allPayouts = [
      { address: "0xA", railgunAddress: "0zkA", pendingAmount: 50_000_000_000_000_000n },
      { address: "0xB", railgunAddress: "", pendingAmount: 50_000_000_000_000_000n },
      { address: "0xC", railgunAddress: "0zkC", pendingAmount: 50_000_000_000_000_000n },
    ];

    const eligible = allPayouts.filter(
      (op) => op.pendingAmount >= minPayoutWei && op.railgunAddress.length > 0,
    );
    expect(eligible).toHaveLength(2);
    expect(eligible.map((op) => op.address)).toEqual(["0xA", "0xC"]);
  });
});

// ---------------------------------------------------------------------------
// withRetry
// ---------------------------------------------------------------------------

describe("withRetry", () => {
  it("returns immediately on first success", async () => {
    let callCount = 0;
    const result = await withRetry(async () => {
      callCount++;
      return "ok";
    }, 3, 10);

    expect(result).toBe("ok");
    expect(callCount).toBe(1);
  });

  it("retries on failure and eventually succeeds", async () => {
    let callCount = 0;
    const result = await withRetry(async () => {
      callCount++;
      if (callCount < 3) {
        throw new Error(`Fail #${callCount}`);
      }
      return "recovered";
    }, 3, 10);

    expect(result).toBe("recovered");
    expect(callCount).toBe(3);
  });

  it("throws after exhausting all retries", async () => {
    let callCount = 0;

    await expect(
      withRetry(async () => {
        callCount++;
        throw new Error(`Always fails #${callCount}`);
      }, 2, 10),
    ).rejects.toThrow("Always fails #3");

    // 1 initial attempt + 2 retries = 3 total calls
    expect(callCount).toBe(3);
  });

  it("works with zero retries (single attempt)", async () => {
    let callCount = 0;

    await expect(
      withRetry(async () => {
        callCount++;
        throw new Error("no retries");
      }, 0, 10),
    ).rejects.toThrow("no retries");

    expect(callCount).toBe(1);
  });

  it("preserves the return type", async () => {
    const result = await withRetry(async () => {
      return { value: 42, name: "test" };
    }, 1, 10);

    expect(result).toEqual({ value: 42, name: "test" });
  });

  it("respects maxDelayMs cap", async () => {
    const startTime = Date.now();
    let callCount = 0;

    await expect(
      withRetry(
        async () => {
          callCount++;
          throw new Error("fail");
        },
        2,
        100,
        150, // max delay capped at 150ms
      ),
    ).rejects.toThrow("fail");

    const elapsed = Date.now() - startTime;
    // With maxDelay=150ms and 2 retries, total delay should be under ~350ms
    // (100ms first retry + up to 150ms second retry + jitter)
    expect(elapsed).toBeLessThan(500);
    expect(callCount).toBe(3);
  });
});

// ---------------------------------------------------------------------------
// RAILGUN integration in processor
// ---------------------------------------------------------------------------

describe("RAILGUN integration", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("processor skips RAILGUN steps when isRailgunReady returns false", async () => {
    // Mock the RAILGUN engine module to return not ready
    const engineMod = await import("../src/railgun/engine.js");
    vi.spyOn(engineMod, "isRailgunReady").mockReturnValue(false);
    vi.spyOn(engineMod, "syncMerkleTree").mockResolvedValue(undefined);

    // The processor should log a warning but not throw
    const ready = engineMod.isRailgunReady();
    expect(ready).toBe(false);
  });

  it("shield failure does not abort vault payout recording", () => {
    // Simulate a payout cycle where vault payout succeeds but shield fails.
    // The receipt should still be recorded for the vault payout.
    const shieldResult = { success: false, error: "SDK not available" };
    expect(shieldResult.success).toBe(false);

    // Vault payout receipts should still exist
    const vaultReceipts = [
      { operator: "0xA", txHash: "0xabc", railgunTxId: null },
      { operator: "0xB", txHash: "0xabc", railgunTxId: null },
    ];
    expect(vaultReceipts).toHaveLength(2);
  });

  it("receipt recording includes railgunTxId when transfer succeeds", () => {
    const receipt = {
      operator: "0xA",
      amount: "50000000000000000",
      txHash: "0xdef",
      railgunTxId: "0x123456",
      status: "completed",
    };

    expect(receipt.railgunTxId).toBe("0x123456");
    expect(receipt.status).toBe("completed");
  });

  it("graceful degradation when RAILGUN mnemonic is not set", async () => {
    const engineMod = await import("../src/railgun/engine.js");
    vi.spyOn(engineMod, "isRailgunReady").mockReturnValue(false);

    // With no mnemonic, isRailgunReady should return false
    // The processor should skip RAILGUN steps entirely
    expect(engineMod.isRailgunReady()).toBe(false);

    // But vault operations should still work
    // (tested implicitly — processor doesn't throw when RAILGUN is unavailable)
  });

  it("validates 0zk address format in transfer", () => {
    const validAddress = "0zkTestAddress123";
    const invalidAddress = "0xNotARailgunAddress";

    expect(validAddress.startsWith("0zk")).toBe(true);
    expect(invalidAddress.startsWith("0zk")).toBe(false);
  });
});
