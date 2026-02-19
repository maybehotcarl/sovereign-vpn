// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/NodeRegistry.sol";
import "../src/SessionManager.sol";

/// @notice Deploy NodeRegistry + SessionManager to Sepolia.
///         Usage: forge script script/DeployNodeRegistry.s.sol --rpc-url $SEPOLIA_RPC --broadcast
contract DeployNodeRegistry is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        vm.startBroadcast(deployerPrivateKey);

        // 1. Deploy NodeRegistry
        //    - minStake: 0.01 ETH (testnet-friendly)
        //    - heartbeatInterval: 1 hour
        NodeRegistry registry = new NodeRegistry(0.01 ether, 1 hours);
        console.log("NodeRegistry deployed at:", address(registry));

        // 2. Deploy SessionManager
        //    - treasury: deployer (for testing, community multi-sig on mainnet)
        //    - operatorShareBps: 8000 (80% to operator, 20% to treasury)
        //    - pricePerHour: 0.001 ETH (testnet pricing)
        //    - maxSessionDuration: 24 hours
        SessionManager sessions = new SessionManager(
            deployer,           // treasury
            8000,               // 80% operator share
            0.001 ether,        // price per hour
            24 hours            // max session duration
        );
        console.log("SessionManager deployed at:", address(sessions));

        vm.stopBroadcast();

        console.log("");
        console.log("=== Deployment Complete ===");
        console.log("NodeRegistry:    ", address(registry));
        console.log("SessionManager:  ", address(sessions));
        console.log("Treasury (owner):", deployer);
        console.log("");
        console.log("Next steps:");
        console.log("  1. Register a test node:");
        console.log("     cast send <NodeRegistry> 'registerNode(string,string,string)' 'vpn.example.com:51820' 'wg-pubkey-base64' 'us-east' --value 0.01ether");
        console.log("  2. Accumulate 50,000 'VPN Operator' rep on 6529:");
        console.log("     https://seize.io/profile/<operator>");
    }
}
