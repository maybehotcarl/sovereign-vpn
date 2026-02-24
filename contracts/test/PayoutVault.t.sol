// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/PayoutVault.sol";

contract PayoutVaultTest is Test {
    PayoutVault public vault;

    // Accept ETH transfers (needed for emergencyWithdrawETH to owner = this contract)
    receive() external payable {}

    address public owner = address(this);
    address public executor = address(0xE0E0);
    address public sessionMgr = address(0xA1A1);
    address public subMgr = address(0xA2A2);
    address public operator1 = address(0x1);
    address public operator2 = address(0x2);
    address public randomUser = address(0x99);

    function setUp() public {
        vault = new PayoutVault(executor);
        vault.authorizeSource(sessionMgr);
        vault.authorizeSource(subMgr);

        vm.deal(sessionMgr, 100 ether);
        vm.deal(subMgr, 100 ether);
        vm.deal(executor, 10 ether);
    }

    // =========================================================================
    //                          CREDIT OPERATOR
    // =========================================================================

    function test_CreditOperator() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        assertEq(vault.pendingPayouts(operator1), 1 ether);
        assertEq(vault.totalPending(), 1 ether);
        assertEq(address(vault).balance, 1 ether);
    }

    function test_CreditMultipleOperators() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        vm.prank(subMgr);
        vault.creditOperator{value: 2 ether}(operator2);

        assertEq(vault.pendingPayouts(operator1), 1 ether);
        assertEq(vault.pendingPayouts(operator2), 2 ether);
        assertEq(vault.totalPending(), 3 ether);
    }

    function test_CreditAccumulates() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 0.5 ether}(operator1);

        vm.prank(subMgr);
        vault.creditOperator{value: 0.3 ether}(operator1);

        assertEq(vault.pendingPayouts(operator1), 0.8 ether);
        assertEq(vault.totalPending(), 0.8 ether);
    }

    function test_CreditRevertsUnauthorized() public {
        vm.prank(randomUser);
        vm.deal(randomUser, 1 ether);
        vm.expectRevert(PayoutVault.Unauthorized.selector);
        vault.creditOperator{value: 1 ether}(operator1);
    }

    function test_CreditRevertsZeroAmount() public {
        vm.prank(sessionMgr);
        vm.expectRevert(PayoutVault.ZeroAmount.selector);
        vault.creditOperator{value: 0}(operator1);
    }

    function test_CreditRevertsZeroAddress() public {
        vm.prank(sessionMgr);
        vm.expectRevert(PayoutVault.ZeroAddress.selector);
        vault.creditOperator{value: 1 ether}(address(0));
    }

    function test_CreditRevertsPaused() public {
        vault.pause();
        vm.prank(sessionMgr);
        vm.expectRevert(PayoutVault.ContractPaused.selector);
        vault.creditOperator{value: 1 ether}(operator1);
    }

    // =========================================================================
    //                          PROCESS PAYOUT
    // =========================================================================

    function test_ProcessPayout() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        uint256 executorBefore = executor.balance;

        vm.prank(executor);
        vault.processPayout(operator1, 1 ether);

        assertEq(vault.pendingPayouts(operator1), 0);
        assertEq(vault.totalPending(), 0);
        assertEq(vault.processedPayouts(operator1), 1 ether);
        assertEq(executor.balance, executorBefore + 1 ether);
    }

    function test_ProcessPartialPayout() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 2 ether}(operator1);

        vm.prank(executor);
        vault.processPayout(operator1, 0.5 ether);

        assertEq(vault.pendingPayouts(operator1), 1.5 ether);
        assertEq(vault.processedPayouts(operator1), 0.5 ether);
    }

    function test_ProcessPayoutRevertsInsufficient() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 0.5 ether}(operator1);

        vm.prank(executor);
        vm.expectRevert(abi.encodeWithSelector(
            PayoutVault.InsufficientPending.selector, operator1, 1 ether, 0.5 ether
        ));
        vault.processPayout(operator1, 1 ether);
    }

    function test_ProcessPayoutRevertsNotExecutor() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        vm.prank(randomUser);
        vm.expectRevert(PayoutVault.Unauthorized.selector);
        vault.processPayout(operator1, 1 ether);
    }

    function test_ProcessPayoutRevertsPaused() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        vault.pause();

        vm.prank(executor);
        vm.expectRevert(PayoutVault.ContractPaused.selector);
        vault.processPayout(operator1, 1 ether);
    }

    // =========================================================================
    //                          BATCH PAYOUT
    // =========================================================================

    function test_ProcessBatchPayout() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        vm.prank(subMgr);
        vault.creditOperator{value: 2 ether}(operator2);

        address[] memory operators = new address[](2);
        operators[0] = operator1;
        operators[1] = operator2;

        uint256[] memory amounts = new uint256[](2);
        amounts[0] = 1 ether;
        amounts[1] = 2 ether;

        uint256 executorBefore = executor.balance;

        vm.prank(executor);
        vault.processBatchPayout(operators, amounts);

        assertEq(vault.pendingPayouts(operator1), 0);
        assertEq(vault.pendingPayouts(operator2), 0);
        assertEq(vault.totalPending(), 0);
        assertEq(executor.balance, executorBefore + 3 ether);
    }

    function test_ProcessBatchPayoutRevertsLengthMismatch() public {
        address[] memory operators = new address[](2);
        uint256[] memory amounts = new uint256[](1);

        vm.prank(executor);
        vm.expectRevert(PayoutVault.ArrayLengthMismatch.selector);
        vault.processBatchPayout(operators, amounts);
    }

    // =========================================================================
    //                          ADMIN
    // =========================================================================

    function test_SetPayoutExecutor() public {
        address newExec = address(0xBEEF);
        vault.setPayoutExecutor(newExec);
        assertEq(vault.payoutExecutor(), newExec);
    }

    function test_AuthorizeAndRevokeSource() public {
        address newSource = address(0xFEED);
        vault.authorizeSource(newSource);
        assertTrue(vault.authorizedSources(newSource));

        vault.revokeSource(newSource);
        assertFalse(vault.authorizedSources(newSource));
    }

    function test_PauseUnpause() public {
        vault.pause();
        assertTrue(vault.paused());

        vault.unpause();
        assertFalse(vault.paused());
    }

    function test_AdminRevertsNotOwner() public {
        vm.startPrank(randomUser);

        vm.expectRevert();
        vault.setPayoutExecutor(randomUser);

        vm.expectRevert();
        vault.authorizeSource(randomUser);

        vm.expectRevert();
        vault.revokeSource(randomUser);

        vm.expectRevert();
        vault.pause();

        vm.expectRevert();
        vault.emergencyWithdrawETH();

        vm.stopPrank();
    }

    // =========================================================================
    //                          EMERGENCY
    // =========================================================================

    function test_EmergencyWithdrawETH() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 5 ether}(operator1);

        uint256 ownerBefore = owner.balance;
        vault.emergencyWithdrawETH();

        assertEq(address(vault).balance, 0);
        assertEq(owner.balance, ownerBefore + 5 ether);
    }

    function test_EmergencyWithdrawETHRevertsZeroBalance() public {
        vm.expectRevert(PayoutVault.ZeroAmount.selector);
        vault.emergencyWithdrawETH();
    }

    // =========================================================================
    //                          RECEIVE
    // =========================================================================

    function test_ReceiveETH() public {
        (bool sent, ) = address(vault).call{value: 1 ether}("");
        assertTrue(sent);
        assertEq(address(vault).balance, 1 ether);
    }

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    function test_EmitsOperatorCredited() public {
        vm.prank(sessionMgr);
        vm.expectEmit(true, true, false, true);
        emit PayoutVault.OperatorCredited(operator1, sessionMgr, 1 ether);
        vault.creditOperator{value: 1 ether}(operator1);
    }

    function test_EmitsPayoutProcessed() public {
        vm.prank(sessionMgr);
        vault.creditOperator{value: 1 ether}(operator1);

        vm.prank(executor);
        vm.expectEmit(true, false, false, true);
        emit PayoutVault.PayoutProcessed(operator1, 1 ether);
        vault.processPayout(operator1, 1 ether);
    }
}
