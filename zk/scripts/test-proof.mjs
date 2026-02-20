/**
 * Test ZK Proof Generation — Memes Membership
 *
 * Adapted from tdh-marketplace-v2/zk/scripts/test-proof.mjs.
 * Builds a small Merkle tree, generates a membership proof, verifies it.
 */

import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import * as snarkjs from 'snarkjs';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// ---- Poseidon setup (identical to marketplace) ----

const circomlibjs = await import('circomlibjs');
const poseidonBuilder = await circomlibjs.buildPoseidon();
const F = poseidonBuilder.F;

function poseidon2(a, b) {
  return F.toObject(poseidonBuilder([a, b]));
}

function poseidon3(a, b, c) {
  return F.toObject(poseidonBuilder([a, b, c]));
}

function addressToField(address) {
  return BigInt('0x' + address.toLowerCase().replace('0x', ''));
}

// ---- Merkle tree (identical to marketplace) ----

function buildMerkleTree(leaves, depth) {
  const layers = [leaves];
  let currentLayer = [...leaves];

  const targetSize = Math.pow(2, depth);
  while (currentLayer.length < targetSize) {
    currentLayer.push(BigInt(0));
  }
  layers[0] = currentLayer;

  while (currentLayer.length > 1) {
    const nextLayer = [];
    for (let i = 0; i < currentLayer.length; i += 2) {
      const left = currentLayer[i];
      const right = currentLayer[i + 1] || BigInt(0);
      nextLayer.push(poseidon2(left, right));
    }
    layers.push(nextLayer);
    currentLayer = nextLayer;
  }

  return layers;
}

function getMerkleProof(layers, index, depth) {
  const pathElements = [];
  const pathIndices = [];
  let currentIndex = index;

  for (let i = 0; i < depth; i++) {
    const layer = layers[i];
    const isLeft = currentIndex % 2 === 0;
    const siblingIndex = isLeft ? currentIndex + 1 : currentIndex - 1;

    pathElements.push(layer[siblingIndex] || BigInt(0));
    pathIndices.push(isLeft ? 0 : 1);

    currentIndex = Math.floor(currentIndex / 2);
  }

  return { pathElements, pathIndices };
}

// ---- Main test ----

async function main() {
  console.log('Memes Membership Proof — Full Test\n');
  console.log('====================================\n');

  const depth = 10; // matches test circuit

  // Test data: Memes NFT holders
  const holders = [
    { address: '0x1234567890123456789012345678901234567890', memesBalance: 12 },
    { address: '0xabcdefabcdefabcdefabcdefabcdefabcdefabcd', memesBalance: 3 },
    { address: '0x9999999999999999999999999999999999999999', memesBalance: 1 },
  ];

  console.log('1. Test holders:');
  holders.forEach((h, i) => {
    console.log(`   Holder ${i + 1}: ${h.address.slice(0, 10)}... holds ${h.memesBalance} Memes`);
  });

  // Build tree: leaves = Poseidon(address, memesBalance)
  console.log('\n2. Building Merkle tree...');
  const leaves = holders.map(h =>
    poseidon2(addressToField(h.address), BigInt(h.memesBalance))
  );

  const layers = buildMerkleTree(leaves, depth);
  const merkleRoot = layers[layers.length - 1][0];
  console.log(`   Root: ${merkleRoot.toString().slice(0, 20)}...`);

  // ---- Test case 1: Free tier (threshold=1, holder with 12 Memes) ----

  const testHolder = holders[0];
  const testIndex = 0;
  const threshold = 1; // free tier
  const scope = BigInt(42); // arbitrary domain separator

  console.log('\n3. Preparing proof inputs...');
  console.log(`   Prover: ${testHolder.address.slice(0, 10)}...`);
  console.log(`   Balance: ${testHolder.memesBalance}`);
  console.log(`   Threshold: ${threshold} (free tier)`);

  const address = addressToField(testHolder.address);
  const memesBalance = BigInt(testHolder.memesBalance);
  const { pathElements, pathIndices } = getMerkleProof(layers, testIndex, depth);

  // Deterministic nullifier: Poseidon(address, scope, merkleRoot)
  const nullifierHash = poseidon3(address, scope, merkleRoot);
  console.log(`   Nullifier: ${nullifierHash.toString().slice(0, 20)}...`);

  const circuitInputs = {
    merkleRoot: merkleRoot.toString(),
    threshold: threshold.toString(),
    nullifierHash: nullifierHash.toString(),
    scope: scope.toString(),
    address: address.toString(),
    memesBalance: memesBalance.toString(),
    merklePathElements: pathElements.map(e => e.toString()),
    merklePathIndices: pathIndices.map(i => i.toString()),
  };

  // Paths
  const buildDir = join(__dirname, '..', 'build');
  const wasmPath = join(buildDir, 'memes_membership_test_js', 'memes_membership_test.wasm');
  const zkeyPath = join(buildDir, 'memes_membership_test_final.zkey');
  const vkeyPath = join(buildDir, 'verification_key.json');

  console.log('\n4. Generating ZK proof...');
  const startTime = Date.now();

  const { proof, publicSignals } = await snarkjs.groth16.fullProve(
    circuitInputs,
    wasmPath,
    zkeyPath
  );

  console.log(`   Proof generated in ${Date.now() - startTime}ms`);

  console.log('\n5. Public signals:');
  console.log(`   [0] merkleRoot:    ${publicSignals[0].slice(0, 20)}...`);
  console.log(`   [1] threshold:     ${publicSignals[1]}`);
  console.log(`   [2] nullifierHash: ${publicSignals[2].slice(0, 20)}...`);
  console.log(`   [3] scope:         ${publicSignals[3]}`);

  console.log('\n6. Verifying proof...');
  const fs = await import('fs/promises');
  const vKey = JSON.parse(await fs.readFile(vkeyPath, 'utf-8'));
  const isValid = await snarkjs.groth16.verify(vKey, publicSignals, proof);

  if (isValid) {
    console.log('   VALID — holder proved membership without revealing wallet\n');
  } else {
    console.log('   INVALID\n');
    process.exit(1);
  }

  // ---- Test case 2: Same holder, same tree → same nullifier ----

  console.log('7. Deterministic nullifier test...');
  const nullifier2 = poseidon3(address, scope, merkleRoot);
  const match = nullifierHash === nullifier2;
  console.log(`   Same inputs → same nullifier: ${match}`);
  if (!match) {
    console.log('   ERROR: nullifier is not deterministic!');
    process.exit(1);
  }

  // ---- Test case 3: Different scope → different nullifier ----

  const scope2 = BigInt(99);
  const nullifier3 = poseidon3(address, scope2, merkleRoot);
  const different = nullifierHash !== nullifier3;
  console.log(`   Different scope → different nullifier: ${different}`);
  if (!different) {
    console.log('   ERROR: scope is not separating nullifiers!');
    process.exit(1);
  }

  console.log('\n====================================');
  console.log('All tests passed!');
}

main().catch(err => {
  console.error('Test failed:', err);
  process.exit(1);
});
