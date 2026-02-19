// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/SessionManager.sol";

contract SessionManagerTest is Test {
    SessionManager public sm;

    address public owner = address(this);
    address public treasury = address(0xDAD);
    address public user1 = address(0x1);
    address public user2 = address(0x2);
    address public nodeOp = address(0xA0A0);
    address public nodeOp2 = address(0xB0B0);

    // 0.001 ETH per hour, 80% operator share, max 24h session
    uint256 constant PRICE_PER_HOUR = 0.001 ether;
    uint256 constant OPERATOR_SHARE = 8000; // 80%
    uint256 constant MAX_DURATION = 86400;  // 24 hours

    function setUp() public {
        sm = new SessionManager(treasury, OPERATOR_SHARE, PRICE_PER_HOUR, MAX_DURATION);
        vm.deal(user1, 10 ether);
        vm.deal(user2, 10 ether);
    }

    // =========================================================================
    //                          OPEN SESSION (PAID)
    // =========================================================================

    function test_OpenSession() public {
        uint256 duration = 3600; // 1 hour
        uint256 price = sm.calculatePrice(duration);
        assertEq(price, PRICE_PER_HOUR);

        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: price}(nodeOp, duration);

        assertEq(sessionId, 1);
        assertEq(sm.activeSession(user1), 1);
        assertEq(sm.totalSessions(), 1);
        assertEq(sm.totalRevenue(), price);

        SessionManager.Session memory s = sm.getSession(1);
        assertEq(s.user, user1);
        assertEq(s.node, nodeOp);
        assertEq(s.payment, price);
        assertEq(s.duration, duration);
        assertTrue(s.active);
        assertFalse(s.settled);
    }

    function test_OpenSessionOverpay() public {
        // Overpaying is allowed — excess is held for operator
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.01 ether}(nodeOp, 3600);
        assertEq(sessionId, 1);

        SessionManager.Session memory s = sm.getSession(1);
        assertEq(s.payment, 0.01 ether);
    }

    function test_OpenSessionRevertsInsufficientPayment() public {
        vm.prank(user1);
        vm.expectRevert(abi.encodeWithSelector(
            SessionManager.InsufficientPayment.selector, 0.0005 ether, 0.001 ether
        ));
        sm.openSession{value: 0.0005 ether}(nodeOp, 3600);
    }

    function test_OpenSessionRevertsAlreadyActive() public {
        vm.prank(user1);
        sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        vm.expectRevert(SessionManager.SessionAlreadyActive.selector);
        sm.openSession{value: 0.001 ether}(nodeOp, 3600);
    }

    function test_OpenSessionRevertsInvalidDuration() public {
        vm.prank(user1);
        vm.expectRevert(SessionManager.InvalidDuration.selector);
        sm.openSession{value: 0.001 ether}(nodeOp, 0);

        vm.prank(user1);
        vm.expectRevert(SessionManager.InvalidDuration.selector);
        sm.openSession{value: 1 ether}(nodeOp, MAX_DURATION + 1);
    }

    // =========================================================================
    //                          OPEN FREE SESSION
    // =========================================================================

    function test_OpenFreeSession() public {
        uint256 sessionId = sm.openFreeSession(user1, nodeOp, 3600);
        assertEq(sessionId, 1);

        SessionManager.Session memory s = sm.getSession(1);
        assertEq(s.user, user1);
        assertEq(s.payment, 0);
        assertTrue(s.active);
    }

    function test_OpenFreeSessionRevertsNotOwner() public {
        vm.prank(user1);
        vm.expectRevert();
        sm.openFreeSession(user1, nodeOp, 3600);
    }

    // =========================================================================
    //                          CLOSE SESSION
    // =========================================================================

    function test_CloseSessionByUser() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(sessionId);

        SessionManager.Session memory s = sm.getSession(sessionId);
        assertFalse(s.active);
        assertTrue(s.settled);
        assertEq(sm.activeSession(user1), 0);

        // Check payment distribution: 80% operator, 20% treasury
        assertEq(sm.operatorBalance(nodeOp), 0.0008 ether);
        assertEq(sm.treasuryBalance(), 0.0002 ether);
    }

    function test_CloseSessionByNodeOperator() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(nodeOp);
        sm.closeSession(sessionId);

        SessionManager.Session memory s = sm.getSession(sessionId);
        assertFalse(s.active);
    }

    function test_CloseSessionByOwner() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        // Owner (governance) can also close
        sm.closeSession(sessionId);

        SessionManager.Session memory s = sm.getSession(sessionId);
        assertFalse(s.active);
    }

    function test_CloseFreeSession() public {
        uint256 sessionId = sm.openFreeSession(user1, nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(sessionId);

        SessionManager.Session memory s = sm.getSession(sessionId);
        assertFalse(s.active);
        assertFalse(s.settled); // no payment to settle
        assertEq(sm.operatorBalance(nodeOp), 0);
        assertEq(sm.treasuryBalance(), 0);
    }

    function test_CloseSessionRevertsNotParticipant() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user2);
        vm.expectRevert(SessionManager.NotSessionParticipant.selector);
        sm.closeSession(sessionId);
    }

    function test_CloseSessionRevertsNotActive() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(sessionId);

        vm.prank(user1);
        vm.expectRevert(SessionManager.SessionNotActive.selector);
        sm.closeSession(sessionId);
    }

    function test_CloseSessionRevertsNotFound() public {
        vm.prank(user1);
        vm.expectRevert(SessionManager.SessionNotFound.selector);
        sm.closeSession(999);
    }

    // =========================================================================
    //                          WITHDRAWALS
    // =========================================================================

    function test_WithdrawOperatorEarnings() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(sessionId);

        uint256 balBefore = nodeOp.balance;

        vm.prank(nodeOp);
        sm.withdrawOperatorEarnings();

        assertEq(nodeOp.balance, balBefore + 0.0008 ether);
        assertEq(sm.operatorBalance(nodeOp), 0);
    }

    function test_WithdrawTreasury() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(sessionId);

        uint256 balBefore = treasury.balance;

        sm.withdrawTreasury();

        assertEq(treasury.balance, balBefore + 0.0002 ether);
        assertEq(sm.treasuryBalance(), 0);
    }

    function test_WithdrawOperatorRevertsNothingToWithdraw() public {
        vm.prank(nodeOp);
        vm.expectRevert(SessionManager.NothingToWithdraw.selector);
        sm.withdrawOperatorEarnings();
    }

    function test_WithdrawTreasuryRevertsNothingToWithdraw() public {
        vm.expectRevert(SessionManager.NothingToWithdraw.selector);
        sm.withdrawTreasury();
    }

    // =========================================================================
    //                          MULTIPLE SESSIONS
    // =========================================================================

    function test_MultipleSessionsAccumulateEarnings() public {
        // User1 opens session with nodeOp
        vm.prank(user1);
        uint256 s1 = sm.openSession{value: 0.001 ether}(nodeOp, 3600);
        vm.prank(user1);
        sm.closeSession(s1);

        // User2 opens session with same nodeOp
        vm.prank(user2);
        uint256 s2 = sm.openSession{value: 0.002 ether}(nodeOp, 7200);
        vm.prank(user2);
        sm.closeSession(s2);

        // Operator should have 80% of both
        assertEq(sm.operatorBalance(nodeOp), 0.0024 ether); // 80% of 0.003
        assertEq(sm.treasuryBalance(), 0.0006 ether);        // 20% of 0.003
        assertEq(sm.totalSessions(), 2);
    }

    function test_UserCanReopenAfterClose() public {
        vm.prank(user1);
        uint256 s1 = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        sm.closeSession(s1);

        // Should be able to open a new session
        vm.prank(user1);
        uint256 s2 = sm.openSession{value: 0.001 ether}(nodeOp2, 3600);

        assertEq(s2, 2);
        assertEq(sm.activeSession(user1), 2);
    }

    // =========================================================================
    //                          EXPIRY
    // =========================================================================

    function test_IsExpired() public {
        vm.prank(user1);
        uint256 sessionId = sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        assertFalse(sm.isExpired(sessionId));

        vm.warp(block.timestamp + 3601);

        assertTrue(sm.isExpired(sessionId));
    }

    // =========================================================================
    //                          ADMIN
    // =========================================================================

    function test_SetPricePerHour() public {
        sm.setPricePerHour(0.01 ether);
        assertEq(sm.pricePerHour(), 0.01 ether);
    }

    function test_SetOperatorShare() public {
        sm.setOperatorShare(7000); // 70%
        assertEq(sm.operatorShareBps(), 7000);
    }

    function test_SetTreasury() public {
        address newTreasury = address(0xBEEF);
        sm.setTreasury(newTreasury);
        assertEq(sm.treasury(), newTreasury);
    }

    function test_SetMaxSessionDuration() public {
        sm.setMaxSessionDuration(172800); // 48h
        assertEq(sm.maxSessionDuration(), 172800);
    }

    function test_AdminRevertsNotOwner() public {
        vm.startPrank(user1);

        vm.expectRevert();
        sm.setPricePerHour(1);

        vm.expectRevert();
        sm.setOperatorShare(1);

        vm.expectRevert();
        sm.setTreasury(user1);

        vm.expectRevert();
        sm.setMaxSessionDuration(1);

        vm.stopPrank();
    }

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    function test_EmitsSessionOpened() public {
        vm.prank(user1);
        vm.expectEmit(true, true, true, true);
        emit SessionManager.SessionOpened(1, user1, nodeOp, 0.001 ether, 3600);
        sm.openSession{value: 0.001 ether}(nodeOp, 3600);
    }

    function test_EmitsSessionClosed() public {
        vm.prank(user1);
        sm.openSession{value: 0.001 ether}(nodeOp, 3600);

        vm.prank(user1);
        vm.expectEmit(true, true, false, true);
        emit SessionManager.SessionClosed(1, user1, 0.0008 ether, 0.0002 ether);
        sm.closeSession(1);
    }
}
