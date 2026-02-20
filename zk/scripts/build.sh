#!/bin/bash

# Memes Membership Circuit Build Script
# Adapted from tdh-marketplace-v2/zk/scripts/build.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ZK_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$ZK_DIR/build"
CIRCUITS_DIR="$ZK_DIR/circuits"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}Memes Membership Circuit Build${NC}"
echo "==============================="

# Find circom
if [ -f "$HOME/.cargo/bin/circom" ]; then
    CIRCOM="$HOME/.cargo/bin/circom"
else
    CIRCOM="circom"
fi

command -v "$CIRCOM" &>/dev/null || { echo -e "${RED}circom not found${NC}"; exit 1; }
command -v snarkjs &>/dev/null || { echo -e "${RED}snarkjs not found (npm i -g snarkjs)${NC}"; exit 1; }

echo "Using circom: $CIRCOM"
$CIRCOM --version

mkdir -p "$BUILD_DIR"
cd "$BUILD_DIR"

CIRCUIT_NAME="${1:-memes_membership_test}"
CIRCUIT_FILE="$CIRCUITS_DIR/${CIRCUIT_NAME}.circom"

if [ ! -f "$CIRCUIT_FILE" ]; then
    echo -e "${RED}Circuit not found: $CIRCUIT_FILE${NC}"
    ls -1 "$CIRCUITS_DIR"/*.circom 2>/dev/null
    exit 1
fi

echo -e "${YELLOW}Building: $CIRCUIT_NAME${NC}"

# Step 1: Compile
echo -e "\n${GREEN}Step 1: Compiling circuit...${NC}"
$CIRCOM "$CIRCUIT_FILE" \
    --r1cs --wasm --sym \
    --output "$BUILD_DIR" \
    -l "$ZK_DIR"

# Step 2: Info
echo -e "\n${GREEN}Step 2: Circuit info${NC}"
snarkjs r1cs info "${CIRCUIT_NAME}.r1cs"

# Step 3: Powers of Tau
PTAU_FILE="$BUILD_DIR/pot14_final.ptau"
if [ ! -f "$PTAU_FILE" ]; then
    echo -e "\n${GREEN}Step 3: Generating Powers of Tau...${NC}"
    echo -e "${YELLOW}Warning: Testing only. Production needs MPC ceremony.${NC}"

    snarkjs powersoftau new bn128 14 pot14_0000.ptau -v
    snarkjs powersoftau contribute pot14_0000.ptau pot14_0001.ptau \
        --name="First contribution" -v -e="random entropy"
    snarkjs powersoftau prepare phase2 pot14_0001.ptau pot14_final.ptau -v
    rm -f pot14_0000.ptau pot14_0001.ptau
else
    echo -e "\n${GREEN}Step 3: Reusing existing Powers of Tau${NC}"
fi

# Step 4: zkey
echo -e "\n${GREEN}Step 4: Generating proving key...${NC}"
snarkjs groth16 setup "${CIRCUIT_NAME}.r1cs" "$PTAU_FILE" "${CIRCUIT_NAME}_0000.zkey"
snarkjs zkey contribute "${CIRCUIT_NAME}_0000.zkey" "${CIRCUIT_NAME}_final.zkey" \
    --name="First contribution" -v -e="more random entropy"
rm -f "${CIRCUIT_NAME}_0000.zkey"

# Step 5: Verification key
echo -e "\n${GREEN}Step 5: Exporting verification key...${NC}"
snarkjs zkey export verificationkey "${CIRCUIT_NAME}_final.zkey" "verification_key.json"

# Step 6: Solidity verifier
echo -e "\n${GREEN}Step 6: Generating Solidity verifier...${NC}"
snarkjs zkey export solidityverifier "${CIRCUIT_NAME}_final.zkey" "Groth16Verifier.sol"
cp "Groth16Verifier.sol" "$ZK_DIR/contracts/"

echo -e "\n${GREEN}Build complete!${NC}"
echo "==============================="
echo "Output: $BUILD_DIR"
echo ""
echo "Next steps:"
echo "  node zk/scripts/test-proof.mjs    # test proof generation"
echo "  Deploy zk/contracts/Groth16Verifier.sol"
