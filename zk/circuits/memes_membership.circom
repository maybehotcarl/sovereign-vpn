pragma circom 2.1.0;

// Memes NFT Membership Proof — Sovereign VPN
//
// Proves "I hold >= N Memes NFTs" without revealing which wallet.
//
// Design decisions vs. tdh_range_proof.circom:
//
//   1. THRESHOLD instead of RANGE — VPN only needs "holds at least 1"
//      (free tier) or "holds at least 5" (paid tier). A single
//      GreaterEqThan replaces the RangeProof template.
//
//   2. DETERMINISTIC NULLIFIER — the marketplace uses a random nonce
//      so each offer gets a unique nullifier. VPN wants the opposite:
//      same wallet → same nullifier → gateway rejects duplicate active
//      sessions. We drop the nonce and compute:
//
//        nullifier = Poseidon(address, scope, merkleRoot)
//
//      - `address` is PRIVATE. Can't reverse Poseidon to learn it.
//      - `scope` is PUBLIC. A fixed value (e.g. contract address) that
//        domain-separates VPN nullifiers from marketplace nullifiers.
//        Same Merkle tree, different scope → different nullifiers.
//      - `merkleRoot` is PUBLIC. Binds nullifier to tree state. When
//        the daily cron rebuilds the tree, the root changes, old
//        nullifiers expire, and the user gets a fresh unlinkable one.
//        An observer can't correlate Monday's session to Tuesday's.
//
//   3. NO COMMITMENT HASH — marketplace needs it for settlement
//      verification. VPN doesn't settle payments in Phase 0.
//
//   4. SAME MERKLE TREE — leaves are Poseidon(address, balance).
//      The cron job that builds the TDH tree can build a Memes
//      holder tree in the same pass (same 6529 API response).
//
// Public signals (4 total):
//   [0] merkleRoot     — current tree root
//   [1] threshold      — minimum balance required
//   [2] nullifierHash  — deterministic session identifier
//   [3] scope          — domain separation constant
//
// Private inputs:
//   address            — Ethereum address as field element
//   memesBalance       — number of Memes NFTs held
//   merklePathElements — 20 sibling hashes
//   merklePathIndices  — 20 left/right indicators

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";
include "node_modules/circomlib/circuits/bitify.circom";

// ---------------------------------------------------------------
// Merkle tree membership (identical to tdh_range_proof.circom)
// ---------------------------------------------------------------
template MerkleTreeChecker(levels) {
    signal input leaf;
    signal input merkleRoot;
    signal input pathElements[levels];
    signal input pathIndices[levels];

    component hasher[levels];
    signal levelHashes[levels + 1];
    levelHashes[0] <== leaf;

    for (var i = 0; i < levels; i++) {
        pathIndices[i] * (1 - pathIndices[i]) === 0;

        hasher[i] = Poseidon(2);
        hasher[i].inputs[0] <== levelHashes[i] - pathIndices[i] * (levelHashes[i] - pathElements[i]);
        hasher[i].inputs[1] <== pathElements[i] - pathIndices[i] * (pathElements[i] - levelHashes[i]);
        levelHashes[i + 1] <== hasher[i].out;
    }

    merkleRoot === levelHashes[levels];
}

// ---------------------------------------------------------------
// Main circuit
// ---------------------------------------------------------------
template MemesMembership(merkleDepth) {
    // === PUBLIC INPUTS ===
    signal input merkleRoot;
    signal input threshold;       // minimum Memes balance (1=free, 5=paid)
    signal input nullifierHash;
    signal input scope;           // domain separator (e.g. VPN contract address)

    // === PRIVATE INPUTS ===
    signal input address;
    signal input memesBalance;
    signal input merklePathElements[merkleDepth];
    signal input merklePathIndices[merkleDepth];

    // -------------------------------------------------------
    // CONSTRAINT 1: Merkle membership
    // Leaf = Poseidon(address, memesBalance) — same structure
    // as TDH tree but with Memes balance instead of TDH.
    // -------------------------------------------------------
    component leafHasher = Poseidon(2);
    leafHasher.inputs[0] <== address;
    leafHasher.inputs[1] <== memesBalance;
    signal leaf <== leafHasher.out;

    component merkleChecker = MerkleTreeChecker(merkleDepth);
    merkleChecker.leaf <== leaf;
    merkleChecker.merkleRoot <== merkleRoot;
    for (var i = 0; i < merkleDepth; i++) {
        merkleChecker.pathElements[i] <== merklePathElements[i];
        merkleChecker.pathIndices[i] <== merklePathIndices[i];
    }

    // -------------------------------------------------------
    // CONSTRAINT 2: Balance >= threshold
    // 32 bits is plenty (max 2^32 = 4 billion NFTs).
    // -------------------------------------------------------
    component gte = GreaterEqThan(32);
    gte.in[0] <== memesBalance;
    gte.in[1] <== threshold;
    gte.out === 1;

    // -------------------------------------------------------
    // CONSTRAINT 3: Deterministic nullifier
    // Poseidon(address, scope, merkleRoot)
    //
    // Same wallet + same scope + same tree = same nullifier.
    // Gateway tracks active nullifiers to prevent concurrent
    // sessions. Tree rebuild rotates the nullifier.
    // -------------------------------------------------------
    component nullifierGenerator = Poseidon(3);
    nullifierGenerator.inputs[0] <== address;
    nullifierGenerator.inputs[1] <== scope;
    nullifierGenerator.inputs[2] <== merkleRoot;
    nullifierHash === nullifierGenerator.out;
}

// Production: 20 levels = ~1M address capacity
component main {public [merkleRoot, threshold, nullifierHash, scope]} = MemesMembership(20);
