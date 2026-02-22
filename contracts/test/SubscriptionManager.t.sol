// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/SubscriptionManager.sol";

contract SubscriptionManagerTest is Test {
    SubscriptionManager public sm;

    address public owner = address(this);
    address public treasury = address(0xDAD);
    address public user1 = address(0x1);
    address public user2 = address(0x2);
    address public nodeOp = address(0xA0A0);
    address public nodeOp2 = address(0xB0B0);

    uint256 constant OPERATOR_SHARE = 8000; // 80%

    // Tier configs
    uint8 constant TIER_7D = 1;
    uint8 constant TIER_30D = 2;
    uint8 constant TIER_90D = 3;
    uint8 constant TIER_365D = 4;

    uint256 constant PRICE_7D = 0.006 ether;
    uint256 constant PRICE_30D = 0.02 ether;
    uint256 constant PRICE_90D = 0.05 ether;
    uint256 constant PRICE_365D = 0.15 ether;

    function setUp() public {
        sm = new SubscriptionManager(treasury, OPERATOR_SHARE);

        // Configure tiers
        sm.setTier(TIER_7D, PRICE_7D, 7 days, true);
        sm.setTier(TIER_30D, PRICE_30D, 30 days, true);
        sm.setTier(TIER_90D, PRICE_90D, 90 days, true);
        sm.setTier(TIER_365D, PRICE_365D, 365 days, true);

        vm.deal(user1, 10 ether);
        vm.deal(user2, 10 ether);
    }

    // =========================================================================
    //                          SUBSCRIBE
    // =========================================================================

    function test_Subscribe() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);

        SubscriptionManager.Subscription memory sub = sm.getSubscription(user1);
        assertEq(sub.user, user1);
        assertEq(sub.node, nodeOp);
        assertEq(sub.payment, PRICE_30D);
        assertEq(sub.expiresAt, block.timestamp + 30 days);
        assertEq(sub.tier, TIER_30D);

        assertTrue(sm.hasActiveSubscription(user1));
        assertEq(sm.totalSubscriptions(), 1);
        assertEq(sm.totalRevenue(), PRICE_30D);

        // Check payment distribution: 80% operator, 20% treasury
        assertEq(sm.operatorBalance(nodeOp), (PRICE_30D * 80) / 100);
        assertEq(sm.treasuryBalance(), (PRICE_30D * 20) / 100);
    }

    function test_SubscribeRevertsInsufficientPayment() public {
        vm.prank(user1);
        vm.expectRevert(abi.encodeWithSelector(
            SubscriptionManager.InsufficientPayment.selector, 0.001 ether, PRICE_7D
        ));
        sm.subscribe{value: 0.001 ether}(nodeOp, TIER_7D);
    }

    function test_SubscribeRevertsInactiveTier() public {
        // Disable tier 1
        sm.setTier(TIER_7D, PRICE_7D, 7 days, false);

        vm.prank(user1);
        vm.expectRevert(SubscriptionManager.TierNotActive.selector);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);
    }

    function test_SubscribeRevertsAlreadySubscribed() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        vm.prank(user1);
        vm.expectRevert(SubscriptionManager.AlreadySubscribed.selector);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);
    }

    function test_SubscribeAfterExpiry() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        // Warp past expiry
        vm.warp(block.timestamp + 7 days + 1);

        // Should be able to subscribe again
        vm.prank(user1);
        sm.subscribe{value: PRICE_30D}(nodeOp2, TIER_30D);

        SubscriptionManager.Subscription memory sub = sm.getSubscription(user1);
        assertEq(sub.node, nodeOp2);
        assertEq(sub.tier, TIER_30D);
    }

    // =========================================================================
    //                          RENEW
    // =========================================================================

    function test_RenewExtendsBeyondExpiry() public {
        // Buy 7 days
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);
        uint256 originalExpiry = block.timestamp + 7 days;

        // Renew with 30 days (while still active)
        vm.prank(user1);
        sm.renewSubscription{value: PRICE_30D}(TIER_30D, address(0));

        SubscriptionManager.Subscription memory sub = sm.getSubscription(user1);
        // Should stack: original expiry + 30 days = 37 days total
        assertEq(sub.expiresAt, originalExpiry + 30 days);
        assertEq(sub.tier, TIER_30D);
        assertEq(sub.node, nodeOp); // kept original node
    }

    function test_RenewAfterExpiry() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        // Warp past expiry
        vm.warp(block.timestamp + 8 days);

        // Renew starts fresh from now
        vm.prank(user1);
        sm.renewSubscription{value: PRICE_30D}(TIER_30D, address(0));

        SubscriptionManager.Subscription memory sub = sm.getSubscription(user1);
        assertEq(sub.expiresAt, block.timestamp + 30 days);
    }

    function test_RenewNodeSwitch() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        // Renew with a different node
        vm.prank(user1);
        sm.renewSubscription{value: PRICE_30D}(TIER_30D, nodeOp2);

        SubscriptionManager.Subscription memory sub = sm.getSubscription(user1);
        assertEq(sub.node, nodeOp2);
    }

    // =========================================================================
    //                          VIEW FUNCTIONS
    // =========================================================================

    function test_HasActiveSubscription() public {
        assertFalse(sm.hasActiveSubscription(user1));

        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);
        assertTrue(sm.hasActiveSubscription(user1));

        // Warp past expiry
        vm.warp(block.timestamp + 7 days + 1);
        assertFalse(sm.hasActiveSubscription(user1));
    }

    function test_RemainingTime() public {
        assertEq(sm.remainingTime(user1), 0);

        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        // Should be close to 7 days
        assertEq(sm.remainingTime(user1), 7 days);

        // Advance 3 days
        vm.warp(block.timestamp + 3 days);
        assertEq(sm.remainingTime(user1), 4 days);

        // Advance past expiry
        vm.warp(block.timestamp + 5 days);
        assertEq(sm.remainingTime(user1), 0);
    }

    function test_GetActiveTierIds() public {
        uint8[] memory ids = sm.getActiveTierIds();
        assertEq(ids.length, 4);
        assertEq(ids[0], TIER_7D);
        assertEq(ids[1], TIER_30D);
        assertEq(ids[2], TIER_90D);
        assertEq(ids[3], TIER_365D);

        // Disable one tier
        sm.setTier(TIER_90D, PRICE_90D, 90 days, false);
        ids = sm.getActiveTierIds();
        assertEq(ids.length, 3);
    }

    // =========================================================================
    //                          WITHDRAWALS
    // =========================================================================

    function test_WithdrawOperatorEarnings() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);

        uint256 expectedOp = (PRICE_30D * 80) / 100;
        uint256 balBefore = nodeOp.balance;

        vm.prank(nodeOp);
        sm.withdrawOperatorEarnings();

        assertEq(nodeOp.balance, balBefore + expectedOp);
        assertEq(sm.operatorBalance(nodeOp), 0);
    }

    function test_WithdrawTreasury() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);

        uint256 expectedTreasury = (PRICE_30D * 20) / 100;
        uint256 balBefore = treasury.balance;

        sm.withdrawTreasury();

        assertEq(treasury.balance, balBefore + expectedTreasury);
        assertEq(sm.treasuryBalance(), 0);
    }

    function test_WithdrawOperatorRevertsNothingToWithdraw() public {
        vm.prank(nodeOp);
        vm.expectRevert(SubscriptionManager.NothingToWithdraw.selector);
        sm.withdrawOperatorEarnings();
    }

    function test_WithdrawTreasuryRevertsNothingToWithdraw() public {
        vm.expectRevert(SubscriptionManager.NothingToWithdraw.selector);
        sm.withdrawTreasury();
    }

    // =========================================================================
    //                          PAYMENT DISTRIBUTION
    // =========================================================================

    function test_PaymentDistribution_80_20() public {
        // Subscribe two users to the same node
        vm.prank(user1);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);

        // Warp past first sub so user2 can subscribe after
        vm.warp(block.timestamp + 31 days);

        vm.prank(user2);
        sm.subscribe{value: PRICE_90D}(nodeOp, TIER_90D);

        uint256 totalPaid = PRICE_30D + PRICE_90D;
        uint256 expectedOp = (PRICE_30D * 80 / 100) + (PRICE_90D * 80 / 100);
        uint256 expectedTreasury = totalPaid - expectedOp;

        assertEq(sm.operatorBalance(nodeOp), expectedOp);
        assertEq(sm.treasuryBalance(), expectedTreasury);
    }

    // =========================================================================
    //                          ADMIN
    // =========================================================================

    function test_SetTier_OwnerOnly() public {
        vm.prank(user1);
        vm.expectRevert();
        sm.setTier(5, 1 ether, 365 days, true);
    }

    function test_SetTierAddsNew() public {
        sm.setTier(5, 0.5 ether, 180 days, true);

        (uint256 price, uint256 duration, bool active) = sm.tiers(5);
        assertEq(price, 0.5 ether);
        assertEq(duration, 180 days);
        assertTrue(active);
    }

    function test_AdminRevertsNotOwner() public {
        vm.startPrank(user1);

        vm.expectRevert();
        sm.setTier(1, 1, 1, true);

        vm.expectRevert();
        sm.setOperatorShare(1);

        vm.expectRevert();
        sm.setTreasury(user1);

        vm.expectRevert();
        sm.withdrawTreasury();

        vm.stopPrank();
    }

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    function test_EmitsSubscribed() public {
        uint256 expectedExpiry = block.timestamp + 30 days;

        vm.prank(user1);
        vm.expectEmit(true, true, true, true);
        emit SubscriptionManager.Subscribed(user1, nodeOp, TIER_30D, PRICE_30D, expectedExpiry);
        sm.subscribe{value: PRICE_30D}(nodeOp, TIER_30D);
    }

    function test_EmitsRenewed() public {
        vm.prank(user1);
        sm.subscribe{value: PRICE_7D}(nodeOp, TIER_7D);

        uint256 expectedExpiry = block.timestamp + 7 days + 30 days;

        vm.prank(user1);
        vm.expectEmit(true, true, true, true);
        emit SubscriptionManager.Renewed(user1, nodeOp, TIER_30D, PRICE_30D, expectedExpiry);
        sm.renewSubscription{value: PRICE_30D}(TIER_30D, address(0));
    }
}
