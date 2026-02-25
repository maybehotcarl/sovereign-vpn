// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/SubscriptionManager.sol";
import "../src/SessionManager.sol";
import "../src/PayoutVault.sol";

/// @notice Complete the mainnet deployment — picks up where DeployMainnet left off.
///         Already deployed: AccessPolicy, NodeRegistry, SessionManager, SubscriptionManager.
///         Remaining: setTier(2-4), deploy PayoutVault, wire everything.
contract CompleteMainnet is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        // Already-deployed contracts
        SubscriptionManager subMgr = SubscriptionManager(payable(0xEb54c8604b7EEADE804d121BD8f158A006827882));
        SessionManager sessMgr = SessionManager(payable(0xb644c990c884911670adc422719243D9F76Df0d6));

        vm.startBroadcast(deployerPrivateKey);

        // 1. Finish SubscriptionManager tiers (tier 1 already set)
        subMgr.setTier(2, 0.02 ether, 30 days, true);
        subMgr.setTier(3, 0.05 ether, 90 days, true);
        subMgr.setTier(4, 0.15 ether, 365 days, true);

        // 2. Deploy PayoutVault (deployer = payout executor)
        PayoutVault vault = new PayoutVault(deployer);

        // 3. Wire vault: authorize revenue sources
        vault.authorizeSource(address(sessMgr));
        vault.authorizeSource(address(subMgr));

        // 4. Wire managers: point at vault
        sessMgr.setPayoutVault(address(vault));
        subMgr.setPayoutVault(address(vault));

        vm.stopBroadcast();

        console.log("=== Mainnet Deployment Completed ===");
        console.log("PayoutVault:", address(vault));
        console.log("Tiers 2-4 set, vault wired to SessionManager + SubscriptionManager");
    }
}
