import { Contract, type Wallet } from "ethers";
import {
  populateShield,
  getShieldPrivateKeySignatureMessage,
} from "@railgun-community/wallet";
import {
  NetworkName,
  NETWORK_CONFIG,
  TXIDVersion,
  EVMGasType,
  type RailgunERC20AmountRecipient,
  type TransactionGasDetails,
} from "@railgun-community/shared-models";
import type { Config } from "../config.js";
import { getWalletId } from "./engine.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ShieldResult {
  success: boolean;
  txHash?: string;
  error?: string;
}

// ---------------------------------------------------------------------------
// WETH ABI (minimal)
// ---------------------------------------------------------------------------

const WETH_ABI = [
  "function deposit() payable",
  "function approve(address spender, uint256 amount) returns (bool)",
] as const;

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
// Shield ETH
// ---------------------------------------------------------------------------

/**
 * Shield ETH into the RAILGUN private balance:
 *   1. Wrap ETH → WETH
 *   2. Approve WETH spend to RAILGUN proxy contract
 *   3. Populate and submit the shield transaction
 */
export async function shieldETH(
  amount: bigint,
  walletSigner: Wallet,
  config: Config,
): Promise<ShieldResult> {
  console.log(`[railgun/shield] Shielding ${amount} wei ETH...`);

  try {
    const networkNameValue = chainIdToNetworkName(config.chainId);
    const network = NETWORK_CONFIG[networkNameValue];

    // 1. Wrap ETH → WETH
    console.log("[railgun/shield] Wrapping ETH to WETH...");
    const wethContract = new Contract(config.wethAddress, WETH_ABI, walletSigner);
    const wrapTx = await wethContract.deposit({ value: amount });
    await wrapTx.wait();
    console.log(`[railgun/shield] WETH wrap confirmed: ${wrapTx.hash}`);

    // 2. Approve RAILGUN proxy to spend WETH
    const proxyAddress = network.proxyContract;
    console.log(`[railgun/shield] Approving RAILGUN proxy ${proxyAddress}...`);
    const approveTx = await wethContract.approve(proxyAddress, amount);
    await approveTx.wait();
    console.log(`[railgun/shield] Approval confirmed: ${approveTx.hash}`);

    // 3. Generate the shield private key by signing the standard message
    const signatureMessage = getShieldPrivateKeySignatureMessage();
    const shieldPrivateKey = await walletSigner.signMessage(signatureMessage);

    // 4. Build shield recipients
    const walletId = getWalletId();
    const erc20AmountRecipients: RailgunERC20AmountRecipient[] = [
      {
        tokenAddress: config.wethAddress,
        amount,
        recipientAddress: walletId, // shield to our own wallet
      },
    ];

    // 5. Estimate gas for the shield transaction
    const feeData = await walletSigner.provider!.getFeeData();
    const gasDetails: TransactionGasDetails = feeData.maxFeePerGas
      ? {
          evmGasType: EVMGasType.Type2,
          gasEstimate: 350_000n, // generous estimate for shield
          maxFeePerGas: feeData.maxFeePerGas,
          maxPriorityFeePerGas: feeData.maxPriorityFeePerGas ?? 1_500_000_000n,
        }
      : {
          evmGasType: EVMGasType.Type0,
          gasEstimate: 350_000n,
          gasPrice: feeData.gasPrice ?? 20_000_000_000n,
        };

    // 6. Populate the shield transaction
    console.log("[railgun/shield] Populating shield transaction...");
    const { transaction } = await populateShield(
      TXIDVersion.V2_PoseidonMerkle,
      networkNameValue,
      shieldPrivateKey,
      erc20AmountRecipients,
      [], // no NFTs
      gasDetails,
    );

    // 7. Submit the shield transaction
    console.log("[railgun/shield] Submitting shield transaction...");
    const tx = await walletSigner.sendTransaction(transaction);
    const receipt = await tx.wait();
    const txHash = receipt?.hash ?? tx.hash;

    console.log(`[railgun/shield] Shield confirmed: ${txHash}`);
    return { success: true, txHash };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    console.error("[railgun/shield] Shield failed:", message);
    return { success: false, error: message };
  }
}
