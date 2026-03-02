// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable2Step.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import {IPayoutVault} from "./SubscriptionManager.sol";

interface INodeRegistry {
    function isRegistered(address operator) external view returns (bool);
}

/// @title SessionManager
/// @notice On-chain VPN session tracking and payment routing.
///         Users pay per-session (paid tier), and payments are split between
///         the node operator and the community treasury.
/// @dev Free-tier users (holding THIS card) skip payment. Paid-tier users
///      send ETH when opening a session; it's split on session close.
///      Designed to be token-agnostic for future $6529 token support.
contract SessionManager is Ownable2Step, ReentrancyGuard {

    // =========================================================================
    //                          TYPES
    // =========================================================================

    struct Session {
        address user;         // user wallet
        address node;         // node operator
        uint256 payment;      // ETH paid (0 for free tier)
        uint256 startedAt;    // timestamp
        uint256 duration;     // requested duration in seconds
        bool    active;       // whether session is still active
        bool    settled;      // whether payment has been distributed
    }

    // =========================================================================
    //                          STATE
    // =========================================================================

    /// @notice Session ID counter.
    uint256 public nextSessionId;

    /// @notice Session ID → session data.
    mapping(uint256 => Session) public sessions;

    /// @notice User → their current active session ID (0 = no active session).
    mapping(address => uint256) public activeSession;

    /// @notice Operator revenue share (basis points, e.g., 8000 = 80%).
    uint256 public operatorShareBps;

    /// @notice Community treasury address.
    address public treasury;

    /// @notice Session price per hour (in wei). 0 = free for all.
    uint256 public pricePerHour;

    /// @notice Maximum session duration (seconds).
    uint256 public maxSessionDuration;

    /// @notice Total sessions created.
    uint256 public totalSessions;

    /// @notice Total revenue collected (ETH).
    uint256 public totalRevenue;

    /// @notice Accumulated operator earnings (withdrawable).
    mapping(address => uint256) public operatorBalance;

    /// @notice Accumulated treasury balance (withdrawable).
    uint256 public treasuryBalance;

    /// @notice PayoutVault address for RAILGUN private payouts. When set, operator
    ///         earnings route to the vault instead of accumulating here.
    address public payoutVault;

    /// @notice NodeRegistry address used to validate node operators.
    address public nodeRegistry;

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    event SessionOpened(uint256 indexed sessionId, address indexed user, address indexed node, uint256 payment, uint256 duration);
    event SessionClosed(uint256 indexed sessionId, address indexed user, uint256 operatorPayout, uint256 treasuryPayout);
    event OperatorWithdrawal(address indexed operator, uint256 amount);
    event TreasuryWithdrawal(address indexed to, uint256 amount);
    event PriceUpdated(uint256 oldPrice, uint256 newPrice);
    event OperatorShareUpdated(uint256 oldShare, uint256 newShare);
    event TreasuryUpdated(address oldTreasury, address newTreasury);
    event NodeRegistryUpdated(address oldRegistry, address newRegistry);
    event RewardsDistributed(address[] operators, uint256[] amounts, uint256 totalDistributed);

    // =========================================================================
    //                          ERRORS
    // =========================================================================

    error SessionAlreadyActive();
    error NoActiveSession();
    error SessionNotActive();
    error SessionNotFound();
    error InsufficientPayment(uint256 sent, uint256 required);
    error InvalidDuration();
    error NotSessionParticipant();
    error NothingToWithdraw();
    error ZeroAddress();
    error ArrayLengthMismatch();
    error InsufficientTreasuryBalance(uint256 requested, uint256 available);
    error NodeNotRegistered();
    error NodeRegistryNotConfigured();

    // =========================================================================
    //                          CONSTRUCTOR
    // =========================================================================

    /// @param _treasury Community treasury address
    /// @param _operatorShareBps Operator revenue share in basis points (e.g., 8000 = 80%)
    /// @param _pricePerHour Session price per hour in wei
    /// @param _maxSessionDuration Maximum session duration in seconds
    constructor(
        address _treasury,
        uint256 _operatorShareBps,
        uint256 _pricePerHour,
        uint256 _maxSessionDuration
    ) Ownable(msg.sender) {
        if (_treasury == address(0)) revert ZeroAddress();
        require(_operatorShareBps <= 10000, "Share > 100%");

        treasury = _treasury;
        operatorShareBps = _operatorShareBps;
        pricePerHour = _pricePerHour;
        maxSessionDuration = _maxSessionDuration;
        nextSessionId = 1; // 0 means "no session"
    }

    // =========================================================================
    //                          SESSION LIFECYCLE
    // =========================================================================

    /// @notice Open a paid VPN session. Send ETH >= required for the duration.
    /// @param node The node operator address to connect to
    /// @param duration Session duration in seconds
    /// @return sessionId The created session ID
    function openSession(address node, uint256 duration) external payable nonReentrant returns (uint256 sessionId) {
        if (node == address(0)) revert ZeroAddress();
        if (activeSession[msg.sender] != 0) revert SessionAlreadyActive();
        if (duration == 0 || duration > maxSessionDuration) revert InvalidDuration();
        _validateNode(node);

        // Calculate required payment
        uint256 required = (pricePerHour * duration) / 3600;
        if (msg.value < required) revert InsufficientPayment(msg.value, required);

        sessionId = nextSessionId++;
        sessions[sessionId] = Session({
            user: msg.sender,
            node: node,
            payment: required,
            startedAt: block.timestamp,
            duration: duration,
            active: true,
            settled: false
        });

        activeSession[msg.sender] = sessionId;
        totalSessions++;
        totalRevenue += required;

        // Refund overpayment
        uint256 refund = msg.value - required;
        if (refund > 0) {
            (bool sent, ) = msg.sender.call{value: refund}("");
            require(sent, "ETH refund failed");
        }

        emit SessionOpened(sessionId, msg.sender, node, required, duration);
    }

    /// @notice Open a free-tier session (no payment required).
    ///         Called by the gateway after verifying free-tier status off-chain.
    /// @param user The user wallet address
    /// @param node The node operator address
    /// @param duration Session duration in seconds
    /// @return sessionId The created session ID
    function openFreeSession(address user, address node, uint256 duration) external onlyOwner returns (uint256 sessionId) {
        if (node == address(0)) revert ZeroAddress();
        if (activeSession[user] != 0) revert SessionAlreadyActive();
        if (duration == 0 || duration > maxSessionDuration) revert InvalidDuration();
        _validateNode(node);

        sessionId = nextSessionId++;
        sessions[sessionId] = Session({
            user: user,
            node: node,
            payment: 0,
            startedAt: block.timestamp,
            duration: duration,
            active: true,
            settled: false
        });

        activeSession[user] = sessionId;
        totalSessions++;

        emit SessionOpened(sessionId, user, node, 0, duration);
    }

    /// @notice Close a session and distribute payment.
    ///         Can be called by the user, the node operator, or governance.
    /// @param sessionId The session to close
    function closeSession(uint256 sessionId) external nonReentrant {
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) revert SessionNotFound();
        if (!s.active) revert SessionNotActive();

        // Only user, node operator, or owner can close
        if (msg.sender != s.user && msg.sender != s.node && msg.sender != owner()) {
            revert NotSessionParticipant();
        }

        s.active = false;
        activeSession[s.user] = 0;

        // Distribute payment if any
        if (s.payment > 0 && !s.settled) {
            s.settled = true;
            uint256 operatorPayout = (s.payment * operatorShareBps) / 10000;
            uint256 treasuryPayout = s.payment - operatorPayout;

            // Route operator share to PayoutVault (RAILGUN) if configured,
            // otherwise accumulate locally for legacy withdrawal.
            // Uses try/catch so a paused/broken vault cannot brick session close.
            if (operatorPayout > 0) {
                if (payoutVault != address(0)) {
                    try IPayoutVault(payoutVault).creditOperator{value: operatorPayout}(s.node) {
                        // credited to vault
                    } catch {
                        // vault reverted (paused, etc.) — fall back to local balance
                        operatorBalance[s.node] += operatorPayout;
                    }
                } else {
                    operatorBalance[s.node] += operatorPayout;
                }
            }
            treasuryBalance += treasuryPayout;

            emit SessionClosed(sessionId, s.user, operatorPayout, treasuryPayout);
        } else {
            emit SessionClosed(sessionId, s.user, 0, 0);
        }
    }

    // =========================================================================
    //                          WITHDRAWALS
    // =========================================================================

    /// @notice Withdraw accumulated earnings (for node operators).
    function withdrawOperatorEarnings() external nonReentrant {
        uint256 amount = operatorBalance[msg.sender];
        if (amount == 0) revert NothingToWithdraw();

        operatorBalance[msg.sender] = 0;
        (bool sent, ) = msg.sender.call{value: amount}("");
        require(sent, "ETH transfer failed");

        emit OperatorWithdrawal(msg.sender, amount);
    }

    /// @notice Withdraw treasury balance to the treasury address.
    function withdrawTreasury() external onlyOwner nonReentrant {
        uint256 amount = treasuryBalance;
        if (amount == 0) revert NothingToWithdraw();

        treasuryBalance = 0;
        (bool sent, ) = treasury.call{value: amount}("");
        require(sent, "ETH transfer failed");

        emit TreasuryWithdrawal(treasury, amount);
    }

    // =========================================================================
    //                          REWARDS DISTRIBUTION
    // =========================================================================

    /// @notice Distribute treasury funds to operators as rewards.
    ///         Governance decides amounts off-chain; this function executes the distribution.
    ///         Operators claim via withdrawOperatorEarnings().
    /// @param operators Array of operator addresses to reward
    /// @param amounts Array of reward amounts (must match operators length)
    function distributeRewards(address[] calldata operators, uint256[] calldata amounts) external onlyOwner nonReentrant {
        if (operators.length != amounts.length) revert ArrayLengthMismatch();

        uint256 total;
        for (uint256 i = 0; i < amounts.length; i++) {
            total += amounts[i];
        }
        if (total > treasuryBalance) revert InsufficientTreasuryBalance(total, treasuryBalance);

        treasuryBalance -= total;
        for (uint256 i = 0; i < operators.length; i++) {
            operatorBalance[operators[i]] += amounts[i];
        }

        emit RewardsDistributed(operators, amounts, total);
    }

    // =========================================================================
    //                          VIEW FUNCTIONS
    // =========================================================================

    /// @notice Get session details.
    function getSession(uint256 sessionId) external view returns (Session memory) {
        return sessions[sessionId];
    }

    /// @notice Get a user's active session ID (0 = none).
    function getActiveSessionId(address user) external view returns (uint256) {
        return activeSession[user];
    }

    /// @notice Check if a session has expired based on its duration.
    function isExpired(uint256 sessionId) external view returns (bool) {
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) return false;
        return block.timestamp > s.startedAt + s.duration;
    }

    /// @notice Calculate the required payment for a given duration.
    function calculatePrice(uint256 duration) external view returns (uint256) {
        return (pricePerHour * duration) / 3600;
    }

    // =========================================================================
    //                          ADMIN FUNCTIONS
    // =========================================================================

    /// @notice Update the price per hour.
    function setPricePerHour(uint256 newPrice) external onlyOwner {
        emit PriceUpdated(pricePerHour, newPrice);
        pricePerHour = newPrice;
    }

    /// @notice Update the operator revenue share.
    function setOperatorShare(uint256 newShareBps) external onlyOwner {
        require(newShareBps <= 10000, "Share > 100%");
        emit OperatorShareUpdated(operatorShareBps, newShareBps);
        operatorShareBps = newShareBps;
    }

    /// @notice Update the treasury address.
    function setTreasury(address newTreasury) external onlyOwner {
        if (newTreasury == address(0)) revert ZeroAddress();
        emit TreasuryUpdated(treasury, newTreasury);
        treasury = newTreasury;
    }

    /// @notice Update the maximum session duration.
    function setMaxSessionDuration(uint256 newMax) external onlyOwner {
        maxSessionDuration = newMax;
    }

    /// @notice Set the PayoutVault address for RAILGUN private payouts.
    ///         Set to address(0) to disable and revert to legacy balance accumulation.
    function setPayoutVault(address _payoutVault) external onlyOwner {
        payoutVault = _payoutVault;
    }

    /// @notice Set the NodeRegistry used to validate nodes in openSession.
    function setNodeRegistry(address _nodeRegistry) external onlyOwner {
        if (_nodeRegistry == address(0)) revert ZeroAddress();
        emit NodeRegistryUpdated(nodeRegistry, _nodeRegistry);
        nodeRegistry = _nodeRegistry;
    }

    function _validateNode(address node) internal view {
        if (nodeRegistry == address(0)) revert NodeRegistryNotConfigured();
        if (!INodeRegistry(nodeRegistry).isRegistered(node)) revert NodeNotRegistered();
    }
}
