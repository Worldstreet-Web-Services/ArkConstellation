# DECISION: Activate 9 EVM static precompiles at mainnet genesis — not 10

**Status:** 🟢 DECIDED 2026-09-11. Genesis sources
(`networks/mainnet/genesis-params.json`, `genesis-DRAFT.json`,
`networks/devnet/genesis-template.json`, `networks/devnet/pystarport.json`,
`scripts/genesis/rehearsal/base-genesis.json`) now carry the 9-address list, and
`app/precompiles.go` is the single source it is checked against. **Eng 3 sign-off on
Staking, ICS20 and `distrclaim` is still outstanding** — see "Sign-off" below. The
running `arkdevnet_9000-1` is unchanged until
`networks/devnet/proposals/evm-active-precompiles.json` is submitted; confirm with
`arkd query evm params` rather than assuming.

**Author:** Engineering, 2026-09-11. Closes the decision left open at
`docs/decisions/module-and-config-decisions.md`'s "EVM Precompile Decisions"
section and tracked as issue #37.

**Scope:** Which EVM static precompiles are active in `evm.params.active_static_precompiles`
at genesis. It does **not** cover `evm.preinstalls` (empty — issue #21),
ERC20 dynamic precompiles, or the Prague precompile set (`0x01`–`0x11`), which
`IsAvailableStaticPrecompile` hardcodes as always-on and which cannot be disabled
by this param at all.

---

## Decision

> ## Activate every static precompile the binary actually registers — **9 addresses**. Exclude `0x…0803`.

```json
"active_static_precompiles": [
  "0x0000000000000000000000000000000000000100",
  "0x0000000000000000000000000000000000000400",
  "0x0000000000000000000000000000000000000800",
  "0x0000000000000000000000000000000000000801",
  "0x0000000000000000000000000000000000000802",
  "0x0000000000000000000000000000000000000804",
  "0x0000000000000000000000000000000000000805",
  "0x0000000000000000000000000000000000000806",
  "0x0000000000000000000000000000000000000a01"
]
```

| Address | Precompile | Exposes | Risk posture |
|---|---|---|---|
| `0x…0100` | P256 | secp256r1 verification (RIP-7212) | Stateless pure compute. WebAuthn/passkey support. |
| `0x…0400` | Bech32 | bech32 ↔ hex conversion | Stateless pure compute. Load-bearing for the dual-address model. |
| `0x…0800` | Staking | `x/staking` | **Highest-risk entry.** State transitions on the bonded set from Solidity. Eng 3 chaos coverage required. |
| `0x…0801` | Distribution | `x/distribution` | Reward/commission withdrawal. Touches the same accounting as Staking. |
| `0x…0802` | ICS20 | ibc-go transfer | **The one with an exploit history** — the Saga incident went through an ICS20 precompile. The fork carries the nested-forwarding reentrancy guard (`docs/proof/fork-audit-cosmos-evm.md`, GHSA-54gx-3cgr-7mfm). Eng 3 reentrancy coverage required. |
| `0x…0804` | Bank | `x/bank` | Native balance reads and sends without an ERC20 wrapper. |
| `0x…0805` | Gov | `x/gov` | Voting and deposits from contracts. |
| `0x…0806` | Slashing | `x/slashing` | Signing-info queries and self-service unjail. |
| `0x…0a01` | `distrclaim` | claim + convert, Ark-original | **Not upstream.** Narrow single purpose, but has not had the fork-audit scrutiny the other nine got. Eng 3 review required. |
| ~~`0x…0803`~~ | ~~Vesting~~ | — | **Excluded. Does not exist.** See "The constraint". |

### Why the full working set rather than a narrow launch set

The conservative instinct here is wrong, and it is worth saying why explicitly,
because "enable less" reads as the safe default.

`active_static_precompiles` is a field of `cosmos.evm.vm.v1.Params`. The authority
is the gov module account (`app/app.go`), and `app/proposals_whitelisting.go`
whitelists every message type, so **a governance proposal can add or remove any
address already compiled into the binary, with no coordinated upgrade, taking
effect on the next block.** Narrowing the set later is cheap.

The reverse is not true in any useful sense. A chain that launches with precompiles
inert is a chain where no Solidity developer can delegate, claim a reward, or move
an IBC asset — and they find out by writing a contract that silently fails. The
cost of launching narrow is paid immediately by every integrator; the cost of
launching wide is recoverable by a 7-day governance vote.

So the asymmetry points the other way from the usual attack-surface argument. What
attack surface *does* buy here is a gate, not a subset: Staking, ICS20 and
`distrclaim` need Eng 3 to actually exercise them before `v1.0.0`, and that is a
blocking requirement recorded below rather than a reason to ship them disabled.

### What this costs

Nine Solidity-callable entry points into Cosmos state from block 1, three of which
are not yet signed off. If Eng 3's review does not complete before the binary is
frozen, the honest move is to **deactivate the unreviewed addresses by editing the
genesis list** — not to launch and review afterwards, and not to quietly treat the
recommendation as the sign-off. That is the failure mode this whole document exists
to prevent; see "How this was missed" below.

## The constraint

**`0x…0803` is advertised but not implemented, and activating it panics the node.**

This is the finding that changed the answer from 10 to 9, and it is not an Ark bug —
it is inherited, and the fork's own reference app has it too.

- `x/vm/types/precompiles.go` exports `VestingPrecompileAddress = 0x…0803` and
  includes it in `AvailableStaticPrecompiles` — a leftover from the evmos lineage.
- The fork ships **no vesting precompile**. `precompiles/` contains `bank bech32
  callbacks common distribution erc20 gov ics20 p256 slashing staking werc20`, and
  `precompiletypes.DefaultStaticPrecompiles` registers only 8 Cosmos-facing
  precompiles plus the Prague set. This repo adds one (`distrclaim`), for 9 total.
- `keeper.GetStaticPrecompileInstance` panics when an address is active in params
  but absent from the keeper map: `panic("precompiled contract not stored in
  memory: …")`, commented "it means we have memory corruption."

Nothing catches this before the call site. `ValidatePrecompiles` checks only that
entries are valid hex, unique and sorted — it never consults
`AvailableStaticPrecompiles` or the keeper's map. The evm module's `InitGenesis`
just calls `SetParams`. So a genesis containing `0x…0803` passes
`arkd genesis validate-genesis`, boots cleanly, produces blocks, and then panics
the first time anything calls that address.

Severity, stated precisely rather than dramatically: inside `runTx`, baseapp
recovers the panic into a failed transaction. It is deterministic across
validators, so it is **not** a chain halt. But the same dispatch path is reachable
from an unauthenticated `eth_call` and is consulted during simulation, so it is an
availability problem on RPC nodes that any caller can trigger at will — and it is a
latent halt the moment a future code path reaches it outside `runTx`.

`STATUS.md`'s audit recommended "Enable — Vesting account queries; low risk" for
this address. `app/app.go`'s `DefaultGenesis()` already activated it. The fork's
`evmd/genesis.go` assigns `AvailableStaticPrecompiles` wholesale while
`evmd/app.go` registers only the 8 — **so the reference implementation the issue
asked whether to copy is itself wrong, and copying it was the bug.**

### Two smaller constraints, both easy to trip

- **Order matters.** `ValidatePrecompiles` rejects an unsorted slice. The
  requirement is softer than it looks, because `SetParams` runs `slices.Sort`
  first — so an unsorted genesis is silently reordered rather than rejected,
  leaving the file disagreeing with chain state. Keep the list sorted.
- **Case matters.** `IsAvailableStaticPrecompile` compares against
  `common.Address.String()`, which is EIP-55 checksummed. All nine addresses
  checksum to their all-lowercase form (`0x…0a01` is the only one containing a
  letter, so it is the only one where this could have bitten). Uppercasing any of
  them would make that precompile silently unreachable — not an error, just inert.

## How this was missed, and what now prevents a repeat

Worth recording, because the mechanism is more instructive than the bug.

`App.DefaultGenesis()` populated `ActiveStaticPrecompiles` correctly-ish, but
**`arkd init` never calls it.** The CLI path is
`genutilcli.InitCmd(basicManager, …)`, which uses the SDK `BasicManager` defaults,
and the evm module's own default for this field is nil. `App.DefaultGenesis()` is
reachable only from `app/test_helpers.go` and `tests/e2e/chain.go`.

So the test suite ran with all precompiles active while every genesis an operator
could actually produce had `[]`. A precompile audit, a Slither run over
`scripts/chaos/contracts/Precompiles.sol`, and a chaos sign-off all passed without
anyone calling a precompile on a chain built the way mainnet would be built.
`app/genesis.go` documents the same class of bug for the deleted
`NewDefaultGenesisState()`; this was the second instance.

Three things now close it:

1. **`app/precompiles.go`** holds `ArkActiveStaticPrecompiles()` as the one
   definition, written out explicitly rather than derived from
   `AvailableStaticPrecompiles` — since deriving from that constant is what
   introduced the vesting hazard.
2. **`app/precompiles_test.go`** asserts the list is sorted and lowercase, that
   every address is actually registered in a booted app's keeper (the check that
   would have caught `0x…0803`), that `0x…0803` still panics when activated, and
   that all four committed genesis JSON files match the Go list exactly. A future
   fork re-pin that adds or drops a precompile fails here, not in production.
3. **`networks/mainnet/RUNBOOK.md`** gained a pre-flight item, because
   `validate-genesis` passing proves nothing about this field.

## Sign-off

| Item | Owner | Status |
|---|---|---|
| Launch set and exclusions | Eng 1 (owns `app/`) | ✅ Decided 2026-09-11 |
| Genesis files populated and kept in sync | Eng 2 (owns `networks/`) | ✅ Applied 2026-09-11 |
| Staking `0x…0800` — state-transition chaos coverage | Eng 3 | ⏳ **Blocking `v1.0.0`** |
| ICS20 `0x…0802` — reentrancy coverage, not just happy-path transfers | Eng 3 | ⏳ **Blocking `v1.0.0`** |
| `distrclaim` `0x…0a01` — first security review of an Ark-original precompile | Eng 3 | ⏳ **Blocking `v1.0.0`** |
| Live devnet proof: every address dispatches | Eng 2 | ⏳ Pending proposal submission — run `scripts/chaos/precompile-verify.sh --rpc <node>:8545 --api <node>:1317` |
| Live devnet proof: state-changing paths from a deployed contract | Eng 3 | ⏳ Deploy `scripts/chaos/contracts/Precompiles.sol`'s `PrecompileHarness` — this is the Staking/ICS20 coverage above, not a separate item |

`scripts/chaos/reports/day1-static-analysis.md` previously claimed to have audited
"all 10 enabled precompile addresses configured in `app/app.go`" with an
address→module table that was wrong in eight of ten rows. It has been corrected.
The three ⏳ rows above are not re-reviews of that work — they are the first real
review of those three precompiles.

## Verifying it took effect

`arkd genesis validate-genesis` tells you nothing here, and neither does querying
the param — an empty list and a populated one both validate, and an address listed
without a registered implementation validates too.

```bash
scripts/chaos/precompile-verify.sh --rpc http://127.0.0.1:8545 --api http://127.0.0.1:1317
```

That script reads the chain's own params, then probes each address. One thing about
it is worth knowing before reading its output: **`eth_getCode` cannot detect an
active precompile** — precompiles hold no bytecode in state, so a correctly
activated one returns `0x` exactly like an empty address. The usable discriminator
is call semantics: a call with an unimplemented selector *succeeds with empty data*
against a codeless address, and *fails* against a live precompile. So in check 1 an
error is the pass condition. Bech32 additionally gets a real functional probe
(`hexToBech32` is pure and needs no chain state), which proves dispatch rather than
inferring it.

The script refuses to call `0x…0803` unless `--api` confirms it is inactive first,
because the call that would reveal an active one is the call that panics the node.

## Rolling back

Deactivating an address is a governance `MsgUpdateParams` against the vm module,
effective next block. Two things to get right:

- `MsgUpdateParams` replaces the **whole** `Params` struct. Every other field
  (`evm_denom`, `extra_eips`, `evm_channels`, `access_control`,
  `history_serve_window`, `extended_denom_options`) must be restated verbatim or it
  resets. Query the live params first; do not copy an older proposal's values.
- Reverting the repo files does **not** change a running chain, and passing a
  proposal does **not** update the repo. Mainnet genesis is immutable after block 1
  regardless. Do both, in that order, and verify with `arkd query evm params` —
  see `networks/devnet/proposals/README.md`'s "Rolling back is asymmetric".

Adding a genuinely **new** precompile is not a param change: the keeper's map is
set once from `app/app.go` and `WithStaticPrecompiles` panics on a second call. That
needs a binary upgrade, and the address must be registered there before any
proposal activates it — activating an unregistered address is the `0x…0803` trap.
