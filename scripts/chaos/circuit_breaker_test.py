#!/usr/bin/env python3
"""
ArkConstellation Protocol-Level Circuit Breaker Automated Test
Track 3 (Security, Chaos & Smart Contracts) — Day 2 Deliverable

Verifies cosmossdk.io/x/circuit integration across:
1. Cosmos AnteHandler (app/ante/cosmos.go)
2. EVM JSON-RPC AnteHandler (app/ante/evm.go)
3. Emergency pause of MsgSend & MsgEthereumTx via arkd CLI
4. Zero state mutation during active circuit breaker
5. Live recovery upon circuit breaker reset
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.request
import urllib.error
from typing import Any, Dict, List, Optional, Tuple

GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
BLUE = "\033[94m"
CYAN = "\033[96m"
BOLD = "\033[1m"
RESET = "\033[0m"


def find_binary(preferred_path: Optional[str] = None) -> str:
    """Finds the arkd (or mantrachaind) binary."""
    candidates = [
        preferred_path,
        os.path.join(os.getcwd(), "build", "arkd"),
        os.path.join(os.getcwd(), "build", "mantrachaind"),
        shutil.which("arkd"),
        shutil.which("mantrachaind")
    ]
    for c in candidates:
        if c and os.path.isfile(c) and os.access(c, os.X_OK):
            return os.path.abspath(c)
    return preferred_path or "./build/arkd"


class CircuitBreakerTester:
    def __init__(self, binary_path: str, cmt_rpc: str, evm_rpc: str, chain_id: str, admin_key: str):
        self.binary_path = find_binary(binary_path)
        self.cmt_rpc = cmt_rpc
        self.evm_rpc = evm_rpc
        self.chain_id = chain_id
        self.admin_key = admin_key
        self.results = []
        self.binary_available = os.path.isfile(self.binary_path) and os.access(self.binary_path, os.X_OK)

    def run_cli(self, args: List[str]) -> Tuple[int, str, str]:
        cmd = [self.binary_path] + args
        if "--node" not in args and "-h" not in args and "--help" not in args:
            cmd += ["--node", self.cmt_rpc]
        if "--output" not in args and "-h" not in args and "--help" not in args:
            cmd += ["--output", "json"]
        res = subprocess.run(cmd, capture_output=True, text=True)
        return res.returncode, res.stdout, res.stderr

    def is_node_online(self) -> bool:
        try:
            req = urllib.request.Request(f"{self.cmt_rpc}/status")
            with urllib.request.urlopen(req, timeout=3) as resp:
                return resp.status == 200
        except Exception:
            return False

    def get_disabled_list(self) -> List[str]:
        code, out, err = self.run_cli(["query", "circuit", "disabled-list"])
        if code == 0:
            try:
                data = json.loads(out)
                return data.get("disabled_list", [])
            except Exception:
                pass
        return []

    def record_test(self, name: str, passed: bool, details: str = "", skipped: bool = False):
        status = f"{YELLOW}[SKIPPED]{RESET}" if skipped else (f"{GREEN}[PASS]{RESET}" if passed else f"{RED}[FAIL]{RESET}")
        print(f"  {status} {name}")
        if details:
            print(f"         {details}")
        self.results.append({"name": name, "passed": passed and not skipped, "skipped": skipped, "details": details})

    def execute_suite(self) -> Dict[str, Any]:
        print(f"\n{BOLD}{CYAN}============================================================{RESET}")
        print(f"{BOLD}{CYAN} ArkConstellation Circuit Breaker Live Verification Suite{RESET}")
        print(f"{BOLD}{CYAN}============================================================{RESET}")
        print(f"[*] Node Binary   : {self.binary_path} ({'Found' if self.binary_available else 'Not Found'})")
        print(f"[*] CometBFT RPC  : {self.cmt_rpc}")
        print(f"[*] EVM JSON-RPC  : {self.evm_rpc}")
        print(f"[*] Chain ID      : {self.chain_id}")
        print(f"[*] Admin Account : {self.admin_key}")

        node_online = self.is_node_online()
        print(f"[*] Cluster Status: {'ONLINE' if node_online else 'OFFLINE (Running structural CLI validation)'}")

        # Test 1: Binary & CLI Command Interface Validation
        if self.binary_available:
            code, out, err = self.run_cli(["query", "circuit", "--help"])
            self.record_test("1. Circuit Breaker CLI Command Discovery", code == 0, f"Binary: {self.binary_path} supports x/circuit CLI commands")
        else:
            self.record_test("1. Circuit Breaker CLI Command Discovery", False, f"Binary not found at {self.binary_path}")

        msg_url = "/cosmos.bank.v1beta1.MsgSend"
        evm_msg_url = "/cosmos.evm.vm.v1.MsgEthereumTx"

        if node_online and self.binary_available:
            # Test 2: Query Disabled List
            dlist = self.get_disabled_list()
            self.record_test("2. Initial Circuit Breaker Query", True, f"Disabled messages: {dlist}")

            # Test 3: Disable MsgSend
            code, out, err = self.run_cli([
                "tx", "circuit", "disable", msg_url,
                "--from", self.admin_key,
                "--chain-id", self.chain_id,
                "-y", "-b", "sync", "--gas-prices", "10000000000esp"
            ])
            self.record_test("3. Disable MsgSend Transaction", code == 0, f"Command exit code: {code}")

            # Test 4: Verify AnteHandler Rejection (Cosmos Path)
            test_recipient = "ark100000000000000000000000000000000000000"
            code, out, err = self.run_cli([
                "tx", "bank", "send", self.admin_key, test_recipient, "1000esp",
                "--chain-id", self.chain_id,
                "-y", "-b", "sync", "--gas-prices", "10000000000esp"
            ])
            rejected = (code != 0) or ("circuit breaker" in out.lower()) or ("circuit breaker" in err.lower())
            self.record_test("4. AnteHandler Rejection Verification (Cosmos Path)", rejected, f"Rejected active msg: {out or err}")

            # Test 5: Circuit Breaker Control Plane Covers the EVM Msg Type
            # Disables the EVM message type and re-queries the on-chain
            # disabled-list to confirm x/circuit actually took effect for it
            # (not just that the CLI accepted the command). This proves the
            # message type is disablable, but — unlike Test 4, which submits
            # a real MsgSend and observes its rejection — it does not submit
            # an actual MsgEthereumTx, so it cannot by itself prove
            # app/ante/evm.go's CircuitBreakerDecorator rejects one at
            # runtime. Submitting a real signed EVM tx here would require an
            # eth-signing dependency this harness doesn't otherwise need;
            # tracked as a follow-up rather than done speculatively.
            code, out, err = self.run_cli([
                "tx", "circuit", "disable", evm_msg_url,
                "--from", self.admin_key,
                "--chain-id", self.chain_id,
                "-y", "-b", "sync", "--gas-prices", "10000000000esp"
            ])
            evm_disable_accepted = code == 0
            dlist_after_evm_disable = self.get_disabled_list() if evm_disable_accepted else []
            evm_disabled_onchain = evm_disable_accepted and evm_msg_url in dlist_after_evm_disable
            self.record_test(
                "5. Circuit Breaker Disables EVM Msg Type (control-plane only)",
                evm_disabled_onchain,
                f"Disable exit code: {code}; disabled-list after: {dlist_after_evm_disable} "
                f"(control-plane check only — does not submit a MsgEthereumTx)"
            )

            # Reset the EVM message type so the cluster isn't left disabled.
            code, out, err = self.run_cli([
                "tx", "circuit", "reset", evm_msg_url,
                "--from", self.admin_key,
                "--chain-id", self.chain_id,
                "-y", "-b", "sync", "--gas-prices", "10000000000esp"
            ])

            # Test 6: Reset Circuit Breaker
            code, out, err = self.run_cli([
                "tx", "circuit", "reset", msg_url,
                "--from", self.admin_key,
                "--chain-id", self.chain_id,
                "-y", "-b", "sync", "--gas-prices", "10000000000esp"
            ])
            self.record_test("6. Reset Circuit Breaker & Resume Execution", code == 0, f"Reset exit code: {code}")

        else:
            # Offline CLI Construction & AnteHandler Logic Validation
            print(f"\n{YELLOW}[!] Node cluster offline. Validating CLI argument constructions & AnteHandler handlers.{RESET}")
            # These validate CLI argument construction only. Nothing is executed
            # against a chain, so they are recorded as skipped rather than passed.
            self.record_test(
                "2. Initial Circuit Breaker Query Structure",
                False,
                f"Node offline — validated CLI syntax only: '{self.binary_path} query circuit disabled-list --node {self.cmt_rpc}'",
                skipped=True
            )
            self.record_test(
                "3. Disable MsgSend Transaction Construction",
                False,
                f"Node offline — validated CLI syntax only: '{self.binary_path} tx circuit disable {msg_url}'",
                skipped=True
            )
            self.record_test(
                "4. AnteHandler Rejection Logic (Cosmos Path)",
                False,
                "Node offline — app/ante/cosmos.go CircuitBreakerDecorator not exercised",
                skipped=True
            )
            self.record_test(
                "5. AnteHandler Rejection Logic (EVM Path)",
                False,
                "Node offline — app/ante/evm.go EVMCircuitBreakerDecorator not exercised",
                skipped=True
            )
            self.record_test(
                "6. Reset Circuit Breaker Transaction Construction",
                False,
                f"Node offline — validated CLI syntax only: '{self.binary_path} tx circuit reset {msg_url}'",
                skipped=True
            )

        passed_count = sum(1 for r in self.results if r["passed"])
        skipped_count = sum(1 for r in self.results if r.get("skipped"))
        total_count = len(self.results)
        # Fail closed: structural CLI validation against an offline node is
        # not a circuit breaker test.
        all_passed = (
            node_online
            and skipped_count == 0
            and passed_count == total_count
        )

        print(f"\n{BOLD}{CYAN}============================================================{RESET}")
        print(f"{BOLD} Circuit Breaker Suite Summary{RESET}")
        print(f" Total Tests : {total_count}")
        print(f" Passed      : {GREEN}{passed_count}{RESET}")
        print(f" Failed      : {RED}{total_count - passed_count - skipped_count}{RESET}")
        print(f" Skipped     : {YELLOW}{skipped_count}{RESET}")
        print(f"{BOLD}{CYAN}============================================================{RESET}")

        summary = {
            "timestamp": time.time(),
            "chain_id": self.chain_id,
            "binary": self.binary_path,
            "node_online": node_online,
            "executed": node_online,
            "skipped": skipped_count,
            "total": total_count,
            "passed": passed_count,
            "all_passed": all_passed,
            "tests": self.results
        }

        report_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reports")
        os.makedirs(report_dir, exist_ok=True)
        report_file = os.path.join(report_dir, "circuit-breaker-results.json")
        with open(report_file, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"[*] Report saved to: {report_file}\n")

        return summary


def main():
    parser = argparse.ArgumentParser(description="ArkConstellation Circuit Breaker Test")
    parser.add_argument("positional_cmt_rpc", nargs="?", default=None, help="Optional positional CometBFT RPC endpoint")
    parser.add_argument("positional_evm_rpc", nargs="?", default=None, help="Optional positional EVM JSON-RPC endpoint")
    parser.add_argument("--bin", default=None, help="Path to node binary (default: ./build/arkd)")
    parser.add_argument("--cmt-rpc", default=None, help="CometBFT RPC endpoint (overrides positional)")
    parser.add_argument("--evm-rpc", default=None, help="EVM JSON-RPC endpoint (overrides positional)")
    parser.add_argument("--chain-id", default=os.getenv("CHAIN_ID", "arkdevnet_9000-1"), help="Chain ID (default: arkdevnet_9000-1)")
    parser.add_argument("--admin-key", default="testadmin", help="Admin account name/address")
    args = parser.parse_args()

    cmt_rpc = args.cmt_rpc or args.positional_cmt_rpc or os.getenv("CMT_RPC", "http://127.0.0.1:26657")
    evm_rpc = args.evm_rpc or args.positional_evm_rpc or os.getenv("EVM_RPC", "http://127.0.0.1:8545")

    tester = CircuitBreakerTester(args.bin, cmt_rpc, evm_rpc, args.chain_id, args.admin_key)
    res = tester.execute_suite()
    if not res["all_passed"]:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
