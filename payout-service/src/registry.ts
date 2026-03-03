import { Contract, JsonRpcProvider } from "ethers";

// ---------------------------------------------------------------------------
// NodeRegistry ABI (read-only subset used by the payout service)
// ---------------------------------------------------------------------------

const NODE_REGISTRY_ABI = [
  // --- State readers ---
  "function nodeList(uint256 index) view returns (address)",
  "function isRegistered(address operator) view returns (bool)",
  "function railgunAddresses(address operator) view returns (string)",
  "function nodeCount() view returns (uint256)",

  // --- Struct reader ---
  "function getNode(address operator) view returns (tuple(address operator, string endpoint, string wgPubKey, string region, uint256 stakedAmount, uint256 registeredAt, uint256 lastHeartbeat, bool active, bool slashed))",

  // --- List helpers ---
  "function getNodeList() view returns (address[])",
  "function getActiveNodes() view returns (tuple(address operator, string endpoint, string wgPubKey, string region, uint256 stakedAmount, uint256 registeredAt, uint256 lastHeartbeat, bool active, bool slashed)[])",

  // --- RAILGUN address ---
  "function getRailgunAddress(address operator) view returns (string)",
] as const;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface NodeInfo {
  operator: string;
  endpoint: string;
  wgPubKey: string;
  region: string;
  stakedAmount: bigint;
  registeredAt: bigint;
  lastHeartbeat: bigint;
  active: boolean;
  slashed: boolean;
}

// ---------------------------------------------------------------------------
// Contract factory
// ---------------------------------------------------------------------------

/**
 * Create a read-only ethers.js v6 Contract instance for the NodeRegistry.
 * No signer is required -- the payout service only reads from this contract.
 */
export function createRegistryContract(
  rpcUrl: string,
  address: string,
): Contract {
  const provider = new JsonRpcProvider(rpcUrl);
  return new Contract(address, NODE_REGISTRY_ABI, provider);
}

// ---------------------------------------------------------------------------
// Query helpers
// ---------------------------------------------------------------------------

/**
 * Fetch all registered operator addresses that have set a RAILGUN 0zk address.
 *
 * @returns A Map where keys are operator Ethereum addresses (checksummed) and
 *          values are the corresponding RAILGUN 0zk addresses.
 */
export async function getOperatorsWithRailgun(
  contract: Contract,
): Promise<Map<string, string>> {
  const result = new Map<string, string>();

  // Step 1: get the full list of registered operator addresses
  // TODO(prod-scale): Replace full-list read with paginated/indexed operator discovery.
  const operators: string[] = await contract.getNodeList();
  console.log(`[registry] Total registered operators: ${operators.length}`);

  if (operators.length === 0) {
    return result;
  }

  // Step 2: batch-fetch RAILGUN addresses in parallel
  const promises = operators.map(async (operator) => {
    const railgunAddress: string = await contract.getRailgunAddress(operator);
    return { operator, railgunAddress };
  });

  const settled = await Promise.allSettled(promises);
  for (const entry of settled) {
    if (entry.status === "fulfilled") {
      const { operator, railgunAddress } = entry.value;
      // Only include operators that have actually set a RAILGUN address
      if (railgunAddress && railgunAddress.length > 0) {
        result.set(operator, railgunAddress);
      }
    } else {
      console.error(
        "[registry] Failed to fetch RAILGUN address:",
        entry.reason,
      );
    }
  }

  console.log(
    `[registry] Operators with RAILGUN address: ${result.size}/${operators.length}`,
  );

  return result;
}

/**
 * Fetch detailed node info for a single operator.
 */
export async function getNodeInfo(
  contract: Contract,
  operator: string,
): Promise<NodeInfo> {
  const raw = await contract.getNode(operator);
  return {
    operator: raw.operator as string,
    endpoint: raw.endpoint as string,
    wgPubKey: raw.wgPubKey as string,
    region: raw.region as string,
    stakedAmount: raw.stakedAmount as bigint,
    registeredAt: raw.registeredAt as bigint,
    lastHeartbeat: raw.lastHeartbeat as bigint,
    active: raw.active as boolean,
    slashed: raw.slashed as boolean,
  };
}

/**
 * Get the total number of registered nodes.
 */
export async function getNodeCount(contract: Contract): Promise<number> {
  const count: bigint = await contract.nodeCount();
  return Number(count);
}
