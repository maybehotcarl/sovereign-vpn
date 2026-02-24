import type { EligibleOperator } from "./processor.js";

// ---------------------------------------------------------------------------
// Batching Utilities
// ---------------------------------------------------------------------------

/**
 * Maximum number of operators to include in a single on-chain
 * `processBatchPayout` call. Larger batches save gas per operator but
 * risk hitting the block gas limit.
 */
export const MAX_BATCH_SIZE = 50;

/**
 * Split an array of eligible operators into batches of at most `maxBatchSize`.
 *
 * @param operators  The full list of operators to batch
 * @param maxBatchSize  Maximum operators per batch (defaults to MAX_BATCH_SIZE)
 * @returns An array of operator batches
 */
export function batchOperators(
  operators: EligibleOperator[],
  maxBatchSize: number = MAX_BATCH_SIZE,
): EligibleOperator[][] {
  if (maxBatchSize <= 0) {
    throw new Error(`maxBatchSize must be positive, got ${maxBatchSize}`);
  }

  if (operators.length === 0) {
    return [];
  }

  const batches: EligibleOperator[][] = [];
  for (let i = 0; i < operators.length; i += maxBatchSize) {
    batches.push(operators.slice(i, i + maxBatchSize));
  }

  return batches;
}
