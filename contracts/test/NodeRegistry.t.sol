// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/NodeRegistry.sol";
import "../src/TestMemes.sol";

contract NodeRegistryTest is Test {
    NodeRegistry public registry;
    TestMemes public memes;

    address public owner = address(this);
    address public operator1 = address(0x1);
    address public operator2 = address(0x2);
    address public operator3 = address(0x3);
    address public randomUser = address(0x99);

    uint256 public constant MIN_STAKE = 0.01 ether;
    uint256 public constant HEARTBEAT_INTERVAL = 3600; // 1 hour
    uint256 public constant OPERATOR_CARD_ID = 1;

    function setUp() public {
        // Deploy TestMemes and mint operator card to test operators
        memes = new TestMemes();
        memes.mint(operator1, OPERATOR_CARD_ID, 1);
        memes.mint(operator2, OPERATOR_CARD_ID, 1);
        memes.mint(operator3, OPERATOR_CARD_ID, 1);

        registry = new NodeRegistry(MIN_STAKE, HEARTBEAT_INTERVAL, address(memes), OPERATOR_CARD_ID);

        // Fund operators
        vm.deal(operator1, 10 ether);
        vm.deal(operator2, 10 ether);
        vm.deal(operator3, 10 ether);
    }

    // =========================================================================
    //                          REGISTRATION
    // =========================================================================

    function test_Register() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "wgPubKey1==", "us-east");

        assertTrue(registry.isRegistered(operator1));
        assertEq(registry.nodeCount(), 1);

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(node.operator, operator1);
        assertEq(node.stakedAmount, 0.05 ether);
        assertTrue(node.active);
        assertFalse(node.slashed);
        assertEq(keccak256(bytes(node.endpoint)), keccak256(bytes("1.2.3.4:51820")));
        assertEq(keccak256(bytes(node.wgPubKey)), keccak256(bytes("wgPubKey1==")));
        assertEq(keccak256(bytes(node.region)), keccak256(bytes("us-east")));
    }

    function test_RegisterMultipleNodes() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key1==", "us-east");

        vm.prank(operator2);
        registry.register{value: 0.1 ether}("5.6.7.8:51820", "key2==", "eu-west");

        assertEq(registry.nodeCount(), 2);

        address[] memory list = registry.getNodeList();
        assertEq(list.length, 2);
    }

    function test_RegisterRevertsInsufficientStake() public {
        vm.prank(operator1);
        vm.expectRevert(abi.encodeWithSelector(NodeRegistry.InsufficientStake.selector, 0.005 ether, MIN_STAKE));
        registry.register{value: 0.005 ether}("1.2.3.4:51820", "key==", "us-east");
    }

    function test_RegisterRevertsAlreadyRegistered() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.AlreadyRegistered.selector);
        registry.register{value: 0.05 ether}("5.6.7.8:51820", "key2==", "eu-west");
    }

    function test_RegisterRevertsEmptyEndpoint() public {
        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.InvalidEndpoint.selector);
        registry.register{value: 0.05 ether}("", "key==", "us-east");
    }

    function test_RegisterRevertsEmptyWgPubKey() public {
        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.InvalidWgPubKey.selector);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "", "us-east");
    }

    // =========================================================================
    //                          STAKE MANAGEMENT
    // =========================================================================

    function test_AddStake() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.addStake{value: 0.1 ether}();

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(node.stakedAmount, 0.15 ether);
    }

    function test_AddStakeRevertsNotRegistered() public {
        vm.deal(randomUser, 1 ether);
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotRegistered.selector);
        registry.addStake{value: 0.1 ether}();
    }

    // =========================================================================
    //                          DEACTIVATE / REACTIVATE
    // =========================================================================

    function test_Deactivate() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.deactivate();

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertFalse(node.active);
    }

    function test_Reactivate() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.deactivate();

        vm.prank(operator1);
        registry.reactivate();

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertTrue(node.active);
    }

    function test_DeactivateRevertsNotActive() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.deactivate();

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.NodeNotActive.selector);
        registry.deactivate();
    }

    function test_ReactivateRevertsAlreadyActive() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.NodeAlreadyActive.selector);
        registry.reactivate();
    }

    function test_ReactivateRevertsIfSlashed() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        // Slash the node (from owner)
        registry.slash(operator1, 50, "misbehavior");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.NodeSlashedCannotReactivate.selector);
        registry.reactivate();
    }

    // =========================================================================
    //                          UNREGISTER
    // =========================================================================

    function test_Unregister() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        uint256 balBefore = operator1.balance;

        vm.prank(operator1);
        registry.unregister();

        assertFalse(registry.isRegistered(operator1));
        assertEq(registry.nodeCount(), 0);
        assertEq(operator1.balance, balBefore + 0.05 ether);
    }

    function test_UnregisterMiddleOfList() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key1==", "us-east");

        vm.prank(operator2);
        registry.register{value: 0.05 ether}("5.6.7.8:51820", "key2==", "eu-west");

        vm.prank(operator3);
        registry.register{value: 0.05 ether}("9.10.11.12:51820", "key3==", "ap-south");

        // Unregister the middle one
        vm.prank(operator2);
        registry.unregister();

        assertEq(registry.nodeCount(), 2);
        assertFalse(registry.isRegistered(operator2));
        assertTrue(registry.isRegistered(operator1));
        assertTrue(registry.isRegistered(operator3));
    }

    function test_UnregisterRevertsNotRegistered() public {
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotRegistered.selector);
        registry.unregister();
    }

    // =========================================================================
    //                          HEARTBEAT
    // =========================================================================

    function test_Heartbeat() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        // Advance time
        vm.warp(block.timestamp + 1800);

        vm.prank(operator1);
        registry.heartbeat();

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(node.lastHeartbeat, block.timestamp);
    }

    function test_HeartbeatOverdue() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        // Not overdue yet
        assertFalse(registry.isHeartbeatOverdue(operator1));

        // Advance past heartbeat interval
        vm.warp(block.timestamp + HEARTBEAT_INTERVAL + 1);

        assertTrue(registry.isHeartbeatOverdue(operator1));

        // Send heartbeat
        vm.prank(operator1);
        registry.heartbeat();

        assertFalse(registry.isHeartbeatOverdue(operator1));
    }

    function test_GetOverdueNodes() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key1==", "us-east");

        vm.prank(operator2);
        registry.register{value: 0.05 ether}("5.6.7.8:51820", "key2==", "eu-west");

        // Advance time so both are overdue
        vm.warp(block.timestamp + HEARTBEAT_INTERVAL + 1);

        address[] memory overdue = registry.getOverdueNodes();
        assertEq(overdue.length, 2);

        // Operator1 sends heartbeat — only operator2 should be overdue
        vm.prank(operator1);
        registry.heartbeat();

        overdue = registry.getOverdueNodes();
        assertEq(overdue.length, 1);
        assertEq(overdue[0], operator2);
    }

    // =========================================================================
    //                          SLASHING (stake only — rep is off-chain via 6529)
    // =========================================================================

    function test_Slash() public {
        vm.prank(operator1);
        registry.register{value: 1 ether}("1.2.3.4:51820", "key==", "us-east");

        // Slash 50% stake
        registry.slash(operator1, 50, "poor uptime");

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(node.stakedAmount, 0.5 ether);
        assertTrue(node.slashed);
        assertFalse(node.active);
        assertEq(registry.slashedFunds(), 0.5 ether);
    }

    function test_SlashEntireStake() public {
        vm.prank(operator1);
        registry.register{value: 1 ether}("1.2.3.4:51820", "key==", "us-east");

        registry.slash(operator1, 100, "severe violation");

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(node.stakedAmount, 0);
        assertEq(registry.slashedFunds(), 1 ether);
    }

    function test_SlashRevertsNotOwner() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(randomUser);
        vm.expectRevert();
        registry.slash(operator1, 50, "unauthorized");
    }

    function test_SlashRevertsNotRegistered() public {
        vm.expectRevert(NodeRegistry.NotRegistered.selector);
        registry.slash(randomUser, 50, "not a node");
    }

    // =========================================================================
    //                          SLASHED FUNDS WITHDRAWAL
    // =========================================================================

    function test_WithdrawSlashedFunds() public {
        vm.prank(operator1);
        registry.register{value: 1 ether}("1.2.3.4:51820", "key==", "us-east");

        registry.slash(operator1, 50, "misbehavior");

        address treasury = address(0xDAD);
        uint256 balBefore = treasury.balance;

        registry.withdrawSlashedFunds(treasury);

        assertEq(treasury.balance, balBefore + 0.5 ether);
        assertEq(registry.slashedFunds(), 0);
    }

    function test_WithdrawSlashedFundsRevertsNoFunds() public {
        vm.expectRevert(NodeRegistry.NoFundsToWithdraw.selector);
        registry.withdrawSlashedFunds(address(0xDAD));
    }

    // =========================================================================
    //                          ENDPOINT UPDATE
    // =========================================================================

    function test_UpdateEndpoint() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.updateEndpoint("99.99.99.99:51820");

        NodeRegistry.Node memory node = registry.getNode(operator1);
        assertEq(keccak256(bytes(node.endpoint)), keccak256(bytes("99.99.99.99:51820")));
    }

    function test_UpdateEndpointRevertsEmpty() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.InvalidEndpoint.selector);
        registry.updateEndpoint("");
    }

    // =========================================================================
    //                          NODE DISCOVERY (VIEW FUNCTIONS)
    // =========================================================================

    function test_GetActiveNodes() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key1==", "us-east");

        vm.prank(operator2);
        registry.register{value: 0.05 ether}("5.6.7.8:51820", "key2==", "eu-west");

        vm.prank(operator3);
        registry.register{value: 0.05 ether}("9.10.11.12:51820", "key3==", "us-east");

        NodeRegistry.Node[] memory active = registry.getActiveNodes();
        assertEq(active.length, 3);

        // Deactivate one
        vm.prank(operator2);
        registry.deactivate();

        active = registry.getActiveNodes();
        assertEq(active.length, 2);
    }

    function test_GetActiveNodesByRegion() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key1==", "us-east");

        vm.prank(operator2);
        registry.register{value: 0.05 ether}("5.6.7.8:51820", "key2==", "eu-west");

        vm.prank(operator3);
        registry.register{value: 0.05 ether}("9.10.11.12:51820", "key3==", "us-east");

        NodeRegistry.Node[] memory usEast = registry.getActiveNodesByRegion("us-east");
        assertEq(usEast.length, 2);

        NodeRegistry.Node[] memory euWest = registry.getActiveNodesByRegion("eu-west");
        assertEq(euWest.length, 1);

        NodeRegistry.Node[] memory apSouth = registry.getActiveNodesByRegion("ap-south");
        assertEq(apSouth.length, 0);
    }

    // =========================================================================
    //                          ADMIN CONFIG
    // =========================================================================

    function test_SetMinStake() public {
        registry.setMinStake(0.1 ether);
        assertEq(registry.minStake(), 0.1 ether);

        // Registration with old min should fail
        vm.prank(operator1);
        vm.expectRevert(abi.encodeWithSelector(NodeRegistry.InsufficientStake.selector, 0.05 ether, 0.1 ether));
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");
    }

    function test_SetHeartbeatInterval() public {
        registry.setHeartbeatInterval(7200);
        assertEq(registry.heartbeatInterval(), 7200);
    }

    function test_AdminFunctionsRevertNotOwner() public {
        vm.startPrank(randomUser);

        vm.expectRevert();
        registry.setMinStake(1 ether);

        vm.expectRevert();
        registry.setHeartbeatInterval(1);

        vm.expectRevert();
        registry.withdrawSlashedFunds(randomUser);

        vm.stopPrank();
    }

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    function test_EmitsNodeRegistered() public {
        vm.prank(operator1);
        vm.expectEmit(true, false, false, true);
        emit NodeRegistry.NodeRegistered(operator1, "1.2.3.4:51820", "us-east", 0.05 ether);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");
    }

    function test_EmitsHeartbeat() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectEmit(true, false, false, true);
        emit NodeRegistry.Heartbeat(operator1, block.timestamp);
        registry.heartbeat();
    }

    function test_EmitsNodeSlashed() public {
        vm.prank(operator1);
        registry.register{value: 1 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.expectEmit(true, false, false, true);
        emit NodeRegistry.NodeSlashed(operator1, 0.5 ether, 0.5 ether, "test slash");
        registry.slash(operator1, 50, "test slash");
    }

    // =========================================================================
    //                          CARD-GATING (Option C)
    // =========================================================================

    function test_RegisterRevertsNotEligibleOperator() public {
        // randomUser has no operator card
        vm.deal(randomUser, 10 ether);
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotEligibleOperator.selector);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");
    }

    function test_IsEligibleOperator() public view {
        // operator1 has the card
        assertTrue(registry.isEligibleOperator(operator1));
        // randomUser does not
        assertFalse(registry.isEligibleOperator(randomUser));
    }

    function test_ReactivateRevertsIfCardTransferred() public {
        // operator1 registers, deactivates, then transfers card away
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.deactivate();

        // Transfer operator card away
        vm.prank(operator1);
        memes.safeTransferFrom(operator1, randomUser, OPERATOR_CARD_ID, 1, "");

        // Reactivation should fail — no longer holds card
        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.NotEligibleOperator.selector);
        registry.reactivate();
    }

    function test_RegisterAfterReceivingCard() public {
        // randomUser starts without card, receives it, then registers
        vm.deal(randomUser, 10 ether);

        // Cannot register without card
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotEligibleOperator.selector);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        // Mint card to randomUser
        memes.mint(randomUser, OPERATOR_CARD_ID, 1);

        // Now registration succeeds
        vm.prank(randomUser);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        assertTrue(registry.isRegistered(randomUser));
    }

    function test_ConstructorStoresMemesAndCardId() public view {
        assertEq(registry.memesContract(), address(memes));
        assertEq(registry.operatorCardId(), OPERATOR_CARD_ID);
    }

    function test_SetOperatorCardId() public {
        uint256 newCardId = 42;

        // randomUser has no card 1, can't register
        vm.deal(randomUser, 10 ether);
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotEligibleOperator.selector);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        // Mint new card to randomUser, change required card ID
        memes.mint(randomUser, newCardId, 1);
        registry.setOperatorCardId(newCardId);
        assertEq(registry.operatorCardId(), newCardId);

        // randomUser can now register with new card
        vm.prank(randomUser);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");
        assertTrue(registry.isRegistered(randomUser));

        // operator1 still has old card (ID 1) but NOT new card — isEligibleOperator returns false
        assertFalse(registry.isEligibleOperator(operator1));
    }

    function test_SetOperatorCardIdRevertsNotOwner() public {
        vm.prank(randomUser);
        vm.expectRevert();
        registry.setOperatorCardId(42);
    }

    function test_SetOperatorCardIdEmitsEvent() public {
        vm.expectEmit(false, false, false, true);
        emit NodeRegistry.OperatorCardIdUpdated(OPERATOR_CARD_ID, 42);
        registry.setOperatorCardId(42);
    }

    // =========================================================================
    //                          RAILGUN ADDRESS
    // =========================================================================

    function test_SetRailgunAddress() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.setRailgunAddress("0zkTestAddress123");

        string memory addr = registry.getRailgunAddress(operator1);
        assertEq(keccak256(bytes(addr)), keccak256(bytes("0zkTestAddress123")));
    }

    function test_UpdateRailgunAddress() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        registry.setRailgunAddress("0zkFirst");

        vm.prank(operator1);
        registry.setRailgunAddress("0zkSecond");

        string memory addr = registry.getRailgunAddress(operator1);
        assertEq(keccak256(bytes(addr)), keccak256(bytes("0zkSecond")));
    }

    function test_SetRailgunAddressRevertsNotRegistered() public {
        vm.prank(randomUser);
        vm.expectRevert(NodeRegistry.NotRegistered.selector);
        registry.setRailgunAddress("0zkTestAddress");
    }

    function test_SetRailgunAddressRevertsInvalidPrefix() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.InvalidRailgunAddress.selector);
        registry.setRailgunAddress("0xNotRailgun");
    }

    function test_SetRailgunAddressRevertsEmpty() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectRevert(NodeRegistry.InvalidRailgunAddress.selector);
        registry.setRailgunAddress("");
    }

    function test_GetRailgunAddressUnregistered() public view {
        string memory addr = registry.getRailgunAddress(randomUser);
        assertEq(bytes(addr).length, 0);
    }

    function test_EmitsRailgunAddressSet() public {
        vm.prank(operator1);
        registry.register{value: 0.05 ether}("1.2.3.4:51820", "key==", "us-east");

        vm.prank(operator1);
        vm.expectEmit(true, false, false, true);
        emit NodeRegistry.RailgunAddressSet(operator1, "0zkTestAddr");
        registry.setRailgunAddress("0zkTestAddr");
    }
}
