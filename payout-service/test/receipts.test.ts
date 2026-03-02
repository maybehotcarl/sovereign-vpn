import { describe, expect, it } from "vitest";
import { ReceiptStore } from "../src/receipts/store.js";

describe("ReceiptStore", () => {
  it("upserts duplicate operator+tx_hash rows instead of inserting duplicates", () => {
    const store = new ReceiptStore(":memory:");
    try {
      store.recordPayout("0xA", "100", "0xvault", null);
      store.recordPayout("0xA", "100", "0xvault", "0xrailgun");

      const rows = store.getPayouts("0xA");
      expect(rows).toHaveLength(1);
      expect(rows[0].tx_hash).toBe("0xvault");
      expect(rows[0].railgun_tx_id).toBe("0xrailgun");
    } finally {
      store.close();
    }
  });

  it("sums completed payout amounts with bigint precision", () => {
    const store = new ReceiptStore(":memory:");
    try {
      const hugeWei = "340282366920938463463374607431768211455"; // 2^128 - 1
      store.recordPayout("0xA", hugeWei, "0x1");
      store.recordPayout("0xB", "5", "0x2");

      const stats = store.getStats();
      expect(stats.totalPayouts).toBe(2);
      expect(stats.uniqueOperators).toBe(2);
      expect(stats.totalAmountWei).toBe(
        (BigInt(hugeWei) + 5n).toString(),
      );
    } finally {
      store.close();
    }
  });
});
