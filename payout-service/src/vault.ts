import { Contract, JsonRpcProvider, Wallet, type ContractTransactionResponse } from "ethers";

// ---------------------------------------------------------------------------
// PayoutVault ABI (minimal interface consumed by the payout service)
// ---------------------------------------------------------------------------

const PAYOUT_VAULT_ABI = [
  // --- State readers ---
  "function pendingPayouts(address operator) view returns (uint256)",
  "function totalPending() view returns (uint256)",
  "function processedPayouts(address operator) view returns (uint256)",
  "function payoutExecutor() view returns (address)",
  "function paused() view returns (bool)",

  // --- View helpers ---
  "function getPendingPayout(address operator) view returns (uint256)",
  "function getProcessedPayout(address operator) view returns (uint256)",

  // --- Executor actions ---
  "function processPayout(address operator, uint256 amount)",
  "function processBatchPayout(address[] operators, uint256[] amounts)",

  // --- Credit (called by authorized sources, not by this service, but useful for monitoring) ---
  "function creditOperator(address operator) payable",

  // --- Events ---
  "event OperatorCredited(address indexed operator, address indexed source, uint256 amount)",
  "event PayoutProcessed(address indexed operator, uint256 amount)",
  "event BatchPayoutProcessed(uint256 operatorCount, uint256 totalAmount)",
] as const;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PendingPayout {
  operator: string;
  amount: bigint;
}

// ---------------------------------------------------------------------------
// Contract factory
// ---------------------------------------------------------------------------

/**
 * Create an ethers.js v6 Contract instance connected to the PayoutVault.
 * Uses a Wallet signer so the service can submit transactions.
 */
export function createVaultContract(
  rpcUrl: string,
  address: string,
  privateKey: string,
): Contract {
  const provider = new JsonRpcProvider(rpcUrl);
  const wallet = new Wallet(privateKey, provider);
  return new Contract(address, PAYOUT_VAULT_ABI, wallet);
}

// ---------------------------------------------------------------------------
// Read helpers
// ---------------------------------------------------------------------------

/**
 * Batch-read pending payouts for a list of operator addresses.
 * Returns an array of { operator, amount } for every address queried
 * (including those with zero balance).
 */
export async function getPendingPayouts(
  contract: Contract,
  operators: string[],
): Promise<PendingPayout[]> {
  const results: PendingPayout[] = [];

  // Parallelize the view calls for performance.
  const promises = operators.map(async (operator) => {
    const amount: bigint = await contract.pendingPayouts(operator);
    return { operator, amount };
  });

  const settled = await Promise.allSettled(promises);
  for (const result of settled) {
    if (result.status === "fulfilled") {
      results.push(result.value);
    } else {
      console.error("[vault] Failed to read pending payout:", result.reason);
    }
  }

  return results;
}

/**
 * Read the total pending ETH held in the vault across all operators.
 */
export async function getTotalPending(contract: Contract): Promise<bigint> {
  return contract.totalPending() as Promise<bigint>;
}

/**
 * Check whether the vault contract is currently paused.
 */
export async function isPaused(contract: Contract): Promise<boolean> {
  return contract.paused() as Promise<boolean>;
}

// ---------------------------------------------------------------------------
// Write helpers
// ---------------------------------------------------------------------------

/**
 * Call `processBatchPayout` on the PayoutVault contract.
 * The vault will transfer the summed ETH to the executor wallet.
 *
 * @returns The transaction response (caller should await `.wait()` for confirmation).
 */
export async function processBatchPayout(
  contract: Contract,
  operators: string[],
  amounts: bigint[],
): Promise<ContractTransactionResponse> {
  if (operators.length !== amounts.length) {
    throw new Error(
      `Operators/amounts length mismatch: ${operators.length} vs ${amounts.length}`,
    );
  }
  if (operators.length === 0) {
    throw new Error("Cannot process an empty batch");
  }

  console.log(
    `[vault] Submitting processBatchPayout for ${operators.length} operators`,
  );

  const tx: ContractTransactionResponse = await contract.processBatchPayout(
    operators,
    amounts,
  );

  console.log(`[vault] Transaction submitted: ${tx.hash}`);
  return tx;
}

/**
 * Call `processPayout` for a single operator.
 *
 * @returns The transaction response.
 */
export async function processSinglePayout(
  contract: Contract,
  operator: string,
  amount: bigint,
): Promise<ContractTransactionResponse> {
  console.log(
    `[vault] Submitting processPayout for ${operator} amount=${amount}`,
  );

  const tx: ContractTransactionResponse = await contract.processPayout(
    operator,
    amount,
  );

  console.log(`[vault] Transaction submitted: ${tx.hash}`);
  return tx;
}
