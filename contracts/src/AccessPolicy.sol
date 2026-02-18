// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable2Step.sol";

/// @dev Minimal ERC-1155 interface -- we only need balanceOf
interface IERC1155Minimal {
    function balanceOf(address account, uint256 id) external view returns (uint256);
}

/// @title AccessPolicy
/// @notice Determines VPN access tier by reading Memes card ownership.
///         Does NOT mint any NFTs -- reads the existing Memes ERC-1155 contract.
/// @dev Access tiers:
///      - Free: holds THIS card (the whitepaper Meme card) -> unlimited VPN, no payment
///      - Paid: holds any other Memes card -> VPN access with payment
///      - Denied: holds no Memes card -> no access
contract AccessPolicy is Ownable2Step {
    /// @notice The Memes by 6529 ERC-1155 contract address.
    ///         Mainnet: 0x33fd426905f149f8376e227d0c9d3340aad17af1
    ///         Set via constructor (testnet uses TestMemes).
    address public immutable memesContract;

    /// @notice Token ID of THIS card (the whitepaper/project card).
    ///         Set once after the card is minted, then setter is renounced.
    uint256 public thisCardTokenId;

    /// @notice Whether thisCardTokenId has been permanently locked.
    bool public thisCardTokenIdLocked;

    /// @notice Token IDs to check for general access. If empty, ANY token ID with balance > 0 grants access.
    ///         For efficiency, we maintain a list of known token IDs to check.
    uint256[] public knownTokenIds;
    mapping(uint256 => bool) public isKnownTokenId;

    /// @notice Additional NFT collections that grant access (future expansion via governance).
    ///         collection address => whether it grants access
    mapping(address => bool) public additionalCollections;

    // -- Events --

    event ThisCardTokenIdSet(uint256 tokenId);
    event ThisCardTokenIdLocked();
    event KnownTokenIdAdded(uint256 tokenId);
    event KnownTokenIdRemoved(uint256 tokenId);
    event CollectionAdded(address indexed collection);
    event CollectionRemoved(address indexed collection);

    // -- Errors --

    error ThisCardTokenIdAlreadyLocked();
    error ThisCardTokenIdNotSet();
    error TokenIdAlreadyKnown(uint256 tokenId);
    error TokenIdNotKnown(uint256 tokenId);
    error ZeroAddress();

    constructor(address _memesContract) Ownable(msg.sender) {
        if (_memesContract == address(0)) revert ZeroAddress();
        memesContract = _memesContract;
    }

    // =========================================================================
    //                          ACCESS CHECKS
    // =========================================================================

    /// @notice Check if a user holds any Memes card (any known token ID with balance > 0).
    /// @param user The wallet address to check
    /// @return True if the user holds at least one Memes card
    function hasAccess(address user) external view returns (bool) {
        return _holdsAnyMemesCard(user);
    }

    /// @notice Check if a user holds THIS card (free tier).
    /// @param user The wallet address to check
    /// @return True if the user holds the whitepaper card
    function hasFreeTier(address user) external view returns (bool) {
        if (thisCardTokenId == 0) revert ThisCardTokenIdNotSet();
        return IERC1155Minimal(memesContract).balanceOf(user, thisCardTokenId) > 0;
    }

    /// @notice Full access check returning both access and tier.
    /// @param user The wallet address to check
    /// @return access True if user can access the VPN at all
    /// @return free True if user gets free tier (holds THIS card)
    function checkAccess(address user) external view returns (bool access, bool free) {
        // Check for THIS card first (it also counts as general access)
        if (thisCardTokenId != 0) {
            if (IERC1155Minimal(memesContract).balanceOf(user, thisCardTokenId) > 0) {
                return (true, true);
            }
        }

        // Check for any other Memes card
        if (_holdsAnyMemesCard(user)) {
            return (true, false);
        }

        // Check additional collections
        // Note: additional collections grant paid access only, not free
        // (free tier is exclusively for THIS card holders)

        return (false, false);
    }

    // =========================================================================
    //                          ADMIN FUNCTIONS
    // =========================================================================

    /// @notice Set the token ID of THIS card (the whitepaper card).
    ///         Can only be called once, then should be locked.
    /// @param tokenId The Memes token ID for this project's card
    function setThisCardTokenId(uint256 tokenId) external onlyOwner {
        if (thisCardTokenIdLocked) revert ThisCardTokenIdAlreadyLocked();
        thisCardTokenId = tokenId;
        // Also add it to known token IDs if not already there
        if (!isKnownTokenId[tokenId]) {
            knownTokenIds.push(tokenId);
            isKnownTokenId[tokenId] = true;
            emit KnownTokenIdAdded(tokenId);
        }
        emit ThisCardTokenIdSet(tokenId);
    }

    /// @notice Permanently lock thisCardTokenId. Cannot be changed after this.
    function lockThisCardTokenId() external onlyOwner {
        if (thisCardTokenId == 0) revert ThisCardTokenIdNotSet();
        thisCardTokenIdLocked = true;
        emit ThisCardTokenIdLocked();
    }

    /// @notice Add a known Memes token ID to check for access.
    /// @param tokenId The Memes token ID to add
    function addKnownTokenId(uint256 tokenId) external onlyOwner {
        if (isKnownTokenId[tokenId]) revert TokenIdAlreadyKnown(tokenId);
        knownTokenIds.push(tokenId);
        isKnownTokenId[tokenId] = true;
        emit KnownTokenIdAdded(tokenId);
    }

    /// @notice Remove a known token ID from the check list.
    /// @param tokenId The token ID to remove
    function removeKnownTokenId(uint256 tokenId) external onlyOwner {
        if (!isKnownTokenId[tokenId]) revert TokenIdNotKnown(tokenId);
        isKnownTokenId[tokenId] = false;
        // Remove from array (swap and pop)
        uint256 len = knownTokenIds.length;
        for (uint256 i = 0; i < len; i++) {
            if (knownTokenIds[i] == tokenId) {
                knownTokenIds[i] = knownTokenIds[len - 1];
                knownTokenIds.pop();
                break;
            }
        }
        emit KnownTokenIdRemoved(tokenId);
    }

    /// @notice Add an additional NFT collection that grants paid VPN access.
    /// @param collection The ERC-1155 or ERC-721 contract address
    function addCollection(address collection) external onlyOwner {
        if (collection == address(0)) revert ZeroAddress();
        additionalCollections[collection] = true;
        emit CollectionAdded(collection);
    }

    /// @notice Remove an additional collection.
    /// @param collection The contract address to remove
    function removeCollection(address collection) external onlyOwner {
        additionalCollections[collection] = false;
        emit CollectionRemoved(collection);
    }

    // =========================================================================
    //                          VIEW HELPERS
    // =========================================================================

    /// @notice Get all known token IDs.
    function getKnownTokenIds() external view returns (uint256[] memory) {
        return knownTokenIds;
    }

    /// @notice Get the number of known token IDs.
    function knownTokenIdCount() external view returns (uint256) {
        return knownTokenIds.length;
    }

    // =========================================================================
    //                          INTERNAL
    // =========================================================================

    /// @dev Check if user holds any Memes card from the known token ID list.
    function _holdsAnyMemesCard(address user) internal view returns (bool) {
        IERC1155Minimal memes = IERC1155Minimal(memesContract);
        uint256 len = knownTokenIds.length;
        for (uint256 i = 0; i < len; i++) {
            if (memes.balanceOf(user, knownTokenIds[i]) > 0) {
                return true;
            }
        }
        return false;
    }
}
