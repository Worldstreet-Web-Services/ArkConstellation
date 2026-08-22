# Track 3 — Security, Chaos & Smart Contracts

**Owner:** Eng 3
**Branch:** `track/3-security-chaos`
**Workspace:** `scripts/chaos/`

---

## Mission

Break the chain in testing, audit inherited code paths, and enforce launch deposit caps.

---

## Prerequisites

- Download the `mantrachaind` binary from GitHub Actions CI artifacts (no Go installation needed)
- Install security tooling:
  ```bash
  brew install semgrep
  go install github.com/securego/gosec/v2/cmd/gosec@latest
  pip install slither-analyzer
  ```

---

## Day 1 Tasks (00:00 – 24:00)

> Static analysis does **not** require the binary — start immediately.

### Static Analysis

- [ ] Run Semgrep on all custom and modified code:
  ```bash
  semgrep --config=auto app/ x/
  ```
- [ ] Run GoSec on `app/` and custom modules:
  ```bash
  gosec ./app/... ./x/...
  ```
- [ ] Run Slither on all enabled `cosmos/evm` precompiles (coordinate with Eng 1 on which are enabled)
- [ ] Triage all findings:
  - **CRITICAL / HIGH** → send to Eng 1 immediately
  - **MEDIUM** → document for Day 2 review
  - **LOW / INFO** → document only

### JSON-RPC Test Suite

**Waits for:** `v0.1.0-alpha` binary from Eng 1

- [ ] Write automated test suite in `scripts/chaos/rpc-tests.sh` covering:
  - Contract deployment via `eth_sendRawTransaction`
  - Contract execution
  - `eth_call` read queries
  - `eth_getLogs` event query

### Deliverable

- [ ] Static analysis report committed to `scripts/chaos/reports/day1-static-analysis.md`
- [ ] Triage list sent to Eng 1

---

## Day 2 Tasks (24:00 – 48:00)

**Waits for:** Eng 2's devnet to be live

### RPC Fuzzing & Mempool Stress

- [ ] Execute mempool transaction floods against JSON-RPC endpoints:
  ```bash
  scripts/chaos/mempool-flood.sh <RPC_ENDPOINT>
  ```
- [ ] Verify base fee scales correctly under load (should not peg at zero or ceiling)
- [ ] Record observed TPS, base fee behavior, and any dropped transactions

### Validator Failure Simulation

- [ ] Kill 33% of validator voting power (1 of 3 nodes in devnet)
- [ ] Verify chain continues producing blocks (liveness boundary test)
- [ ] Bring the node back and verify it re-syncs without manual intervention

### Circuit Breaker Test

- [ ] Trigger `x/circuit` message-pause for a specific message type via governance or admin tx
- [ ] Confirm the paused message type is rejected while others continue
- [ ] Unpause and confirm normal operation resumes

### Deliverable

- [ ] Chaos test completion report: `scripts/chaos/reports/day2-chaos-report.md`
- [ ] Signed sign-off confirming the chain is stable enough for mainnet consideration

---

## Day 3 Tasks (48:00 – 72:00)

### Rate-Limit / Deposit-Cap Contracts

- [ ] Write Solidity contract enforcing initial TVL/deposit rate limits
- [ ] Verify contract via unit tests
- [ ] Verify contract deployment and execution on devnet
- [ ] Commit contracts to `scripts/chaos/contracts/`

### State Consistency Check

- [ ] Simulate hard reboot (kill all nodes, restart from persisted state)
- [ ] Verify block height, account balances, and state root are consistent post-restart

### Deliverable

- [ ] Verified smart contracts ready for deployment at genesis
- [ ] Final sign-off document: `scripts/chaos/reports/day3-final-signoff.md`

---

## Handoff Artifacts

| Artifact | Destination | When |
|----------|-------------|------|
| `scripts/chaos/reports/day1-static-analysis.md` | Eng 1 (bug fixes) | End of Day 1 |
| `scripts/chaos/reports/day2-chaos-report.md` + sign-off | All engineers (gate for mainnet) | End of Day 2 |
| `scripts/chaos/contracts/` (rate-limit contracts) | Eng 4 (deploy at genesis) | Day 3 |
