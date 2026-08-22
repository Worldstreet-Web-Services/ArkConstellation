# Day 1 Module & Configuration Decisions

**Status:** 🟡 Partially Locked
**Deadline:** End of Day 1 — decisions that slip here delay the entire timeline
**Owner:** All engineers collectively (Eng 1 implements)

---

## How to Use This Document

When a decision is made, update the Status column with ✅ Keep, ❌ Strip, or 🔒 Locked and record the rationale. Eng 1 implements every decided item.

---

## Module Decisions

### 1. `x/sanction` — ✅ Keep

**Decision:** Keep the sanction module.

**Rationale:** Retained for compliance and operational safety — the ability to blacklist addresses on-chain is a useful control, especially during a 3-day launch where the review window is compressed and on-chain guardrails matter more than usual.

**Note for Eng 1:** Ensure the admin key controlling `x/sanction` is the same multisig as the upgrade authority — it must not be a single hot key.

---

### 2. `x/tokenfactory` — ❌ Strip

**Decision:** Strip the token factory module.

**Rationale:** The chain is designed around a single native token (`KASH`). Enabling permissionless native token creation introduces unnecessary attack surface, token spoofing/impersonation risks, and extra maintenance overhead without a valid product use case.

**Action for Eng 1:**
- Remove `x/tokenfactory` registration, keeper wiring, and store keys from `app/app.go`.
- Check and clean up any module dependencies or CLI commands referencing tokenfactory.

---

### 3. `x/tax` — ❌ Strip (Recommended)

**Decision:** Strip. Pending final confirmation.

**Rationale:** Based on reading [`x/tax/readme.md`](../../x/tax/readme.md), this module is built specifically around **MCA (Mantra Chain Authority)** — MANTRA's own governance/authority body. The `BeginBlocker` allocates tax to MCA-designated addresses every block. ArkConstellation has no MCA. Keeping this module means:
- Every block, tax allocation logic runs against an authority that doesn't exist on this chain
- The module is tightly coupled to MANTRA's specific governance structure, not a generic tax system
- It adds a `BeginBlocker` (runs every block) that would need to be reconfigured or it allocates to a dead address

**Action for Eng 1:** Before stripping, check `app/app.go` for any modules that import `x/tax`'s keeper. Remove any such references before removing the module registration.

---

### 4. `cosmossdk.io/x/circuit` — ✅ Keep

**Decision:** Keep enabled.

**Rationale:** Emergency pause capability. Allows specific message types to be paused without a full chain halt. Critical safety valve for a 3-day launch with a compressed review window.

---

### 5. IBC-go — ✅ Enable at Genesis

**Decision:** IBC enabled from Day 1.

**What Eng 1 must do:**
- Confirm IBC module is registered in `app/app.go` with `IBCKeeper`
- Confirm `transfer` module (ICS-20) is included — this is what enables token transfers over IBC
- No custom light client setup is needed for genesis itself; light clients are created per-connection when channels are opened post-launch

**What Eng 2 must include in genesis.json:**
```json
"ibc": {
  "client_genesis": { "clients": [], "clients_consensus": [], "create_localhost": false },
  "connection_genesis": { "connections": [], "client_connection_paths": [] },
  "channel_genesis": { "channels": [], "acknowledgements": [], "commitments": [], "receipts": [], "send_sequences": [], "recv_sequences": [], "ack_sequences": [] }
},
"transfer": {
  "port_id": "transfer",
  "denom_traces": [],
  "params": { "send_enabled": true, "receive_enabled": true }
}
```

**Security considerations for IBC at genesis:**
- IBC itself does not increase the attack surface of the chain's core state machine — light clients are created only when a relayer opens a connection, not at genesis
- The `transfer` module's `send_enabled` and `receive_enabled` params can be toggled via governance — if a vulnerability is found in IBC post-launch, these can be disabled without a chain upgrade
- Do **not** enable `create_localhost` — it's a development utility with no production use
- Ensure `x/circuit` can pause `MsgTransfer` (ICS-20 transfer messages) as a kill switch if needed
- IBC does not require any external validator key configuration — relayers are permissionless third parties

---

## Chain Configuration Decisions

### 6. Cosmos Chain ID — 🔒 `arkconstellation-1`

**Rationale:** Follows the standard Cosmos chain ID format (`name-number`). The `1` suffix indicates the first instance of this chain. **This cannot be changed after genesis without a full chain migration.**

---

### 7. EVM Chain ID — 🔒 `11199`

**Decision:** Set to numeric `11199`.

**Rationale:** Globally unique integer identifier for the EVM execution environment used by MetaMask, web3 tooling, and transaction signing (EIP-155 replay protection). Distinct from other network IDs.

---

### 8. Native Token Denomination & Symbol — 🔒 `aesp` / `esp` / `KASH`

**Decision:**
- **Base EVM denom (smallest unit):** `aesp` (atto-esp) — this is what `cosmos/evm` operates on internally
- **Intermediate denom:** `esp` = 10¹² `aesp`
- **Display denom (what wallets show):** `KASH` = 10¹⁸ `aesp` = **1,000,000 esp**

**This satisfies your requirement:** `1,000,000 esp = 1 KASH` ✅
**This satisfies the EVM constraint:** 18 decimal places ✅

**Why `aesp` must exist:** `cosmos/evm` has a hard constraint that the gas token uses 18 decimal places. This is baked into how EVM gas calculations convert between Cosmos token amounts and EVM Wei values. It cannot be changed without forking the EVM module itself.

**How users experience this:**
- MetaMask and block explorers will show `KASH` as the balance
- Sending transactions will be denominated in `KASH`
- `esp` can be exposed as a sub-unit (like gwei on Ethereum) for developer tooling
- `aesp` is invisible to end users — it only appears in raw Cosmos transaction data

**Cosmos bank denom metadata to configure in genesis:**
```json
{
  "base": "aesp",
  "display": "KASH",
  "name": "KASH",
  "symbol": "KASH",
  "denom_units": [
    { "denom": "aesp", "exponent": 0, "aliases": ["atto-esp"] },
    { "denom": "esp",  "exponent": 12, "aliases": [] },
    { "denom": "KASH", "exponent": 18, "aliases": [] }
  ]
}
```

---

### 9. Token Decimals — 🔒 18

**Hard constraint of `cosmos/evm`.** Not configurable.

---

### 10. Unbonding Period — 🔒 21 days (504h)

**Rationale:** Longer unbonding period means a malicious validator has to wait 21 days after unstaking before they can exit with their stake — giving slashing time to catch up. Standard across major Cosmos chains (Cosmos Hub, Osmosis, etc.).

---

### 11. Slashing Parameters — 🔒 SDK Defaults

| Parameter | Value | Notes |
|-----------|-------|-------|
| Double-sign slash | 5% | SDK default — do not modify under time pressure |
| Downtime slash | 0.01% | SDK default — do not modify under time pressure |
| Downtime jail duration | 10 minutes | SDK default |

---

### 12. Block Time Target — 🔒 1–2 seconds

**Implementation:** Set `timeout_commit = "2s"` in CometBFT config. Eng 2 configures this in the `pystarport` devnet config and genesis. Do not chase sub-second block times on a 3-day build.

---

### 13. Governance Voting Period — 🔒 7 days

**Rationale:** Gives token holders sufficient time to coordinate a response to any proposal, including malicious ones. Shorter periods increase the risk of governance attacks passing before anyone notices.

---

### 14. Governance Timelock — 🔒 Required (duration TBD)

**Decision:** Mandatory timelock on all governance execution. Minimum recommended: 48 hours for the launch period, with a view to extending post-launch.

**Rationale:** Even a passed governance proposal should not execute instantly. The timelock gives users time to exit positions if they disagree with the outcome of a vote.

---

### 15. Min Governance Deposit — ⏳ Pending

**Blocked by:** Total supply not yet defined. Once total supply is set by Eng 2 during genesis parameterization, set this proportionally — high enough to deter spam proposals, low enough that non-whale participants can still submit.

---

### 16. Initial Validator Count — 🔒 10

**Rationale:** CometBFT (Tendermint BFT) consensus requires a 2/3 majority to produce blocks. 10 validators is a practical starting size for a permissioned set — large enough to provide meaningful geographic and infrastructure diversity, small enough for coordinated key ceremonies and rapid incident response.

**Requirements for each validator:**
- Real, identified entity (not anonymous)
- Sentry node architecture (signing node never publicly reachable)
- Remote signing via Horcrux or Tendermint KMS
- Geographically distinct from at least 2 other validators
- Different cloud provider from at least 2 other validators

---

### 17. Admin/Upgrade Multisig — ⏳ Pending

**Decision:** To be defined. Eng 4 executes the key ceremony on Day 3.

**Minimum requirements when decided:**
- Hardware-backed signers (Ledger or HSM) — no software keys
- Threshold signature (e.g. `3-of-5` or `2-of-3`)
- Geographically distributed key holders
- Documented key ceremony procedure in `ops/runbooks/key-ceremony.md`

---

## EVM Precompile Decisions

> To be filled in by Eng 1 after inspecting `app/app.go` for registered precompiles.

| Precompile | Status | Decision | Notes |
|------------|--------|----------|-------|
| Staking precompile | ⏳ | — | Allows staking ops from Solidity |
| Bank precompile | ⏳ | — | Allows token transfers from Solidity |
| IBC precompile | ⏳ | — | Relevant — IBC is enabled at genesis |
| Distribution precompile | ⏳ | — | Allows claiming staking rewards from Solidity |
| Gov precompile | ⏳ | — | Allows governance votes from Solidity |

> **Rule:** Enable only precompiles with a concrete product use case. Each is a Solidity-callable entry point into chain state — additional attack surface.

---

## Token Distribution (Genesis Allocations)

> To be completed by Eng 2 in `networks/mainnet/genesis-draft.json`. **Blocked pending total supply decision.**

| Allocation | Amount | Vesting Schedule | Notes |
|------------|--------|-----------------|-------|
| Team | — | — | Document vesting clearly and publish publicly |
| Foundation reserve | — | — | — |
| Validator incentives | — | — | — |
| Community / ecosystem | — | — | — |
| Initial validator bonds | — | — | Must meet min self-delegation threshold |

> **Lesson from OM crash:** Avoid concentrated allocations. Publish the full vesting schedule before mainnet genesis — not after.

