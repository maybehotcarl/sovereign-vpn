// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/SubscriptionManager.sol";

/// @notice Deploy SubscriptionManager to Sepolia and configure tiers.
contract DeploySubscriptionManager is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        vm.startBroadcast(deployerPrivateKey);

        // treasury: deployer, 80% operator share
        SubscriptionManager subs = new SubscriptionManager(deployer, 8000);

        // Tier 1: 7 days — 0.006 ETH
        subs.setTier(1, 0.006 ether, 7 days, true);

        // Tier 2: 30 days — 0.02 ETH
        subs.setTier(2, 0.02 ether, 30 days, true);

        // Tier 3: 90 days — 0.05 ETH
        subs.setTier(3, 0.05 ether, 90 days, true);

        // Tier 4: 365 days — 0.15 ETH
        subs.setTier(4, 0.15 ether, 365 days, true);

        console.log("SubscriptionManager deployed at:", address(subs));

        vm.stopBroadcast();
    }
}
