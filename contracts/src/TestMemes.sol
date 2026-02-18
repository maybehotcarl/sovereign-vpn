// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/token/ERC1155/ERC1155.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

/// @title TestMemes
/// @notice Test ERC-1155 that mimics The Memes by 6529 for Sepolia development.
///         Real Memes contract: 0x33fd426905f149f8376e227d0c9d3340aad17af1
/// @dev Token ID 1 = "THIS card" (whitepaper card, grants free VPN)
///      Token ID 2+ = "other Memes cards" (grant paid VPN access)
///      This contract is for TESTING ONLY. On mainnet, AccessPolicy reads the real Memes contract.
contract TestMemes is ERC1155, Ownable {
    string public name = "Test Memes by 6529";
    string public symbol = "TMEMES";

    constructor() ERC1155("https://test.6529.io/api/token/{id}") Ownable(msg.sender) {}

    /// @notice Mint tokens to an address. Owner only.
    /// @param to Recipient address
    /// @param tokenId The token ID to mint
    /// @param amount Number of tokens to mint
    function mint(address to, uint256 tokenId, uint256 amount) external onlyOwner {
        _mint(to, tokenId, amount, "");
    }

    /// @notice Batch mint multiple token IDs to an address. Owner only.
    /// @param to Recipient address
    /// @param tokenIds Array of token IDs
    /// @param amounts Array of amounts (must match tokenIds length)
    function mintBatch(address to, uint256[] calldata tokenIds, uint256[] calldata amounts) external onlyOwner {
        _mintBatch(to, tokenIds, amounts, "");
    }
}
