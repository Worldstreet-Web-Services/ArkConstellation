#!/usr/bin/env python3
"""Fail loudly if genesis-template.json and pystarport.json's embedded
"genesis" merge-patch drift apart, or if active_static_precompiles drifts
apart across every other file that hardcodes a copy of that same list.

genesis-template.json is the reviewable, standalone copy of the genesis
override patch (see its _comment field for why it can't just be the literal
file pystarport applies). pystarport.json embeds the same values directly
under <chain_id>.genesis because pystarport has no file-include mechanism
for plain JSON configs. Nothing enforces these two stay identical except
this check - run it before every devnet-init so a hand-edit to one file
can never silently apply different genesis parameters than the ones a
human reviewed in the other.

Separately, active_static_precompiles (the list that was empty since
genesis - see proposals/activate-static-precompiles.json for the incident)
is hardcoded verbatim in three more files with no shared source: the devnet
proposal, the mainnet genesis draft, and the rehearsal fixture. This check
catches the copies drifting from each other without needing a Go toolchain;
app/precompiles_test.go is the authoritative check that they all match what
app.go actually registers with the EVM keeper.
"""
import json
import sys
from pathlib import Path

HERE = Path(__file__).parent
REPO_ROOT = HERE.parent.parent
TEMPLATE_PATH = HERE / "genesis-template.json"
PYSTARPORT_PATH = HERE / "pystarport.json"
PROPOSAL_PATH = HERE / "proposals" / "activate-static-precompiles.json"
MAINNET_DRAFT_PATH = REPO_ROOT / "networks" / "mainnet" / "genesis-DRAFT.json"
REHEARSAL_PATH = REPO_ROOT / "scripts" / "genesis" / "rehearsal" / "base-genesis.json"
CHAIN_ID = "arkdevnet_9000-1"


def load_json(path):
    try:
        text = path.read_text()
    except FileNotFoundError:
        sys.exit(f"error: {path} not found")
    try:
        return json.loads(text)
    except json.JSONDecodeError as e:
        sys.exit(f"error: {path} is not valid JSON: {e}")


def get_precompiles(doc, source):
    try:
        return doc["app_state"]["evm"]["params"]["active_static_precompiles"]
    except (KeyError, TypeError) as e:
        sys.exit(f"error: {source} missing app_state.evm.params.active_static_precompiles: {e}")


def as_address_set(addrs, source):
    lowered = [a.lower() for a in addrs]
    if len(set(lowered)) != len(lowered):
        sys.exit(f"error: {source} lists a precompile address more than once: {addrs}")
    return set(lowered)


def check_precompile_list_sync(template):
    source_list = get_precompiles(template, TEMPLATE_PATH)
    if not source_list:
        sys.exit(
            f"error: {TEMPLATE_PATH}'s active_static_precompiles is empty - "
            "nothing to sync the other copies against."
        )
    source_set = as_address_set(source_list, TEMPLATE_PATH)

    mainnet_draft = load_json(MAINNET_DRAFT_PATH)
    rehearsal = load_json(REHEARSAL_PATH)
    proposal = load_json(PROPOSAL_PATH)

    try:
        proposal_list = proposal["messages"][0]["params"]["active_static_precompiles"]
    except (KeyError, IndexError, TypeError) as e:
        sys.exit(
            f"error: {PROPOSAL_PATH} missing "
            f"messages[0].params.active_static_precompiles: {e}"
        )

    candidates = [
        (MAINNET_DRAFT_PATH, get_precompiles(mainnet_draft, MAINNET_DRAFT_PATH)),
        (REHEARSAL_PATH, get_precompiles(rehearsal, REHEARSAL_PATH)),
        (PROPOSAL_PATH, proposal_list),
    ]
    # Activation is a set: order is irrelevant on-chain, so don't fail on a reorder.
    mismatches = [
        (path, lst) for path, lst in candidates
        if as_address_set(lst, path) != source_set
    ]
    if mismatches:
        details = "\n\n".join(
            f"{path}:\n{json.dumps(lst, indent=2)}" for path, lst in mismatches
        )
        sys.exit(
            "error: active_static_precompiles has drifted apart across files "
            "that must all carry the identical list - this is the exact class "
            "of bug that left it empty on devnet in the first place (see "
            f"{PROPOSAL_PATH}).\n\n"
            f"{TEMPLATE_PATH} (source of truth):\n"
            f"{json.dumps(source_list, indent=2)}\n\n"
            f"{details}"
        )

    print(
        "OK: active_static_precompiles is identical across genesis-template.json, "
        "the devnet proposal, the mainnet draft, and the rehearsal fixture."
    )


def main():
    template = load_json(TEMPLATE_PATH)
    if not isinstance(template, dict):
        sys.exit(f"error: {TEMPLATE_PATH} must contain a JSON object, got {type(template).__name__}")
    template.pop("_comment", None)

    pystarport_cfg = load_json(PYSTARPORT_PATH)
    try:
        embedded = pystarport_cfg[CHAIN_ID]["genesis"]
    except (KeyError, TypeError) as e:
        sys.exit(
            f"error: pystarport.json missing expected key path "
            f"['{CHAIN_ID}']['genesis']: {e}"
        )

    if template != embedded:
        sys.exit(
            "error: genesis-template.json and pystarport.json's embedded "
            "genesis patch have drifted apart.\n"
            "These must be edited together - pystarport.json is what "
            "actually gets applied to the devnet, genesis-template.json is "
            "the reviewable copy. Update both, or the devnet will run "
            "different parameters than the ones committed for review.\n\n"
            f"genesis-template.json (minus _comment):\n"
            f"{json.dumps(template, indent=2, sort_keys=True)}\n\n"
            f"pystarport.json[{CHAIN_ID}].genesis:\n"
            f"{json.dumps(embedded, indent=2, sort_keys=True)}"
        )

    print("OK: genesis-template.json and pystarport.json genesis patch are in sync.")

    check_precompile_list_sync(template)


if __name__ == "__main__":
    main()
