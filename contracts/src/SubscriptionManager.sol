// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable2Step.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title SubscriptionManager
/// @notice Time-based VPN subscription management with tiered pricing.
///         Users buy 7/30/90/365-day access in a single transaction.
///         Payments are split between the node operator and the community treasury.
/// @dev Coexists with SessionManager — gateway checks subscription first, falls
///      back to 24h sessions. Subscribers can disconnect and reconnect freely
///      within their subscription period.
contract SubscriptionManager is Ownable2Step, ReentrancyGuard {

    // =========================================================================
    //                          TYPES
    // =========================================================================

    struct TierConfig {
        uint256 price;      // wei
        uint256 duration;   // seconds
        bool    active;
    }

    struct Subscription {
        address user;
        address node;       // operator that receives 80% of payment
        uint256 payment;    // most recent payment (wei)
        uint256 startedAt;
        uint256 expiresAt;
        uint8   tier;
    }

    // =========================================================================
    //                          STATE
    // =========================================================================

    /// @notice Tier ID → configuration.
    mapping(uint8 => TierConfig) public tiers;

    /// @notice User → their active subscription.
    mapping(address => Subscription) public subscriptions;

    /// @notice Operator revenue share (basis points, e.g., 8000 = 80%).
    uint256 public operatorShareBps;

    /// @notice Community treasury address.
    address public treasury;

    /// @notice Accumulated operator earnings (withdrawable).
    mapping(address => uint256) public operatorBalance;

    /// @notice Accumulated treasury balance (withdrawable).
    uint256 public treasuryBalance;

    /// @notice Total subscriptions created.
    uint256 public totalSubscriptions;

    /// @notice Total revenue collected (ETH).
    uint256 public totalRevenue;

    /// @notice Tracks which tier IDs have been configured.
    uint8[] internal _tierIds;
    mapping(uint8 => bool) internal _tierExists;

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    event Subscribed(address indexed user, address indexed node, uint8 indexed tier, uint256 payment, uint256 expiresAt);
    event Renewed(address indexed user, address indexed node, uint8 indexed tier, uint256 payment, uint256 expiresAt);
    event OperatorWithdrawal(address indexed operator, uint256 amount);
    event TreasuryWithdrawal(address indexed to, uint256 amount);
    event TierUpdated(uint8 indexed tierId, uint256 price, uint256 duration, bool active);

    // =========================================================================
    //                          ERRORS
    // =========================================================================

    error AlreadySubscribed();
    error NoActiveSubscription();
    error TierNotActive();
    error TierNotFound();
    error InsufficientPayment(uint256 sent, uint256 required);
    error NothingToWithdraw();
    error ZeroAddress();

    // =========================================================================
    //                          CONSTRUCTOR
    // =========================================================================

    /// @param _treasury Community treasury address
    /// @param _operatorShareBps Operator revenue share in basis points (e.g., 8000 = 80%)
    constructor(
        address _treasury,
        uint256 _operatorShareBps
    ) Ownable(msg.sender) {
        if (_treasury == address(0)) revert ZeroAddress();
        require(_operatorShareBps <= 10000, "Share > 100%");

        treasury = _treasury;
        operatorShareBps = _operatorShareBps;
    }

    // =========================================================================
    //                          SUBSCRIPTION LIFECYCLE
    // =========================================================================

    /// @notice Purchase a new subscription. Send ETH >= tier price.
    /// @param node The node operator address to connect to
    /// @param tierId The subscription tier ID
    function subscribe(address node, uint8 tierId) external payable nonReentrant {
        // Must not have an active subscription
        Subscription storage existing = subscriptions[msg.sender];
        if (existing.expiresAt > block.timestamp) revert AlreadySubscribed();

        TierConfig storage t = tiers[tierId];
        if (!t.active) revert TierNotActive();
        if (t.duration == 0) revert TierNotFound();
        if (msg.value < t.price) revert InsufficientPayment(msg.value, t.price);

        uint256 expiresAt = block.timestamp + t.duration;

        subscriptions[msg.sender] = Subscription({
            user: msg.sender,
            node: node,
            payment: msg.value,
            startedAt: block.timestamp,
            expiresAt: expiresAt,
            tier: tierId
        });

        // Distribute payment immediately (80/20 split)
        _distributePayment(node, msg.value);

        totalSubscriptions++;
        totalRevenue += msg.value;

        emit Subscribed(msg.sender, node, tierId, msg.value, expiresAt);
    }

    /// @notice Renew or extend a subscription. Stacks on top of remaining time.
    /// @param tierId The subscription tier ID for the renewal
    /// @param node The node operator (pass address(0) to keep current node)
    function renewSubscription(uint8 tierId, address node) external payable nonReentrant {
        TierConfig storage t = tiers[tierId];
        if (!t.active) revert TierNotActive();
        if (t.duration == 0) revert TierNotFound();
        if (msg.value < t.price) revert InsufficientPayment(msg.value, t.price);

        Subscription storage sub = subscriptions[msg.sender];

        // Determine the effective node
        address effectiveNode = node;
        if (effectiveNode == address(0)) {
            if (sub.expiresAt == 0) revert NoActiveSubscription();
            effectiveNode = sub.node;
        }

        // Stack: new expiry = max(now, currentExpiry) + tier.duration
        uint256 base = block.timestamp;
        if (sub.expiresAt > base) {
            base = sub.expiresAt;
        }
        uint256 newExpiresAt = base + t.duration;

        sub.user = msg.sender;
        sub.node = effectiveNode;
        sub.payment = msg.value;
        sub.startedAt = block.timestamp;
        sub.expiresAt = newExpiresAt;
        sub.tier = tierId;

        // Distribute payment immediately
        _distributePayment(effectiveNode, msg.value);

        totalRevenue += msg.value;

        emit Renewed(msg.sender, effectiveNode, tierId, msg.value, newExpiresAt);
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
    //                          VIEW FUNCTIONS
    // =========================================================================

    /// @notice Check if a user has an active subscription.
    function hasActiveSubscription(address user) external view returns (bool) {
        return subscriptions[user].expiresAt > block.timestamp;
    }

    /// @notice Get a user's subscription details.
    function getSubscription(address user) external view returns (Subscription memory) {
        return subscriptions[user];
    }

    /// @notice Get remaining subscription time in seconds (0 if expired).
    function remainingTime(address user) external view returns (uint256) {
        Subscription storage sub = subscriptions[user];
        if (sub.expiresAt <= block.timestamp) return 0;
        return sub.expiresAt - block.timestamp;
    }

    /// @notice Get all active tier IDs.
    function getActiveTierIds() external view returns (uint8[] memory) {
        uint256 count;
        for (uint256 i = 0; i < _tierIds.length; i++) {
            if (tiers[_tierIds[i]].active) count++;
        }

        uint8[] memory result = new uint8[](count);
        uint256 idx;
        for (uint256 i = 0; i < _tierIds.length; i++) {
            if (tiers[_tierIds[i]].active) {
                result[idx++] = _tierIds[i];
            }
        }
        return result;
    }

    // =========================================================================
    //                          ADMIN FUNCTIONS
    // =========================================================================

    /// @notice Add or modify a subscription tier.
    function setTier(uint8 id, uint256 price, uint256 duration, bool active) external onlyOwner {
        tiers[id] = TierConfig({price: price, duration: duration, active: active});

        if (!_tierExists[id]) {
            _tierIds.push(id);
            _tierExists[id] = true;
        }

        emit TierUpdated(id, price, duration, active);
    }

    /// @notice Update the operator revenue share.
    function setOperatorShare(uint256 newShareBps) external onlyOwner {
        require(newShareBps <= 10000, "Share > 100%");
        operatorShareBps = newShareBps;
    }

    /// @notice Update the treasury address.
    function setTreasury(address newTreasury) external onlyOwner {
        if (newTreasury == address(0)) revert ZeroAddress();
        treasury = newTreasury;
    }

    // =========================================================================
    //                          INTERNAL
    // =========================================================================

    /// @dev Split payment between operator and treasury.
    function _distributePayment(address node, uint256 amount) internal {
        uint256 operatorPayout = (amount * operatorShareBps) / 10000;
        uint256 treasuryPayout = amount - operatorPayout;

        operatorBalance[node] += operatorPayout;
        treasuryBalance += treasuryPayout;
    }
}
