# Day 1 Module & Configuration Decisions

**Status:** ⏳ Pending
**Deadline:** End of Day 1 — decisions that slip here delay the entire timeline
**Owner:** All engineers collectively (Eng 1 implements)

---

## How to Use This Document

When a decision is made, update the Status column with ✅ or ❌ and record the decision and rationale. Eng 1 implements every decision marked ✅ Keep or ❌ Strip.

---

## Module Decisions

| Module | Status | Decision | Rationale |
|--------|--------|----------|-----------|
| `x/sanction` | ⏳ | — | Do we need on-chain address blacklisting for compliance? |
| `x/tokenfactory` | ⏳ | — | Do users need to create their own tokens on this chain? Strip if not — it's extra attack surface |
| `x/tax` | ⏳ | — | Read `x/tax/` source code before deciding — unclear what it taxes |
| `cosmossdk.io/x/circuit` | ✅ Keep | Keep enabled | Emergency pause capability — valuable safety valve for a 3-day launch |
| IBC-go | ⏳ | — | Enable at genesis? If yes, light-client setup needs review now |

> **Rule:** If a module is stripped, check that no other module in `app/app.go` has an import or `keeper` dependency on it before removing.

---

## Chain Configuration Decisions

| Decision | Status | Value | Notes |
|----------|--------|-------|-------|
| Cosmos Chain ID | ⏳ | — | Must be globally unique. Format: `<name>-<number>`, e.g. `arkconstellation-1` |
| EVM Chain ID | ⏳ | — | Must not conflict with any existing EVM chain. Check https://chainlist.org |
| Native token name | ⏳ | — | e.g. `uark` (micro-denomination) |
| Native token display symbol | ⏳ | — | e.g. `ARK` |
| Token decimals | ✅ Locked | 18 | Hard constraint of `cosmos/evm` — not configurable |
| Unbonding period | ⏳ | — | Recommend: 21 days (504h). Longer = safer |
| Slashing: double-sign | ✅ Use default | 5% | Do not modify under time pressure |
| Slashing: downtime | ✅ Use default | 0.01% | Do not modify under time pressure |
| Block time target | ⏳ | — | Recommend: 1–2s via CometBFT `timeout_commit` |
| Governance voting period | ⏳ | — | Recommend: 7 days minimum |
| Governance timelock | ⏳ | — | Mandatory — no instant parameter changes post-vote |
| Min deposit for governance | ⏳ | — | Set high enough to prevent spam |
| Initial validator count | ⏳ | — | Recommend: 3–5 identified entities |
| Admin/upgrade multisig | ⏳ | — | Hardware-backed (Ledger/HSM) — Eng 4 runs ceremony Day 3 |

---

## EVM Precompile Decisions

> Fill in after Eng 1 lists all enabled precompiles from `cosmos/evm`.

| Precompile | Status | Decision | Notes |
|------------|--------|----------|-------|
| Staking precompile | ⏳ | — | Allows staking ops from Solidity |
| Bank precompile | ⏳ | — | Allows token transfers from Solidity |
| IBC precompile | ⏳ | — | Only relevant if IBC is enabled at genesis |
| Distribution precompile | ⏳ | — | Allows claiming staking rewards from Solidity |
| Gov precompile | ⏳ | — | Allows governance votes from Solidity |

> **Rule:** Enable only precompiles you have a product reason to support. Each one is additional attack surface.

---

## Token Distribution (Genesis Allocations)

> To be completed by Eng 2 in `networks/mainnet/genesis-draft.json`. Document here for transparency.

| Allocation | Amount | Vesting Schedule | Notes |
|------------|--------|-----------------|-------|
| Team | — | — | Document clearly |
| Foundation reserve | — | — | — |
| Validator incentives | — | — | — |
| Community / ecosystem | — | — | — |
| Initial validator bonds | — | — | Enough to meet min self-delegation |

> **Lesson from OM crash:** Avoid extreme concentration. Publish vesting schedule publicly before mainnet.
