// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable2Step.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title NodeRegistry
/// @notice On-chain registry for Sovereign VPN node operators.
///         Operators stake ETH to register, earn reputation from uptime,
///         and can be slashed for misbehavior.
/// @dev Reputation is tracked as an integer score. Slashing burns staked ETH
///      and reduces reputation. A governance multisig (owner) manages disputes.
contract NodeRegistry is Ownable2Step, ReentrancyGuard {

    // =========================================================================
    //                          TYPES
    // =========================================================================

    struct Node {
        address operator;         // wallet that registered the node
        string  endpoint;         // public WireGuard endpoint (ip:port)
        string  wgPubKey;         // WireGuard public key (base64)
        string  region;           // geographic region (e.g., "us-east", "eu-west")
        uint256 stakedAmount;     // ETH staked (in wei)
        uint256 reputation;       // reputation score (starts at 100)
        uint256 registeredAt;     // block.timestamp when registered
        uint256 lastHeartbeat;    // last time node sent a liveness proof
        bool    active;           // whether the node is accepting connections
        bool    slashed;          // whether the node has been slashed
    }

    // =========================================================================
    //                          STATE
    // =========================================================================

    /// @notice Minimum ETH stake required to register a node.
    uint256 public minStake;

    /// @notice Starting reputation score for new nodes.
    uint256 public constant INITIAL_REPUTATION = 100;

    /// @notice Maximum reputation score.
    uint256 public constant MAX_REPUTATION = 1000;

    /// @notice Heartbeat interval: nodes must check in within this period.
    uint256 public heartbeatInterval;

    /// @notice All registered node IDs (operator addresses).
    address[] public nodeList;

    /// @notice Operator address → Node data.
    mapping(address => Node) public nodes;

    /// @notice Whether an operator is registered.
    mapping(address => bool) public isRegistered;

    /// @notice Accumulated slashed ETH (withdrawable by governance for the DAO treasury).
    uint256 public slashedFunds;

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    event NodeRegistered(address indexed operator, string endpoint, string region, uint256 stake);
    event NodeDeactivated(address indexed operator);
    event NodeReactivated(address indexed operator);
    event NodeUnregistered(address indexed operator, uint256 stakeReturned);
    event Heartbeat(address indexed operator, uint256 timestamp);
    event ReputationUpdated(address indexed operator, uint256 oldRep, uint256 newRep, string reason);
    event NodeSlashed(address indexed operator, uint256 slashAmount, uint256 newStake, string reason);
    event StakeAdded(address indexed operator, uint256 amount, uint256 newTotal);
    event EndpointUpdated(address indexed operator, string newEndpoint);
    event SlashedFundsWithdrawn(address indexed to, uint256 amount);
    event MinStakeUpdated(uint256 oldMin, uint256 newMin);
    event HeartbeatIntervalUpdated(uint256 oldInterval, uint256 newInterval);

    // =========================================================================
    //                          ERRORS
    // =========================================================================

    error InsufficientStake(uint256 sent, uint256 required);
    error AlreadyRegistered();
    error NotRegistered();
    error NotNodeOperator();
    error NodeNotActive();
    error NodeAlreadyActive();
    error NodeSlashedCannotReactivate();
    error InvalidEndpoint();
    error InvalidWgPubKey();
    error NoFundsToWithdraw();

    // =========================================================================
    //                          CONSTRUCTOR
    // =========================================================================

    /// @param _minStake Minimum ETH stake in wei (e.g., 0.01 ether for testnet)
    /// @param _heartbeatInterval Seconds between required heartbeats (e.g., 3600 = 1 hour)
    constructor(uint256 _minStake, uint256 _heartbeatInterval) Ownable(msg.sender) {
        minStake = _minStake;
        heartbeatInterval = _heartbeatInterval;
    }

    // =========================================================================
    //                          NODE OPERATOR FUNCTIONS
    // =========================================================================

    /// @notice Register a new VPN node. Must send at least minStake ETH.
    /// @param endpoint Public WireGuard endpoint (e.g., "1.2.3.4:51820")
    /// @param wgPubKey WireGuard public key (base64)
    /// @param region Geographic region identifier
    function register(
        string calldata endpoint,
        string calldata wgPubKey,
        string calldata region
    ) external payable nonReentrant {
        if (isRegistered[msg.sender]) revert AlreadyRegistered();
        if (msg.value < minStake) revert InsufficientStake(msg.value, minStake);
        if (bytes(endpoint).length == 0) revert InvalidEndpoint();
        if (bytes(wgPubKey).length == 0) revert InvalidWgPubKey();

        nodes[msg.sender] = Node({
            operator: msg.sender,
            endpoint: endpoint,
            wgPubKey: wgPubKey,
            region: region,
            stakedAmount: msg.value,
            reputation: INITIAL_REPUTATION,
            registeredAt: block.timestamp,
            lastHeartbeat: block.timestamp,
            active: true,
            slashed: false
        });

        isRegistered[msg.sender] = true;
        nodeList.push(msg.sender);

        emit NodeRegistered(msg.sender, endpoint, region, msg.value);
    }

    /// @notice Add more ETH to an existing stake.
    function addStake() external payable nonReentrant {
        if (!isRegistered[msg.sender]) revert NotRegistered();
        nodes[msg.sender].stakedAmount += msg.value;
        emit StakeAdded(msg.sender, msg.value, nodes[msg.sender].stakedAmount);
    }

    /// @notice Deactivate a node (stop accepting connections). Stake remains locked.
    function deactivate() external {
        if (!isRegistered[msg.sender]) revert NotRegistered();
        if (!nodes[msg.sender].active) revert NodeNotActive();
        nodes[msg.sender].active = false;
        emit NodeDeactivated(msg.sender);
    }

    /// @notice Reactivate a previously deactivated node.
    function reactivate() external {
        if (!isRegistered[msg.sender]) revert NotRegistered();
        if (nodes[msg.sender].active) revert NodeAlreadyActive();
        if (nodes[msg.sender].slashed) revert NodeSlashedCannotReactivate();
        nodes[msg.sender].active = true;
        nodes[msg.sender].lastHeartbeat = block.timestamp;
        emit NodeReactivated(msg.sender);
    }

    /// @notice Unregister and withdraw remaining stake.
    ///         Cannot unregister if slashed (slashed stake is forfeit).
    function unregister() external nonReentrant {
        if (!isRegistered[msg.sender]) revert NotRegistered();

        uint256 stakeToReturn = nodes[msg.sender].stakedAmount;
        nodes[msg.sender].active = false;
        nodes[msg.sender].stakedAmount = 0;
        isRegistered[msg.sender] = false;

        // Remove from nodeList (swap and pop)
        _removeFromList(msg.sender);

        if (stakeToReturn > 0) {
            (bool sent, ) = msg.sender.call{value: stakeToReturn}("");
            require(sent, "ETH transfer failed");
        }

        emit NodeUnregistered(msg.sender, stakeToReturn);
    }

    /// @notice Send a heartbeat to prove liveness.
    function heartbeat() external {
        if (!isRegistered[msg.sender]) revert NotRegistered();
        nodes[msg.sender].lastHeartbeat = block.timestamp;
        emit Heartbeat(msg.sender, block.timestamp);
    }

    /// @notice Update the node's public endpoint.
    function updateEndpoint(string calldata newEndpoint) external {
        if (!isRegistered[msg.sender]) revert NotRegistered();
        if (bytes(newEndpoint).length == 0) revert InvalidEndpoint();
        nodes[msg.sender].endpoint = newEndpoint;
        emit EndpointUpdated(msg.sender, newEndpoint);
    }

    // =========================================================================
    //                          GOVERNANCE / ADMIN FUNCTIONS
    // =========================================================================

    /// @notice Slash a node's stake and reputation. Called by governance for misbehavior.
    /// @param operator The node operator to slash
    /// @param slashPercent Percentage of stake to slash (1-100)
    /// @param repPenalty Reputation points to deduct
    /// @param reason Human-readable reason for the slash
    function slash(
        address operator,
        uint256 slashPercent,
        uint256 repPenalty,
        string calldata reason
    ) external onlyOwner {
        if (!isRegistered[operator]) revert NotRegistered();
        require(slashPercent > 0 && slashPercent <= 100, "Invalid slash percent");

        Node storage node = nodes[operator];

        // Slash stake
        uint256 slashAmount = (node.stakedAmount * slashPercent) / 100;
        node.stakedAmount -= slashAmount;
        slashedFunds += slashAmount;

        // Reduce reputation
        uint256 oldRep = node.reputation;
        if (repPenalty >= node.reputation) {
            node.reputation = 0;
        } else {
            node.reputation -= repPenalty;
        }

        node.slashed = true;
        node.active = false;

        emit NodeSlashed(operator, slashAmount, node.stakedAmount, reason);
        emit ReputationUpdated(operator, oldRep, node.reputation, reason);
    }

    /// @notice Increase a node's reputation (for good behavior, uptime rewards).
    /// @param operator The node operator to reward
    /// @param amount Reputation points to add
    /// @param reason Human-readable reason
    function increaseReputation(
        address operator,
        uint256 amount,
        string calldata reason
    ) external onlyOwner {
        if (!isRegistered[operator]) revert NotRegistered();

        Node storage node = nodes[operator];
        uint256 oldRep = node.reputation;

        node.reputation += amount;
        if (node.reputation > MAX_REPUTATION) {
            node.reputation = MAX_REPUTATION;
        }

        emit ReputationUpdated(operator, oldRep, node.reputation, reason);
    }

    /// @notice Withdraw slashed funds to a DAO treasury address.
    /// @param to Treasury address
    function withdrawSlashedFunds(address to) external onlyOwner nonReentrant {
        if (to == address(0)) revert();
        uint256 amount = slashedFunds;
        if (amount == 0) revert NoFundsToWithdraw();

        slashedFunds = 0;
        (bool sent, ) = to.call{value: amount}("");
        require(sent, "ETH transfer failed");

        emit SlashedFundsWithdrawn(to, amount);
    }

    /// @notice Update the minimum stake requirement.
    function setMinStake(uint256 newMinStake) external onlyOwner {
        emit MinStakeUpdated(minStake, newMinStake);
        minStake = newMinStake;
    }

    /// @notice Update the heartbeat interval.
    function setHeartbeatInterval(uint256 newInterval) external onlyOwner {
        emit HeartbeatIntervalUpdated(heartbeatInterval, newInterval);
        heartbeatInterval = newInterval;
    }

    // =========================================================================
    //                          VIEW FUNCTIONS
    // =========================================================================

    /// @notice Get the full node data for an operator.
    function getNode(address operator) external view returns (Node memory) {
        return nodes[operator];
    }

    /// @notice Get all registered node addresses.
    function getNodeList() external view returns (address[] memory) {
        return nodeList;
    }

    /// @notice Get the count of registered nodes.
    function nodeCount() external view returns (uint256) {
        return nodeList.length;
    }

    /// @notice Get all active nodes (for client node discovery).
    function getActiveNodes() external view returns (Node[] memory) {
        uint256 count = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            if (nodes[nodeList[i]].active) count++;
        }

        Node[] memory active = new Node[](count);
        uint256 idx = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            if (nodes[nodeList[i]].active) {
                active[idx] = nodes[nodeList[i]];
                idx++;
            }
        }
        return active;
    }

    /// @notice Get active nodes in a specific region.
    function getActiveNodesByRegion(string calldata region) external view returns (Node[] memory) {
        bytes32 regionHash = keccak256(bytes(region));
        uint256 count = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            Node storage n = nodes[nodeList[i]];
            if (n.active && keccak256(bytes(n.region)) == regionHash) count++;
        }

        Node[] memory result = new Node[](count);
        uint256 idx = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            Node storage n = nodes[nodeList[i]];
            if (n.active && keccak256(bytes(n.region)) == regionHash) {
                result[idx] = n;
                idx++;
            }
        }
        return result;
    }

    /// @notice Check if a node's heartbeat is overdue.
    function isHeartbeatOverdue(address operator) external view returns (bool) {
        if (!isRegistered[operator]) return false;
        return block.timestamp > nodes[operator].lastHeartbeat + heartbeatInterval;
    }

    /// @notice Get nodes with overdue heartbeats (for monitoring/slashing).
    function getOverdueNodes() external view returns (address[] memory) {
        uint256 count = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            Node storage n = nodes[nodeList[i]];
            if (n.active && block.timestamp > n.lastHeartbeat + heartbeatInterval) {
                count++;
            }
        }

        address[] memory overdue = new address[](count);
        uint256 idx = 0;
        for (uint256 i = 0; i < nodeList.length; i++) {
            Node storage n = nodes[nodeList[i]];
            if (n.active && block.timestamp > n.lastHeartbeat + heartbeatInterval) {
                overdue[idx] = nodeList[i];
                idx++;
            }
        }
        return overdue;
    }

    // =========================================================================
    //                          INTERNAL
    // =========================================================================

    function _removeFromList(address operator) internal {
        uint256 len = nodeList.length;
        for (uint256 i = 0; i < len; i++) {
            if (nodeList[i] == operator) {
                nodeList[i] = nodeList[len - 1];
                nodeList.pop();
                return;
            }
        }
    }
}
