// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/NodeRegistry.sol";

/// @notice Deploy a fresh mainnet NodeRegistry under a known owner wallet.
///
/// Required env vars:
///   PRIVATE_KEY                          funded deployer key
///
/// Optional env vars:
///   MEMES_CONTRACT                       defaults to mainnet Memes
///   OPERATOR_CARD_ID                     defaults to 1
///   NODE_REGISTRY_MIN_STAKE_WEI          defaults to 0.1 ether
///   NODE_REGISTRY_HEARTBEAT_INTERVAL_SECONDS defaults to 3600
///   NODE_REGISTRY_TRANSFER_OWNERSHIP_TO  optional pending owner after deploy
///
/// Usage:
///   forge script script/DeployNodeRegistryMainnet.s.sol \
///     --rpc-url mainnet --broadcast --verify \
///     --etherscan-api-key $ETHERSCAN_API_KEY
contract DeployNodeRegistryMainnet is Script {
    address internal constant DEFAULT_MAINNET_MEMES =
        0x33FD426905F149f8376e227d0C9D3340AaD17aF1;

    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        address memesContract = vm.envOr("MEMES_CONTRACT", DEFAULT_MAINNET_MEMES);
        uint256 operatorCardId = vm.envOr("OPERATOR_CARD_ID", uint256(1));
        uint256 minStake = vm.envOr("NODE_REGISTRY_MIN_STAKE_WEI", uint256(0.1 ether));
        uint256 heartbeatInterval =
            vm.envOr("NODE_REGISTRY_HEARTBEAT_INTERVAL_SECONDS", uint256(1 hours));
        address transferOwnershipTo =
            vm.envOr("NODE_REGISTRY_TRANSFER_OWNERSHIP_TO", address(0));

        vm.startBroadcast(deployerPrivateKey);

        NodeRegistry registry = new NodeRegistry(
            minStake,
            heartbeatInterval,
            memesContract,
            operatorCardId
        );

        if (transferOwnershipTo != address(0) && transferOwnershipTo != deployer) {
            registry.transferOwnership(transferOwnershipTo);
        }

        vm.stopBroadcast();

        console.log("");
        console.log("=== Mainnet NodeRegistry Deployment Complete ===");
        console.log("NodeRegistry:     ", address(registry));
        console.log("Owner:            ", deployer);
        console.log("Memes Contract:   ", memesContract);
        console.log("Operator Card ID: ", operatorCardId);
        console.log("Min Stake (wei):  ", minStake);
        console.log("Heartbeat (secs): ", heartbeatInterval);
        if (transferOwnershipTo != address(0) && transferOwnershipTo != deployer) {
            console.log("Pending Owner:    ", transferOwnershipTo);
        }
        console.log("");
        console.log("Post-deploy:");
        console.log("  1. Verify the contract on Etherscan");
        console.log("  2. Add the address to MAINNET_ADDRESSES.md");
        console.log("  3. Register a treasury-owned operator as the first beta node");
        console.log("  4. Repoint gateway / payout-service consumers to the new registry");
    }
}
