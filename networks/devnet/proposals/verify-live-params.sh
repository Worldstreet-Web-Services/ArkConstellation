#!/usr/bin/env bash
# Refuses to let activate-static-precompiles.json be submitted if live
# devnet's evm params have drifted from the snapshot hardcoded into the
# proposal for every field other than active_static_precompiles.
#
# MsgUpdateParams replaces the whole params struct, not just the field
# being changed (see the proposal's own _comment). If any other field
# changed on live devnet after this snapshot was taken - another proposal
# landed first, or the snapshot query was stale - submitting as-is would
# silently revert that change back to what's written here, with no error
# from anything: gov, the tx, or the chain.
set -euo pipefail

# DevSkim: ignore DS162092 -- local devnet tool; override with
# verify-live-params.sh <lcd-url> to point at any node.
LCD="${1:-http://127.0.0.1:1317}"
PROPOSAL="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/activate-static-precompiles.json"
URL="$LCD/cosmos/evm/vm/v1/params"

tmp_live="$(mktemp)"
trap 'rm -f "$tmp_live"' EXIT

# --fail turns a 404/500/proxy error page into a non-zero exit instead of a
# "successful" download of an HTML body that the JSON parse below would choke on.
curl -sS --fail -m 5 -o "$tmp_live" "$URL" || {
  echo "error: could not fetch $URL - is the devnet running, and is this the LCD (REST) port?" >&2
  exit 1
}

jq -e '.params | type == "object"' "$tmp_live" >/dev/null 2>&1 || {
  echo "error: $URL did not return a {\"params\": {...}} object. Raw response:" >&2
  head -c 500 "$tmp_live" >&2
  echo >&2
  exit 1
}

python3 - "$PROPOSAL" "$tmp_live" <<'PYEOF'
import json
import sys

proposal_path, live_path = sys.argv[1], sys.argv[2]
proposal = json.load(open(proposal_path))["messages"][0]["params"]
live = json.load(open(live_path))["params"]

# active_static_precompiles is what this proposal intentionally changes.
# Every other field - including any the chain added after this script was
# written - must still match live, or the wholesale MsgUpdateParams replace
# will silently revert a field nobody meant to touch here.
target = "active_static_precompiles"
fields = sorted((set(proposal) | set(live)) - {target})
drifted = {
    k: {"proposal": proposal.get(k, "<missing>"), "live": live.get(k, "<missing>")}
    for k in fields
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
    f"OK: live devnet evm params match this proposal's snapshot for every "
    f"field except {target} ({len(fields)} fields compared) - safe to submit."
)
PYEOF
