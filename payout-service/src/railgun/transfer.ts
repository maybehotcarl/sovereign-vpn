import type { Wallet } from "ethers";
import {
  generateTransferProof,
  populateProvedTransfer,
} from "@railgun-community/wallet";
import {
  NetworkName,
  TXIDVersion,
  EVMGasType,
  type RailgunERC20AmountRecipient,
  type TransactionGasDetails,
} from "@railgun-community/shared-models";
import { getWalletId, getEncryptionKey } from "./engine.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface PrivateTransferResult {
  success: boolean;
  railgunTxId?: string;
  txHash?: string;
  error?: string;
}

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

// ---------------------------------------------------------------------------
// Private Transfer
// ---------------------------------------------------------------------------

/**
 * Send a private (0zk-to-0zk) transfer of shielded WETH within RAILGUN.
 *
 * Flow:
 *   1. Validate 0zk address
 *   2. Build recipients
 *   3. Generate ZK proof (~10-30s)
 *   4. Populate proved transaction
 *   5. Submit on-chain via executor wallet
 */
export async function sendPrivateTransfer(
  toRailgunAddress: string,
  amount: bigint,
  wethAddress: string,
  walletSigner: Wallet,
  chainId: number,
  sendWithPublicWallet: boolean = true,
): Promise<PrivateTransferResult> {
  console.log(
    `[railgun/transfer] Private transfer of ${amount} wei to ${toRailgunAddress}`,
  );

  // Validate the recipient address format
  if (!toRailgunAddress.startsWith("0zk")) {
    return {
      success: false,
      error: `Invalid RAILGUN address (must start with "0zk"): ${toRailgunAddress}`,
    };
  }

  try {
    const networkNameValue = chainIdToNetworkName(chainId);
    const walletId = getWalletId();
    const encKey = getEncryptionKey();

    // Build recipients
    const erc20AmountRecipients: RailgunERC20AmountRecipient[] = [
      {
        tokenAddress: wethAddress,
        amount,
        recipientAddress: toRailgunAddress,
      },
    ];

    // Generate ZK proof (CPU-intensive, ~10-30 seconds)
    console.log("[railgun/transfer] Generating ZK proof...");
    await generateTransferProof(
      TXIDVersion.V2_PoseidonMerkle,
      networkNameValue,
      walletId,
      encKey,
      false,                        // showSenderAddressToRecipient
      undefined,                    // memoText
      erc20AmountRecipients,
      [],                           // no NFTs
      undefined,                    // no broadcaster fee
      sendWithPublicWallet,
      undefined,                    // overallBatchMinGasPrice
      (progress: number, status: string) => {
        console.log(`[railgun/transfer] Proof progress: ${progress}% - ${status}`);
      },
    );
    console.log("[railgun/transfer] Proof generated");

    // Estimate gas
    const feeData = await walletSigner.provider!.getFeeData();
    const gasDetails: TransactionGasDetails = feeData.maxFeePerGas
      ? {
          evmGasType: EVMGasType.Type2,
          gasEstimate: 1_500_000n, // generous estimate for proved transfer
          maxFeePerGas: feeData.maxFeePerGas,
          maxPriorityFeePerGas: feeData.maxPriorityFeePerGas ?? 1_500_000_000n,
        }
      : {
          evmGasType: EVMGasType.Type0,
          gasEstimate: 1_500_000n,
          gasPrice: feeData.gasPrice ?? 20_000_000_000n,
        };

    // Populate the proved transfer transaction
    console.log("[railgun/transfer] Populating proved transfer...");
    const { transaction } = await populateProvedTransfer(
      TXIDVersion.V2_PoseidonMerkle,
      networkNameValue,
      walletId,
      false,                        // showSenderAddressToRecipient
      undefined,                    // memoText
      erc20AmountRecipients,
      [],                           // no NFTs
      undefined,                    // no broadcaster fee
      sendWithPublicWallet,
      undefined,                    // overallBatchMinGasPrice
      gasDetails,
    );

    // Submit on-chain
    console.log("[railgun/transfer] Submitting transfer transaction...");
    const tx = await walletSigner.sendTransaction(transaction);
    const receipt = await tx.wait();
    const txHash = receipt?.hash ?? tx.hash;

    console.log(`[railgun/transfer] Transfer confirmed: ${txHash}`);

    return {
      success: true,
      txHash,
      railgunTxId: txHash, // use on-chain hash as identifier
    };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error(`[railgun/transfer] Transfer failed:`, message);
    return { success: false, error: message };
  }
}
