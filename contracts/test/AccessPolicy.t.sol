// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "../src/AccessPolicy.sol";
import "../src/TestMemes.sol";

contract AccessPolicyTest is Test {
    AccessPolicy public policy;
    TestMemes public memes;

    address public owner = address(this);
    address public thisCardHolder = address(0x1);    // Holds THIS card -> free tier
    address public otherCardHolder = address(0x2);   // Holds another Memes card -> paid tier
    address public bothCardsHolder = address(0x3);   // Holds both -> free tier
    address public noCardHolder = address(0x4);      // Holds nothing -> denied
    address public multiCardHolder = address(0x5);   // Holds multiple non-THIS cards -> paid tier

    uint256 public constant THIS_CARD_ID = 1;
    uint256 public constant OTHER_CARD_ID = 2;
    uint256 public constant ANOTHER_CARD_ID = 3;

    function setUp() public {
        // Deploy test Memes contract
        memes = new TestMemes();

        // Deploy AccessPolicy pointing at test Memes
        policy = new AccessPolicy(address(memes));

        // Add known token IDs
        policy.addKnownTokenId(THIS_CARD_ID);
        policy.addKnownTokenId(OTHER_CARD_ID);
        policy.addKnownTokenId(ANOTHER_CARD_ID);

        // Set THIS card token ID
        policy.setThisCardTokenId(THIS_CARD_ID);

        // Distribute test tokens
        memes.mint(thisCardHolder, THIS_CARD_ID, 1);
        memes.mint(otherCardHolder, OTHER_CARD_ID, 1);
        memes.mint(bothCardsHolder, THIS_CARD_ID, 1);
        memes.mint(bothCardsHolder, OTHER_CARD_ID, 1);
        memes.mint(multiCardHolder, OTHER_CARD_ID, 2);
        memes.mint(multiCardHolder, ANOTHER_CARD_ID, 1);
        // noCardHolder gets nothing
    }

    // =========================================================================
    //                          hasAccess()
    // =========================================================================

    function test_hasAccess_thisCardHolder() public view {
        assertTrue(policy.hasAccess(thisCardHolder));
    }

    function test_hasAccess_otherCardHolder() public view {
        assertTrue(policy.hasAccess(otherCardHolder));
    }

    function test_hasAccess_bothCardsHolder() public view {
        assertTrue(policy.hasAccess(bothCardsHolder));
    }

    function test_hasAccess_multiCardHolder() public view {
        assertTrue(policy.hasAccess(multiCardHolder));
    }

    function test_hasAccess_noCardHolder() public view {
        assertFalse(policy.hasAccess(noCardHolder));
    }

    // =========================================================================
    //                          hasFreeTier()
    // =========================================================================

    function test_hasFreeTier_thisCardHolder() public view {
        assertTrue(policy.hasFreeTier(thisCardHolder));
    }

    function test_hasFreeTier_otherCardHolder() public view {
        assertFalse(policy.hasFreeTier(otherCardHolder));
    }

    function test_hasFreeTier_bothCardsHolder() public view {
        assertTrue(policy.hasFreeTier(bothCardsHolder));
    }

    function test_hasFreeTier_noCardHolder() public view {
        assertFalse(policy.hasFreeTier(noCardHolder));
    }

    // =========================================================================
    //                          checkAccess()
    // =========================================================================

    function test_checkAccess_thisCardHolder() public view {
        (bool access, bool free) = policy.checkAccess(thisCardHolder);
        assertTrue(access);
        assertTrue(free);
    }

    function test_checkAccess_otherCardHolder() public view {
        (bool access, bool free) = policy.checkAccess(otherCardHolder);
        assertTrue(access);
        assertFalse(free);
    }

    function test_checkAccess_bothCardsHolder() public view {
        (bool access, bool free) = policy.checkAccess(bothCardsHolder);
        assertTrue(access);
        assertTrue(free);  // THIS card takes priority -> free
    }

    function test_checkAccess_noCardHolder() public view {
        (bool access, bool free) = policy.checkAccess(noCardHolder);
        assertFalse(access);
        assertFalse(free);
    }

    function test_checkAccess_multiCardHolder() public view {
        (bool access, bool free) = policy.checkAccess(multiCardHolder);
        assertTrue(access);
        assertFalse(free);  // Has cards but not THIS card -> paid
    }

    // =========================================================================
    //                    ACCESS AFTER TRANSFER (revocation)
    // =========================================================================

    function test_accessRevokedAfterTransfer() public {
        // otherCardHolder has access
        assertTrue(policy.hasAccess(otherCardHolder));

        // Transfer the card away
        vm.prank(otherCardHolder);
        memes.safeTransferFrom(otherCardHolder, noCardHolder, OTHER_CARD_ID, 1, "");

        // otherCardHolder no longer has access
        assertFalse(policy.hasAccess(otherCardHolder));

        // noCardHolder now has access
        assertTrue(policy.hasAccess(noCardHolder));
    }

    function test_freeTierRevokedAfterTransfer() public {
        // thisCardHolder has free tier
        assertTrue(policy.hasFreeTier(thisCardHolder));

        // Transfer THIS card away
        vm.prank(thisCardHolder);
        memes.safeTransferFrom(thisCardHolder, noCardHolder, THIS_CARD_ID, 1, "");

        // thisCardHolder no longer has free tier
        assertFalse(policy.hasFreeTier(thisCardHolder));
        assertFalse(policy.hasAccess(thisCardHolder));

        // noCardHolder now has free tier
        assertTrue(policy.hasFreeTier(noCardHolder));
    }

    // =========================================================================
    //                      thisCardTokenId MANAGEMENT
    // =========================================================================

    function test_setThisCardTokenId() public {
        // Deploy fresh policy without thisCardTokenId set
        AccessPolicy freshPolicy = new AccessPolicy(address(memes));
        assertEq(freshPolicy.thisCardTokenId(), 0);

        freshPolicy.setThisCardTokenId(42);
        assertEq(freshPolicy.thisCardTokenId(), 42);
    }

    function test_lockThisCardTokenId() public {
        assertFalse(policy.thisCardTokenIdLocked());

        policy.lockThisCardTokenId();
        assertTrue(policy.thisCardTokenIdLocked());

        // Cannot change after lock
        vm.expectRevert(AccessPolicy.ThisCardTokenIdAlreadyLocked.selector);
        policy.setThisCardTokenId(99);
    }

    function test_lockRequiresTokenIdSet() public {
        AccessPolicy freshPolicy = new AccessPolicy(address(memes));

        vm.expectRevert(AccessPolicy.ThisCardTokenIdNotSet.selector);
        freshPolicy.lockThisCardTokenId();
    }

    function test_hasFreeTierRevertsIfNotSet() public {
        AccessPolicy freshPolicy = new AccessPolicy(address(memes));

        vm.expectRevert(AccessPolicy.ThisCardTokenIdNotSet.selector);
        freshPolicy.hasFreeTier(thisCardHolder);
    }

    // =========================================================================
    //                      KNOWN TOKEN ID MANAGEMENT
    // =========================================================================

    function test_addKnownTokenId() public {
        uint256 newId = 100;
        policy.addKnownTokenId(newId);
        assertTrue(policy.isKnownTokenId(newId));

        uint256[] memory ids = policy.getKnownTokenIds();
        bool found = false;
        for (uint256 i = 0; i < ids.length; i++) {
            if (ids[i] == newId) found = true;
        }
        assertTrue(found);
    }

    function test_addKnownTokenId_duplicate() public {
        vm.expectRevert(abi.encodeWithSelector(AccessPolicy.TokenIdAlreadyKnown.selector, THIS_CARD_ID));
        policy.addKnownTokenId(THIS_CARD_ID);
    }

    function test_removeKnownTokenId() public {
        policy.removeKnownTokenId(ANOTHER_CARD_ID);
        assertFalse(policy.isKnownTokenId(ANOTHER_CARD_ID));
    }

    function test_removeKnownTokenId_unknown() public {
        vm.expectRevert(abi.encodeWithSelector(AccessPolicy.TokenIdNotKnown.selector, 999));
        policy.removeKnownTokenId(999);
    }

    function test_accessDeniedAfterTokenIdRemoved() public {
        // multiCardHolder has OTHER_CARD_ID and ANOTHER_CARD_ID
        assertTrue(policy.hasAccess(multiCardHolder));

        // Remove both token IDs from known list
        policy.removeKnownTokenId(OTHER_CARD_ID);
        policy.removeKnownTokenId(ANOTHER_CARD_ID);

        // multiCardHolder still holds tokens, but they're no longer recognized
        assertFalse(policy.hasAccess(multiCardHolder));
    }

    // =========================================================================
    //                          OWNER-ONLY
    // =========================================================================

    function test_onlyOwnerCanSetThisCardTokenId() public {
        vm.prank(thisCardHolder);
        vm.expectRevert();
        policy.setThisCardTokenId(99);
    }

    function test_onlyOwnerCanAddKnownTokenId() public {
        vm.prank(thisCardHolder);
        vm.expectRevert();
        policy.addKnownTokenId(100);
    }

    function test_onlyOwnerCanAddCollection() public {
        vm.prank(thisCardHolder);
        vm.expectRevert();
        policy.addCollection(address(0x999));
    }

    // =========================================================================
    //                      ADDITIONAL COLLECTIONS
    // =========================================================================

    function test_addCollection() public {
        address fakeCollection = address(0xFACE);
        policy.addCollection(fakeCollection);
        assertTrue(policy.additionalCollections(fakeCollection));
    }

    function test_removeCollection() public {
        address fakeCollection = address(0xFACE);
        policy.addCollection(fakeCollection);
        policy.removeCollection(fakeCollection);
        assertFalse(policy.additionalCollections(fakeCollection));
    }

    function test_addCollection_zeroAddress() public {
        vm.expectRevert(AccessPolicy.ZeroAddress.selector);
        policy.addCollection(address(0));
    }

    // =========================================================================
    //                      CONSTRUCTOR
    // =========================================================================

    function test_constructorSetsMemesContract() public view {
        assertEq(policy.memesContract(), address(memes));
    }

    function test_constructorRevertsZeroAddress() public {
        vm.expectRevert(AccessPolicy.ZeroAddress.selector);
        new AccessPolicy(address(0));
    }
}
