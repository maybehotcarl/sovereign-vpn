import Database from "better-sqlite3";

// ---------------------------------------------------------------------------
// Payout Receipt Storage (SQLite)
// ---------------------------------------------------------------------------

export interface PayoutReceipt {
  id: number;
  operator: string;
  amount: string;
  tx_hash: string;
  railgun_tx_id: string | null;
  status: string;
  created_at: string;
}

/**
 * Persistent receipt storage backed by SQLite via better-sqlite3.
 *
 * Every successful (or attempted) payout is recorded here so operators
 * and the service admin can audit the payout history.
 */
export class ReceiptStore {
  private readonly db: Database.Database;

  /**
   * Open (or create) the SQLite database at `dbPath` and ensure the
   * `payout_receipts` table exists.
   *
   * @param dbPath  File path for the SQLite database (e.g., "./data/receipts.db")
   */
  constructor(dbPath: string) {
    this.db = new Database(dbPath);

    // Enable WAL mode for better concurrent read performance
    this.db.pragma("journal_mode = WAL");

    this.db.exec(`
      CREATE TABLE IF NOT EXISTS payout_receipts (
        id             INTEGER PRIMARY KEY AUTOINCREMENT,
        operator       TEXT    NOT NULL,
        amount         TEXT    NOT NULL,
        tx_hash        TEXT    NOT NULL,
        railgun_tx_id  TEXT,
        status         TEXT    NOT NULL DEFAULT 'completed',
        created_at     TEXT    NOT NULL DEFAULT (datetime('now'))
      )
    `);

    // Index for operator queries
    this.db.exec(`
      CREATE INDEX IF NOT EXISTS idx_payout_receipts_operator
        ON payout_receipts (operator)
    `);

    // Index for time-ordered queries
    this.db.exec(`
      CREATE INDEX IF NOT EXISTS idx_payout_receipts_created_at
        ON payout_receipts (created_at DESC)
    `);

    // Speed up idempotent upsert lookups by operator+tx_hash.
    this.db.exec(`
      CREATE INDEX IF NOT EXISTS idx_payout_receipts_operator_tx_hash
        ON payout_receipts (operator, tx_hash)
    `);
  }

  /**
   * Record a payout receipt.
   *
   * @param operator      Ethereum address of the operator
   * @param amount        Payout amount in wei (stored as TEXT to preserve precision)
   * @param txHash        On-chain transaction hash from processBatchPayout
   * @param railgunTxId   RAILGUN private transfer ID (null if not yet available)
   * @param status        Receipt status (default: "completed")
   */
  recordPayout(
    operator: string,
    amount: string,
    txHash: string,
    railgunTxId: string | null = null,
    status: string = "completed",
  ): void {
    const updateStmt = this.db.prepare(`
      UPDATE payout_receipts
      SET
        amount = ?,
        railgun_tx_id = COALESCE(?, railgun_tx_id),
        status = ?
      WHERE operator = ? AND tx_hash = ?
    `);

    const insertStmt = this.db.prepare(`
      INSERT INTO payout_receipts (operator, amount, tx_hash, railgun_tx_id, status)
      VALUES (?, ?, ?, ?, ?)
    `);

    const tx = this.db.transaction(() => {
      const result = updateStmt.run(amount, railgunTxId, status, operator, txHash);
      if (result.changes === 0) {
        insertStmt.run(operator, amount, txHash, railgunTxId, status);
      }
    });

    tx();
  }

  /**
   * Query all payout receipts for a specific operator, ordered by most recent first.
   *
   * @param operator  Ethereum address of the operator
   */
  getPayouts(operator: string): PayoutReceipt[] {
    const stmt = this.db.prepare(`
      SELECT id, operator, amount, tx_hash, railgun_tx_id, status, created_at
      FROM payout_receipts
      WHERE operator = ?
      ORDER BY created_at DESC
    `);

    return stmt.all(operator) as PayoutReceipt[];
  }

  /**
   * Get the most recent N payout receipts across all operators.
   *
   * @param limit  Maximum number of receipts to return (default: 50)
   */
  getRecentPayouts(limit: number = 50): PayoutReceipt[] {
    const stmt = this.db.prepare(`
      SELECT id, operator, amount, tx_hash, railgun_tx_id, status, created_at
      FROM payout_receipts
      ORDER BY created_at DESC
      LIMIT ?
    `);

    return stmt.all(limit) as PayoutReceipt[];
  }

  /**
   * Get aggregate statistics for the receipt store.
   */
  getStats(): { totalPayouts: number; totalAmountWei: string; uniqueOperators: number } {
    const rows = this.db.prepare(`
      SELECT
        pr.operator as operator,
        pr.amount as amount
      FROM payout_receipts pr
      INNER JOIN (
        SELECT operator, tx_hash, MAX(id) AS keep_id
        FROM payout_receipts
        WHERE status = 'completed'
        GROUP BY operator, tx_hash
      ) dedup ON pr.id = dedup.keep_id
    `).all() as Array<{ operator: string; amount: string }>;

    let totalAmountWei = 0n;
    const operators = new Set<string>();
    for (const row of rows) {
      totalAmountWei += BigInt(row.amount);
      operators.add(row.operator);
    }

    return {
      totalPayouts: rows.length,
      totalAmountWei: totalAmountWei.toString(),
      uniqueOperators: operators.size,
    };
  }

  /**
   * Close the database connection. Call this during graceful shutdown.
   */
  close(): void {
    this.db.close();
  }
}
