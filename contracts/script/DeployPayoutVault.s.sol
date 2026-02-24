// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/PayoutVault.sol";

/// @notice Deploy PayoutVault and wire it to existing SessionManager + SubscriptionManager.
///         Usage: forge script script/DeployPayoutVault.s.sol --rpc-url $SEPOLIA_RPC --broadcast
///
///         Required env vars:
///           PRIVATE_KEY          — deployer / contract owner private key
///           PAYOUT_EXECUTOR      — address of the payout service executor wallet
///           SESSION_MANAGER      — deployed SessionManager address
///           SUBSCRIPTION_MANAGER — deployed SubscriptionManager address
contract DeployPayoutVault is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address payoutExecutor = vm.envAddress("PAYOUT_EXECUTOR");
        address sessionManager = vm.envAddress("SESSION_MANAGER");
        address subscriptionManager = vm.envAddress("SUBSCRIPTION_MANAGER");

        vm.startBroadcast(deployerPrivateKey);

        // 1. Deploy PayoutVault
        PayoutVault vault = new PayoutVault(payoutExecutor);
        console.log("PayoutVault deployed at:", address(vault));

        // 2. Authorize SessionManager and SubscriptionManager as sources
        vault.authorizeSource(sessionManager);
        console.log("Authorized SessionManager:", sessionManager);

        vault.authorizeSource(subscriptionManager);
        console.log("Authorized SubscriptionManager:", subscriptionManager);

        // 3. Wire SessionManager to use PayoutVault
        //    Caller must be owner of SessionManager
        (bool ok1, ) = sessionManager.call(
            abi.encodeWithSignature("setPayoutVault(address)", address(vault))
        );
        require(ok1, "Failed to set PayoutVault on SessionManager");
        console.log("SessionManager.setPayoutVault() called");

        // 4. Wire SubscriptionManager to use PayoutVault
        (bool ok2, ) = subscriptionManager.call(
            abi.encodeWithSignature("setPayoutVault(address)", address(vault))
        );
        require(ok2, "Failed to set PayoutVault on SubscriptionManager");
        console.log("SubscriptionManager.setPayoutVault() called");

        vm.stopBroadcast();

        console.log("");
        console.log("=== PayoutVault Deployment Complete ===");
        console.log("PayoutVault:          ", address(vault));
        console.log("Payout Executor:      ", payoutExecutor);
        console.log("SessionManager:       ", sessionManager);
        console.log("SubscriptionManager:  ", subscriptionManager);
        console.log("");
        console.log("Next steps:");
        console.log("  1. Operators register 0zk addresses on NodeRegistry:");
        console.log("     cast send <NodeRegistry> 'setRailgunAddress(string)' '0zk...'");
        console.log("  2. Start payout-service in dry-run mode");
        console.log("  3. Verify payouts on testnet, then switch to live mode");
    }
}
