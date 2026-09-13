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

### 3. `x/tax` — ❌ Strip

**Decision:** Strip. Confirmed 2026-08-24 (previously "pending final confirmation" — the code had already stripped this module ahead of the doc; this closes that gap so doc and code agree). Includes removal of `DefaultMcaAddress`, a hardcoded real MANTRA mainnet address that shipped as this module's default and has no place in Ark's codebase.

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

### 8. Native Token Denomination & Symbol — 🔒 `esp` / `espees` / `KASH`

**Decision:**
- **Base unit (smallest integer unit in Cosmos Bank & EVM state):** `esp` (1 Wei equivalent, exponent 0)
- **Intermediate unit (gas pricing):** `espees` = 10⁹ `esp` (1 Gwei equivalent, exponent 9)
- **Display denom (what wallets and MetaMask show):** `KASH` = 10¹⁸ `esp` = 10⁹ `espees` (1 Ether equivalent, exponent 18)

**This satisfies exact Ethereum decimal parity:**
- 1 `esp` = 1 Wei
- 1 `espees` = 1 Gwei
- 1 `KASH` = 1 Ether (18 decimal places)

**Cosmos bank denom metadata to configure in genesis:**
```json
{
  "base": "esp",
  "display": "KASH",
  "name": "KASH",
  "symbol": "KASH",
  "denom_units": [
    { "denom": "esp",   "exponent": 0,  "aliases": ["wei"] },
    { "denom": "espees", "exponent": 9,  "aliases": ["gwei", "nano-kash"] },
    { "denom": "KASH",  "exponent": 18, "aliases": ["ether"] }
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
| Downtime slash | 1% | SDK default — do not modify under time pressure. (Corrected from an earlier "0.01%" here: verified against the vendored `x/slashing/types/params.go`, `DefaultSlashFractionDowntime = 1/100 = 0.01` as a raw decimal fraction, i.e. 1%, not 0.01%.) |
| Downtime jail duration | 10 minutes | SDK default |

---

### 12. Block Time Target — 🔒 1–2 seconds

**Implementation:** Set `timeout_commit = "2s"` in CometBFT config. Eng 2 configures this in the `pystarport` devnet config and genesis. Do not chase sub-second block times on a 3-day build.

---

### 13. Governance Voting Period — 🔒 7 days

**Rationale:** Gives token holders sufficient time to coordinate a response to any proposal, including malicious ones. Shorter periods increase the risk of governance attacks passing before anyone notices.

---

### 14. Governance Timelock — 🔒 Implemented (48 hours)

**Decision:** Mandatory 48-hour timelock on all governance execution, including expedited proposals, for the launch period, with a view to extending post-launch.

**Rationale:** Even a passed governance proposal should not execute instantly. The timelock gives users time to exit positions if they disagree with the outcome of a vote.

**Implementation:** The Ark-local [`x/govtimelock`](../../x/govtimelock/README.md) module wraps the stock `x/gov` end blocker. Passing a tally schedules execution for `block_time + 48h`; execution cannot occur before that timestamp and remains atomic when it becomes due. The schedule is included in genesis export/import, and scheduling and completion emit explicit events for indexers.

---

### 15. Min Governance Deposit — 🔒 Locked

**Decision:** `88,888 KASH` for ordinary proposals (`gov.min_deposit`) and `888,888 KASH` for expedited proposals (`gov.expedited_min_deposit`).

**Rationale:** Locked at the values already in `networks/mainnet/genesis-params.json`. Against the approved 1,000,000,000 KASH total supply, the ordinary deposit is ~0.0089% of supply and the expedited deposit is ~0.0889% of supply — high enough to deter spam, low enough for real participation.

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

### 18. Account Abstraction Model — 🔒 EIP-7702 only

**Decision:** ArkConstellation supports account abstraction through EIP-7702 only. No canonical ERC-4337 EntryPoint is deployed at genesis, at either the v0.6 (`0x5FF137D4b0FDCD49DcA30c7CF57E578a026d2789`) or v0.7 (`0x0000000071727De22E5E9d8BAf0edAc6f37da032`) address, and the chain runs no bundler and no alt-mempool. This records an existing property of the chain as a deliberate choice rather than leaving it as an undocumented default.

**Rationale:** EIP-7702 is native protocol-level delegation shipped as part of Prague/Pectra, and it is already real and tested here rather than aspirational — the ante handlers ([`ante/evm/06_account_verification.go`](https://github.com/Worldstreet-Web-Services/evm/blob/base-genesis/ante/evm/06_account_verification.go), `ante/evm/mono_decorator.go`), txpool validation, the `SetCodeTx` transaction type, and a dedicated system-test suite under `tests/systemtests/accountabstraction/` all exist in the [`evm`](https://github.com/Worldstreet-Web-Services/evm) fork. It gives an ordinary EOA smart-account behaviour — batching, sponsorship, session keys — without a second transaction format, a second mempool, or off-chain infrastructure the chain has to keep alive. ERC-4337 predates EIP-7702 and exists precisely because protocol-level delegation was not available at the time; on a chain where it is available, adding the EntryPoint model means carrying two parallel account-abstraction stacks whose failure modes, fee accounting, and security assumptions differ.

The cost of ERC-4337 here is not a one-line genesis change. The upstream `DefaultPreinstalls` set in `x/vm/types/preinstall.go` contains Create2, Multicall3, Permit2, the Safe singleton factory, and the EIP-2935 history storage contract — it does **not** include an EntryPoint. Deploying one would mean vendoring the EntryPoint's third-party audited bytecode into Ark's genesis — a contract substantially larger than any preinstall currently in that set, where the biggest is Permit2 at roughly 9KB — and taking on responsibility for its correctness at our own canonical address, then running or partnering for a bundler as production infrastructure with its own uptime, DoS surface, and mempool policy. A half-delivered version of this is worse than nothing: an EntryPoint deployed at the canonical address with no bundler behind it makes tooling detect 4337 as supported and then fail at submission, which is a more confusing failure than a clean absence.

**Consequences — what builders should expect:** Wallet SDKs and smart-account tooling that hardcode an EntryPoint address and submit `UserOperation`s will not work on ArkConstellation. This affects a real slice of tooling ported from Ethereum mainnet and other EVM chains, including some social-recovery and session-key products. The supported path for those use cases is EIP-7702 delegation. Because the absence is silent at the RPC level — an `eth_call` to the canonical EntryPoint address returns empty code rather than a meaningful error — this must be stated in public-facing chain documentation so builders learn it from the docs rather than by trial and error.

**Revisiting:** This is a launch-scope decision, not a permanent architectural bar. If concrete demand appears from integrators who cannot migrate to EIP-7702, an EntryPoint can be added post-launch via the same `preinstalls` genesis mechanism used for Create2/Multicall3/Permit2, or by governance-gated deploy. Reopening it should be driven by a named integration that needs it, and should be taken together with the bundler commitment rather than separately — the contract without the infrastructure is not a usable answer.

---

## EVM Precompile Decisions

**What `app/app.go` registers (the only set that *can* be active):** `precompiletypes.DefaultStaticPrecompiles(...)` — p256, bech32, staking, distribution, ICS20, bank, gov, slashing — plus Ark's custom `distrclaim`. The canonical list is `app.StaticPrecompileAddresses` (`app/precompiles.go`); `app/precompiles_test.go` fails if any genesis fixture in the repo activates a different set. Registration alone does nothing: a precompile is only callable if its address is also in `evm.params.active_static_precompiles`, and a call to a registered-but-inactive address *succeeds while doing nothing* (no error, real gas burned).

**Current state:** every registered precompile is active. On `arkdevnet_9000-1` since governance proposal #8 (`networks/devnet/proposals/activate-static-precompiles.json`), and in every genesis source — devnet template, mainnet draft, rehearsal fixture — since PR #39. Before that the list was empty everywhere, not by decision but because `arkd init`'s raw default is empty and the genesis-merge-patch process never set it; the gap was found when IBC-via-EVM-wallet transfers silently produced no packets.

| Precompile | Address | Status | Decision | Notes |
|------------|---------|--------|----------|-------|
| p256 | `0x…0100` | ✅ Active | Enable | Pure secp256r1 signature verification (EIP-7212); no chain-state access. Needed for passkey/WebAuthn-style wallets. |
| bech32 | `0x…0400` | ✅ Active | Enable | Pure address-format conversion; no chain-state access. Prerequisite for any contract that takes a Cosmos address. |
| Staking | `0x…0800` | ✅ Active | Enable | Delegate/undelegate/redelegate from Solidity — staking UX from EVM wallets. |
| Distribution | `0x…0801` | ✅ Active | Enable | Withdraw staking rewards and set withdraw address from Solidity. |
| ICS20 (IBC transfer) | `0x…0802` | ✅ Active | Enable | The concrete product use case that surfaced the gap: IBC transfers initiated from an EVM wallet. IBC is enabled at genesis (#5). |
| Bank | `0x…0804` | ✅ Active | Enable | Native-denom balance queries and transfers from Solidity. |
| Gov | `0x…0805` | ✅ Active | Enable | Governance votes from Solidity. |
| Slashing | `0x…0806` | ✅ Active | Enable | Unjail from Solidity; signing-info queries. |
| `distrclaim` (Ark custom) | `0x…0a01` | ✅ Active | Enable | Claim-and-convert rewards in one call. Ark-owned code (`app/precompiles/distrclaim`), not upstream. |
| Vesting | `0x…0803` | ❌ Not registered | Cannot enable | Listed in upstream `evmtypes.AvailableStaticPrecompiles` but this cosmos/evm version ships no vesting precompile. Activating it would panic on first call (`precompiled contract not stored in memory`); `app.DefaultGenesis()` used to do exactly that until PR #39. |

> **Rule:** Enable only precompiles with a concrete product use case. Each is a Solidity-callable entry point into chain state — additional attack surface.

> **⏳ Still open for mainnet:** the per-precompile security review this rule implies has not been recorded. The set above is the upstream cosmos/evm default that every evmd-based chain ships, but there is no Ark-specific sign-off on the stateful ones (staking, distribution, ICS20, bank, gov, slashing, `distrclaim`). `scripts/chaos/reports/day1-static-analysis.md` §2 does **not** count: its address→module matrix is mislabelled throughout (e.g. `0x…0100` as Staking, `0x…0803` as IBC, `0x…0804` as Wasm, `0x…0a01` as Sanction) and audits a "10 enabled" set that includes the unregistered vesting slot, so whatever it scanned was not the precompiles actually wired into `app.go`. Owner: Eng 1. Until that is done, this section records *what is active and why*, not a completed review.

---

## Token Distribution (Genesis Allocations)

> Approved total supply: **1,000,000,000 KASH** (one billion). All amounts below sum to this total.

| Category | KASH amount | % of total | Vesting / lockup | Notes |
|------------|-------------|-----------|------------------|-------|
| Community / ecosystem | 450,000,000 | 45% | 10% (45M) into `x/gov` community pool at genesis; remaining 405M vests over 8 years in 32 quarterly tranches | Governance-gated; every spend needs a passed proposal |
| Validator & staking incentives | 220,000,000 | 22% | None — streams over 6 years into `x/distribution` | Never held as a liquid wallet; module-to-module distribution |
| Foundation reserve | 160,000,000 | 16% | 15-month cliff, then 45-month linear vest | Longest vesting horizon of any category |
| Team / core contributors | 150,000,000 | 15% | 12-month cliff, then 36-month linear vest (60-month if any single grant >1% of supply) | Size-tiered extension for large grants |
| Initial validator self-delegation bonds | 20,000,000 | 2% | Immediately liquid and self-bonded (2M × 10 validators) | Subject to standard 21-day unbonding if unbonded |
| **Total** | **1,000,000,000** | **100%** | | |

> **Lesson from OM crash:** Avoid concentrated allocations. Publish the full vesting schedule before mainnet genesis — not after.
