# Eng 1 (State Machine) — Status

**Branch:** `track/1-state-machine`, targeting `base-genesis`.
**Tag naming:** `ark-v0.1.0-alpha` (and mirror `v0.1.0-alpha`) cut at HEAD.

---

## Day 1

### Ground truth (confirmed independently, not assumed)

- `origin/main` HEAD: `3698b48b` ("feat: change dukong v8.4.0 upgrade height (#683)")
- `origin/base-genesis` HEAD: `026e64c8`
- `ark-v0.1.0-alpha` and `v0.1.0-alpha` tags cut and validated.

### Key Decisions & Module Architecture

1. **`app/genesis.go`'s `NewDefaultGenesisState()` removed**: Confirmed dead from the CLI `init` path. Genesis-time denom/erc20/`bank.denom_metadata` customization is handled by the deployment-time genesis-merge-patch process (`networks/devnet/README.md` / `networks/mainnet/RUNBOOK.md`).
2. **Module Pruning (`x/tax`, `x/tokenfactory`)**: Fully removed from `app/app.go`, `maccPerms`, keeper structs, store keys, IBC-transfer middleware, module manager, begin/end/init/export ordering, wasm capability flags (`app/wasm.go`), and Stargate query whitelist (`app/queries/queries.go`). Deleted `x/tax`, `x/tokenfactory`, proto files, testutil helpers, and e2e test files.
3. **Module Retention (`x/sanction`)**: Retained per Decision #1 for compliance and operational guardrails. Keeper, store keys, genesis, and module manager wiring active in `app/app.go`. `BlacklistCheckDecorator` wired in `app/ante/cosmos.go` and `EVMBlacklistCheckDecorator` wired in `app/ante/evm.go`. Protobuf source definitions present in `proto/mantrachain/sanction/`, address normalization for both Bech32 `ark1...` and `0x...` hex inputs supported, unit tests for both Cosmos and EVM decorators passing, and e2e tests restored in `tests/e2e/e2e_sanction_test.go`.
4. **Circuit Breaker (`cosmossdk.io/x/circuit`)**: Wired in `app/app.go`, `SetCircuitBreaker(&app.CircuitKeeper)`, wasm message router, and `circuitante.NewCircuitBreakerDecorator` across **both** Cosmos ante path (`app/ante/cosmos.go`) and EVM JSON-RPC ante path (`app/ante/evm.go`). Verified live via local node testing (`MsgSend` disable -> reject -> reset -> allow). Proof log committed in `docs/proof/circuit-breaker-verification.log`.
5. **Identity & Denom Rebrand**:
   - **Bech32 Prefix**: `mantra` → `ark` (`ark`, `arkpub`, `arkvaloper`, `arkvaloperpub`, `arkvalcons`, `arkvalconspub`).
   - **Denom Hierarchy**: Base unit `esp` (0 decimals, exponent 0, 1 wei equivalent, staking/bonding), intermediate `espees` ($10^9$ `esp`, 1 gwei equivalent), display/EVM unit `KASH` ($10^{18}$ `esp`, 1 ether equivalent) per locked Decision #8.
   - `EVMCoinInfo`: `Denom: "esp"`, `ExtendedDenom: "esp"`, `DisplayDenom: "KASH"`, `Decimals: 18`.
   - `MinGasPrices`: Default set to `"0esp"`.
   - `EVMChainIDMap`: Mapped `"arkconstellation-1": 11199` (locked Decisions #6 & #7) and `"arkdevnet_9000-1": 9000`.
   - `IBC Unwrap Memo`: `{"ark":{"unwrap":true}}`.
6. **Fork Audits Completed**:
   - `MANTRA-Chain/cosmos-sdk@v0.53.6-v8-mantra-1` vs upstream `v0.53.6` documented in `docs/proof/fork-audit-cosmos-sdk.md`.
   - `MANTRA-Chain/evm@v0.6.2-v8-mantra-1` vs upstream `v0.6.2` documented in `docs/proof/fork-audit-cosmos-evm.md` (confirming critical ICS20 reentrancy guard).

### Smoke Test Verification

Single local node booted from HEAD:
1. `genesis validate-genesis` passes.
2. Node boots, produces blocks.
3. Real signed `MsgSend` (`1000000000000000000esp`) broadcasts and confirms receipt.
4. Circuit breaker disable/reject/reset/allow cycle verified live (`docs/proof/circuit-breaker-verification.log`).

---

## Day 2 (partial)

### Precompile Audit (`x/vm/types.AvailableStaticPrecompiles` + Ark custom `distrclaim`)

| Precompile | Address | Recommendation | Reason |
|---|---|---|---|
| Bech32 | `...0400` | **Enable** | Directly supports the dual bech32/0x address model; zero state access. |
| Bank | `...0804` | **Enable** | Native-side balance/transfer access without ERC20 wrapper. |
| Staking | `...0800` | **Enable, flag for Eng 3** | Direct-delegate UX, staking state transitions from EVM. |
| Distribution | `...0801` | **Enable** | Reward claiming/withdrawal from EVM. |
| ICS20 | `...0802` | **Enable, flag for Eng 3** | EVM×IBC interop (fork includes critical reentrancy guard). |
| Gov | `...0805` | **Enable** | On-chain governance voting from EVM. |
| Vesting | `...0803` | **Enable** | Vesting account queries; low risk. |
| Slashing | `...0806` | **Enable** | Query + self-service unjail; low risk. |
| P256 | `...0100` | **Enable** | WebAuthn/passkey signature verification (RIP-7212). |
| `distrclaim` | `...0a01` | **Enable** | Narrow, single-purpose reward claiming + ERC20 wrapper conversion. |

### `skip-mev/feemarket` Dynamic Fee Configuration
- **Decision**: Keep defaults (`base_fee_change_denominator: 8`, `elasticity_multiplier: 2`, `min_gas_price: 0`, `base_fee: 10^9` atto-units ≈ 1 gwei) for Paymaster/gasless transaction compatibility.

  > ⚠️ **The live devnet does not match this.** `/cosmos/evm/feemarket/v1/params` on
  > `arkdevnet_9000-1` returns `min_gas_price: 1000000000`, not `0`, as of 2026-09-04.
  > A non-zero chain-wide floor means genuinely zero-fee sponsored transactions are
  > not possible — a paymaster still pays 1 gwei per gas unit. That is directly
  > relevant to #28 (ERC-4337 Paymaster). Either the decision changed and this line
  > is stale, or genesis did not apply it. **Left as-is deliberately rather than
  > edited to match the chain** — this records an intent, and whoever owns it should
  > decide which of the two is wrong.

### Eng 3 Dependency & Next Tags
- Blocked on Eng 3 (Security/Chaos) reporting before tagging `ark-v1.0.0-rc1`.
