#!/bin/sh
# setup-devnet.sh - One-shot init container that generates the full devnet
# genesis with funded accounts, validator keys, and gentxs.
#
# Outputs to /config/ (shared volume):
#   genesis.json              - Final genesis file
#   node-{role}-{index}/      - Pre-built node homes for each node
#   peers/{role}-{index}.env  - Resolved peer configuration per node
#
set -eu

if [ -f /config/genesis.json ]; then
  echo "=== Genesis already exists — skipping setup ==="
  exit 0
fi

CHAIN_ID="${CHAIN_ID:-arkdevnet_9000-1}"
OUTPUT_DIR="${OUTPUT_DIR:-/config}"
VALIDATOR_COUNT="${VALIDATOR_COUNT:-2}"
SENTRY_COUNT="${SENTRY_COUNT:-2}"

echo "=== ArkConstellation Devnet Genesis Setup ==="
echo "  Chain ID:     ${CHAIN_ID}"
echo "  Validators:   ${VALIDATOR_COUNT}"
echo "  Sentries:     ${SENTRY_COUNT}"
echo "  Output:       ${OUTPUT_DIR}"
echo "============================================="

mkdir -p "${OUTPUT_DIR}"

# --- Phase 1: Initialize ALL node homes to get node IDs ---
echo ""
echo ">>> Phase 1: Initializing node homes and generating keys..."

VALIDATOR_HOME="${OUTPUT_DIR}/node-validator-0"

# Initialize first validator (genesis built from this home)
echo "  Initializing validator-0..."
arkd init "validator-0" \
    --chain-id "$CHAIN_ID" \
    --home "$VALIDATOR_HOME" \
    --default-denom esp \
    > /dev/null 2>&1

# Create all validator keys in the main keyring
echo "  Creating validator keys..."
for i in $(seq 0 $((VALIDATOR_COUNT - 1))); do
    echo "y" | arkd keys add "validator-${i}" \
        --home "$VALIDATOR_HOME" \
        --keyring-backend test \
        > /dev/null 2>&1
    ADDR=$(arkd keys show "validator-${i}" --home "$VALIDATOR_HOME" --keyring-backend test --address)
    echo "    validator-${i}: ${ADDR}"
done

if [ -n "${FAUCET_MNEMONIC:-}" ]; then
    echo "  Recovering faucet key from FAUCET_MNEMONIC..."
    echo "$FAUCET_MNEMONIC" | arkd keys add faucet \
        --home "$VALIDATOR_HOME" \
        --keyring-backend test \
        --recover \
        > /dev/null 2>&1
else
    echo "y" | arkd keys add faucet \
        --home "$VALIDATOR_HOME" \
        --keyring-backend test \
        > /dev/null 2>&1
fi
FAUCET_ADDR=$(arkd keys show faucet --home "$VALIDATOR_HOME" --keyring-backend test --address)
echo "    faucet: ${FAUCET_ADDR}"

# Initialize ALL other node homes (validators + sentries) so we have node IDs
for i in $(seq 1 $((VALIDATOR_COUNT - 1))); do
    echo "  Initializing validator-${i}..."
    arkd init "validator-${i}" \
        --chain-id "$CHAIN_ID" \
        --home "${OUTPUT_DIR}/node-validator-${i}" \
        --default-denom esp \
        > /dev/null 2>&1
done

for i in $(seq 0 $((SENTRY_COUNT - 1))); do
    echo "  Initializing sentry-${i}..."
    arkd init "sentry-${i}" \
        --chain-id "$CHAIN_ID" \
        --home "${OUTPUT_DIR}/node-sentry-${i}" \
        --default-denom esp \
        > /dev/null 2>&1
done

# --- Phase 2: Extract node IDs ---
echo ""
echo ">>> Phase 2: Extracting node IDs..."

# Helper: extract node_id from node_key.json
# CometBFT node_key.json has no .id field; derive it from the public key.
# arkd cometbft show-node-id does: hex(SHA256(pub_key_bytes))[:40]
get_node_id() {
    local home="$1"
    arkd cometbft show-node-id --home "$home"
}

VAL_IDS=""
for i in $(seq 0 $((VALIDATOR_COUNT - 1))); do
    ID=$(get_node_id "${OUTPUT_DIR}/node-validator-${i}")
    VAL_IDS="${VAL_IDS} ${ID}"
    echo "  validator-${i}: ${ID}"
done

SEN_IDS=""
for i in $(seq 0 $((SENTRY_COUNT - 1))); do
    ID=$(get_node_id "${OUTPUT_DIR}/node-sentry-${i}")
    SEN_IDS="${SEN_IDS} ${ID}"
    echo "  sentry-${i}: ${ID}"
done

# Convert to arrays for indexing
set -- $VAL_IDS
VAL_ID_0="${1:-}"; VAL_ID_1="${2:-}"
set -- $SEN_IDS
SEN_ID_0="${1:-}"; SEN_ID_1="${2:-}"

echo "  VAL_ID_0=${VAL_ID_0}"
echo "  VAL_ID_1=${VAL_ID_1}"
echo "  SEN_ID_0=${SEN_ID_0}"
echo "  SEN_ID_1=${SEN_ID_1}"

# --- Phase 3: Write resolved peer configs ---
echo ""
echo ">>> Phase 3: Writing resolved peer configs..."
mkdir -p "${OUTPUT_DIR}/peers"

# sentry-0 peers: validator-0 (persistent) + sentry-1 (relay)
cat > "${OUTPUT_DIR}/peers/sentry-0.env" <<EOF
PERSISTENT_PEERS=${VAL_ID_0}@validator-0:26656,${SEN_ID_1}@sentry-1:26656
PRIVATE_PEER_IDS=${VAL_ID_0}
UNCONDITIONAL_PEER_IDS=${VAL_ID_0}
SENTRY_NODE_ID=${SEN_ID_0}
EOF

# validator-0 peers: sentry-0 only
cat > "${OUTPUT_DIR}/peers/validator-0.env" <<EOF
PERSISTENT_PEERS=${SEN_ID_0}@sentry-0:26656
VALIDATOR_NODE_ID=${VAL_ID_0}
EOF

# sentry-1 peers: validator-1 (persistent) + sentry-0 (relay)
cat > "${OUTPUT_DIR}/peers/sentry-1.env" <<EOF
PERSISTENT_PEERS=${VAL_ID_1}@validator-1:26656,${SEN_ID_0}@sentry-0:26656
PRIVATE_PEER_IDS=${VAL_ID_1}
UNCONDITIONAL_PEER_IDS=${VAL_ID_1}
SENTRY_NODE_ID=${SEN_ID_1}
EOF

# validator-1 peers: sentry-1 only
cat > "${OUTPUT_DIR}/peers/validator-1.env" <<EOF
PERSISTENT_PEERS=${SEN_ID_1}@sentry-1:26656
VALIDATOR_NODE_ID=${VAL_ID_1}
EOF

echo "  Peer configs written."

# --- Phase 4: Apply genesis patch ---
echo ""
echo ">>> Phase 4: Applying genesis patch..."

GENESIS="${VALIDATOR_HOME}/config/genesis.json"

if [ -f /genesis-template.json ]; then
    jq -s '.[0] * .[1]' "$GENESIS" <(jq 'del(._comment)' /genesis-template.json) > "${GENESIS}.tmp" \
        && mv "${GENESIS}.tmp" "$GENESIS"
    echo "  Genesis patch applied."
fi

# --- Phase 5: Add genesis accounts ---
echo ""
echo ">>> Phase 5: Adding genesis accounts..."

echo "  Adding faucet account: ${FAUCET_ADDR}"
arkd genesis add-genesis-account "${FAUCET_ADDR}" 100000000000000000000000000esp \
    --home "$VALIDATOR_HOME" --keyring-backend test

for i in $(seq 0 $((VALIDATOR_COUNT - 1))); do
    ADDR=$(arkd keys show "validator-${i}" --home "$VALIDATOR_HOME" --keyring-backend test --address)
    echo "  Adding validator-${i} account: ${ADDR}"
    arkd genesis add-genesis-account "${ADDR}" 100000000000000000000000000esp \
        --home "$VALIDATOR_HOME" --keyring-backend test --append
done

# --- Phase 6: Create gentxs ---
echo ""
echo ">>> Phase 6: Creating validator gentxs..."

FINAL_GENTX_DIR="${VALIDATOR_HOME}/config/gentx"
mkdir -p "$FINAL_GENTX_DIR"

for i in $(seq 0 $((VALIDATOR_COUNT - 1))); do
    echo "  Creating gentx for validator-${i}..."
    TMP_HOME="${OUTPUT_DIR}/tmp-val-${i}"
    rm -rf "$TMP_HOME"

    # Each gentx needs its own home with a unique node_key.json
    arkd init "tmp-${i}" --chain-id "$CHAIN_ID" --home "$TMP_HOME" --default-denom esp > /dev/null 2>&1
    cp -r "${VALIDATOR_HOME}/keyring-test" "${TMP_HOME}/keyring-test"
    chmod -R a+rX "${TMP_HOME}/keyring-test"
    cp "$GENESIS" "${TMP_HOME}/config/genesis.json"

    GENTX_DOC="${FINAL_GENTX_DIR}/gentx-val${i}.json"
    if ! arkd genesis gentx "validator-${i}" 90000000000000000000000000esp \
        --chain-id "$CHAIN_ID" \
        --home "$TMP_HOME" \
        --keyring-backend test \
        --moniker "validator-${i}" \
        --output-document "$GENTX_DOC"; then
        echo "  ERROR: gentx for validator-${i} failed"
        exit 1
    fi

    # Copy the consensus key back so the node's priv_validator_key matches the gentx
    # (Don't overwrite node_key.json — peer configs depend on original node IDs)
    cp "${TMP_HOME}/config/priv_validator_key.json" "${OUTPUT_DIR}/node-validator-${i}/config/priv_validator_key.json"

    rm -rf "$TMP_HOME"
    echo "  gentx for validator-${i} created"
done

# --- Phase 7: Collect gentxs and validate ---
echo ""
echo ">>> Phase 7: Collecting gentxs..."
if ! arkd genesis collect-gentxs --home "$VALIDATOR_HOME"; then
    echo "  ERROR: collect-gentxs failed"
    exit 1
fi

echo "  Validating genesis..."
arkd genesis validate-genesis --home "$VALIDATOR_HOME" || true

if [ -f "$GENESIS" ]; then
    HEIGHT=$(jq -r '.initial_height // "0"' "$GENESIS" 2>/dev/null || echo "0")
    echo "  Genesis ready. Height: ${HEIGHT}"
else
    echo "  ERROR: Genesis not found"
    exit 1
fi

# --- Phase 8: Distribute genesis to all nodes ---
echo ""
echo ">>> Phase 8: Distributing genesis..."

for i in $(seq 0 $((SENTRY_COUNT - 1))); do
    cp "$GENESIS" "${OUTPUT_DIR}/node-sentry-${i}/config/genesis.json"
done

for i in $(seq 1 $((VALIDATOR_COUNT - 1))); do
    cp "$GENESIS" "${OUTPUT_DIR}/node-validator-${i}/config/genesis.json"
    cp -r "${VALIDATOR_HOME}/keyring-test" "${OUTPUT_DIR}/node-validator-${i}/keyring-test"
    mkdir -p "${OUTPUT_DIR}/node-validator-${i}/data"
    cp "${VALIDATOR_HOME}/data/priv_validator_state.json" "${OUTPUT_DIR}/node-validator-${i}/data/" 2>/dev/null || true
done

cp "$GENESIS" "${OUTPUT_DIR}/genesis.json"

# Ensure priv_validator_state.json exists for validator-0
mkdir -p "${VALIDATOR_HOME}/data"
[ -f "${VALIDATOR_HOME}/data/priv_validator_state.json" ] || echo '{}' > "${VALIDATOR_HOME}/data/priv_validator_state.json"

# Fix permissions for nonroot user (uid 1025)
echo "  Fixing file permissions..."
chmod -R a+rwX "${OUTPUT_DIR}/node-"* 2>/dev/null || true
chmod -R a+rwX "${OUTPUT_DIR}/peers" 2>/dev/null || true
chmod -R a+rwX "${OUTPUT_DIR}/genesis.json" 2>/dev/null || true

echo ""
echo "=== Genesis Setup Complete ==="
echo "  Genesis: ${OUTPUT_DIR}/genesis.json"
echo "  Peers:   ${OUTPUT_DIR}/peers/*.env"
echo "=============================="
