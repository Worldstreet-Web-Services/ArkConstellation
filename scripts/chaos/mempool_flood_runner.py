#!/usr/bin/env python3
"""
ArkConstellation Mempool Transaction Flooder & Fee Market Scaling Benchmark
Track 3 (Security, Chaos & Smart Contracts) — Day 2 Deliverable

Simulates high-throughput transaction bursts against JSON-RPC endpoints to test:
1. Mempool capacity, ingestion rates, and queueing.
2. Dynamic base fee scaling under skip-mev/feemarket (EIP-1559).
3. Base fee decay curve when mempool traffic subsides.
4. Transaction drop rates and gas consumption metrics.
"""

import argparse
import concurrent.futures
import json
import os
import shutil
import subprocess
import sys
import time
import urllib.request
import urllib.error
from typing import Any, Dict, List, Optional, Tuple

try:
    from eth_account import Account
    from eth_account.signers.local import LocalAccount
    import eth_utils
    from web3 import Web3
except ImportError as e:
    print(f"[-] Missing Python dependencies: {e}")
    print("[-] Please install required dependencies using: pip install -r scripts/chaos/requirements.txt")
    sys.exit(1)

GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
BLUE = "\033[94m"
CYAN = "\033[96m"
BOLD = "\033[1m"
RESET = "\033[0m"


class RPCClient:
    def __init__(self, rpc_url: str):
        self.rpc_url = rpc_url
        self.req_id = 1

    def call(self, method: str, params: Optional[list] = None) -> Any:
        if params is None:
            params = []
        payload = json.dumps({
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
            "id": self.req_id
        }).encode("utf-8")
        self.req_id += 1

        req = urllib.request.Request(
            self.rpc_url,
            data=payload,
            headers={"Content-Type": "application/json"}
        )

        try:
            with urllib.request.urlopen(req, timeout=15) as response:
                res_data = json.loads(response.read().decode("utf-8"))
                if "error" in res_data:
                    raise RuntimeError(f"RPC Error ({method}): {res_data['error']}")
                return res_data.get("result")
        except urllib.error.URLError as e:
            raise ConnectionError(f"Failed to connect to RPC at {self.rpc_url}: {e}")


def get_default_solc() -> str:
    custom_path = os.path.expanduser("~/.solc-bin/solc")
    if os.path.isfile(custom_path) and os.access(custom_path, os.X_OK):
        return custom_path
    solc_path = shutil.which("solc")
    if solc_path:
        try:
            res = subprocess.run([solc_path, "--version"], capture_output=True)
            if res.returncode == 0:
                return solc_path
        except Exception:
            pass
    return "solc"


def compile_storage_contract(sol_path: str, solc_bin: str = "solc") -> Tuple[str, list]:
    if not os.path.exists(sol_path):
        raise FileNotFoundError(f"Solidity file not found: {sol_path}")
    cmd = [solc_bin, "--combined-json", "bin,abi", sol_path]
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode != 0:
        raise RuntimeError(f"solc compilation failed:\n{res.stderr}")
    data = json.loads(res.stdout)
    contract_key = next((k for k in data.get("contracts", {}) if "TestStorage" in k), None)
    if not contract_key:
        raise RuntimeError("TestStorage contract not found in compilation output")
    bin_hex = data["contracts"][contract_key]["bin"]
    raw_abi = data["contracts"][contract_key]["abi"]
    abi = json.loads(raw_abi) if isinstance(raw_abi, str) else raw_abi
    return "0x" + bin_hex, abi


def get_base_fee_and_block(client: RPCClient) -> Tuple[int, int]:
    """Retrieves current block number and base fee / gas price in wei."""
    block_hex = client.call("eth_blockNumber")
    block_num = int(block_hex, 16)
    
    try:
        block_data = client.call("eth_getBlockByNumber", ["latest", False])
        if block_data and "baseFeePerGas" in block_data and block_data["baseFeePerGas"]:
            return int(block_data["baseFeePerGas"], 16), block_num
    except Exception:
        pass
    
    gas_price_hex = client.call("eth_gasPrice")
    return int(gas_price_hex, 16), block_num


def send_raw_tx_worker(rpc_url: str, raw_tx_hex: str) -> Dict[str, Any]:
    client = RPCClient(rpc_url)
    start_t = time.time()
    try:
        tx_hash = client.call("eth_sendRawTransaction", [raw_tx_hex])
        elapsed = time.time() - start_t
        return {"success": True, "tx_hash": tx_hash, "latency": elapsed, "error": None}
    except Exception as e:
        elapsed = time.time() - start_t
        return {"success": False, "tx_hash": None, "latency": elapsed, "error": str(e)}


def run_mempool_flood(
    rpc_url: str,
    chain_id: int,
    private_key: str,
    tx_count: int,
    concurrency: int,
    flood_type: str,
    solc_path: str = "solc"
) -> Dict[str, Any]:
    print(f"\n{BOLD}{CYAN}============================================================{RESET}")
    print(f"{BOLD}{CYAN} ArkConstellation Mempool Transaction Flood & Fee Benchmark{RESET}")
    print(f"{BOLD}{CYAN}============================================================{RESET}")
    print(f"[*] Target JSON-RPC   : {rpc_url}")
    print(f"[*] EVM Chain ID      : {chain_id}")
    print(f"[*] Total Tx Count    : {tx_count}")
    print(f"[*] Concurrency Level : {concurrency}")
    print(f"[*] Flood Type        : {flood_type}")

    client = RPCClient(rpc_url)
    account: LocalAccount = Account.from_key(private_key)
    print(f"[*] Spammer Address   : {account.address}")

    # Baseline Measurements
    initial_base_fee = 10**9
    start_block = 0
    initial_balance = 0
    start_nonce = 0
    live_rpc = True

    try:
        initial_base_fee, start_block = get_base_fee_and_block(client)
        initial_balance_hex = client.call("eth_getBalance", [account.address, "latest"])
        initial_balance = int(initial_balance_hex, 16)
        start_nonce_hex = client.call("eth_getTransactionCount", [account.address, "latest"])
        start_nonce = int(start_nonce_hex, 16)
        print(f"[*] Initial Base Fee  : {initial_base_fee} esp ({initial_base_fee / 10**9:.2f} Gwei / espes)")
        print(f"[*] Initial Balance   : {initial_balance} esp ({initial_balance / 10**18:.4f} KASH)")
        print(f"[*] Starting Nonce    : {start_nonce}")
        print(f"[*] Starting Block    : {start_block}")
    except Exception as e:
        print(f"{YELLOW}[!] Notice: Could not connect to live RPC at {rpc_url} ({e}).{RESET}")
        print(f"{YELLOW}[!] Running in offline transaction generation & validation mode.{RESET}")
        live_rpc = False

    # Pre-generate signed raw transactions
    print(f"\n{BOLD}[1/4] Pre-generating {tx_count} signed EIP-155 {flood_type} transactions...{RESET}")
    raw_txs = []
    prep_start = time.time()

    w3 = Web3()
    recipient = "0x000000000000000000000000000000000000dEaD"
    gas_limit = 21000
    gas_price = max(initial_base_fee * 2, 10**9)

    contract_bytecode = None
    contract_abi = None
    if flood_type == "contract":
        script_dir = os.path.dirname(os.path.abspath(__file__))
        sol_path = os.path.join(script_dir, "contracts", "TestStorage.sol")
        try:
            contract_bytecode, contract_abi = compile_storage_contract(sol_path, solc_path)
            gas_limit = 200000
            print(f"[*] Compiled TestStorage.sol for contract calldata flood.")
        except Exception as e:
            print(f"{YELLOW}[!] Contract compilation note: {e}. Using simulated contract calldata.{RESET}")

    for i in range(tx_count):
        nonce = start_nonce + i
        if flood_type == "contract" and contract_abi:
            inst = w3.eth.contract(abi=contract_abi)
            tx_data = inst.functions.setValue(1000 + i)._encode_transaction_data()
            to_addr = recipient
        elif flood_type == "contract":
            tx_data = "0x55241077" + eth_utils.encode_hex(eth_utils.to_bytes(primitive=1000 + i)).rjust(64, '0')[2:]
            to_addr = recipient
        else:
            tx_data = b""
            to_addr = recipient

        tx_dict = {
            "to": to_addr,
            "value": 1000 if flood_type == "transfer" else 0,
            "gas": gas_limit,
            "gasPrice": gas_price,
            "nonce": nonce,
            "chainId": chain_id,
            "data": tx_data
        }
        signed = account.sign_transaction(tx_dict)
        raw_hex = signed.raw_transaction.hex()
        raw_txs.append(raw_hex if raw_hex.startswith("0x") else f"0x{raw_hex}")

    prep_duration = time.time() - prep_start
    print(f"[*] Generated {len(raw_txs)} transactions in {prep_duration:.2f}s ({len(raw_txs)/prep_duration:.1f} tx/s)")

    # Execute Flood
    submission_tps = 0.0
    successful_submits = []
    failed_submits = []
    blocks_tracked = []
    fee_curve = []
    peak_base_fee = initial_base_fee
    final_base_fee = initial_base_fee
    mined_txs = 0

    if live_rpc and initial_balance > 0:
        print(f"\n{BOLD}[2/4] Broadcasting {tx_count} transactions across {concurrency} concurrent threads...{RESET}")
        broadcast_start = time.time()
        results = []

        with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as executor:
            futures = [executor.submit(send_raw_tx_worker, rpc_url, tx) for tx in raw_txs]
            for f in concurrent.futures.as_completed(futures):
                results.append(f.result())

        broadcast_duration = time.time() - broadcast_start
        successful_submits = [r for r in results if r["success"]]
        failed_submits = [r for r in results if not r["success"]]
        submission_tps = len(successful_submits) / broadcast_duration if broadcast_duration > 0 else 0

        print(f"[*] Broadcast Complete in {broadcast_duration:.2f}s")
        print(f"[*] Submitted Successfully : {GREEN}{len(successful_submits)}{RESET} / {tx_count}")
        print(f"[*] Failed Submissions     : {RED if failed_submits else GREEN}{len(failed_submits)}{RESET}")
        print(f"[*] Peak Submission TPS    : {BOLD}{submission_tps:.2f} tx/s{RESET}")

        # Monitor Block Inclusion & Base Fee Evolution
        print(f"\n{BOLD}[3/4] Monitoring block inclusion and dynamic fee market adjustments...{RESET}")
        for cycle in range(10):
            time.sleep(1.5)
            try:
                current_fee, current_block = get_base_fee_and_block(client)
                fee_curve.append({"timestamp": time.time(), "block": current_block, "base_fee": current_fee})
                if current_block not in blocks_tracked:
                    blocks_tracked.append(current_block)
                    print(f"  --> Block #{current_block}: Base Fee = {current_fee} esp ({current_fee / 10**9:.2f} espes / Gwei)")
            except Exception:
                pass

        if fee_curve:
            peak_base_fee = max(f["base_fee"] for f in fee_curve)
            final_base_fee = fee_curve[-1]["base_fee"]
        try:
            final_nonce_hex = client.call("eth_getTransactionCount", [account.address, "latest"])
            mined_txs = int(final_nonce_hex, 16) - start_nonce
        except Exception:
            mined_txs = len(successful_submits)
    else:
        print(f"\n{BOLD}[2/4] Validating offline signed raw transaction payload structures...{RESET}")
        print(f"[*] All {len(raw_txs)} transactions signed with correct EIP-155 chainId={chain_id} and 0x hex prefixes.")
        print(f"\n{BOLD}[3/4] Fee market monitoring skipped (running in offline validation mode).{RESET}")

    # Strict '>': at zero load peak == initial, which is not evidence of scaling.
    # Only meaningful if blocks were actually observed during the flood.
    fee_scaled_up = bool(blocks_tracked) and peak_base_fee > initial_base_fee
    # Signing transactions offline proves nothing about the mempool; require
    # that they were actually submitted and mined.
    mempool_healthy = (
        live_rpc
        and len(raw_txs) == tx_count
        and len(successful_submits) > 0
        and mined_txs > 0
    )

    print(f"\n{BOLD}[4/4] Final Results & Analysis{RESET}")
    print(f"{BOLD}{CYAN}============================================================{RESET}")
    print(f" Total Signed Txs       : {len(raw_txs)} / {tx_count}")
    print(f" Initial Base Fee       : {initial_base_fee} esp ({initial_base_fee / 10**9:.2f} Gwei)")
    print(f" Peak Base Fee          : {peak_base_fee} esp ({peak_base_fee / 10**9:.2f} Gwei)")
    print(f" Final Base Fee (Decay) : {final_base_fee} esp ({final_base_fee / 10**9:.2f} Gwei)")
    print(f" Total Mined Tx Count   : {mined_txs} / {tx_count}")
    print(f" Blocks Spanned         : {len(blocks_tracked)} blocks")
    print(f" Transaction Gen Rate   : {GREEN}{len(raw_txs)/prep_duration:.1f} tx/s{RESET}")
    print(f"{BOLD}{CYAN}============================================================{RESET}")

    summary = {
        "timestamp": time.time(),
        "rpc_url": rpc_url,
        "chain_id": chain_id,
        "tx_count": tx_count,
        "concurrency": concurrency,
        "flood_type": flood_type,
        "submission_tps": submission_tps,
        "successful_submits": len(successful_submits),
        "failed_submits": len(failed_submits),
        "mined_txs": mined_txs,
        "initial_base_fee": initial_base_fee,
        "peak_base_fee": peak_base_fee,
        "final_base_fee": final_base_fee,
        "fee_scaled_up": fee_scaled_up,
        "blocks_tracked": blocks_tracked,
        "fee_curve": fee_curve,
        "executed": bool(live_rpc),
        "pass": mempool_healthy and fee_scaled_up
    }

    report_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "reports")
    os.makedirs(report_dir, exist_ok=True)
    report_file = os.path.join(report_dir, "mempool-flood-results.json")
    with open(report_file, "w") as f:
        json.dump(summary, f, indent=2)
    print(f"[*] Detailed metrics written to: {report_file}\n")

    return summary


def main():
    parser = argparse.ArgumentParser(description="ArkConstellation Mempool Transaction Flooder")
    parser.add_argument("positional_rpc", nargs="?", default=None, help="Optional positional JSON-RPC URL")
    parser.add_argument("--rpc", default=None, help="Target JSON-RPC URL (overrides positional)")
    parser.add_argument("--chain-id", type=int, default=int(os.getenv("CHAIN_ID", "9000")), help="EVM Chain ID (default: 9000 devnet)")
    parser.add_argument("--private-key", default=os.getenv("PRIVATE_KEY", "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"), help="Spammer account private key")
    parser.add_argument("--txs", type=int, default=200, help="Number of transactions to flood")
    parser.add_argument("--concurrency", type=int, default=20, help="Concurrent worker threads")
    parser.add_argument("--type", default="transfer", choices=["transfer", "contract"], help="Transaction payload type")
    parser.add_argument("--solc", default=get_default_solc(), help="Path to solc compiler binary")
    args = parser.parse_args()

    rpc_target = args.rpc or args.positional_rpc or os.getenv("EVM_RPC", "http://127.0.0.1:8545")

    summary = run_mempool_flood(rpc_target, args.chain_id, args.private_key, args.txs, args.concurrency, args.type, args.solc)
    if not summary["pass"]:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
