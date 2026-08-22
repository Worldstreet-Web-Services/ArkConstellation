# PRD — Sovereign EVM-Compatible L1 Fork of MANTRA Chain

**Status:** Draft v2 — timeline compressed to 3 days
**Base:** Fork of `MANTRA-Chain/mantrachain` (Cosmos SDK + CometBFT + `cosmos/evm`)
**Timeline:** 3 days to mainnet genesis

---

## 1. Background

MANTRA is a Cosmos SDK chain purpose-built for regulated real-world-asset (RWA) tokenization, running CometBFT consensus with `cosmos/evm` for EVM compatibility. It has custom compliance and token-issuance modules on top of the standard Cosmos SDK module set, and has been through a third-party audit (Hacken). Forking it gives us a codebase that has already made most of the hard integration decisions (EVM wiring, gas token config, precompiles) and has real production mileage — but it also means we inherit a codebase built around a **specific use case (regulated RWA)** that may include modules and assumptions we don't need. This PRD exists to force every one of those inherited decisions to an explicit yes/no before we touch mainnet.

The timeline has been compressed to 3 days. This is not a faster version of the one-week plan — entire activities are cut, not shortened:

- No chaos testing beyond a light smoke test
- No external/outside-eyes review — automated tooling (Slither/Semgrep) plus internal review is the entire security review budget
- No full diff review of Mantra's `cosmos-sdk`/`cosmos/evm` forks against upstream — only a fast scan for high-risk changed files
- No load testing under realistic traffic

**Because that review time is gone, launch caps are no longer a safety margin — they are the primary security control.**

---

## 2. Goal

Ship a sovereign, EVM-compatible L1, forked from `mantrachain`, capable of safely holding real user funds from the moment of genesis, with a validator set, module set, and parameter set that are all explicit product decisions rather than inherited defaults.

---

## 3. Non-Goals (Out of Scope for the 3-Day Build)

- Permissionless validator onboarding
- Custom bridge development beyond IBC
- Full RWA/tokenization tooling
- Full external audit — treat launch as canary-scale only

---

## 4. Architecture

- **Consensus:** CometBFT (unmodified — do not touch consensus code)
- **Execution:** `cosmos/evm` module, inherited via the MANTRA fork
- **State machine:** Cosmos SDK, standard + MANTRA custom modules (evaluated in §6)
- **Language:** Go, entirely
- **Explorer:** Fork MANTRA's Blockscout-based EVM explorer (`mantra-explorer-evm-blockscout`)
- **Local devnet tooling:** `pystarport` (already adapted to this fork)

---

## 5. Dependency Map (from `go.mod`)

| Dependency | Version | Source |
|------------|---------|--------|
| CometBFT | `v0.38.23` | Upstream, unmodified |
| Cosmos SDK | `v0.53.6-v8-mantra-1` | **MANTRA fork** — diff required |
| `cosmos/evm` | `v0.6.2-v8-mantra-1` | **MANTRA fork** — diff required |
| go-ethereum | `v1.16.2-cosmos-1` | Cosmos Labs fork (transitive) |
| IBC-go | `v10.5.1` | Upstream, unmodified |

Toolchain: **Go 1.25.0**

---

## 6. Module Decision Checklist

> **These decisions must be locked by end of Day 1. If they slip, the timeline slips.**

| Module | What it does | Decision |
|--------|-------------|----------|
| `auth`, `bank`, `staking`, `slashing`, `distribution`, `gov`, `upgrade`, `evidence` | Core SDK modules | **Keep unmodified** |
| `evm`, `feemarket` | EVM execution, dynamic fee market | **Keep — configure params** |
| `x/sanction` | Blacklists addresses from transacting | Team decision |
| `x/tokenfactory` | Permissionless token creation | Team decision — strip if not needed |
| `x/tax` | Tax-related operations | Read module source before deciding |
| `cosmossdk.io/x/circuit` | Emergency pause of specific message types | **Recommend: keep enabled** |
| IBC-go | Cross-chain messaging | Decide explicitly — enabling at genesis means light-client review now |

---

## 7. Configuration Decisions (Must Be Finalized Before Day 3)

| Decision | Constraint |
|----------|-----------|
| Chain ID & EVM chain ID | Must be globally unique |
| Native token decimals | **18 decimal places only** — hard constraint of `cosmos/evm` |
| Token distribution & genesis allocations | Document vesting clearly |
| Bonding/unbonding period | Longer = harder for malicious validators to exit |
| Slashing parameters | Do not modify from SDK defaults under time pressure |
| Gas schedule / fee market | Start conservative |
| Block time | 1–2s target |
| Governance voting period & timelock | Mandatory timelock on execution |
| Initial validator set | Small, known, identified entities |
| Admin/upgrade multisig | Hardware-backed signers, documented key ceremony |
| Precompiles enabled | Enable only what's needed — each is attack surface |

---

## 8. Three-Day Timeline

| Day | Focus | Exit Criteria |
|-----|-------|--------------|
| **1** | Module decisions locked, upstream diffs scanned, `app.go` pruned, single node running | MetaMask connects, test contract deploys, decisions locked |
| **2** | Multi-validator testnet via `pystarport`, security tooling pass, key ceremony | Multi-validator testnet stable, monitoring live, key ceremony done |
| **3** | Genesis dress rehearsal, mainnet genesis with conservative caps, bug bounty live | Live chain, capped exposure, active on-call |

---

## 9. Risks

- **Compressed review window** — mitigated only by aggressive caps and fast post-launch iteration
- **`cosmos/evm` is pre-v1** — module not yet marked stable by maintainers
- **Inherited modules built for a different use case** — strip carefully; watch for inter-module dependencies
- **Tokenomics concentration** — avoid concentrated allocations, publish vesting schedules

---

## 10. Launch Readiness Checklist

- [ ] Module keep/strip decisions documented and implemented
- [ ] All config decisions in §7 finalized and reflected in genesis
- [ ] Diff against upstream `cosmos-sdk` reviewed
- [ ] Multi-validator testnet stable
- [ ] Genesis dress rehearsal completed
- [ ] Admin/upgrade multisig live with hardware-backed signers
- [ ] Monitoring and alerting live and tested
- [ ] Incident runbook written and reviewed
- [ ] Bug bounty live
- [ ] Launch caps set and publicly documented
- [ ] Public trust-assumptions statement published
