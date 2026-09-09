#!/usr/bin/env python3
"""Fail loudly if genesis-template.json and pystarport.json's embedded
"genesis" merge-patch drift apart.

genesis-template.json is the reviewable, standalone copy of the genesis
override patch (see its _comment field for why it can't just be the literal
file pystarport applies). pystarport.json embeds the same values directly
under <chain_id>.genesis because pystarport has no file-include mechanism
for plain JSON configs. Nothing enforces these two stay identical except
this check - run it before every devnet-init so a hand-edit to one file
can never silently apply different genesis parameters than the ones a
human reviewed in the other.
"""
import json
import sys
from pathlib import Path

HERE = Path(__file__).parent
TEMPLATE_PATH = HERE / "genesis-template.json"
PYSTARPORT_PATH = HERE / "pystarport.json"
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


def main():
    template = load_json(TEMPLATE_PATH)
    template.pop("_comment", None)

    pystarport_cfg = load_json(PYSTARPORT_PATH)
    try:
        embedded = pystarport_cfg[CHAIN_ID]["genesis"]
    except KeyError as e:
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


if __name__ == "__main__":
    main()
