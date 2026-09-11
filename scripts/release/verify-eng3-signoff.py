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

import re
import sys
import json
import subprocess
from pathlib import Path
from typing import Optional

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

# Paths whose changes require fresh Eng 3 evidence: the AnteHandler/circuit
# breaker wiring under test, the guardrail contracts, the harnesses
# themselves (both the Python drivers and any shell wrappers), and the
# release/gate machinery that produces or enforces this evidence. If any of
# these changed more recently (in commit-graph terms) than the committed
# evidence, the evidence is stale and must not certify the release.
EVIDENCE_PATHS = ("scripts/chaos/reports",)
SOURCE_PATHS = (
    "app/ante",
    "scripts/chaos/contracts",
    "scripts/chaos/*.py",
    "scripts/chaos/*.sh",
    "scripts/release",
    ".github/workflows/release.yml",
)


def git_output(args) -> Optional[str]:
    try:
        res = subprocess.run(
            ["git", *args], cwd=REPO_ROOT, capture_output=True, text=True, timeout=15
        )
    except Exception:
        return None
    if res.returncode != 0:
        return None
    out = res.stdout.strip()
    return out or None


def resolve_commit(ref: str) -> Optional[str]:
    return git_output(["rev-parse", "--verify", "--quiet", f"{ref}^{{commit}}"])


def last_commit_touching(commit: str, paths: tuple) -> Optional[str]:
    return git_output(["log", "-1", "--format=%H", commit, "--", *paths])


def is_ancestor(older: str, newer: str) -> bool:
    if older == newer:
        return True
    res = subprocess.run(
        ["git", "merge-base", "--is-ancestor", older, newer],
        cwd=REPO_ROOT, capture_output=True, timeout=15,
    )
    return res.returncode == 0


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


# Matches Slither's own filename-bearing keys (filename_relative,
# filename_short, filename_absolute, filename_used, a "filenames" manifest
# map, etc.) so a .sol path is only treated as coverage evidence when it
# names the file a finding/unit is *about* — never when it merely appears
# inside free-text like a description, an import statement, or a compiler
# diagnostic message that happens to mention another contract's file.
FILENAME_KEY_RE = re.compile(r"filename", re.IGNORECASE)


def collect_sol_filenames(obj) -> set:
    """
    Recursively collect every ``*.sol`` basename held under a filename-like
    key anywhere in a parsed Slither JSON document.

    Restricting coverage detection to `detectors[].elements[]` (findings
    only) makes a contract that Slither genuinely analyzed but which
    triggered zero findings of any severity indistinguishable from one
    that was never scanned. Slither's `--json-types` can include a
    `compilation_units` section (or other sections) that name every
    analyzed source file even with no findings; walking the whole document
    picks those up too, whatever shape they take, without this script
    having to hardcode a specific Slither schema version — but only values
    reached through a filename-like key are trusted, so a stray mention in
    a description string can't manufacture false coverage.
    """
    found = set()

    def walk(node, key_hint: str = ""):
        if isinstance(node, dict):
            for k, v in node.items():
                walk(v, k)
        elif isinstance(node, list):
            for v in node:
                walk(v, key_hint)
        elif isinstance(node, str) and node.endswith(".sol") and FILENAME_KEY_RE.search(key_hint):
            found.add(Path(node).name)

    walk(obj)
    return found


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
    # 0. Verify Eng 3 evidence is bound to the commit being released
    # ---------------------------------------------------------
    # A checked-in evidence tree satisfies every downstream check even when
    # it was captured against an earlier commit — nothing above ties the
    # report contents to what is actually being tagged. Close that gap by
    # requiring the evidence directory to have last changed at or after the
    # last change to the code/contracts it certifies, as of the commit
    # being released.
    print(f"{BOLD}0. Verifying Eng 3 Evidence Is Bound To The Release Commit...{RESET}")
    target_commit = resolve_commit(target_tag) if target_tag else None
    if target_commit is None:
        # tag-release.sh runs this before the tag exists, and a plain local
        # invocation has no tag at all — HEAD is the commit that would be
        # tagged in both cases.
        target_commit = resolve_commit("HEAD")

    if target_commit is None:
        gate.fail(
            "Commit binding: could not resolve a git commit for "
            f"{target_tag or 'HEAD'} — is this a full git checkout "
            "(fetch-depth: 0, fetch-tags: true)?"
        )
    else:
        evidence_commit = last_commit_touching(target_commit, EVIDENCE_PATHS)
        source_commit = last_commit_touching(target_commit, SOURCE_PATHS)
        if evidence_commit is None:
            gate.fail(
                f"Commit binding: {EVIDENCE_PATHS[0]} has no history as of "
                f"{target_commit[:12]} — no evidence was ever committed for this ref"
            )
        elif source_commit is not None and not is_ancestor(source_commit, evidence_commit):
            gate.fail(
                f"Commit binding: evidence last updated at {evidence_commit[:12]} but "
                f"security-relevant source last changed at {source_commit[:12]} (both "
                f"as of {target_commit[:12]}) — evidence predates the code it must "
                f"certify, re-run the Eng 3 harnesses against this commit"
            )
        else:
            log_pass(
                f"Commit binding: evidence ({evidence_commit[:12]}) covers the "
                f"latest relevant source change ({source_commit[:12] if source_commit else 'n/a'}) "
                f"as of {target_commit[:12]}"
            )

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
        scanned = collect_sol_filenames(slither.get("results") or {})
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
        # Recompute liveness/fast-sync from the raw per-cycle samples rather
        # than trusting the harness's self-reported booleans — same
        # principle already applied to Mempool Flood above.
        fault_blocks = validator.get("fault_blocks", []) or []
        gate.check(
            len(fault_blocks) > 0,
            f"Validator Failure: {len(fault_blocks)} block(s) independently observed "
            f"committing during the simulated outage",
            "Validator Failure: no blocks observed committing during the outage "
            "(fault_blocks empty) — liveness not proven by raw data",
        )
        fault_window_last_height = validator.get("fault_window_last_height", 0)
        gate.check(
            fault_window_last_height > 0 and end_h >= fault_window_last_height,
            f"Validator Failure: recovered height {end_h} reached the outage-window "
            f"height {fault_window_last_height}",
            f"Validator Failure: recovered height {end_h} never reached the "
            f"outage-window height {fault_window_last_height} — fast-sync not proven "
            f"by raw data",
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
