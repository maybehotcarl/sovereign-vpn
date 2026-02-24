import { JsonRpcProvider, Wallet } from "ethers";

/**
 * Create an ethers.js Wallet signer connected to the given RPC provider.
 * Shared between vault operations and RAILGUN shielding.
 */
export function createExecutorWallet(
  rpcUrl: string,
  privateKey: string,
): Wallet {
  const provider = new JsonRpcProvider(rpcUrl);
  return new Wallet(privateKey, provider);
}

/**
 * Get the public Ethereum address for a wallet.
 */
export function getExecutorAddress(wallet: Wallet): string {
  return wallet.address;
}
