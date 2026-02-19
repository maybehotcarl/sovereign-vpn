// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/SessionManager.sol";

/// @notice Deploy just SessionManager to Sepolia.
contract DeploySessionManagerOnly is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");
        address deployer = vm.addr(deployerPrivateKey);

        vm.startBroadcast(deployerPrivateKey);

        // treasury: deployer, 80% operator share, 0.001 ETH/hr, 24hr max
        SessionManager sessions = new SessionManager(
            deployer, 8000, 0.001 ether, 24 hours
        );
        console.log("SessionManager deployed at:", address(sessions));

        vm.stopBroadcast();
    }
}
