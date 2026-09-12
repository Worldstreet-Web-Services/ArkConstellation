#!/usr/bin/env bash
# Refuses to let activate-static-precompiles.json be submitted if live
# devnet's evm params have drifted from the snapshot hardcoded into this
# proposal's evm_denom/extra_eips/evm_channels/access_control/
# history_serve_window/extended_denom_options fields.
#
# MsgUpdateParams replaces the whole params struct, not just the field
# being changed (see the proposal's own _comment). If any of those other
# fields changed on live devnet after this snapshot was taken - another
# proposal landed first, or the snapshot query was stale - submitting as-is
# would silently revert that change back to what's written here, with no
# error from anything: gov, the tx, or the chain.
set -euo pipefail

# DevSkim: ignore DS162092 -- local devnet tool; override with
# verify-live-params.sh <lcd-url> to point at any node.
LCD="${1:-http://127.0.0.1:1317}"
PROPOSAL="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/activate-static-precompiles.json"

live="$(curl -sS -m 5 "$LCD/cosmos/evm/vm/v1/params")" || {
  echo "error: could not reach $LCD/cosmos/evm/vm/v1/params - is the devnet running?" >&2
  exit 1
}

tmp_live="$(mktemp)"
trap 'rm -f "$tmp_live"' EXIT
printf '%s' "$live" > "$tmp_live"

python3 - "$PROPOSAL" "$tmp_live" <<'PYEOF'
import json
import sys

proposal_path, live_path = sys.argv[1], sys.argv[2]
proposal = json.load(open(proposal_path))["messages"][0]["params"]
live = json.load(open(live_path))["params"]

# active_static_precompiles is what this proposal intentionally changes.
# Everything else must still match live, or the wholesale MsgUpdateParams
# replace will silently revert a field nobody meant to touch here.
watched = [
    "evm_denom",
    "extra_eips",
    "evm_channels",
    "access_control",
    "history_serve_window",
    "extended_denom_options",
]
drifted = {
    k: {"proposal": proposal.get(k), "live": live.get(k)}
    for k in watched
    if proposal.get(k) != live.get(k)
}

if drifted:
    print(
        "error: live devnet evm params have drifted from the snapshot baked "
        "into activate-static-precompiles.json. Submitting this proposal as "
        "written would silently revert the drifted field(s) below back to "
        "their stale values, since MsgUpdateParams replaces the whole params "
        "struct. Re-query live params, update the proposal's non-precompile "
        "fields to match, and re-run this check before submitting.\n",
        file=sys.stderr,
    )
    print(json.dumps(drifted, indent=2), file=sys.stderr)
    sys.exit(1)

print(
    "OK: live devnet evm params match this proposal's snapshot for every "
    "field except active_static_precompiles - safe to submit."
)
PYEOF
