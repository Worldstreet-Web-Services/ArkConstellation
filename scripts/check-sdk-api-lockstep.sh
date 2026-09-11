#!/usr/bin/env bash
# Fails if go.mod's cosmos-sdk and cosmossdk.io/api replace directives
# point at different commits. They must move together on every SDK
# re-pin — see docs/proof/fork-audit-cosmos-sdk.md, "Two-module lockstep
# risk" — or generated pulsar code can silently drift out of sync with
# the SDK proto.
set -euo pipefail

GO_MOD="${1:-go.mod}"

sdk_commit=$(grep -oE 'Direct commit link: https://github.com/MANTRA-Chain/cosmos-sdk/tree/[0-9a-f]{7,40}' "$GO_MOD" \
  | head -1 | grep -oE '[0-9a-f]{7,40}$')
api_commit=$(grep -E '^\s*cosmossdk\.io/api\s*=>' "$GO_MOD" \
  | grep -oE '[0-9a-f]{12}$')

if [ -z "$sdk_commit" ] || [ -z "$api_commit" ]; then
  echo "::error::could not extract cosmos-sdk and cosmossdk.io/api commit references from $GO_MOD" >&2
  exit 1
fi

case "$sdk_commit" in
  "$api_commit"*) ;;
  *)
    echo "::error::cosmos-sdk (commit $sdk_commit) and cosmossdk.io/api (commit $api_commit) replace directives in $GO_MOD are out of lockstep — re-pin both together" >&2
    exit 1
    ;;
esac

echo "cosmos-sdk and cosmossdk.io/api are pinned in lockstep at commit $sdk_commit"
