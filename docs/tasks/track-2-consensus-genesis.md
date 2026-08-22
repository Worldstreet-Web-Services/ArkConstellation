# Track 2 — Consensus & Genesis Ops

**Owner:** Eng 2
**Branch:** `track/2-consensus-genesis`
**Workspace:** `networks/`, `scripts/genesis/`

---

## Mission

Manage genesis parameters, validate the permissioned validator set, and guarantee deterministic block production.

---

## Prerequisites

- Download the `mantrachaind` binary from GitHub Actions CI artifacts (no Go installation needed)
- See [CONTRIBUTING.md](../../CONTRIBUTING.md) for download instructions

---

## Day 1 Tasks (00:00 – 24:00)

**Waits for:** `v0.1.0-alpha` binary from Eng 1

### Genesis Parameterization

- [ ] Draft `networks/mainnet/genesis.json` skeleton with:
  - Chain ID (team decision required — must be globally unique)
  - EVM chain ID (team decision required)
  - Native token name, symbol, 18-decimal denomination
  - Initial account allocations and vesting schedules
  - Staking parameters: bonding denom, max validators
  - Unbonding period (recommend: 21 days)
  - Slashing parameters: use SDK defaults
  - Governance: voting period, quorum, timelock on execution

- [ ] Configure CometBFT consensus parameters:
  - `timeout_commit`: target 1–2s block time
  - `timeout_propose`, `timeout_prevote`, `timeout_precommit`: leave at defaults unless block time is wrong

### pystarport Local Devnet

- [ ] Install `pystarport` (from MANTRA's tooling)
- [ ] Create `networks/devnet/pystarport.yaml` defining a 4-node local testnet
- [ ] Confirm devnet starts and produces blocks with `v0.1.0-alpha` binary

### Deliverables

- [ ] `networks/devnet/pystarport.yaml` committed to branch
- [ ] Draft genesis template at `networks/mainnet/genesis-draft.json`

---

## Day 2 Tasks (24:00 – 48:00)

**Waits for:** `v1.0.0-rc1` binary from Eng 1

### Multi-Validator Devnet

- [ ] Deploy 4-node devnet using `v1.0.0-rc1` binary
- [ ] Integrate genesis validator cohort:
  - Each validator must use Sentry Node Architecture (no signing node directly public)
  - Document each validator's node ID, public key, and region

### Remote Signing Setup

- [ ] Verify remote signing pipeline for each validator via Horcrux or Tendermint KMS
- [ ] Confirm no validator is running with raw private keys on the validator box

### Deliverable

- [ ] Stable multi-validator devnet producing blocks — confirm to Eng 3 and Eng 4

---

## Day 3 Tasks (48:00 – 72:00)

### gentx Collection & Validation

- [ ] Collect `gentx` files from all production validators into `scripts/genesis/gentxs/`
- [ ] Validate each `gentx` using:
  ```bash
  ./mantrachaind genesis validate-genesis networks/mainnet/genesis.json
  ```
- [ ] Run gentx collection script: `scripts/genesis/collect-gentxs.sh`

### Final Genesis Assembly

- [ ] Add all validated `gentx` entries into `networks/mainnet/genesis.json`
- [ ] Run final validation pass
- [ ] Calculate canonical SHA-256 hash:
  ```bash
  sha256sum networks/mainnet/genesis.json
  ```
- [ ] Publish hash publicly (in the repo and via agreed communication channel to all validators)

### Deliverable

- [ ] `networks/mainnet/genesis.json` committed and pushed
- [ ] Genesis hash published and confirmed by all validators

---

## Handoff Artifacts

| Artifact | Destination | When |
|----------|-------------|------|
| `networks/devnet/pystarport.yaml` | Eng 3 (chaos tests), Eng 4 (Blockscout) | End of Day 1 |
| Stable devnet confirmation | Eng 3 (load tests), Eng 4 (explorer deployment) | End of Day 2 |
| `networks/mainnet/genesis.json` + hash | Eng 4 (multisig ceremony), all validators | Day 3 |
