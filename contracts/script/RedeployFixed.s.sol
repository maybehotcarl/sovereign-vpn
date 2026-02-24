// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/SubscriptionManager.sol";
import "../src/SessionManager.sol";
import "../src/PayoutVault.sol";

/// @notice Redeploy fixed SessionManager + SubscriptionManager + rewire PayoutVault.
contract RedeployFixed is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);
        address payoutVault = vm.envAddress("PAYOUT_VAULT");

        vm.startBroadcast(deployerPrivateKey);

        // 1. Deploy fixed SubscriptionManager
        SubscriptionManager subMgr = new SubscriptionManager(deployer, 0);
        subMgr.setTier(1, 0.006 ether, 7 days, true);
        subMgr.setTier(2, 0.02 ether, 30 days, true);
        subMgr.setTier(3, 0.05 ether, 90 days, true);
        subMgr.setTier(4, 0.15 ether, 365 days, true);
        console.log("SubscriptionManager:", address(subMgr));

        // 2. Deploy fixed SessionManager
        SessionManager sessMgr = new SessionManager(deployer, 0, 0.001 ether, 24 hours);
        console.log("SessionManager:", address(sessMgr));

        // 3. Wire PayoutVault to new contracts
        PayoutVault vault = PayoutVault(payable(payoutVault));
        vault.authorizeSource(address(subMgr));
        vault.authorizeSource(address(sessMgr));
        subMgr.setPayoutVault(payoutVault);
        sessMgr.setPayoutVault(payoutVault);
        console.log("PayoutVault wired:", payoutVault);

        vm.stopBroadcast();
    }
}
