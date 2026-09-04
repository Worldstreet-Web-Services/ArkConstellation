#!/bin/sh
# entrypoint.sh - Configures and starts an ArkConstellation node.
#
# Environment variables:
#   NODE_ROLE          "validator" or "sentry" (required)
#   CHAIN_ID           Cosmos chain ID (default: "arkdevnet_9000-1")
#   VALIDATOR_INDEX    Node index for validators (0, 1, 2, ...)
#   SENTRY_INDEX       Node index for sentries (0, 1, 2, ...)
#   MIN_GAS_PRICES     Minimum gas prices (default: "1000000000esp" = 1 gwei,
#                      matching the chain-wide feemarket floor)
#   PROMETHEUS         Enable Prometheus metrics (default: "true")
#   ENABLE_API         Enable Cosmos LCD/API (default: "false")
#   ENABLE_EVM_RPC     Enable EVM JSON-RPC (default: "false")
#
set -eu

NODE_ROLE="${NODE_ROLE:?NODE_ROLE is required: 'validator' or 'sentry'}"
CHAIN_ID="${CHAIN_ID:-arkdevnet_9000-1}"
# Default must match the chain-wide floor cosmos/evm enforces via feemarket
# min_gas_price (1 gwei). A node serves this over
# /cosmos/base/node/v1beta1/config, which is what clients auto-fill fees from,
# so a lower default advertises a fee that consensus rejects.
MIN_GAS_PRICES="${MIN_GAS_PRICES:-1000000000esp}"
PROMETHEUS="${PROMETHEUS:-true}"
ENABLE_API="${ENABLE_API:-false}"
ENABLE_EVM_RPC="${ENABLE_EVM_RPC:-false}"

HOME_DIR="/home/nonroot/.ark"

# Derive node index and moniker
if [ "$NODE_ROLE" = "validator" ]; then
    INDEX="${VALIDATOR_INDEX:?VALIDATOR_INDEX required for validator role}"
    MONIKER="${MONIKER:-validator-${INDEX}}"
    NODE_HOME="${HOME_DIR}/node-validator-${INDEX}"
    PEX="${PEX:-false}"
elif [ "$NODE_ROLE" = "sentry" ]; then
    INDEX="${SENTRY_INDEX:?SENTRY_INDEX required for sentry role}"
    MONIKER="${MONIKER:-sentry-${INDEX}}"
    NODE_HOME="${HOME_DIR}/node-sentry-${INDEX}"
    PEX="${PEX:-true}"
else
    echo "ERROR: NODE_ROLE must be 'validator' or 'sentry', got: '$NODE_ROLE'" >&2
    exit 1
fi

# Load resolved peers from setup container's output
PEER_ENV="/config/peers/${NODE_ROLE}-${INDEX}.env"
if [ -f "$PEER_ENV" ]; then
    echo ">>> Loading peer config from ${PEER_ENV}..."
    # shellcheck disable=SC1090
    . "$PEER_ENV"
    echo "  PERSISTENT_PEERS=${PERSISTENT_PEERS:-}"
fi

# Copy pre-built node home from setup container
PREBUILT_HOME="/config/node-${NODE_ROLE}-${INDEX}"

if [ -d "${PREBUILT_HOME}/config" ] && [ -f "${PREBUILT_HOME}/config/config.toml" ]; then
    echo ">>> Using pre-built node home from ${PREBUILT_HOME}..."
    mkdir -p "${NODE_HOME}/config" "${NODE_HOME}/data"
    cp -r "${PREBUILT_HOME}/config/"* "${NODE_HOME}/config/"
    [ -d "${PREBUILT_HOME}/keyring-test" ] && cp -r "${PREBUILT_HOME}/keyring-test" "${NODE_HOME}/keyring-test"
    [ -d "${PREBUILT_HOME}/data" ] && cp -r "${PREBUILT_HOME}/data/"* "${NODE_HOME}/data/"
    echo ">>> Pre-built node home copied."
else
    echo ">>> No pre-built home found, initializing fresh..."
    arkd init "$MONIKER" \
        --chain-id "$CHAIN_ID" \
        --home "$NODE_HOME" \
        --default-denom esp \
        > /dev/null 2>&1

    if [ -f /genesis-template.json ]; then
        echo ">>> Applying genesis patch..."
        GENESIS="${NODE_HOME}/config/genesis.json"
        jq -s '.[0] * .[1]' "$GENESIS" <(jq 'del(._comment)' /genesis-template.json) > "${GENESIS}.tmp" \
            && mv "${GENESIS}.tmp" "$GENESIS"
    fi
fi

# Update config.toml (always re-apply)
CONFIG="${NODE_HOME}/config/config.toml"

sed -i "s/^moniker = .*/moniker = \"${MONIKER}\"/" "$CONFIG"

# Bind RPC to 0.0.0.0 so Docker port mappings work (key is "laddr" under [rpc])
sed -i 's|^laddr = "tcp://127.0.0.1:26657"|laddr = "tcp://0.0.0.0:26657"|' "$CONFIG"

# Allow private IPs (Docker bridge network uses 172.x.x.x)
sed -i '/^\[p2p\]/a allow_private_ip = true' "$CONFIG"

if [ -n "${PERSISTENT_PEERS:-}" ]; then
    sed -i "s/^persistent_peers = .*/persistent_peers = \"${PERSISTENT_PEERS}\"/" "$CONFIG"
fi

sed -i "s/^pex = .*/pex = ${PEX}/" "$CONFIG"

if [ "$NODE_ROLE" = "sentry" ]; then
    [ -n "${PRIVATE_PEER_IDS:-}" ] && \
        sed -i "s/^private_peer_ids = .*/private_peer_ids = \"${PRIVATE_PEER_IDS}\"/" "$CONFIG"
    [ -n "${UNCONDITIONAL_PEER_IDS:-}" ] && \
        sed -i "s/^unconditional_peer_ids = .*/unconditional_peer_ids = \"${UNCONDITIONAL_PEER_IDS}\"/" "$CONFIG"
fi

if [ "$PROMETHEUS" = "true" ]; then
    sed -i "s/^prometheus = .*/prometheus = true/" "$CONFIG"
    sed -i "s/^prometheus_listen_addr = .*/prometheus_listen_addr = \":9090\"/" "$CONFIG"
    # Enable telemetry Prometheus sink (retention in seconds)
    sed -i 's/^prometheus-retention-time = 0$/prometheus-retention-time = 60/' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
    # Enable EVM geth metrics on 0.0.0.0 so Prometheus can scrape from Docker network
    sed -i 's|^geth-metrics-address = "127.0.0.1:8100"|geth-metrics-address = "0.0.0.0:8100"|' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
fi

# Ensure gRPC is on 9095 to avoid port collision with CometBFT Prometheus on 9090
sed -i 's|^address = "localhost:9090"|address = "0.0.0.0:9095"|' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
sed -i 's|^address = "0.0.0.0:9090"|address = "0.0.0.0:9095"|' "${NODE_HOME}/config/app.toml" 2>/dev/null || true

# Enable Cosmos LCD/API server
if [ "$ENABLE_API" = "true" ]; then
    sed -i '/^\[api\]$/,/^enable/{s/^enable = false/enable = true/}' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
    sed -i 's|^address = "tcp://localhost:1317"|address = "tcp://0.0.0.0:1317"|' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
    echo "  LCD/API enabled on 0.0.0.0:1317"
fi

# Enable EVM JSON-RPC with metrics endpoint
if [ "$ENABLE_EVM_RPC" = "true" ]; then
    # Enable the json-rpc server (the only "enable = false" after [json-rpc] header)
    sed -i '/^\[json-rpc\]$/,/^enable/{s/^enable = false/enable = true/}' "${NODE_HOME}/config/app.toml" 2>/dev/null || true
    # Bind EVM RPC to 0.0.0.0 so Prometheus can scrape metrics from the Docker network
    sed -i 's|^address = "127.0.0.1:8545"|address = "0.0.0.0:8545"|' "${NODE_HOME}/config/app.toml"
    sed -i 's|^ws-address = "127.0.0.1:8546"|ws-address = "0.0.0.0:8546"|' "${NODE_HOME}/config/app.toml"
    echo "  EVM RPC enabled on 0.0.0.0:8545"
fi

echo "=== Node Configuration ==="
echo "  Role:       ${NODE_ROLE}"
echo "  Index:      ${INDEX}"
echo "  Moniker:    ${MONIKER}"
echo "  Chain ID:   ${CHAIN_ID}"
echo "  Home:       ${NODE_HOME}"
echo "  PEX:        ${PEX}"
echo "  Peers:      ${PERSISTENT_PEERS:-none}"
echo "=========================="

exec arkd start \
    --home "$NODE_HOME" \
    --minimum-gas-prices "$MIN_GAS_PRICES" \
    --trace \
    "$@"
