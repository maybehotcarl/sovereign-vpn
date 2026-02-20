pragma circom 2.1.0;

// Test version: 10 levels = ~1K addresses (faster proof generation)

include "node_modules/circomlib/circuits/poseidon.circom";
include "node_modules/circomlib/circuits/comparators.circom";
include "node_modules/circomlib/circuits/bitify.circom";

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

template MemesMembership(merkleDepth) {
    signal input merkleRoot;
    signal input threshold;
    signal input nullifierHash;
    signal input scope;

    signal input address;
    signal input memesBalance;
    signal input merklePathElements[merkleDepth];
    signal input merklePathIndices[merkleDepth];

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

    component gte = GreaterEqThan(32);
    gte.in[0] <== memesBalance;
    gte.in[1] <== threshold;
    gte.out === 1;

    component nullifierGenerator = Poseidon(3);
    nullifierGenerator.inputs[0] <== address;
    nullifierGenerator.inputs[1] <== scope;
    nullifierGenerator.inputs[2] <== merkleRoot;
    nullifierHash === nullifierGenerator.out;
}

// Test circuit: 10 levels = ~1K address capacity
component main {public [merkleRoot, threshold, nullifierHash, scope]} = MemesMembership(10);
