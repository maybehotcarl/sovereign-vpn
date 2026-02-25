// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import {AccessPolicy} from "../src/AccessPolicy.sol";
import {NodeRegistry} from "../src/NodeRegistry.sol";
import "../src/SessionManager.sol";
import "../src/SubscriptionManager.sol";
import "../src/PayoutVault.sol";

/// @notice All-in-one mainnet deployment: 5 contracts + wiring in a single broadcast.
///
///         Required env vars:
///           PRIVATE_KEY      — funded mainnet wallet (also becomes payout executor)
///           MEMES_CONTRACT   — Memes by 6529 ERC-1155 (0x33FD426905F149f8376e227d0C9D3340AaD17aF1)
///           OPERATOR_CARD_ID — token ID required to operate a node (e.g. 1)
///           TREASURY         — treasury address for revenue (0xBEEF2fc53b21bCC120B5f3696CdD5Ddd584Ac337)
///
///         Usage:
///           forge script script/DeployMainnet.s.sol \
///             --rpc-url mainnet --broadcast --verify \
///             --etherscan-api-key $ETHERSCAN_API_KEY
contract DeployMainnet is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        address memesContract = vm.envAddress("MEMES_CONTRACT");
        uint256 operatorCardId = vm.envUint("OPERATOR_CARD_ID");
        address treasury = vm.envAddress("TREASURY");

        vm.startBroadcast(deployerPrivateKey);

        // 1. AccessPolicy — reads Memes ERC-1155 for access gating
        AccessPolicy accessPolicy = new AccessPolicy(memesContract);
        accessPolicy.addKnownTokenId(1);
        accessPolicy.addKnownTokenId(2);
        accessPolicy.addKnownTokenId(3);
        accessPolicy.addKnownTokenId(4);
        accessPolicy.addKnownTokenId(5);
        accessPolicy.setThisCardTokenId(1);

        // 2. NodeRegistry — card-gated, 0.1 ETH stake, 1h heartbeat
        NodeRegistry nodeRegistry = new NodeRegistry(
            0.1 ether,
            1 hours,
            memesContract,
            operatorCardId
        );

        // 3. SessionManager — 100% to treasury (operatorShareBps = 0)
        SessionManager sessionMgr = new SessionManager(
            treasury,
            0,              // operatorShareBps = 0 → 100% to treasury
            0.001 ether,    // pricePerHour
            24 hours        // maxSessionDuration
        );

        // 4. SubscriptionManager — 100% to treasury + 4 tiers
        SubscriptionManager subMgr = new SubscriptionManager(treasury, 0);
        subMgr.setTier(1, 0.006 ether, 7 days, true);
        subMgr.setTier(2, 0.02 ether, 30 days, true);
        subMgr.setTier(3, 0.05 ether, 90 days, true);
        subMgr.setTier(4, 0.15 ether, 365 days, true);

        // 5. PayoutVault — deployer is the payout executor
        PayoutVault vault = new PayoutVault(deployer);

        // 6. Wire vault: authorize revenue sources
        vault.authorizeSource(address(sessionMgr));
        vault.authorizeSource(address(subMgr));

        // 7. Wire managers: point at vault
        sessionMgr.setPayoutVault(address(vault));
        subMgr.setPayoutVault(address(vault));

        vm.stopBroadcast();

        console.log("");
        console.log("=== Mainnet Deployment Complete ===");
        console.log("AccessPolicy:        ", address(accessPolicy));
        console.log("NodeRegistry:        ", address(nodeRegistry));
        console.log("SessionManager:      ", address(sessionMgr));
        console.log("SubscriptionManager: ", address(subMgr));
        console.log("PayoutVault:         ", address(vault));
        console.log("");
        console.log("Config:");
        console.log("  Memes Contract:  ", memesContract);
        console.log("  Operator Card ID:", operatorCardId);
        console.log("  Treasury:        ", treasury);
        console.log("  Payout Executor: ", deployer);
        console.log("  Operator Share:    0% (100% to treasury)");
        console.log("  Node Stake:        0.1 ETH");
        console.log("");
        console.log("Post-deploy:");
        console.log("  1. Update .env files with deployed addresses");
        console.log("  2. Verify on Etherscan (if --verify was used)");
        console.log("  3. Transfer ownership to treasury multisig:");
        console.log("     cast send <contract> 'transferOwnership(address)' ", treasury);
    }
}
