#!/usr/bin/env python3
"""
ArkConstellation EVM static precompile verification - issue #37.

Proves that evm.params.active_static_precompiles actually took effect: that every
address in the decided set dispatches into its Cosmos module, and that the
deliberately-excluded 0x...0803 is not active.

Why a separate runner rather than more rows in rpc_test_runner.py: that suite
proves the JSON-RPC surface works. This proves a genesis *parameter* took effect.
They fail for different reasons - a chain can have a flawless RPC surface and zero
reachable precompiles, which is the exact state ArkConstellation was in when #37
was filed, through a full audit cycle.

HOW ACTIVATION IS DETECTED, and why the obvious method doesn't work
-------------------------------------------------------------------
eth_getCode is NOT a usable signal. Precompiles have no bytecode in state, so a
correctly activated precompile returns "0x" just like an empty address does.

The discriminator that does work is EVM call semantics. Calling an address with a
selector it does not implement:

  * inactive address (no code, not a precompile) -> call SUCCEEDS, returns 0x.
    The EVM treats a call to a codeless address as a no-op success.
  * active precompile -> its Run() cannot match the selector and FAILS, surfacing
    as a JSON-RPC error / "execution reverted".

So "a bare unknown selector errors" is positive evidence of an active precompile,
and "it succeeds with empty data" is positive evidence of an inactive one. That
inverts the usual intuition about what a passing call means, which is why it is
spelled out here rather than left implicit in the code.

On top of that, Bech32 gets a real functional probe: hexToBech32 is pure, needs no
chain state, and returns a predictable string, so a correct answer from it proves
dispatch end-to-end rather than by inference.

Authoritative cross-check: --api queries the chain's own evm params over the
Cosmos REST endpoint. That is the only source that says what the chain thinks is
active, as opposed to what we infer from call behaviour. It also gates the
0x...0803 probe - see --check-vesting.

Usage:
    scripts/chaos/precompile-verify.sh --rpc http://127.0.0.1:8545
    scripts/chaos/precompile-verify.sh --rpc http://sentry-0:8545 \\
        --api http://sentry-0:1317 --check-vesting
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
BLUE = "\033[94m"
BOLD = "\033[1m"
RESET = "\033[0m"

# Must match app/precompiles.go's ArkActiveStaticPrecompiles(). Kept as an
# explicit literal rather than read from the chain, so that a chain configured
# differently than decided shows up as a failure here instead of being accepted.
EXPECTED = [
    ("0x0000000000000000000000000000000000000100", "P256"),
    ("0x0000000000000000000000000000000000000400", "Bech32"),
    ("0x0000000000000000000000000000000000000800", "Staking"),
    ("0x0000000000000000000000000000000000000801", "Distribution"),
    ("0x0000000000000000000000000000000000000802", "ICS20"),
    ("0x0000000000000000000000000000000000000804", "Bank"),
    ("0x0000000000000000000000000000000000000805", "Gov"),
    ("0x0000000000000000000000000000000000000806", "Slashing"),
    ("0x0000000000000000000000000000000000000a01", "distrclaim"),
]

# Advertised by evmtypes.AvailableStaticPrecompiles, implemented nowhere.
# Activating it makes the keeper panic on call. See app/precompiles.go.
VESTING = "0x0000000000000000000000000000000000000803"

BECH32_ADDR = "0x0000000000000000000000000000000000000400"

# A selector no precompile implements. Picked to be obviously not a real function
# rather than risk colliding with one.
UNKNOWN_SELECTOR = "0xdeadbeef"

# hexToBech32(address,string) - keccak256 of the signature, first 4 bytes.
# Signature from scripts/chaos/contracts/Precompiles.sol's IBech32.
HEX_TO_BECH32 = "0xf958a98c"


def abi_encode_hex_to_bech32(addr_hex: str, prefix: str) -> str:
    """ABI-encode hexToBech32(address,string). Hand-rolled so this script keeps
    working with nothing but the standard library - scripts/chaos/requirements.txt
    is not needed to run it."""
    addr = addr_hex.lower().replace("0x", "").rjust(64, "0")
    # head: address (32B) + offset to the string (32B, = 0x40)
    head = addr + "40".rjust(64, "0")
    raw = prefix.encode()
    length = format(len(raw), "064x")
    body = raw.hex().ljust(64, "0")  # one 32-byte word; every bech32 prefix fits
    return HEX_TO_BECH32 + head + length + body


class RPC:
    def __init__(self, url):
        self.url = url
        self.req_id = 1

    def call(self, method, params=None):
        """Returns (result, error). Never raises on a JSON-RPC error - here an
        error response is usually the informative outcome."""
        payload = json.dumps(
            {"jsonrpc": "2.0", "method": method, "params": params or [], "id": self.req_id}
        ).encode()
        self.req_id += 1
        req = urllib.request.Request(
            self.url, data=payload, headers={"Content-Type": "application/json"}
        )
        try:
            with urllib.request.urlopen(req, timeout=15) as resp:
                data = json.loads(resp.read().decode())
        except urllib.error.URLError as e:
            raise ConnectionError(f"cannot reach {self.url}: {e}")
        return data.get("result"), data.get("error")


def query_active_params(api_url):
    """Returns the chain's active_static_precompiles, or None if unavailable."""
    url = api_url.rstrip("/") + "/cosmos/evm/vm/v1/params"
    try:
        with urllib.request.urlopen(url, timeout=15) as resp:
            data = json.loads(resp.read().decode())
    except (urllib.error.URLError, json.JSONDecodeError, OSError):
        return None
    return (data.get("params") or {}).get("active_static_precompiles")


def main():
    ap = argparse.ArgumentParser(
        description="Verify ArkConstellation's EVM static precompiles are active and dispatching."
    )
    ap.add_argument("--rpc", default=os.getenv("EVM_RPC", "http://127.0.0.1:8545"),
                    help="Ethereum JSON-RPC endpoint (default: http://127.0.0.1:8545)")
    ap.add_argument("--api", default=os.getenv("COSMOS_API"),
                    help="Cosmos REST endpoint, e.g. http://127.0.0.1:1317. Optional but "
                         "strongly recommended: it is the only authoritative source for "
                         "what the chain considers active, and it gates --check-vesting.")
    ap.add_argument("--bech32-prefix", default="ark",
                    help="Expected bech32 HRP for the functional Bech32 probe (default: ark)")
    ap.add_argument("--check-vesting", action="store_true",
                    help="Also call 0x...0803. Requires --api, and runs ONLY after the "
                         "params query confirms the address is inactive. Calling an "
                         "active-but-unregistered precompile panics the node, so this "
                         "refuses to guess.")
    args = ap.parse_args()

    rpc = RPC(args.rpc)
    results = []

    def record(name, ok, detail):
        results.append((name, ok, detail))
        colour, mark = (GREEN, "PASS") if ok else (RED, "FAIL")
        print(f"  [{colour}{mark}{RESET}] {name}: {detail}")

    print(f"{BOLD}ArkConstellation EVM static precompile verification{RESET}")
    print(f"RPC: {args.rpc}")
    print(f"API: {args.api or '(not provided - skipping authoritative param check)'}\n")

    chain_id, err = rpc.call("eth_chainId")
    if err or not chain_id:
        print(f"{RED}[-] cannot read eth_chainId: {err}{RESET}")
        return 1
    print(f"{BLUE}[i]{RESET} chain id {int(chain_id, 16)} ({chain_id})\n")

    # ---- 0. authoritative param list ----------------------------------------
    active = None
    if args.api:
        print(f"{BOLD}0. Chain's own evm params (authoritative){RESET}")
        active = query_active_params(args.api)
        if active is None:
            record("params query", False,
                   f"could not read {args.api}/cosmos/evm/vm/v1/params - is the REST "
                   "API enabled? Falling back to call-behaviour inference only.")
        else:
            expected = [a for a, _ in EXPECTED]
            if active == expected:
                record("params match decided set", True, f"{len(active)} addresses, in order")
            else:
                missing = [a for a in expected if a not in active]
                extra = [a for a in active if a not in expected]
                detail = f"got {len(active)}, expected {len(expected)}."
                if not active:
                    detail += (" Empty - this is issue #37's original state: no Cosmos "
                               "module is reachable from Solidity.")
                if missing:
                    detail += f" Missing: {', '.join(a[-6:] for a in missing)}."
                if extra:
                    detail += f" Unexpected: {', '.join(a[-6:] for a in extra)}."
                if VESTING in active:
                    detail += (f" {BOLD}0x...0803 IS ACTIVE - the node will panic on the "
                               f"next call to it. Remove it now.{RESET}")
                record("params match decided set", False, detail)
        print()

    # ---- 1. dispatch probe per address --------------------------------------
    print(f"{BOLD}1. Each precompile dispatches (unknown-selector probe){RESET}")
    print(f"  {BLUE}note{RESET}: an ERROR here is the PASS condition - see module docstring\n")
    for addr, label in EXPECTED:
        result, call_err = rpc.call(
            "eth_call", [{"to": addr, "data": UNKNOWN_SELECTOR}, "latest"]
        )
        name = f"{label} {addr[-6:]}"
        if call_err:
            record(name, True,
                   f"rejected the unknown selector, so it is active "
                   f"({str(call_err.get('message'))[:70]})")
        elif result in (None, "0x", "0x0"):
            record(name, False,
                   "call succeeded with empty data - nothing is registered at this "
                   "address, i.e. the precompile is NOT active. Populate "
                   "evm.params.active_static_precompiles (issue #37).")
        else:
            record(name, True, f"returned data ({result[:18]}...), so it is active")

    # ---- 2. functional probe: Bech32 actually works -------------------------
    print(f"\n{BOLD}2. Functional probe - Bech32 hexToBech32 returns a real answer{RESET}")
    probe_addr = "0x000000000000000000000000000000000000dead"
    data = abi_encode_hex_to_bech32(probe_addr, args.bech32_prefix)
    result, call_err = rpc.call("eth_call", [{"to": BECH32_ADDR, "data": data}, "latest"])
    if call_err:
        record("bech32 hexToBech32", False,
               f"reverted: {str(call_err.get('message'))[:90]}. The precompile is "
               "reachable (see check 1) but the call failed - check the selector and "
               "the --bech32-prefix.")
    elif result in (None, "0x", "0x0"):
        record("bech32 hexToBech32", False, "empty result - precompile not active")
    else:
        # The returned ABI string should contain the HRP. Decode loosely: find the
        # prefix bytes in the returned payload rather than fully ABI-decoding.
        try:
            decoded = bytes.fromhex(result[2:]).decode("utf-8", errors="ignore")
        except ValueError:
            decoded = ""
        if args.bech32_prefix in decoded:
            record("bech32 hexToBech32", True,
                   f"converted {probe_addr[:10]}... to a '{args.bech32_prefix}'-prefixed "
                   f"address - dispatch proven end-to-end, not inferred")
        else:
            record("bech32 hexToBech32", False,
                   f"returned data but no '{args.bech32_prefix}' prefix found in it "
                   f"({result[:40]}...) - wrong HRP, or the selector is stale")

    # ---- 3. the excluded address --------------------------------------------
    print(f"\n{BOLD}3. Excluded precompile 0x...0803 (Vesting){RESET}")
    if active is None:
        print(f"  {YELLOW}[skip]{RESET} needs --api to check safely. Inferring activation "
              f"from call behaviour is not good enough here: if 0x...0803 IS active, the "
              f"call that would tell us panics the node.")
    elif VESTING in active:
        record("vesting not active", False,
               f"{VESTING} is in the chain's active list and the binary registers no "
               "contract for it. The next call to it panics the node, reachable over "
               "unauthenticated eth_call. Remove it from "
               "evm.params.active_static_precompiles immediately - see "
               "docs/decisions/proposals/precompile-enablement-proposal.md.")
    else:
        record("vesting not active", True,
               "absent from the chain's active list - correct. The fork advertises this "
               "address in AvailableStaticPrecompiles but implements nothing behind it.")
        if args.check_vesting:
            result, call_err = rpc.call(
                "eth_call", [{"to": VESTING, "data": UNKNOWN_SELECTOR}, "latest"]
            )
            if call_err:
                record("vesting call is inert", False,
                       f"expected a codeless no-op, got an error: "
                       f"{str(call_err.get('message'))[:70]}. Something IS registered "
                       "there - investigate before launching.")
            else:
                record("vesting call is inert", True,
                       f"call returned {result or '0x'} with no execution, as a codeless "
                       "address should")

    # ---- summary ------------------------------------------------------------
    failures = [n for n, ok, _ in results if not ok]
    passed = len(results) - len(failures)
    print(f"\n{BOLD}Summary:{RESET} {passed}/{len(results)} checks passed")
    if failures:
        print(f"{RED}Failed: {', '.join(failures)}{RESET}")
        print(f"\n{YELLOW}If these fail on a freshly built chain:{RESET} check the genesis "
              "actually carries the list. `arkd init` emits an empty one, and "
              "App.DefaultGenesis() - which does populate it - is not on that code path. "
              "That gap is issue #37.")
        return 1

    print(f"{GREEN}All precompile checks passed.{RESET}")
    print(f"{YELLOW}Still not covered here:{RESET} state-changing paths (delegate, IBC "
          "transfer, reward claim) driven from a deployed contract. Deploy "
          "scripts/chaos/contracts/Precompiles.sol's PrecompileHarness for those - they "
          "are the Eng 3 items recorded as blocking v1.0.0.")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except ConnectionError as e:
        print(f"{RED}[-] {e}{RESET}")
        sys.exit(1)
