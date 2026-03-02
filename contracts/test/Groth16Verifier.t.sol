// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "forge-std/Test.sol";
import "@zk/Groth16Verifier.sol";

/// @notice Integration tests for the snarkJS-generated Groth16Verifier.
/// Test vectors generated from the memes_membership_test circuit (depth=10).
contract Groth16VerifierTest is Test {
    Groth16Verifier public verifier;

    // --- Valid proof generated via snarkjs groth16.fullProve ---
    // Prover: 0x1234...7890, memesBalance=12, threshold=1 (free tier), scope=42

    uint256[2] internal validA = [
        0x01766a4a7ebd0782c9dd1f9e5c6a72538456a25984d4ec22e10ef348d77b370b,
        0x0349c53059763016dede49499c4ba9a3a2b5c1e8b2af90ebfdb4b2da3868471e
    ];

    uint256[2][2] internal validB = [
        [
            0x0cb352aec8629c2747117850f5c05df67cf8b351311ed71ca3ab069cd14dc2a2,
            0x18e6c78de02ef11bd984be1d27f496e3b3a47379b28cf5517bb740bdccbd1add
        ],
        [
            0x283882b82532fd07a4d1773216b940e1c964c1cac6b66c23ba1d130179eb9260,
            0x01096a7d6b940826a97836a9a1ef36ca07358cd145b79d2e1e1d8b550f7a85df
        ]
    ];

    uint256[2] internal validC = [
        0x2d8dc1c5fbcf353264c7719be067b2dbeda9af88f983d4d1fb5468da8d598120,
        0x03f68bb9524ab0f402e3d663819a00b71056388cd7fe344d3b9cc5ed51618083
    ];

    // Public signals: [merkleRoot, threshold, nullifierHash, scope]
    uint256[4] internal validPub = [
        0x1f258e60a3206347a7a8174686eb2af336dd31f36030cf830ed931b647fa4c6d,
        0x0000000000000000000000000000000000000000000000000000000000000001,
        0x1b94ab258923073e9cb05d3837d88df3bf8d13513fbc17f3d6b034ca52c0ec75,
        0x000000000000000000000000000000000000000000000000000000000000002a
    ];

    function setUp() public {
        verifier = new Groth16Verifier();
    }

    function test_validProofAccepted() public view {
        bool ok = verifier.verifyProof(validA, validB, validC, validPub);
        assertTrue(ok, "valid proof must be accepted");
    }

    function test_tamperedProofARejected() public view {
        uint256[2] memory badA = validA;
        badA[0] = badA[0] ^ 1; // flip one bit
        bool ok = verifier.verifyProof(badA, validB, validC, validPub);
        assertFalse(ok, "tampered pA must be rejected");
    }

    function test_tamperedPublicSignalRejected() public view {
        uint256[4] memory badPub = validPub;
        badPub[1] = 5; // change threshold from 1 to 5
        bool ok = verifier.verifyProof(validA, validB, validC, badPub);
        assertFalse(ok, "tampered public signal must be rejected");
    }

    function test_tamperedProofCRejected() public view {
        uint256[2] memory badC = validC;
        badC[1] = badC[1] ^ 1;
        bool ok = verifier.verifyProof(validA, validB, badC, validPub);
        assertFalse(ok, "tampered pC must be rejected");
    }

    function test_zeroProofRejected() public view {
        uint256[2] memory zeroA = [uint256(0), uint256(0)];
        uint256[2][2] memory zeroB = [[uint256(0), uint256(0)], [uint256(0), uint256(0)]];
        uint256[2] memory zeroC = [uint256(0), uint256(0)];
        bool ok = verifier.verifyProof(zeroA, zeroB, zeroC, validPub);
        assertFalse(ok, "zero proof must be rejected");
    }
}
