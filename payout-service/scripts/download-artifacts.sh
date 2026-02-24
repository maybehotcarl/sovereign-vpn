#!/bin/bash
# ---------------------------------------------------------------------------
# Download RAILGUN proving key artifacts
# ---------------------------------------------------------------------------
# The @railgun-community/wallet SDK auto-downloads artifacts on first use,
# but pre-creating the directory prevents permission issues in containers
# and avoids timeouts during the first payout cycle.
# ---------------------------------------------------------------------------

set -euo pipefail

ARTIFACTS_DIR="${RAILGUN_ARTIFACTS_PATH:-./data/railgun-artifacts}"

echo "[artifacts] Ensuring artifacts directory exists: $ARTIFACTS_DIR"
mkdir -p "$ARTIFACTS_DIR"

echo "[artifacts] Directory ready. The RAILGUN SDK will download"
echo "            proving key artifacts on first proof generation."
echo "            This may take several minutes (~100-500MB)."
echo ""
echo "[artifacts] Artifacts path: $(realpath "$ARTIFACTS_DIR")"
