// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/NodeRegistry.sol";

/// @notice Deploy just NodeRegistry to Sepolia.
contract DeployNodeRegistryOnly is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address memesContract = vm.envAddress("MEMES_CONTRACT");
        uint256 operatorCardId = vm.envUint("OPERATOR_CARD_ID");

        vm.startBroadcast(deployerPrivateKey);

        // minStake: 0.01 ETH (testnet-friendly), heartbeatInterval: 1 hour, card-gated
        NodeRegistry registry = new NodeRegistry(0.01 ether, 1 hours, memesContract, operatorCardId);
        console.log("NodeRegistry deployed at:", address(registry));

        vm.stopBroadcast();
    }
}
