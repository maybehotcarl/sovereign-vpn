// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Script.sol";
import "../src/TestMemes.sol";
import "../src/AccessPolicy.sol";

/// @notice Deploy TestMemes + AccessPolicy to Sepolia.
///         Usage: forge script script/DeploySepolia.s.sol --rpc-url sepolia --broadcast
contract DeploySepolia is Script {
    function run() external {
        uint256 deployerPrivateKey = vm.envUint("PRIVATE_KEY");

        vm.startBroadcast(deployerPrivateKey);

        // 1. Deploy TestMemes
        TestMemes memes = new TestMemes();
        console.log("TestMemes deployed at:", address(memes));

        // 2. Deploy AccessPolicy pointing at TestMemes
        AccessPolicy policy = new AccessPolicy(address(memes));
        console.log("AccessPolicy deployed at:", address(policy));

        // 3. Add known token IDs (1 = THIS card, 2-5 = other cards)
        policy.addKnownTokenId(1);
        policy.addKnownTokenId(2);
        policy.addKnownTokenId(3);
        policy.addKnownTokenId(4);
        policy.addKnownTokenId(5);

        // 4. Set THIS card token ID
        policy.setThisCardTokenId(1);
        console.log("thisCardTokenId set to: 1");

        // 5. Mint test tokens to the deployer
        //    Token ID 1 = THIS card (free tier)
        //    Token ID 2 = other Memes card (paid tier)
        address deployer = vm.addr(deployerPrivateKey);
        memes.mint(deployer, 1, 1);
        memes.mint(deployer, 2, 1);
        console.log("Minted token IDs 1 and 2 to deployer:", deployer);

        vm.stopBroadcast();

        console.log("");
        console.log("=== Deployment Complete ===");
        console.log("TestMemes:    ", address(memes));
        console.log("AccessPolicy: ", address(policy));
        console.log("");
        console.log("Next steps:");
        console.log("  1. Mint THIS card (ID 1) to free-tier test wallets:");
        console.log("     cast send <TestMemes> 'mint(address,uint256,uint256)' <wallet> 1 1");
        console.log("  2. Mint other cards (ID 2+) to paid-tier test wallets:");
        console.log("     cast send <TestMemes> 'mint(address,uint256,uint256)' <wallet> 2 1");
        console.log("  3. Verify access:");
        console.log("     cast call <AccessPolicy> 'checkAccess(address)(bool,bool)' <wallet>");
    }
}
