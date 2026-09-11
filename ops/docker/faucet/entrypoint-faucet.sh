#!/bin/sh
set -eu

HOME_DIR="${HOME_DIR:-/home/nonroot/.ark/faucet}"
KEY_NAME="${KEY_NAME:-faucet}"

mkdir -p "$HOME_DIR"

if ! arkd keys show "$KEY_NAME" --home "$HOME_DIR" --keyring-backend test >/dev/null 2>&1; then
    echo ">>> Importing faucet key from FAUCET_MNEMONIC..."
    echo "$FAUCET_MNEMONIC" | arkd keys add "$KEY_NAME" --recover --home "$HOME_DIR" --keyring-backend test
fi

exec python3 /faucet.py
