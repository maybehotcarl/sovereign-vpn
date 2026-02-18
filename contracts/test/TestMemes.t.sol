// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/TestMemes.sol";

contract TestMemesTest is Test {
    TestMemes public memes;
    address public owner = address(this);
    address public alice = address(0xA11CE);
    address public bob = address(0xB0B);

    function setUp() public {
        memes = new TestMemes();
    }

    function test_mint() public {
        memes.mint(alice, 1, 1);
        assertEq(memes.balanceOf(alice, 1), 1);
        assertEq(memes.balanceOf(alice, 2), 0);
    }

    function test_mintBatch() public {
        uint256[] memory ids = new uint256[](2);
        ids[0] = 1;
        ids[1] = 2;
        uint256[] memory amounts = new uint256[](2);
        amounts[0] = 1;
        amounts[1] = 3;

        memes.mintBatch(alice, ids, amounts);
        assertEq(memes.balanceOf(alice, 1), 1);
        assertEq(memes.balanceOf(alice, 2), 3);
    }

    function test_onlyOwnerCanMint() public {
        vm.prank(alice);
        vm.expectRevert();
        memes.mint(bob, 1, 1);
    }

    function test_transfer() public {
        memes.mint(alice, 1, 1);
        assertEq(memes.balanceOf(alice, 1), 1);

        vm.prank(alice);
        memes.safeTransferFrom(alice, bob, 1, 1, "");
        assertEq(memes.balanceOf(alice, 1), 0);
        assertEq(memes.balanceOf(bob, 1), 1);
    }

    function test_nameAndSymbol() public view {
        assertEq(memes.name(), "Test Memes by 6529");
        assertEq(memes.symbol(), "TMEMES");
    }
}
