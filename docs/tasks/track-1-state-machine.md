# Track 1 — State Machine Lead

**Owner:** Eng 1
**Branch:** `track/1-state-machine`
**Workspace:** `app/`, `cmd/`, `x/`

---

## Mission

Deliver a minimal, compiling, and secure blockchain binary stripped of all non-essential inherited attack surface.

---

## Day 1 Tasks (00:00 – 24:00)

### Upstream Diffing

- [ ] Diff `MANTRA-Chain/cosmos-sdk` (`v0.53.6-v8-mantra-1`) against upstream `cosmos-sdk v0.53.x`
  ```bash
  # Add upstream cosmos-sdk as a reference remote
  git remote add cosmos-sdk-upstream https://github.com/cosmos/cosmos-sdk.git
  git fetch cosmos-sdk-upstream v0.53.6
  # Diff the MANTRA fork files against upstream tag
  ```
- [ ] Diff `MANTRA-Chain/evm` (`v0.6.2-v8-mantra-1`) against upstream `cosmos/evm`
- [ ] Flag any changes touching:
  - Consensus-adjacent code
  - Money-movement paths (`bank`, `transfer`, `evm` execution)
  - Signature verification
- [ ] Document findings → `docs/decisions/upstream-diff-findings.md`

### Module Pruning (in `app/app.go`)

- [ ] Decision locked: **`x/sanction`** — keep or strip?
- [ ] Decision locked: **`x/tokenfactory`** — keep or strip?
- [ ] Decision locked: **`x/tax`** — read module source at `x/tax/`, then decide
- [ ] Strip decided modules from `app/app.go` module manager
- [ ] Retain and confirm `cosmossdk.io/x/circuit` is enabled
- [ ] Confirm no other modules implicitly depend on stripped modules (check imports)

### 18-Decimal Gas Token

- [ ] Locate `cosmos/evm` denomination config in `app/app.go` or `app/params/`
- [ ] Confirm native token is configured with **18 decimal places**
- [ ] Confirm `evmtypes.DefaultEVMDenom` matches your chain's native token name

### Smoke Test

- [ ] Run a single local node: `./build/mantrachaind start`
- [ ] Connect MetaMask to `http://localhost:8545` (EVM JSON-RPC)
- [ ] Deploy a test contract via Remix or `cast`

### Release

- [ ] Tag `v0.1.0-alpha`
  ```bash
  git tag v0.1.0-alpha
  git push origin v0.1.0-alpha
  ```
- [ ] Confirm CI builds the binary and uploads artifact ✅
- [ ] Notify Eng 2 and Eng 3 — binary available for download from GitHub Actions

---

## Day 2 Tasks (24:00 – 48:00)

### Precompile Audit

- [ ] List all enabled `cosmos/evm` precompiles (typically in `app/app.go` or `app/evm.go`)
- [ ] For each precompile, document: what it does, whether it's needed, decision
- [ ] Disable all non-essential precompiles
- [ ] Document decisions → `docs/decisions/precompile-decisions.md`

### Fee Market Configuration

- [ ] Configure `skip-mev/feemarket` parameters:
  - `MinBaseFee` — start conservative
  - `MaxBlockUtilization` — leave at default unless there's a specific reason
  - `FeeDenom` — must match 18-decimal native token
- [ ] Set EVM gas schedule (use default unless Eng 3 surfaces a specific issue)

### Bug Fixes from Eng 3

- [ ] Triage static analysis report from Eng 3
- [ ] Fix any findings rated HIGH or CRITICAL before `v1.0.0-rc1`

### Release

- [ ] Tag `v1.0.0-rc1`
  ```bash
  git tag v1.0.0-rc1
  git push origin v1.0.0-rc1
  ```
- [ ] Notify Eng 2 — use `v1.0.0-rc1` for multi-validator devnet

---

## Day 3 Tasks (48:00 – 72:00)

### Build Verification

- [ ] Produce a reproducible build: build the binary on two separate machines, compare SHA-256 hashes
  ```bash
  sha256sum build/mantrachaind
  ```
- [ ] Confirm `build/sha256sum.txt` matches CI artifact checksum

### Final Release

- [ ] Confirm all Day 2 precompile and fee market decisions are committed
- [ ] Confirm `v1.0.0-rc1` has been stable on Eng 2's devnet (no consensus failures)
- [ ] Tag `v1.0.0`
  ```bash
  git tag v1.0.0
  git push origin v1.0.0
  ```
- [ ] Publish release notes listing:
  - Modules stripped
  - Precompiles enabled/disabled
  - SDK diff summary
  - Reproducible build hash

---

## Handoff Artifacts

| Artifact | Destination | When |
|----------|-------------|------|
| `v0.1.0-alpha` binary (CI artifact) | Eng 2 devnet, Eng 3 static analysis | End of Day 1 |
| `docs/decisions/upstream-diff-findings.md` | All engineers | End of Day 1 |
| `v1.0.0-rc1` binary (CI artifact) | Eng 2 multi-validator devnet | End of Day 2 |
| `v1.0.0` binary (CI artifact) | Eng 4 mainnet nodes | Day 3 morning |

---

## Key Files to Know

| File | Purpose |
|------|---------|
| [`app/app.go`](../../app/app.go) | Module wiring — primary file for pruning decisions |
| [`go.mod`](../../go.mod) | Dependency versions and `replace` directives |
| [`Makefile`](../../Makefile) | Build targets — use `make build` |
| [`cmd/mantrachaind/main.go`](../../cmd/mantrachaind/main.go) | Binary entrypoint |
