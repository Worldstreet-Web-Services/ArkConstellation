#!/usr/bin/env python3
"""Fail loudly if genesis-DRAFT.json stops describing the chain that
genesis-params.json would actually produce.

genesis-params.json is the reviewed input to RUNBOOK.md Step 1 - the patch
that gets merged over `arkd init`'s output on genesis day. genesis-DRAFT.json
is the full genesis document people read to see what mainnet will look like.
Both are hand-maintained and no build step links them, so a change to one
silently diverges from the other and nothing surfaces it until launch. This
check makes the two agree, in the same way the devnet check-genesis-sync.py
keeps genesis-template.json and pystarport.json honest.

It is a SUBSET check, not an identity check. The draft legitimately carries
everything the patch does not mention (allocations, gentxs, every module's
untouched defaults), and it cannot be regenerated from the patch because the
allocation accounts and gentxs were signed by keys that are not in this
repo. So the assertion is: every key present in the patch has the same value
at the corresponding path in the draft. Keys present only in the draft are
the base genesis's contribution and are expected.

Merge semantics mirrored here - these MUST match the inline `merge()` in
RUNBOOK.md Step 1, which is the real merge (a plain recursive dict merge, not
RFC 7386 JSON Merge Patch). It lives in a `python3 -c` heredoc in the
runbook rather than an importable module, so it is restated here; if you
change one, change the other:

  * dict over dict  -> recurse; keys the patch omits are left untouched.
  * anything else   -> the patch value replaces the base value wholesale.
                       That includes arrays (element-by-element identity
                       is required, no set semantics) and scalars.
  * null            -> an ordinary value that is written verbatim, NOT a
                       delete instruction as it would be under RFC 7386.

One tolerance, one direction only: the draft is the codec-normalised form of
the merged genesis (`arkd` re-marshals it), and proto3 JSON emits zero-values
for fields the patch never set. So inside a structure the patch replaces
wholesale, a key that the patch omits and the draft carries as "", [], {},
0 or false is fine (today: uri/uri_hash inside bank.denom_metadata). A
NON-zero draft-only key inside such a structure is drift - the real merge
would drop it - and fails. Nothing is tolerated in the other direction: a
key the patch sets must be present in the draft with the same value.

Two more assertions, because that is where #36 came from:

  * `_comment` is stripped from the patch's top level (the runbook does the
    same) and is an error anywhere else in either file - a nested one would
    ship into the real genesis.
  * No address may appear in both app_state.evm.accounts and
    app_state.evm.preinstalls (compared case-insensitively), and every
    preinstall address must be EIP-55 checksummed.

EIP-55 needs Keccak-256, which the Python standard library does not ship
(hashlib.sha3_256 is NIST SHA-3, different padding), so a minimal Keccak-f
is included below and self-tested against the EIP-55 reference vectors on
every run rather than adding a dependency to the CI runner.

Usage: check-genesis-sync.py [--params PATH] [--draft PATH]
Defaults are the two files next to this script.
"""
import argparse
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).parent
PARAMS_PATH = HERE / "genesis-params.json"
DRAFT_PATH = HERE / "genesis-DRAFT.json"

_MISSING = object()  # sentinel: "key absent", distinct from a JSON null


def load_json(path):
    try:
        text = path.read_text()
    except FileNotFoundError:
        sys.exit(f"error: {path} not found")
    try:
        return json.loads(text)
    except json.JSONDecodeError as e:
        sys.exit(f"error: {path} is not valid JSON: {e}")


def fmt(value):
    if value is _MISSING:
        return "<absent>"
    return json.dumps(value, sort_keys=True)


def is_zero_value(v):
    """proto3 JSON zero-values the SDK emits for fields nobody set."""
    if isinstance(v, bool):
        return v is False
    if isinstance(v, (int, float)):
        return v == 0
    if isinstance(v, (str, list, dict)):
        return len(v) == 0
    return False  # null is not a zero-value here; it is written verbatim


def scalars_equal(a, b):
    if isinstance(a, bool) or isinstance(b, bool):
        return type(a) is type(b) and a == b  # True != 1, per JSON
    if isinstance(a, (int, float)) and isinstance(b, (int, float)):
        return a == b
    return type(a) is type(b) and a == b


def compare(patch, draft, path, strict, drift):
    """Walk the patch, checking each of its keys against the draft.

    strict=False: we are inside a dict the merge recurses into, so draft-only
    keys are the base genesis's and ignored. strict=True: we are inside a
    value the merge replaced wholesale (an array element, or anything below
    one), so draft-only keys are drift unless they are proto zero-values.
    """
    if isinstance(patch, dict) and isinstance(draft, dict):
        for k, v in patch.items():
            child = f"{path}.{k}" if path else k
            if k not in draft:
                drift.append((child, v, _MISSING))
            else:
                compare(v, draft[k], child, strict, drift)
        if strict:
            for k, v in draft.items():
                if k not in patch and not is_zero_value(v):
                    drift.append((f"{path}.{k}" if path else k, _MISSING, v))
        return
    if isinstance(patch, list) and isinstance(draft, list):
        if len(patch) != len(draft):
            drift.append((path, patch, draft))
            return
        for i, (a, b) in enumerate(zip(patch, draft)):
            compare(a, b, f"{path}[{i}]", True, drift)
        return
    if isinstance(patch, (dict, list)) or isinstance(draft, (dict, list)):
        drift.append((path, patch, draft))  # container vs non-container
        return
    if not scalars_equal(patch, draft):
        drift.append((path, patch, draft))


def find_comments(obj, path, found):
    if isinstance(obj, dict):
        for k, v in obj.items():
            child = f"{path}.{k}" if path else k
            if k == "_comment":
                found.append(child)
            find_comments(v, child, found)
    elif isinstance(obj, list):
        for i, v in enumerate(obj):
            find_comments(v, f"{path}[{i}]", found)


# --- Keccak-256 / EIP-55 ---------------------------------------------------

_MASK = (1 << 64) - 1
_RC = [
    0x0000000000000001, 0x0000000000008082, 0x800000000000808A, 0x8000000080008000,
    0x000000000000808B, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
    0x000000000000008A, 0x0000000000000088, 0x0000000080008009, 0x000000008000000A,
    0x000000008000808B, 0x800000000000008B, 0x8000000000008089, 0x8000000000008003,
    0x8000000000008002, 0x8000000000000080, 0x000000000000800A, 0x800000008000000A,
    0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
]
_ROT = [  # _ROT[x][y]
    [0, 36, 3, 41, 18],
    [1, 44, 10, 45, 2],
    [62, 6, 43, 15, 61],
    [28, 55, 25, 21, 56],
    [27, 20, 39, 8, 14],
]


def _rol(v, n):
    n %= 64
    return ((v << n) | (v >> (64 - n))) & _MASK if n else v


def _keccak_f(A):
    # A is the 5x5 lane state flattened as A[x + 5*y]
    for rc in _RC:
        C = [A[x] ^ A[x + 5] ^ A[x + 10] ^ A[x + 15] ^ A[x + 20] for x in range(5)]
        D = [C[(x - 1) % 5] ^ _rol(C[(x + 1) % 5], 1) for x in range(5)]
        A = [A[i] ^ D[i % 5] for i in range(25)]
        B = [0] * 25
        for x in range(5):
            for y in range(5):
                B[y + 5 * ((2 * x + 3 * y) % 5)] = _rol(A[x + 5 * y], _ROT[x][y])
        A = [
            B[i] ^ ((~B[(i % 5 + 1) % 5 + 5 * (i // 5)] & _MASK) & B[(i % 5 + 2) % 5 + 5 * (i // 5)])
            for i in range(25)
        ]
        A[0] ^= rc
    return A


def keccak256(data):
    rate = 136
    buf = bytearray(data)
    buf.append(0x01)  # original Keccak pad10*1 domain byte (SHA-3 uses 0x06)
    while len(buf) % rate:
        buf.append(0)
    buf[-1] |= 0x80
    A = [0] * 25
    for off in range(0, len(buf), rate):
        for i in range(rate // 8):
            A[i] ^= int.from_bytes(buf[off + 8 * i:off + 8 * i + 8], "little")
        A = _keccak_f(A)
    return b"".join(A[i].to_bytes(8, "little") for i in range(4))


def to_checksum_address(addr):
    hexpart = addr[2:].lower()
    digest = keccak256(hexpart.encode("ascii")).hex()
    return "0x" + "".join(
        c.upper() if int(digest[i], 16) >= 8 else c for i, c in enumerate(hexpart)
    )


_ADDR_RE = re.compile(r"^0x[0-9a-fA-F]{40}$")

# Reference vectors from EIP-55 itself. If the Keccak port above is broken
# these will not round-trip and the check must refuse to run rather than
# quietly accept whatever it is given.
_EIP55_VECTORS = [
    "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
    "0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
    "0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
    "0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
    "0x52908400098527886E0F7030069857D2E4169EE7",
    "0xde709f2102306220921060314715629080e2fb77",
]


def self_test_eip55():
    for want in _EIP55_VECTORS:
        got = to_checksum_address(want)
        if got != want:
            sys.exit(
                "error: built-in Keccak-256 failed its EIP-55 self-test "
                f"(expected {want}, computed {got}). Refusing to run - the "
                "preinstall checksum assertion cannot be trusted."
            )


def evm_addresses(doc, field):
    """Addresses under app_state.evm.<field>[*].address, or [] if absent."""
    entries = doc.get("app_state", {}).get("evm", {}).get(field, [])
    if not isinstance(entries, list):
        return []
    return [e.get("address") for e in entries if isinstance(e, dict) and "address" in e]


def check_preinstalls(docs, errors):
    """docs: [(display name, parsed genesis document), ...]. The checksum
    assertion is per file; the accounts/preinstalls overlap is checked over
    the union of both, since the real genesis is the two merged together."""
    accounts = set()
    preinstalls = {}
    for name, doc in docs:
        for addr in evm_addresses(doc, "accounts"):
            if isinstance(addr, str):
                accounts.add(addr.lower())
        for addr in evm_addresses(doc, "preinstalls"):
            if not isinstance(addr, str) or not _ADDR_RE.match(addr):
                errors.append(f"{name}: preinstall address {addr!r} is not a 0x-prefixed 20-byte hex address")
                continue
            want = to_checksum_address(addr)
            if addr != want:
                errors.append(
                    f"{name}: preinstall address {addr} is not EIP-55 checksummed "
                    f"(expected {want})"
                )
            preinstalls.setdefault(addr.lower(), addr)

    for low, addr in preinstalls.items():
        if low in accounts:
            errors.append(
                f"address {addr} appears in both app_state.evm.accounts and "
                "app_state.evm.preinstalls - the preinstall would collide with "
                "an existing account at InitGenesis"
            )


# ---------------------------------------------------------------------------


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--params", type=Path, default=PARAMS_PATH,
                    help=f"patch file (default: {PARAMS_PATH})")
    ap.add_argument("--draft", type=Path, default=DRAFT_PATH,
                    help=f"full draft genesis (default: {DRAFT_PATH})")
    args = ap.parse_args()

    self_test_eip55()

    patch = load_json(args.params)
    if not isinstance(patch, dict):
        sys.exit(f"error: {args.params} must contain a JSON object, got {type(patch).__name__}")
    patch.pop("_comment", None)  # top-level only, exactly as RUNBOOK Step 1 does

    draft = load_json(args.draft)
    if not isinstance(draft, dict):
        sys.exit(f"error: {args.draft} must contain a JSON object, got {type(draft).__name__}")

    errors = []

    comments = []
    find_comments(patch, "", comments)
    for p in comments:
        errors.append(
            f"{args.params.name}: nested _comment at {p} - RUNBOOK Step 1 only "
            "strips the top-level one, this would ship into the real genesis"
        )
    comments = []
    find_comments(draft, "", comments)
    for p in comments:
        errors.append(
            f"{args.draft.name}: _comment at {p} - the draft is a real genesis "
            "document and must not carry comments"
        )

    drift = []
    compare(patch, draft, "", False, drift)

    check_preinstalls([(args.params.name, patch), (args.draft.name, draft)], errors)

    if drift or errors:
        if drift:
            lines = [
                f"error: {args.params.name} and {args.draft.name} have drifted apart.",
                f"{args.params.name} is what RUNBOOK Step 1 actually merges into the "
                f"mainnet genesis; {args.draft.name} is what people read to see what "
                "mainnet will look like. These must be edited together, or the "
                "reviewed parameters and the published picture of the chain will "
                "describe two different networks.",
            ]
        else:
            lines = [f"error: {args.params.name} / {args.draft.name} failed mainnet genesis checks."]
        if drift:
            lines.append("")
            lines.append(f"Drifted paths ({len(drift)}):")
            for path, a, b in drift:
                lines.append(f"  {path}")
                lines.append(f"    {args.params.name}: {fmt(a)}")
                lines.append(f"    {args.draft.name}: {fmt(b)}")
        if errors:
            lines.append("")
            lines.append(f"Other failures ({len(errors)}):")
            for e in errors:
                lines.append(f"  - {e}")
        sys.exit("\n".join(lines))

    print(
        f"OK: {args.params.name} is a subset of {args.draft.name} "
        "(recursive-merge semantics, proto zero-values tolerated), no nested "
        "_comment, evm preinstalls checksummed and disjoint from evm accounts."
    )


if __name__ == "__main__":
    main()
