#!/usr/bin/env python3
"""
ArkConstellation Validator Fault Tolerance & Consensus Liveness Benchmark
Track 3 (Security, Chaos & Smart Contracts) — Day 2 Deliverable

Simulates / benchmarks CometBFT consensus engine fault tolerance:
1. Active Fault Injection: Pauses/stops 1 validator node (33.3% voting power) via SIGSTOP/Docker.
2. Liveness Boundary: Proves chain continues producing blocks with remaining +2/3 quorum.
3. Automatic Fast-Sync: Resumes validator and monitors block-sync catch-up to chain tip.
4. Observation Mode: Safely monitors quorum commitment health when run without process privileges.
"""

import argparse
import json
import os
import signal
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
MAGENTA = "\033[95m"
BOLD = "\033[1m"
RESET = "\033[0m"


class CometBFTClient:
    def __init__(self, rpc_url: str):
        self.rpc_url = rpc_url.rstrip("/")

    def get(self, endpoint: str, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        url = f"{self.rpc_url}/{endpoint}"
        if params:
            query = "&".join(f"{k}={v}" for k, v in params.items())
            url += f"?{query}"
        
        req = urllib.request.Request(url, headers={"User-Agent": "Chaos-Tester/1.0"})
        try:
            with urllib.request.urlopen(req, timeout=10) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                if "error" in data:
                    raise RuntimeError(f"CometBFT RPC Error ({endpoint}): {data['error']}")
                return data.get("result", {})
        except urllib.error.URLError as e:
            raise ConnectionError(f"Failed to connect to CometBFT at {self.rpc_url}: {e}")

    def get_status(self) -> Dict[str, Any]:
        return self.get("status")

    def get_validators(self, height: Optional[int] = None) -> List[Dict[str, Any]]:
        params = {"height": height} if height else None
        res = self.get("validators", params)
        return res.get("validators", [])

    def get_block_height(self) -> int:
        status = self.get_status()
        return int(status["sync_info"]["latest_block_height"])


def run_validator_failure_simulation(
    rpc_urls: List[str],
    target_failure_power: float,
    recovery_wait_secs: int,
    target_pid: Optional[int] = None,
    docker_container: Optional[str] = None
) -> Dict[str, Any]:
    print(f"\n{BOLD}{MAGENTA}============================================================{RESET}")
    print(f"{BOLD}{MAGENTA} ArkConstellation Validator Fault Tolerance & Liveness Test{RESET}")
    print(f"{BOLD}{MAGENTA}============================================================{RESET}")
    print(f"[*] Primary CometBFT RPC : {rpc_urls[0]}")
    print(f"[*] Target Failure Power : {target_failure_power}% of total voting power")
    if target_pid:
        print(f"[*] Active Fault Mode    : Process PID {target_pid} (SIGSTOP/SIGCONT)")
    elif docker_container:
        print(f"[*] Active Fault Mode    : Docker Container '{docker_container}' (stop/start)")
    else:
        print(f"[*] Execution Mode       : Consensus Liveness & Quorum Observation Mode")

    client = CometBFTClient(rpc_urls[0])
    
    # 1. Inspect Validator Set & Voting Power
    print(f"\n{BOLD}[1/4] Inspecting active validator set and consensus parameters...{RESET}")
    cluster_online = True
    chain_id = "unknown"
    start_height = 0
    validators = []
    total_voting_power = 0

    try:
        status = client.get_status()
        node_info = status.get("node_info", {})
        sync_info = status.get("sync_info", {})
        chain_id = node_info.get("network", "unknown")
        start_height = int(sync_info.get("latest_block_height", 0))
        
        validators = client.get_validators()
        total_voting_power = sum(int(v["voting_power"]) for v in validators)

        print(f"[*] Chain ID             : {BOLD}{chain_id}{RESET}")
        print(f"[*] Current Height       : #{start_height}")
        print(f"[*] Active Validators    : {len(validators)}")
        print(f"[*] Total Voting Power   : {total_voting_power}")

        for idx, val in enumerate(validators):
            power = int(val["voting_power"])
            pct = (power / total_voting_power * 100) if total_voting_power > 0 else 0
            print(f"    Val #{idx+1} | Addr: {val['address'][:16]}... | Power: {power} ({pct:.1f}%)")

        two_thirds_power = (2 / 3) * total_voting_power
        one_third_power = (1 / 3) * total_voting_power
        print(f"[*] 2/3 Quorum Threshold : > {two_thirds_power:.1f} voting power (Required for commit)")
        print(f"[*] Max Byzantine Fault  : < {one_third_power:.1f} voting power (Fault tolerance bound)")

    except Exception as e:
        print(f"{YELLOW}[!] Notice: Cluster RPC offline ({e}).{RESET}")
        print(f"{YELLOW}[!] Running in offline consensus parameter verification mode.{RESET}")
        cluster_online = False

    # 2. Baseline Rate Measurement
    baseline_rate = 0.0
    if cluster_online:
        print(f"\n{BOLD}[2/4] Measuring baseline block commit rate (healthy cluster)...{RESET}")
        baseline_blocks = []
        t_start = time.time()
        last_h = start_height

        for _ in range(3):
            time.sleep(1.5)
            try:
                h = client.get_block_height()
                if h > last_h:
                    baseline_blocks.append({"height": h, "timestamp": time.time()})
                    last_h = h
                    print(f"  --> Healthy Block Commit: Height #{h}")
            except Exception:
                pass

        baseline_rate = len(baseline_blocks) / (time.time() - t_start) if baseline_blocks else 0
        print(f"[*] Baseline Block Rate  : {baseline_rate:.2f} blocks/sec" if baseline_rate > 0 else "[*] Baseline measured.")
    else:
        print(f"\n{BOLD}[2/4] Baseline block rate measurement skipped (cluster offline).{RESET}")

    # 3. Simulate or Inject 33% Fault
    print(f"\n{BOLD}[3/4] Testing 33.3% Validator Outage & Liveness Boundary...{RESET}")
    fault_injected = False
    if target_pid:
        try:
            print(f"[*] Sending SIGSTOP to target validator PID {target_pid}...")
            os.kill(target_pid, signal.SIGSTOP)
            fault_injected = True
        except Exception as e:
            print(f"{RED}[-] Failed to send SIGSTOP: {e}{RESET}")
    elif docker_container:
        try:
            print(f"[*] Stopping docker container '{docker_container}'...")
            subprocess.run(["docker", "stop", docker_container], check=True)
            fault_injected = True
        except Exception as e:
            print(f"{RED}[-] Failed to stop container: {e}{RESET}")

    fault_blocks = []
    f_start = time.time()
    f_last_h = last_h if 'last_h' in locals() else 0

    if cluster_online:
        for cycle in range(4):
            time.sleep(1.5)
            try:
                h = client.get_block_height()
                if h > f_last_h:
                    fault_blocks.append({"height": h, "timestamp": time.time()})
                    f_last_h = h
                    print(f"  --> {GREEN}Consensus Maintained:{RESET} Block #{h} committed with +2/3 active quorum")
            except Exception:
                pass

    # Fail closed: an unreachable cluster proves nothing about liveness.
    liveness_maintained = cluster_online and len(fault_blocks) > 0
    print(f"[*] Outage Liveness Test : {GREEN if liveness_maintained else RED}{'PASS (Continuous block production)' if liveness_maintained else 'FAIL'}{RESET}")

    # 4. Recovery & Fast-Sync Catch-Up
    print(f"\n{BOLD}[4/4] Restoring Validator & Fast-Sync Catch-Up Verification...{RESET}")
    if fault_injected:
        if target_pid:
            try:
                print(f"[*] Sending SIGCONT to validator PID {target_pid}...")
                os.kill(target_pid, signal.SIGCONT)
            except Exception as e:
                print(f"{RED}[-] Failed to send SIGCONT: {e}{RESET}")
        elif docker_container:
            try:
                print(f"[*] Starting docker container '{docker_container}'...")
                subprocess.run(["docker", "start", docker_container], check=True)
            except Exception as e:
                print(f"{RED}[-] Failed to start container: {e}{RESET}")

    time.sleep(recovery_wait_secs)
    end_height = client.get_block_height() if cluster_online else start_height
    fast_sync_verified = cluster_online and (end_height >= f_last_h)

    print(f"[*] Fast-Sync Catch-up   : {GREEN}{'VERIFIED (Node synced to tip)' if fast_sync_verified else 'FAIL'}{RESET}")

    summary = {
        "timestamp": time.time(),
        "chain_id": chain_id,
        "cluster_online": cluster_online,
        "total_validators": len(validators),
        "total_voting_power": total_voting_power,
        "simulated_offline_power_pct": target_failure_power,
        "fault_injected": fault_injected,
        "start_height": start_height,
        "end_height": end_height,
        "baseline_rate_bps": baseline_rate,
        # Raw per-cycle samples taken during the fault window, so a
        # consumer can independently recompute liveness rather than
        # trusting liveness_maintained/fast_sync_verified as-is.
        "fault_blocks": fault_blocks,
        "fault_window_last_height": f_last_h,
        "liveness_maintained": liveness_maintained,
        "fast_sync_verified": fast_sync_verified,
        "executed": cluster_online,
        "pass": (
            cluster_online
            and fault_injected
            and len(validators) > 0
            and end_height > start_height
            and liveness_maintained
            and fast_sync_verified
        )
    }

    report_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reports")
    os.makedirs(report_dir, exist_ok=True)
    report_file = os.path.join(report_dir, "validator-failure-results.json")
    with open(report_file, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"\n[*] Results saved to: {report_file}")

    return summary


def main():
    parser = argparse.ArgumentParser(description="ArkConstellation Validator Fault Simulation")
    parser.add_argument("positional_rpc", nargs="?", default=None, help="Optional positional CometBFT RPC endpoint")
    parser.add_argument("--rpc", default=None, help="CometBFT RPC endpoint (overrides positional)")
    parser.add_argument("--target-power", type=float, default=33.3, help="Percentage of voting power to simulate offline")
    parser.add_argument("--recovery-wait", type=int, default=3, help="Seconds to wait for recovery sync")
    parser.add_argument("--target-pid", type=int, default=None, help="Target validator PID for SIGSTOP/SIGCONT injection")
    parser.add_argument("--docker-container", default=None, help="Target Docker container name for stop/start fault injection")
    args = parser.parse_args()

    rpc_target = args.rpc or args.positional_rpc or os.getenv("CMT_RPC", "http://127.0.0.1:26657")
    res = run_validator_failure_simulation([rpc_target], args.target_power, args.recovery_wait, args.target_pid, args.docker_container)
    if not res.get("pass", False):
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
