// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/NodeRegistry.sol";
import "../src/SubscriptionManager.sol";
import "../src/SessionManager.sol";

/// @notice Deploy Option C: Card-gated NodeRegistry + treasury-funded reward contracts.
///         All revenue goes to treasury (operatorShareBps = 0).
///         Governance distributes rewards to operators via distributeRewards().
///
///         Required env vars:
///           PRIVATE_KEY         — deployer private key
///           MEMES_CONTRACT      — Memes ERC-1155 contract address
///           OPERATOR_CARD_ID    — token ID required to operate a node
///
///         Usage: forge script script/DeployOptionC.s.sol --rpc-url sepolia --broadcast
contract DeployOptionC is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address memesContract = vm.envAddress("MEMES_CONTRACT");
        uint256 operatorCardId = vm.envUint("OPERATOR_CARD_ID");
        address deployer = vm.addr(deployerPrivateKey);

        vm.startBroadcast(deployerPrivateKey);

        // 1. Deploy NodeRegistry (card-gated, 0.01 ETH stake, 1h heartbeat)
        NodeRegistry registry = new NodeRegistry(
            0.01 ether,     // minStake
            1 hours,        // heartbeatInterval
            memesContract,  // memesContract
            operatorCardId  // operatorCardId
        );
        console.log("NodeRegistry deployed at:", address(registry));

        // 2. Deploy SubscriptionManager (0% operator share — 100% to treasury)
        SubscriptionManager subMgr = new SubscriptionManager(
            deployer,   // treasury (deployer acts as treasury initially)
            0           // operatorShareBps = 0
        );

        // Configure subscription tiers
        subMgr.setTier(1, 0.006 ether, 7 days, true);    // 7-day
        subMgr.setTier(2, 0.02 ether, 30 days, true);    // 30-day
        subMgr.setTier(3, 0.05 ether, 90 days, true);    // 90-day
        subMgr.setTier(4, 0.15 ether, 365 days, true);   // 365-day
        console.log("SubscriptionManager deployed at:", address(subMgr));

        // 3. Deploy SessionManager (0% operator share, 0.001 ETH/hr, 24h max)
        SessionManager sessMgr = new SessionManager(
            deployer,       // treasury
            0,              // operatorShareBps = 0
            0.001 ether,    // pricePerHour
            24 hours        // maxSessionDuration
        );
        console.log("SessionManager deployed at:", address(sessMgr));

        vm.stopBroadcast();

        console.log("");
        console.log("=== Option C Deployment Complete ===");
        console.log("NodeRegistry:        ", address(registry));
        console.log("SubscriptionManager: ", address(subMgr));
        console.log("SessionManager:      ", address(sessMgr));
        console.log("Memes Contract:      ", memesContract);
        console.log("Operator Card ID:    ", operatorCardId);
        console.log("");
        console.log("Key settings:");
        console.log("  Operator share: 0% (all revenue to treasury)");
        console.log("  Rewards via:    distributeRewards() on Sub/SessionManager");
        console.log("  Node gate:      Memes card ownership (on-chain)");
    }
}
