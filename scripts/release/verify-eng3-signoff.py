#!/usr/bin/env python3
"""
scripts/release/verify-eng3-signoff.py

Automated Security & Chaos Release Gate Validator for ArkConstellation.
Enforces the mandatory requirement:
"Do not tag ark-v1.0.0-rc1 without real Eng 3 security/chaos sign-off"

The gate asserts on the *raw metrics* recorded by each harness, never on the
summary booleans the harness reports about itself. A suite that never reached a
running chain records `cluster_online: false` / zero counters and is rejected,
regardless of what its own `pass` field claims.

Checks:
1. Formal Track 3 milestone reports (Day 1 / Day 2 / Day 3 certification).
2. Static analysis severities (GoSec HIGH, Semgrep ERROR, Slither High/Medium)
   plus Slither coverage of the contract that actually ships.
3. Mempool, validator resiliency, circuit breaker and JSON-RPC results.
4. Launch guardrail contracts and hard-reboot state recovery.
"""

import sys
import json
from pathlib import Path

# ANSI Color Codes
GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
CYAN = "\033[96m"
BOLD = "\033[1m"
RESET = "\033[0m"

REPO_ROOT = Path(__file__).resolve().parent.parent.parent
CHAOS_DIR = REPO_ROOT / "scripts" / "chaos"
REPORTS_DIR = CHAOS_DIR / "reports"
CONTRACTS_DIR = CHAOS_DIR / "contracts"

# gosec records absolute paths from whichever machine ran it, so generated-file
# exclusion has to match on path suffix rather than stat() the file locally.
GENERATED_SUFFIXES = (".pb.go", ".pb.gw.go")

GUARDRAIL_CONTRACT = "LaunchGuardrail.sol"


def log_pass(message: str):
    print(f"  [{GREEN}PASS{RESET}] {message}")


def log_fail(message: str):
    print(f"  [{RED}FAIL{RESET}] {message}")


def log_info(message: str):
    print(f"  [{CYAN}INFO{RESET}] {message}")


class Gate:
    """Collects failures while logging each check as it is evaluated."""

    def __init__(self):
        self.errors = []

    def check(self, condition: bool, ok_msg: str, fail_msg: str):
        if condition:
            log_pass(ok_msg)
        else:
            self.errors.append(fail_msg)
            log_fail(fail_msg)
        return bool(condition)

    def fail(self, message: str):
        self.errors.append(message)
        log_fail(message)

    def load_json(self, path: Path, label: str):
        """Return parsed JSON, or None (recording a failure) if unusable."""
        if not path.exists():
            self.fail(f"{label}: missing artifact {path.relative_to(REPO_ROOT)}")
            return None
        try:
            return json.loads(path.read_text(encoding="utf-8"))
        except Exception as exc:
            self.fail(f"{label}: unparseable JSON in {path.name} ({exc})")
            return None


def is_generated(path: str) -> bool:
    return path.endswith(GENERATED_SUFFIXES)


def check_suite_counts(gate: Gate, data: dict, label: str):
    """Shared assertion for the pass/fail/skip suites (rate-limit, rpc)."""
    total = data.get("total", 0)
    passed = data.get("passed", 0)
    failed = data.get("failed", 1)
    skipped = data.get("skipped", 0)
    gate.check(
        total > 0 and failed == 0 and skipped == 0 and passed == total,
        f"{label}: {passed}/{total} checks passed, none skipped",
        f"{label}: {passed}/{total} passed, {failed} failed, {skipped} skipped "
        f"(require all executed and passing)",
    )


def check_executed(gate: Gate, data: dict, label: str, *liveness_fields: str):
    """
    Reject any artifact produced without a reachable chain.

    Accepts the explicit `executed` flag when the harness emits one, and falls
    back to the liveness field for artifacts predating it.
    """
    executed = data.get("executed")
    if executed is None:
        executed = all(data.get(f) is True for f in liveness_fields)
        detail = " / ".join(f"{f}={data.get(f)!r}" for f in liveness_fields)
    else:
        detail = f"executed={executed!r}"
    return gate.check(
        executed is True,
        f"{label}: executed against a live chain ({detail})",
        f"{label}: NOT executed against a live chain ({detail}) — "
        f"evidence was produced with nothing running",
    )


def verify_gate(target_tag: str = "") -> bool:
    print(f"\n{BOLD}{CYAN}======================================================{RESET}")
    print(f"{BOLD}{CYAN}   ARKCONSTELLATION RELEASE GATE ENFORCEMENT ENGINE   {RESET}")
    print(f"{BOLD}{CYAN}======================================================{RESET}")
    if target_tag:
        print(f"  Target Release / Tag: {BOLD}{target_tag}{RESET}")
    print(f"  Evaluating Eng 3 (Security & Chaos) Prerequisites...\n")

    gate = Gate()

    # ---------------------------------------------------------
    # 1. Verify Markdown Milestone Reports
    # ---------------------------------------------------------
    print(f"{BOLD}1. Checking Milestone Markdown Reports...{RESET}")
    md_reports = {
        "Day 1 Static Analysis": REPORTS_DIR / "day1-static-analysis.md",
        "Day 2 Chaos Report": REPORTS_DIR / "day2-chaos-report.md",
        "Day 3 Final Sign-Off": REPORTS_DIR / "day3-final-signoff.md",
    }

    for name, path in md_reports.items():
        gate.check(
            path.exists(),
            f"{name} present: {path.name}",
            f"Missing markdown report: {path.relative_to(REPO_ROOT)}",
        )

    day3_path = md_reports["Day 3 Final Sign-Off"]
    if day3_path.exists():
        content = day3_path.read_text(encoding="utf-8")
        gate.check(
            "CERTIFIED PRODUCTION READY" in content,
            "Day 3 Sign-Off contains 'CERTIFIED PRODUCTION READY'",
            "Day 3 report missing explicit 'CERTIFIED PRODUCTION READY' certification",
        )

    # ---------------------------------------------------------
    # 2. Static Analysis — evaluate severities, not just parseability
    # ---------------------------------------------------------
    print(f"\n{BOLD}2. Checking Static Analysis Severities...{RESET}")

    gosec = gate.load_json(REPORTS_DIR / "gosec-raw.json", "GoSec")
    if gosec is not None:
        golang_errors = gosec.get("Golang errors", {})
        gate.check(
            not golang_errors,
            "GoSec: 0 fatal Go AST errors",
            f"GoSec reported fatal Golang errors: {golang_errors}",
        )

        issues = gosec.get("Issues", []) or []
        blocking = [
            i for i in issues
            if str(i.get("severity", "")).upper() == "HIGH"
            and not is_generated(str(i.get("file", "")))
        ]
        excluded = sum(
            1 for i in issues
            if str(i.get("severity", "")).upper() == "HIGH"
            and is_generated(str(i.get("file", "")))
        )
        if excluded:
            log_info(f"GoSec: {excluded} HIGH finding(s) in generated code excluded")
        if blocking:
            gate.fail(
                f"GoSec: {len(blocking)} HIGH severity finding(s) in hand-written code"
            )
            for i in blocking:
                rel = str(i.get("file", "")).split("ArkConstellation/")[-1]
                print(f"         - {i.get('rule_id')} HIGH {rel}:{i.get('line')} "
                      f"{str(i.get('details', ''))[:70]}")
        else:
            log_pass("GoSec: 0 HIGH severity findings in hand-written code")

    semgrep = gate.load_json(REPORTS_DIR / "semgrep-raw.json", "Semgrep")
    if semgrep is not None:
        scan_errors = semgrep.get("errors", []) or []
        gate.check(
            not scan_errors,
            "Semgrep: scan completed with no scan errors",
            f"Semgrep: scan reported {len(scan_errors)} error(s) — incomplete scan",
        )
        blocking = [
            r for r in semgrep.get("results", []) or []
            if str(r.get("extra", {}).get("severity", "")).upper() == "ERROR"
        ]
        if blocking:
            gate.fail(f"Semgrep: {len(blocking)} ERROR severity finding(s)")
            for r in blocking:
                print(f"         - {r.get('check_id')} {r.get('path')}:"
                      f"{r.get('start', {}).get('line')}")
        else:
            log_pass("Semgrep: 0 ERROR severity findings")

    slither = gate.load_json(REPORTS_DIR / "slither-raw.json", "Slither")
    if slither is not None:
        gate.check(
            slither.get("success") is True,
            "Slither: analysis completed successfully",
            f"Slither: analysis did not succeed (error: {slither.get('error')!r})",
        )
        detectors = (slither.get("results") or {}).get("detectors", []) or []
        blocking = [
            d for d in detectors
            if str(d.get("impact", "")).lower() in ("high", "medium")
        ]
        if blocking:
            gate.fail(f"Slither: {len(blocking)} High/Medium impact finding(s)")
            for d in blocking:
                print(f"         - {d.get('impact')} {d.get('check')} "
                      f"{str(d.get('description', ''))[:70]}")
        else:
            log_pass("Slither: 0 High/Medium impact findings")

        # Severity alone is meaningless if the shipping contract was never scanned.
        scanned = set()
        for d in detectors:
            for el in d.get("elements", []) or []:
                sm = el.get("source_mapping") or {}
                name = sm.get("filename_relative") or sm.get("filename_short") or ""
                if name:
                    scanned.add(Path(name).name)
        gate.check(
            GUARDRAIL_CONTRACT in scanned,
            f"Slither: coverage includes {GUARDRAIL_CONTRACT}",
            f"Slither: {GUARDRAIL_CONTRACT} was never scanned "
            f"(covered: {sorted(scanned) or 'nothing'}) — an unscanned contract "
            f"trivially reports zero findings",
        )

    # ---------------------------------------------------------
    # 3. Chaos & Resilience Test Results — assert on raw metrics
    # ---------------------------------------------------------
    print(f"\n{BOLD}3. Checking Chaos & Adversarial Test Results...{RESET}")

    mempool = gate.load_json(REPORTS_DIR / "mempool-flood-results.json", "Mempool Flood")
    if mempool is not None:
        submits = mempool.get("successful_submits", 0)
        mined = mempool.get("mined_txs", 0)
        blocks = mempool.get("blocks_tracked", []) or []
        initial = mempool.get("initial_base_fee", 0)
        peak = mempool.get("peak_base_fee", 0)
        gate.check(
            submits > 0 and mined > 0,
            f"Mempool Flood: {submits} submitted, {mined} mined",
            f"Mempool Flood: {submits} submitted / {mined} mined — no transactions "
            f"reached the chain",
        )
        gate.check(
            len(blocks) > 0,
            f"Mempool Flood: {len(blocks)} block(s) observed under load",
            "Mempool Flood: no blocks observed under load (blocks_tracked empty)",
        )
        gate.check(
            peak > initial,
            f"Mempool Flood: base fee scaled {initial} -> {peak}",
            f"Mempool Flood: base fee never rose under load ({initial} -> {peak})",
        )

    validator = gate.load_json(
        REPORTS_DIR / "validator-failure-results.json", "Validator Failure"
    )
    if validator is not None:
        check_executed(gate, validator, "Validator Failure", "cluster_online")
        gate.check(
            validator.get("fault_injected") is True,
            "Validator Failure: fault actually injected",
            "Validator Failure: fault_injected is false — no outage was ever simulated",
        )
        gate.check(
            validator.get("total_validators", 0) > 0,
            f"Validator Failure: {validator.get('total_validators')} validator(s) observed",
            "Validator Failure: total_validators is 0 — no cluster was observed",
        )
        start_h = validator.get("start_height", 0)
        end_h = validator.get("end_height", 0)
        gate.check(
            end_h > start_h,
            f"Validator Failure: chain advanced {start_h} -> {end_h} during outage",
            f"Validator Failure: no block progress ({start_h} -> {end_h})",
        )
        gate.check(
            validator.get("liveness_maintained") is True
            and validator.get("fast_sync_verified") is True,
            "Validator Failure: liveness maintained and fast-sync verified",
            "Validator Failure: liveness or fast-sync not verified",
        )

    breaker = gate.load_json(
        REPORTS_DIR / "circuit-breaker-results.json", "Circuit Breaker"
    )
    if breaker is not None:
        check_executed(gate, breaker, "Circuit Breaker", "node_online")
        total = breaker.get("total", 0)
        passed = breaker.get("passed", 0)
        gate.check(
            breaker.get("all_passed") is True and total > 0 and passed == total,
            f"Circuit Breaker (x/circuit): {passed}/{total} checks passed",
            f"Circuit Breaker: {passed}/{total} checks passed (require all)",
        )

    rpc = gate.load_json(REPORTS_DIR / "rpc-test-results.json", "JSON-RPC Suite")
    if rpc is not None:
        check_suite_counts(gate, rpc, "JSON-RPC Suite")

    # ---------------------------------------------------------
    # 4. Launch Guardrails & StateDB Invariants
    # ---------------------------------------------------------
    print(f"\n{BOLD}4. Checking Launch Guardrail Contracts & State Recovery...{RESET}")

    guardrail_sol = CONTRACTS_DIR / GUARDRAIL_CONTRACT
    gate.check(
        guardrail_sol.exists() and guardrail_sol.stat().st_size > 0,
        f"{GUARDRAIL_CONTRACT} present in {CONTRACTS_DIR.relative_to(REPO_ROOT)}",
        f"Missing or empty {GUARDRAIL_CONTRACT} contract",
    )

    rate_limit = gate.load_json(REPORTS_DIR / "rate-limit-results.json", "Rate Limiter")
    if rate_limit is not None:
        check_suite_counts(gate, rate_limit, "Rate Limiter & Deposit Caps")

    reboot = gate.load_json(REPORTS_DIR / "hard-reboot-results.json", "Hard Reboot")
    if reboot is not None:
        check_executed(gate, reboot, "Hard Reboot", "cluster_online")
        pre_height = reboot.get("pre_height", 0)
        pre_hash = reboot.get("pre_app_hash", "") or ""
        gate.check(
            pre_height > 0,
            f"Hard Reboot: pre-reboot height captured ({pre_height})",
            f"Hard Reboot: pre_height is {pre_height} — no pre-reboot state was captured",
        )
        gate.check(
            bool(pre_hash.strip()),
            f"Hard Reboot: pre-reboot app hash captured ({pre_hash[:16]}...)",
            "Hard Reboot: pre_app_hash is empty — no state root to compare against",
        )
        gate.check(
            reboot.get("all_passed") is True,
            "Hard Reboot: dual-engine state consistency verified",
            "Hard Reboot: invariant checks failed",
        )

    # ---------------------------------------------------------
    # Final Decision
    # ---------------------------------------------------------
    print(f"\n{BOLD}{CYAN}======================================================{RESET}")
    if gate.errors:
        print(f"{BOLD}{RED}RELEASE GATE FAILED: ENG 3 SIGN-OFF INCOMPLETE{RESET}")
        print(f"{BOLD}{RED}The following requirements were not satisfied:{RESET}")
        for err in gate.errors:
            print(f"  - {RED}{err}{RESET}")
        print(f"\n{YELLOW}Tagging or publishing an RC/Release is strictly blocked.{RESET}")
        print(f"{BOLD}{CYAN}======================================================{RESET}\n")
        return False

    print(f"{BOLD}{GREEN}RELEASE GATE PASSED: ENG 3 SIGN-OFF FULLY CERTIFIED{RESET}")
    print(f"{GREEN}All static analysis, chaos, consensus, and guardrail requirements are met.{RESET}")
    if target_tag:
        print(f"{GREEN}Proceed with tagging: {BOLD}{target_tag}{RESET}")
    print(f"{BOLD}{CYAN}======================================================{RESET}\n")
    return True


def main():
    target_tag = sys.argv[1] if len(sys.argv) > 1 else ""
    passed = verify_gate(target_tag)
    sys.exit(0 if passed else 1)


if __name__ == "__main__":
    main()
