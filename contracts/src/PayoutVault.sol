// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable2Step.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/// @title PayoutVault
/// @notice Central aggregation point for operator earnings before RAILGUN shielding.
///         SessionManager and SubscriptionManager credit operator shares here.
///         A payout executor (the TypeScript payout service) periodically withdraws
///         funds and routes them through RAILGUN for private payouts.
contract PayoutVault is Ownable2Step, ReentrancyGuard {
    using SafeERC20 for IERC20;

    // =========================================================================
    //                          STATE
    // =========================================================================

    /// @notice Pending ETH payout per operator (credited by authorized sources).
    mapping(address => uint256) public pendingPayouts;

    /// @notice Total pending ETH across all operators.
    uint256 public totalPending;

    /// @notice Lifetime processed ETH per operator.
    mapping(address => uint256) public processedPayouts;

    /// @notice Address authorized to call processPayout / processBatchPayout.
    address public payoutExecutor;

    /// @notice Addresses authorized to call creditOperator (SessionManager, SubscriptionManager).
    mapping(address => bool) public authorizedSources;

    /// @notice Emergency pause flag.
    bool public paused;

    // =========================================================================
    //                          EVENTS
    // =========================================================================

    event OperatorCredited(address indexed operator, address indexed source, uint256 amount);
    event PayoutProcessed(address indexed operator, uint256 amount);
    event BatchPayoutProcessed(uint256 operatorCount, uint256 totalAmount);
    event PayoutExecutorUpdated(address indexed oldExecutor, address indexed newExecutor);
    event SourceAuthorized(address indexed source);
    event SourceRevoked(address indexed source);
    event Paused(address indexed by);
    event Unpaused(address indexed by);
    event EmergencyWithdrawETH(address indexed to, uint256 amount);
    event EmergencyWithdrawToken(address indexed token, address indexed to, uint256 amount);

    // =========================================================================
    //                          ERRORS
    // =========================================================================

    error Unauthorized();
    error ContractPaused();
    error ZeroAddress();
    error ZeroAmount();
    error InsufficientPending(address operator, uint256 requested, uint256 available);
    error ArrayLengthMismatch();
    error TransferFailed();

    // =========================================================================
    //                          MODIFIERS
    // =========================================================================

    modifier onlyAuthorizedSource() {
        if (!authorizedSources[msg.sender]) revert Unauthorized();
        _;
    }

    modifier onlyPayoutExecutor() {
        if (msg.sender != payoutExecutor) revert Unauthorized();
        _;
    }

    modifier whenNotPaused() {
        if (paused) revert ContractPaused();
        _;
    }

    // =========================================================================
    //                          CONSTRUCTOR
    // =========================================================================

    /// @param _payoutExecutor Address of the payout service executor wallet
    constructor(address _payoutExecutor) Ownable(msg.sender) {
        if (_payoutExecutor == address(0)) revert ZeroAddress();
        payoutExecutor = _payoutExecutor;
    }

    // =========================================================================
    //                          CREDITING (called by SessionManager / SubscriptionManager)
    // =========================================================================

    /// @notice Credit an operator's pending payout. Called by authorized sources
    ///         (SessionManager, SubscriptionManager) when distributing operator share.
    /// @param operator The node operator receiving the credit
    function creditOperator(address operator) external payable onlyAuthorizedSource whenNotPaused {
        if (operator == address(0)) revert ZeroAddress();
        if (msg.value == 0) revert ZeroAmount();

        pendingPayouts[operator] += msg.value;
        totalPending += msg.value;

        emit OperatorCredited(operator, msg.sender, msg.value);
    }

    // =========================================================================
    //                          PAYOUT PROCESSING (called by payout service)
    // =========================================================================

    /// @notice Withdraw pending funds for a single operator. The payout executor
    ///         then shields and privately transfers via RAILGUN.
    /// @param operator The operator whose funds to process
    /// @param amount Amount to process (must be <= pendingPayouts[operator])
    function processPayout(address operator, uint256 amount) external onlyPayoutExecutor whenNotPaused nonReentrant {
        _processSinglePayout(operator, amount);
    }

    /// @notice Batch process payouts for multiple operators in a single transaction.
    /// @param operators Array of operator addresses
    /// @param amounts Array of amounts to process per operator
    function processBatchPayout(
        address[] calldata operators,
        uint256[] calldata amounts
    ) external onlyPayoutExecutor whenNotPaused nonReentrant {
        if (operators.length != amounts.length) revert ArrayLengthMismatch();

        uint256 totalAmount;
        for (uint256 i = 0; i < operators.length; i++) {
            _processSinglePayout(operators[i], amounts[i]);
            totalAmount += amounts[i];
        }

        emit BatchPayoutProcessed(operators.length, totalAmount);
    }

    // =========================================================================
    //                          ADMIN FUNCTIONS
    // =========================================================================

    /// @notice Set the payout executor address.
    function setPayoutExecutor(address newExecutor) external onlyOwner {
        if (newExecutor == address(0)) revert ZeroAddress();
        emit PayoutExecutorUpdated(payoutExecutor, newExecutor);
        payoutExecutor = newExecutor;
    }

    /// @notice Authorize a source contract to call creditOperator.
    function authorizeSource(address source) external onlyOwner {
        if (source == address(0)) revert ZeroAddress();
        authorizedSources[source] = true;
        emit SourceAuthorized(source);
    }

    /// @notice Revoke a source contract's authorization.
    function revokeSource(address source) external onlyOwner {
        authorizedSources[source] = false;
        emit SourceRevoked(source);
    }

    /// @notice Pause all credit and payout operations.
    function pause() external onlyOwner {
        paused = true;
        emit Paused(msg.sender);
    }

    /// @notice Unpause operations.
    function unpause() external onlyOwner {
        paused = false;
        emit Unpaused(msg.sender);
    }

    /// @notice Emergency withdraw all ETH to owner.
    ///         Resets totalPending to 0. Individual pendingPayouts mappings become
    ///         stale but cannot be processed (no ETH to send). Re-crediting after
    ///         re-funding requires manual reconciliation off-chain.
    /// @dev TODO(prod): Define an on-chain reconciliation/reset path for stale
    ///      pendingPayouts after emergency withdrawal (batch clear or migration).
    function emergencyWithdrawETH() external onlyOwner nonReentrant {
        uint256 balance = address(this).balance;
        if (balance == 0) revert ZeroAmount();

        totalPending = 0;

        (bool sent, ) = owner().call{value: balance}("");
        if (!sent) revert TransferFailed();

        emit EmergencyWithdrawETH(owner(), balance);
    }

    /// @notice Emergency withdraw ERC-20 tokens to owner.
    function emergencyWithdrawToken(address token) external onlyOwner nonReentrant {
        if (token == address(0)) revert ZeroAddress();

        uint256 balance = IERC20(token).balanceOf(address(this));
        if (balance == 0) revert ZeroAmount();

        IERC20(token).safeTransfer(owner(), balance);

        emit EmergencyWithdrawToken(token, owner(), balance);
    }

    // =========================================================================
    //                          VIEW FUNCTIONS
    // =========================================================================

    /// @notice Get the pending payout for an operator.
    function getPendingPayout(address operator) external view returns (uint256) {
        return pendingPayouts[operator];
    }

    /// @notice Get the lifetime processed amount for an operator.
    function getProcessedPayout(address operator) external view returns (uint256) {
        return processedPayouts[operator];
    }

    // =========================================================================
    //                          INTERNAL
    // =========================================================================

    /// @dev TODO(prod): Add explicit `amount <= address(this).balance` guard with
    ///      dedicated error so vault insolvency fails fast with clear diagnostics.
    ///      Keep `pendingPayouts` as entitlement source of truth (do not replace
    ///      operator accounting with global vault balance checks).
    function _processSinglePayout(address operator, uint256 amount) internal {
        if (operator == address(0)) revert ZeroAddress();
        if (amount == 0) revert ZeroAmount();
        if (pendingPayouts[operator] < amount) {
            revert InsufficientPending(operator, amount, pendingPayouts[operator]);
        }

        pendingPayouts[operator] -= amount;
        totalPending -= amount;
        processedPayouts[operator] += amount;

        (bool sent, ) = payoutExecutor.call{value: amount}("");
        if (!sent) revert TransferFailed();

        emit PayoutProcessed(operator, amount);
    }
}
