# ArkConstellation + Ark-EVM — Architecture, Fork Lineage & Security Breakdown

**Audience:** the person leading this chain's development, who does not write Go daily.
**Scope:** `ArkConstellation@base-genesis` and `Ark-Evm@base-genesis`, their forks, and their forks' forks.
**Written:** 2026-08-31. Verified against the working trees on this machine, not from memory.

---

## 0. How to read this document

Sections 1–3 are orientation: the family tree, and enough Cosmos SDK mental model that
the rest of the document lands. Section 4 is a Go survival guide — read it once and the
code stops looking like noise. Sections 5–6 are the actual diffs, layer by layer.
Section 7 is the security register (the part to act on). Sections 8–10 are strategy:
what this stack buys you, what it costs you, and what you still have to decide.

Every claim below was checked against the repos. Where something is *inferred* rather
than verified, it says so.

---

## 1. TL;DR — where you actually are

You have a **sovereign Cosmos SDK Layer-1 with a full EVM execution environment**, forked
from MANTRA Chain, with MANTRA's business-logic modules stripped out and your own
identity (bech32 prefix, denom, chain IDs) substituted in. It boots, produces blocks,
serves EVM JSON-RPC, and has a 4-node dockerised devnet with monitoring, a block
explorer, and a faucet.

**What is genuinely done:**

- Chain identity fully rebranded: `ark` bech32 prefix, `esp`/`espees`/`KASH` denom tier,
  binary renamed `mantrachaind` → `arkd`, home dir `~/.mantrachain` → `~/.ark`.
- MANTRA's `x/tax` and `x/tokenfactory` modules surgically removed (~13,000 lines) with
  every wiring point cleaned up — store keys, module accounts, IBC middleware stack,
  begin/end/init-genesis orderings, wasm capabilities, Stargate query allowlist.
- `x/sanction` retained and *improved* (dual bech32/hex address support, EVM ante path).
- Circuit breaker (`cosmossdk.io/x/circuit`) wired into **both** the Cosmos and EVM ante
  paths — this is the emergency stop, and it is the single most important operational
  control you own.
- Real ops substrate: sentry topology, TMKMS remote signing, Prometheus/Grafana/
  Alertmanager, Blockscout, chaos test suites, genesis ceremony tooling.
- Two genuinely good fork audits already written by your team
  (`docs/proof/fork-audit-cosmos-sdk.md`, `docs/proof/fork-audit-cosmos-evm.md`).

**What is not done, and is load-bearing:**

- The Go module path is still `github.com/MANTRA-Chain/mantrachain/v8` and your protobuf
  namespace is still `mantrachain.*`. **Protobuf type URLs are consensus data.** Changing
  this after mainnet genesis is a coordinated hard fork. See §7-A1.
- A live GitHub personal access token is embedded in both repos' git remotes. See §7-C1.
- The one commit you added to the EVM fork trades a startup-failure signal for a
  liveness workaround, in a way that will bite you in production. See §6.2.
- `arkd init` produces a **structurally invalid genesis for this chain** (denoms are
  still `aatom`/`stake`). This is deliberate and documented, but there is no guard rail
  that stops someone starting a node from it. See §7-B3.

**Maturity read:** this is a well-documented, honestly-audited *late devnet*. It is not
yet a testnet you would put third-party value on, and the gap between here and mainnet
is mostly governance/key-management/decision debt, not code debt.

---

## 2. The family tree

Two repos, two lineages. Neither is a fork of the other — they are joined by a Go
`replace` directive.

### 2.1 ArkConstellation

```
cosmos/cosmos-sdk  ──┐
CosmWasm/wasmd     ──┤
cosmos/evm         ──┼──> MANTRA-Chain/mantrachain ──> Worldstreet-Web-Services/ArkConstellation
skip-mev/connect   ──┤         (v8.4.0 / main)                  (base-genesis)
cosmos/ibc-go      ──┘                                          81 commits ahead
```

`ArkConstellation@base-genesis` is **81 commits ahead of, and 0 commits behind,
`main`** — and `main` in your repo is a faithful mirror of MANTRA's upstream `main`.
That is an unusually clean position: you have not diverged from a moving target, you
have branched from a fixed one. The diff is **237 files, +15,246 / −17,149 lines**
(net negative, because you deleted two whole modules).

MANTRA in turn does not use vanilla upstream dependencies. From `go.mod`:

| Dependency | Replaced with | Why it matters |
|---|---|---|
| `cosmos/cosmos-sdk` | `MANTRA-Chain/cosmos-sdk v0.53.6-v8-mantra-1` | You run a **forked consensus/state layer**, pinned at v0.53.6 |
| `cosmos/evm` | `Worldstreet-Web-Services/evm v0.6.2-ark-1` | **This is your Ark-Evm repo** — you already own this link |
| `CosmWasm/wasmd` | `MANTRA-Chain/wasmd` (mantra/v0.61.1) | Forked CosmWasm host |
| `skip-mev/connect/v2` | `MANTRA-Chain/connect v2.3.0-mantra-1.4` | Forked oracle/price-feed sidecar |
| `ethereum/go-ethereum` | `cosmos/go-ethereum v1.16.2-cosmos-1` | Cosmos' own geth fork (standard, not MANTRA's) |

**Read that table again.** You are downstream of **four separate MANTRA forks**, only
one of which (`evm`) you have taken ownership of. The other three — SDK, wasmd,
connect — you consume as opaque pinned artifacts from an organisation that has no
obligation to you. That is the single biggest structural risk in the stack, and §8.2
comes back to it.

### 2.2 Ark-Evm

```
evmos/ethermint (historical) ──> cosmos/evm ──> MANTRA-Chain/evm ──> Worldstreet-Web-Services/evm
                                  (v0.6.2)      (mantra/v0.6.x)          (base-genesis)
                                                 31 commits ahead         1 commit ahead
```

`Ark-Evm@base-genesis` carries **exactly one commit** of your own
(`4bae3f8b`, "fix(rpc): decouple JSON-RPC HTTP server lifecycle from transient init
context") on top of `mantra/v0.6.x`. Two files, +21/−32.

`mantra/v0.6.x` carries **31 commits** on top of `cosmos/evm`. Your team's audit
(`docs/proof/fork-audit-cosmos-evm.md`) already dissected these properly, and its
conclusion holds up: the MANTRA patch set is lean, contains **zero branding or vanity
glue** (the module path is still `github.com/cosmos/evm`), and is mostly *backports of
upstream fixes that upstream never shipped to the v0.6.x line*. Notably it includes the
ICS20 source-callback reentrancy guard (upstream PR #1061) — the follow-up hardening for
the critical advisory behind the ~$7M Saga exploit — which upstream still has not
backported to v0.6.x. On disclosed advisories, this fork is **ahead** of the release line
it tracks.

### 2.3 Where the two repos meet

```
ArkConstellation/go.mod:14
    github.com/cosmos/evm => github.com/Worldstreet-Web-Services/evm v0.6.2-ark-1
```

That one line is the whole coupling. Every `import "github.com/cosmos/evm/..."` in
ArkConstellation resolves to your fork at build time. The EVM repo has no idea Ark
exists — it is a drop-in replacement, which is exactly the property that makes it easy
to rebase onto future upstream.

> **Note the version-string mismatch.** `go.mod` pins the tag `v0.6.2-ark-1`, but your
> team's audit doc was written against `v0.6.2-v8-mantra-1`. The `ark-1` tag is your own
> retag of `base-genesis`. The audit's *findings* still apply (the underlying code is
> mantra/v0.6.x + your one commit), but the doc's version references are now stale by one
> hop. Worth a one-line correction so a future auditor is not misled.

---

## 3. Cosmos SDK architecture, for someone who does not write Go

Skip this if you already know it. If you do not, everything downstream depends on it.

### 3.1 The three-layer sandwich

An Ethereum L1 is one program. A Cosmos chain is three, stacked:

```
┌──────────────────────────────────────────────────────────┐
│  APPLICATION   — your business logic. "What is a valid   │  <- app/, x/
│  (Cosmos SDK)    transaction? What does it do to state?" │
├──────────────────────────────────────────────────────────┤
│  ABCI          — a narrow socket-like interface. The     │  <- you rarely touch this
│                  only way the two layers talk.           │
├──────────────────────────────────────────────────────────┤
│  CONSENSUS     — "which transactions, in what order?"    │  <- CometBFT, config/
│  (CometBFT)      Networking, mempool, BFT voting.        │
└──────────────────────────────────────────────────────────┘
```

CometBFT does not know what a token is. Your app does not know what a validator vote is.
They exchange opaque byte strings through ABCI. This separation is the single best thing
about the stack and the source of most of its surprises.

**Consequence you will feel:** CometBFT gives you **instant finality**. When a block
commits, it is final — no reorgs, no confirmations to wait for, no "12 blocks deep"
heuristics. This is strictly better than Ethereum's probabilistic finality for
exchanges, bridges, and settlement. The cost is that CometBFT needs 2/3+ of stake online
and voting; if it does not have that, the chain **halts** rather than forking. Cosmos
chains do not reorg — they stop.

### 3.2 The block lifecycle (where your code runs)

Every block, ABCI calls into your app in a fixed sequence. Almost all customisation
happens by hooking one of these:

```
  PreBlocker      ──  before anything (upgrade scheduling lives here)
  BeginBlocker    ──  per-module, in an order YOU define (app.go SetOrderBeginBlockers)
  ┌ for each tx ─────────────────────────────────────────────┐
  │   AnteHandler  ──  the gatekeeper. Runs BEFORE execution. │
  │                    Signature check, fees, gas, YOUR       │
  │                    blacklist + circuit breaker.           │
  │   MsgServer    ──  the actual state change                │
  │   PostHandler  ──  after (rarely used)                    │
  └───────────────────────────────────────────────────────────┘
  EndBlocker      ──  per-module, ordered (validator set changes here)
  Commit          ──  hash the state, write to disk
```

**The AnteHandler is the most security-relevant thing you own.** It is a chain of
"decorators", each of which can reject a transaction before a single byte of state is
touched. Your `x/sanction` blacklist and your circuit breaker are both ante decorators.
A transaction that the AnteHandler rejects costs the chain almost nothing — which is
also why an *expensive* ante decorator is a DoS vector.

Ark has **two separate ante chains** (`app/ante/cosmos.go` and `app/ante/evm.go`)
because a Cosmos transaction and an Ethereum transaction are structurally different
objects. **Any control you add must be added to both.** Your team got this right for
both the blacklist and the circuit breaker — that is genuinely good discipline, and it
is the #1 thing that goes wrong on EVM-on-Cosmos chains.

### 3.3 Modules and keepers — the object model

A **module** (`x/sanction`, `x/bank`, `x/staking`) is a self-contained feature. Each owns:

- a **store key** — its private namespace in the global key-value database. `x/bank`
  physically cannot write into `x/staking`'s keyspace.
- a **keeper** — the *only* object that can read/write that store. Think of it as the
  module's API. When `x/staking` needs to move tokens, it does not touch the bank store;
  it holds a `BankKeeper` reference and calls a method.
- **proto messages** — the wire format for its transactions and queries.
- **genesis** import/export.

`app/app.go` is the wiring diagram: it constructs every keeper, hands each one
references to the keepers it depends on, and declares the orderings. It is ~1,500 lines
of almost pure plumbing, and it is where you look for "what modules does this chain
actually have?"

**Module accounts** (`maccPerms` in `app.go`) are addresses owned by code, not keys.
`{authtypes.Minter, authtypes.Burner}` on a module means that module is allowed to
create and destroy tokens. Auditing that map is a five-minute, high-value exercise:
anything with `Minter` can inflate your supply.

### 3.4 How the EVM fits in

`cosmos/evm` (your Ark-Evm) is *just another Cosmos module*, `x/vm`. An Ethereum
transaction arrives at the JSON-RPC endpoint, gets wrapped in a Cosmos message
(`MsgEthereumTx`), goes through the EVM ante chain, and lands in `x/vm`'s message
server, which runs it against a `StateDB` that is a shim over the Cosmos KV store.

This has three consequences worth internalising:

1. **One account, two address formats.** `ark1abc...` (bech32) and `0xABC...` (hex) are
   the *same 20 bytes*, differently encoded. Your `x/sanction` normalisation code exists
   precisely because of this — and getting it wrong means a blacklist that can be
   trivially bypassed by switching encoding. Your team handled this correctly.
2. **The EVM does not own the ledger.** `x/bank` does. `x/vm` translates between EVM
   balance semantics and bank coins. This is where the nastiest bug classes live —
   MANTRA's `SetBalanceWithLocked` hardening (§6.3) is exactly one of these.
3. **Precompiles are your Cosmos modules exposed to Solidity.** A contract can call
   `0x...0800` and delegate stake. This is a genuine feature and a genuine attack
   surface — the ICS20 precompile is what the Saga exploit went through.

### 3.5 Gas and fees, in this chain specifically

There are **two** fee systems and they interact:

- **`minimum-gas-prices`** — a per-*node* config (in `app.toml`). Each validator
  independently refuses transactions paying below its own threshold. Not consensus.
- **`x/feemarket`** — an EIP-1559-style dynamic base fee, on-chain and consensus-relevant.

Ark uses `skip-mev/feemarket`, kept at vendored defaults (base fee 10⁹ esp ≈ 1 gwei,
change denominator 8, elasticity 2) as a deliberate decision to keep gasless/Paymaster
flows viable. Note this chain prices **even native Cosmos bank sends** off the EVM fee
market — your own faucet code discovered this the hard way and now queries
`feemarket base-fee` before every broadcast. That is a real, non-obvious property of the
stack and it should be in your integrator docs.

### 3.6 Governance and upgrades

`x/gov` proposals can change module parameters, spend the community pool, and — via
`x/upgrade` — **schedule a binary upgrade at a specific block height**. At that height
every node halts, operators swap binaries, and an upgrade handler migrates state. This
is how Cosmos chains do hard forks: coordinated, scheduled, and governance-gated.

Ark's locked parameters: 7-day voting period, 21-day unbonding, 10 initial validators,
SDK-default slashing (5% double-sign, 1% downtime, 10-minute jail).

> **Note on the unbonding period.** 21 days is the Cosmos standard and it is what makes
> slashing economically meaningful — but it is also your *security window*. An attacker
> who acquires 1/3+ of stake can halt the chain, and 2/3+ can rewrite it, and the
> 21 days is how long their capital is trapped if they misbehave. With **10 validators**
> at genesis, the concentration risk is high enough that the circuit breaker and the
> upgrade multisig (still ⏳ Pending, §10) are not optional niceties.

---

## 4. Go survival guide for this codebase

Enough to read a diff and know whether it is dangerous.

### 4.1 The eight things that will confuse you

| You see | It means |
|---|---|
| `func (k Keeper) Foo(...)` | Method on `Keeper`. The `(k Keeper)` is the receiver — Go's `this`. |
| `func (k *Keeper) Foo(...)` | Same, but takes a **pointer**. Can mutate the keeper. Value receivers get a copy. |
| `x, err := doThing()` | Go returns errors as **values**, not exceptions. Every call returns two things. |
| `if err != nil { return err }` | The only error handling idiom. 40% of the code is this line. Its *absence* after a fallible call is a bug. |
| `_ = doThing()` | "Deliberately ignore this error." **Always suspicious.** Grep for it in security review. |
| `defer f.Close()` | Run this when the function exits, no matter how. Used for cleanup. Placement matters (§5.3). |
| `ctx context.Context` | A cancellation signal + deadline + values, threaded through every call. Cancelling it unwinds everything downstream. |
| `interface{ Foo() }` | A contract. Any type with a `Foo()` method satisfies it — no `implements` keyword. This is how keepers depend on each other loosely. |

### 4.2 Goroutines and channels — the concurrency bits

- `go f()` — run `f` concurrently. Fire and forget. **Nothing waits for it.**
- `ch <- v` / `<-ch` — send/receive on a channel (a typed, blocking queue).
- `errgroup.Group` — "run these concurrently; if any returns an error, cancel the rest
  and report it." This is the pattern the Ark-Evm patch **removed**, and §6.2 is about
  why that matters.
- `panic` — Go's crash. In a blockchain node, a panic during block execution is
  catastrophic (chain halt), which is why the SDK wraps message execution in recovery
  and why `MustUnmarshal`-style functions are only acceptable in genesis/init paths.

### 4.3 Where to look for what

```
app/app.go            <- THE wiring diagram. Start here, always.
app/ante/             <- The gatekeepers. cosmos.go + evm.go. Highest security density.
app/config.go         <- Chain ID resolution (and a package-level init() that reads a file)
app/genesis.go        <- FeeDenom, and a deliberately-removed function (§7-B3)
app/params/config.go  <- bech32 prefix, denom constants
app/precompiles/      <- Ark's custom Solidity-callable modules (distrclaim)
x/sanction/           <- the only surviving custom module
cmd/arkd/             <- CLI entry point, binary name, SDK config sealing
proto/                <- message definitions. THE WIRE FORMAT. Consensus-relevant.
networks/             <- devnet + mainnet genesis material, runbooks
ops/                  <- docker, monitoring, explorer, faucet
scripts/chaos/        <- adversarial test suites + tool reports
docs/decisions/       <- the authoritative locked-parameters record
docs/proof/           <- fork audits + verification logs
```

### 4.4 Reading a Cosmos diff safely — a checklist

When reviewing any change to this repo, ask in this order:

1. **Does it touch `proto/`?** If yes, it may be state-breaking or wire-breaking. Stop
   and think about migration.
2. **Does it touch an ante decorator or its ordering?** If yes, it changes what
   transactions are admissible. Both chains, or only one?
3. **Does it touch `maccPerms` or a keeper constructor?** If yes, it may change who can
   mint or move funds.
4. **Does it change a Begin/EndBlocker ordering?** Ordering is consensus. Two nodes with
   different orderings will disagree on state and halt the chain.
5. **Does it change gas accounting?** That is consensus. See §6.3 item 3c.
6. **Is there a new `_ =` or a swallowed error?** See §6.2.

---

## 5. What ArkConstellation changed (the 81 commits, by theme)

### 5.1 Identity and denomination

| Item | MANTRA | Ark | File |
|---|---|---|---|
| bech32 prefix | `mantra` | `ark` | `app/params/config.go`, `cmd/arkd/main.go` |
| Base denom | `amantra` | `esp` | `app/genesis.go` |
| Display denom | `mantra` | `KASH` | `app/app.go` (`EVMCoinInfo`) |
| Intermediate | — | `espees` (10⁹ esp) | decisions doc #8 |
| Binary | `mantrachaind` | `arkd` | `cmd/arkd/` |
| Home dir | `.mantrachain` | `.ark` | `app/app.go` (`NodeDir`) |
| Default EVM chain ID | `262144` | `11199` | `app/config.go` |
| IBC unwrap memo | `{"mantra":...}` | `{"ark":...}` | `app/ibc_middleware/unwrap_erc20.go` |

The denom design is **Ethereum parity**: `esp` = wei, `espees` = gwei, `KASH` = ether,
with `KASH` carrying 18 decimals. This is the right call — it means every Ethereum tool,
wallet, and mental model transfers directly, and `x/vm`'s `LoadEvmCoinInfo` resolves 18
decimals from the `KASH` denom unit without needing the non-18-decimal extended-denom
path. Your team verified this against `x/vm/keeper/coin_info.go` rather than assuming it.

There is one wrinkle worth knowing: the SDK's default denom regex rejects uppercase, so
`ArkCoinDenomRegex()` overrides it to permit `KASH`. That override is set in **two
places** (`app/params/config.go`'s `init()` and `cmd/arkd/main.go`'s `main()`), and the
seal/no-seal dance between them is commented but fragile. See §7-B4.

### 5.2 Module surgery

**Removed — `x/tax`** (MANTRA's protocol-level transaction tax routing a cut to a
foundation address) and **`x/tokenfactory`** (permissionless denom creation, Osmosis-derived).

The removal is thorough — not just deleting directories, but unwinding every wiring
point: keeper struct fields, `maccPerms` entries (`tokenfactory` had **Minter+Burner**),
store keys, the bank hooks registration, the IBC transfer middleware stack, all four
module orderings, `app/wasm.go` capabilities, `app/queries/queries.go` Stargate
allowlist, proto files, testutil helpers, and 1,083 lines of e2e tests. That is the
correct way to remove a module and it is easy to do badly.

Also removed: `MigrateUomIBCModule` (MANTRA's legacy `uom` token migration middleware,
220 lines) and the `PreBlocker` hardcoded emergency-upgrade scheduling for MANTRA's
v8.4.0 delegate-precompile exploit fix — both dead weight referencing MANTRA's
production incident history and mainnet block heights.

**Retained and improved — `x/sanction`.** The blacklist now accepts both `ark1...` and
`0x...` inputs and normalises to bech32 at the keeper boundary
(`x/sanction/keeper/blacklist.go`), and a new `EVMBlacklistCheckDecorator`
(`x/sanction/keeper/evm_ante.go`) enforces it on the EVM path. Without that second
decorator the blacklist would have been bypassable by simply sending an Ethereum
transaction instead of a Cosmos one. This was the right catch.

The Cosmos-side decorator also handles the subtle cases: fee granters, and `authz`
`MsgExec` inner-message signers — with nested and multiple `MsgExec` rejected outright
because a flat check cannot safely inspect them. That is thoughtful defensive design.

**Retained — `cosmossdk.io/x/circuit`.** `SetCircuitBreaker(&app.CircuitKeeper)` plus
`circuitante.NewCircuitBreakerDecorator` on both ante paths. This lets an authorised
account disable specific message types chain-wide without a binary upgrade. Verified
live (`docs/proof/circuit-breaker-verification.log`). **This is your emergency brake.**
Who holds its authority is currently the same `x/gov` module address as everything else
— and a 7-day voting period is not an emergency response time. See §10.

### 5.3 Correctness fixes made along the way

Three small ones worth naming because they show the review quality:

- **File descriptor leak** (`app/config.go`): `defer reader.Close()` was placed *inside*
  a conditional branch after an early `return`, so the genesis file handle leaked on the
  success path. Moved to immediately after `os.Open`. Correct.
- **Integer overflow** (`app/config.go`): `strconv.Atoi` → `strconv.ParseUint(.., 64)`
  for chain-ID parsing. On 32-bit platforms `Atoi` would have silently truncated.
- **Negative block height** (`app/precompiles/distrclaim/events.go`): clamped before
  `uint64()` conversion. Defensive; only reachable in odd test contexts, but free.

### 5.4 The operational layer (most of the 81 commits, by volume)

This is where the bulk of the work went, and it is substantial:

- **`networks/devnet/`** — pystarport-driven 4-node devnet, sentry topology applied via
  script, genesis template, TMKMS remote signing with live proof logs, block-time
  measurement.
- **`networks/mainnet/`** — `RUNBOOK.md`, `genesis-params.json`, a 1,331-line
  `genesis-DRAFT.json`.
- **`scripts/genesis/`** — gentx collection with per-file validation (boots a throwaway
  node per gentx), canonical genesis hashing, bech32 re-encoding, and a **rehearsal
  fixture set including deliberately-bad gentxs** (tampered signature, overclaimed
  stake) that are verified to be rejected. This is genuinely good ceremony practice.
- **`ops/docker/`** — full devnet compose with sentries, monitoring, Blockscout, faucet.
- **`ops/monitoring/`** — Prometheus, 4 Grafana dashboards, Alertmanager rules.
- **`scripts/chaos/`** — circuit-breaker, mempool-flood, rate-limit, hard-reboot, and
  validator-failure runners in Python, plus a `LaunchGuardrail.sol` contract hardened
  with SafeERC20 and Ownable2Step, and raw gosec/semgrep/slither output committed.

---

## 6. Ark-Evm: what changed, at each hop

### 6.1 cosmos/evm → MANTRA-Chain/evm (31 commits) — inherited

Your team's `docs/proof/fork-audit-cosmos-evm.md` is a better document than most paid
audits and I am not going to restate it. The short version, with my own read layered on:

**Genuinely valuable, keep:**
- ICS20 source-callback **reentrancy guard** (upstream PR #1061). Follow-up hardening
  for the critical advisory behind the Saga exploit. **Upstream has still not backported
  this to v0.6.x.** You have it; a chain on vanilla v0.6.2 does not.
- `SetBalanceWithLocked` now blocks EVM balance writes to any bank-`BlockedAddr`, not
  just instantiated module accounts. Closes a force-set-funds gap.
- `JSONRPC.HTTPBodyLimit` (5MB default) — real DoS hardening, not upstream.
- Two verified backports of upstream v0.7.2 fixes: state-override precompile
  registration, and the default-path dynamic-precompile dispatch bug.

**Behaviour changes to tell your integrators about:**
- **Event output changed.** A duplicate-emission bug in `PostTxProcessing` hooks was
  fixed, so any indexer or explorer built against vanilla v0.6.x will see *fewer* events
  on your chain. Blockscout is fine; a custom indexer might not be.
- **`eth_call` / `eth_estimateGas` return different numbers** for transactions touching
  native precompiles (previously over-estimated). Wallets and relayers that hardcode
  margins may need retuning.
- **`GetCoinbaseAddress` now returns the zero address instead of erroring** on failed
  validator lookup. This converts a per-transaction hard failure into a silent one. It
  is a defensible liveness-over-strictness trade, but it is now *your* choice, inherited
  by default. Decide it explicitly.

**The one thing to test before mainnet:**
- **EIP-7623 post-refund calldata floor** (`x/vm/keeper/state_transition.go`). This
  appears to be MANTRA-original and is absent from upstream through v1.0.0-rc2 — it has
  not had upstream's review. It is a **consensus-critical gas accounting change**:

  ```go
  if rules.IsPrague {
      gasUsed = math.LegacyMaxDec(gasUsed, math.LegacyNewDec(int64(floorDataGas))) //#nosec G115
  }
  leftoverGas = msg.GasLimit - gasUsed.TruncateInt().Uint64()
  ```

  Two specific things I would fuzz. First, that `leftoverGas` line is an **unsigned
  subtraction** — if `gasUsed` could ever exceed `msg.GasLimit`, it underflows to a
  near-`2^64` value. It cannot today, because ~120 lines earlier an intrinsic-gas check
  rejects `GasLimit < floorDataGas`. But the safety of one line now depends on an
  invariant established far away in the same function, under the same `IsPrague` gate.
  That is exactly the shape of bug that survives review and dies in production. Second,
  the `//#nosec G115` suppresses the uint64→int64 conversion warning rather than
  bounding the value. Practically unreachable (calldata size bounds it), but the
  suppression hides the reasoning.

  **Target the tests at: Prague-rules transactions with large calldata and heavy
  refunds** (SSTORE-clearing patterns, selfdestruct-era refunds). Compare `gasUsed`
  against a geth reference implementation on identical inputs.

### 6.2 MANTRA-Chain/evm → Ark-Evm: your one commit, reviewed

This is the only Go code your organisation has authored in the EVM layer, so it deserves
full scrutiny. The intent — keep the JSON-RPC and LCD servers alive when the context
they were started with gets cancelled during init — is legitimate. The implementation
has four problems, and one of them is serious.

**Before** (`server/json_rpc.go`), simplified:

```go
g.Go(func() error {                       // errgroup: supervises the goroutine
    errCh := make(chan error)
    go func() { errCh <- httpSrv.Serve(ln) }()
    select {
    case <-ctx.Done():                    // node shutting down -> graceful stop
        httpSrv.Shutdown(ctxShutdown)
        return nil
    case err := <-errCh:                  // server died -> propagate, ABORT STARTUP
        return err
    }
})
```

**After:**

```go
go func() {                               // plain goroutine: nothing supervises it
    if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
        srvCtx.Logger.Error("failed to start JSON-RPC server", "error", err.Error())
    }                                     // <- error is LOGGED, then discarded
}()

sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)   // <- library grabs signals
go func() {
    <-sigCh
    httpSrv.Shutdown(ctxShutdown)         // hardcoded 5s, config value ignored
}()
```

And in `server/start.go`:

```go
- g.Go(func() error { return apiSrv.Start(ctx, svrCfg) })
+ go func() { _ = apiSrv.Start(context.Background(), svrCfg) }()
```

**Problem 1 — silent RPC death (this is the serious one).** Previously, if the JSON-RPC
server failed to bind or crashed, the errgroup propagated the error and the node refused
to start. Now it logs and the node comes up **healthy from CometBFT's perspective with
no JSON-RPC at all**. For an EVM chain, JSON-RPC *is* the product. Your Prometheus
scrape of the node will be green, consensus will be green, and every wallet and dApp
will be down. Your alerting (`ops/monitoring/alerts/alerts.yml`) should be checked for
whether it probes `eth_blockNumber` on 8545 — if it only scrapes CometBFT metrics, this
failure mode is invisible.

**Problem 2 — a library function now owns process signals.** `signal.Notify` inside
`StartJSONRPC` means this package competes with the Cosmos SDK's own
`server.ListenForQuitSignals`, which is already installed. Two independent handlers now
receive SIGTERM. Neither coordinates with the other, so RPC shutdown and consensus
shutdown race — the RPC server may stop draining requests while blocks are still
committing, or vice versa. Container orchestrators send SIGTERM then SIGKILL after a
grace period; a non-deterministic shutdown order under that clock is how you get
corrupted `application.db` on rolling restarts. `signal.Stop` is also never called.

**Problem 3 — the LCD API server can never shut down.** `context.Background()` is a
context that is *never cancelled, by construction*. `apiSrv.Start` will run until the
process is killed. No graceful drain, no in-flight request completion, and the returned
error is explicitly discarded with `_ =`. This is a strictly worse version of the same
issue as Problem 1, applied to port 1317.

**Problem 4 — config drift and lint.** The configurable `shutdownTimeout` was replaced
with a hardcoded `5 * time.Second`; the operator-facing config value is now silently
ignored. And the new imports (`errors`, `os`, `os/signal`, `syscall`) were inserted
*before* `fmt` in the import block — not sorted — which will fail the repo's own
`gofumpt`/`gci` lint config.

**What the fix should actually be.** The commit message names the real bug correctly:
"transient init context." The problem is that `StartJSONRPC` was handed a context that
gets cancelled during initialisation, rather than the long-lived server context. The
correct fix is to **pass the right context**, keeping the errgroup supervision and the
graceful shutdown intact:

```go
// caller side: pass the server's lifetime context, not the init-scoped one
g.Go(func() error {
    // ... unchanged upstream body, but ctx is now the long-lived server ctx
})
```

That is a one-line change at the call site instead of a restructure at the callee, it
keeps startup failures fatal, it keeps shutdown deterministic, and it stays close to
upstream so future rebases are trivial. **I would treat reverting this commit in favour
of a call-site fix as a pre-testnet task, not a nice-to-have.**

### 6.3 What this means for your rebase story

Because your fork is one commit deep and MANTRA's is 31, rebasing onto a future
`cosmos/evm` release is mostly a *MANTRA* problem, not yours. If MANTRA moves to
v0.7.x/v1.0.x, you inherit it cheaply. If MANTRA stops maintaining the fork, you inherit
31 commits of backport archaeology. That is a real dependency risk (§8.2) and it is
worth knowing now which of those 31 commits are already upstream (most) versus
MANTRA-original (essentially one — the EIP-7623 change).

---

## 7. Security register

Ranked by what I would fix first. Severity is *my* assessment for a chain heading to
public testnet; adjust for your actual timeline.

### A. Do before mainnet genesis (irreversible after)

**A1 — Protobuf namespace and Go module path are still MANTRA's.**
`module github.com/MANTRA-Chain/mantrachain/v8`, and proto packages are `mantrachain.*`.

This is flagged in `GAPS.md` as a deferred decision, but it is under-rated there. The Go
module path is cosmetic and renameable any time. **The protobuf package names are not.**
Message type URLs like `/mantrachain.sanction.v1.MsgAddBlacklist` are embedded in:
- transaction signing bytes (so every historical tx signature depends on them),
- the `Any` type URLs stored in state,
- every client SDK, indexer, and wallet integration.

Changing them post-genesis requires a coordinated upgrade with a state migration and
breaks every client. Changing them **now** costs a regeneration and a test run.

*Recommendation:* rename `proto/mantrachain/` → `proto/ark/` (or similar) and regenerate,
before genesis. Do the Go module path in the same pass since you are touching every
import anyway. If you deliberately choose *not* to, write that down as a locked decision
with the reasoning, because someone will ask in six months.

**A2 — Two Cosmos chain IDs map to the same EVM chain ID.**
`app/config.go`:
```go
"arkconstellation-1":  11199,   // mainnet
"ark_11199-1":         11199,   // "EVM format"
```
EVM chain ID is the replay-protection domain separator (EIP-155). If a network ever runs
under `ark_11199-1` while mainnet runs `arkconstellation-1`, **every EVM transaction
signed for one is valid on the other.** A testnet with a colliding EVM chain ID is a
replay attack against mainnet.

Also still present: `mantra-1`, `mantra-dukong-1`, `mantra-canary-net-1`. Dead entries
pointing at another chain's production identity.

*Recommendation:* one Cosmos chain ID per EVM chain ID, enforced by a unit test that
asserts the map's values are unique. Delete the `mantra-*` entries. Register your chain
IDs at chainlist/ethereum-lists to stake the claim.

**A3 — Upgrade/emergency authority is unresolved.** Decision #17 in
`docs/decisions/module-and-config-decisions.md` is still ⏳ Pending. Right now the
circuit breaker, the upgrade handler, and every module's param authority are all the
`x/gov` module address, gated behind a **7-day voting period**.

Seven days is a fine deliberation window and a useless incident-response window. The
Saga exploit that the ICS20 guard defends against drained ~$7M in minutes. You need a
faster path for `MsgTripCircuitBreaker` specifically.

*Recommendation:* a small multisig with circuit-breaker-only authority (not upgrade
authority), documented, with a rehearsed runbook and named humans in timezones that
cover 24h. Test it on devnet with a real key ceremony before you need it.

### B. Do before public testnet

**B1 — `minimum-gas-prices` is effectively zero and looks like it is not.** ✅
**RESOLVED 2026-09-04.** `MIN_GAS_PRICES` is now `1000000000esp` (1 gwei) in
`entrypoint.sh`'s default, all four compose services, and `pystarport.json` — the
recommendation below, taken as written. A second consequence surfaced while fixing
it: the node *advertises* its local value over `/cosmos/base/node/v1beta1/config`,
which clients auto-fill fees from, so `0.01esp` did not merely under-defend the
mempool — it actively handed tooling a fee that consensus rejects. The remaining
half of the recommendation, an alert on `feemarket` `NoBaseFee`, is still open.

The original finding, for context:

`ops/docker/entrypoint.sh` and every service in `docker-compose.devnet.yml` set
`MIN_GAS_PRICES: "0.01esp"`. Since `esp` is the **wei-equivalent base unit**, 0.01 esp
per gas means a 21,000-gas transfer costs 210 esp = 2.1 × 10⁻¹⁶ KASH. That is free.

Today the `x/feemarket` base fee (10⁹ esp ≈ 1 gwei) is the real gate, so it does not
bite. But it means your node-level spam defence is **entirely dependent on the fee
market being active**. If anyone ever sets `NoBaseFee: true` or the base fee decays to
its floor under low load, you have an unmetered public mempool.

*Recommendation:* set `MIN_GAS_PRICES` to a real fraction of the expected base fee (e.g.
`1000000000esp` for 1 gwei, or explicitly document `0.01esp` as intentional-and-why in
the same line). Add an alert on `feemarket` `NoBaseFee`. Note that `STATUS.md` claims the
default is `0esp` while the ops layer uses `0.01esp` — reconcile those.

**B2 — Node RPC ports are published on all interfaces.**
`docker-compose.devnet.yml` publishes `26657` (CometBFT RPC), `1317` (LCD), `8545`/`8546`
(EVM RPC/WS), `9090` (metrics) as bare `"26657:26657"` — which binds `0.0.0.0` on the
host. On a laptop that is fine. On any cloud VM those are **internet-facing**.

Publicly exposed CometBFT RPC is a well-known DoS surface: unbounded WebSocket
subscriptions, `/broadcast_tx_commit` holding connections, `num_unconfirmed_txs`
dumping the mempool. Exposed metrics leak validator topology.

Note the *faucet* service in the same file correctly uses `127.0.0.1:8088:8088`. So the
pattern is known — it just was not applied consistently.

*Recommendation:* bind everything to `127.0.0.1` in compose and put a reverse proxy
(rate limiting, TLS, method allowlist) in front of the endpoints that must be public.
For public EVM RPC specifically, allowlist methods and disable `debug_*`/`personal_*`.

**B3 — `arkd init` produces a genesis that is wrong for this chain, with no guard.**
`NewDefaultGenesisState()` was removed (correctly — it was dead *and* broken; it would
have panicked indexing `TokenPairs[0]` on an always-empty slice). Genesis customisation
now lives entirely in the deployment-time merge-patch process.

The reasoning is sound and well-documented. The gap is that there is now **nothing
stopping someone from running `arkd init && arkd start`** and getting a chain with
`evm_denom: "aatom"`, staking denom `"stake"`, and no `bank.denom_metadata` — which
`x/vm`'s `LoadEvmCoinInfo` requires. That is a confusing, hard-to-diagnose failure for
any new validator or integrator.

*Recommendation:* add a startup assertion — if `evm.params.evm_denom != "esp"` or the
`KASH` denom metadata is missing, refuse to start with a message pointing at the runbook.
Twenty lines, and it converts a mystery into an instruction.

**B4 — SDK config is initialised in two places with a fragile seal contract.**
`app/params/config.go`'s `init()` sets prefixes but deliberately does *not* `Seal()`;
`cmd/arkd/main.go`'s `setupConfig()` sets them again and seals. The constants are
duplicated verbatim in both files. It is commented, and it works — but it depends on Go
package-init ordering, and a divergence between the two copies would produce addresses
that differ depending on which code path ran. Tests import `app/params` without going
through `main`.

*Recommendation:* have `cmd/arkd/main.go` import the constants from `app/params` rather
than redeclaring them. Add a test asserting `sdk.GetConfig().GetBech32AccountAddrPrefix()
== params.Bech32Prefix` after full app construction.

**B5 — Sanction blacklist is transaction-origin only. Understand what it does not do.**
Both decorators check the **signer** (`GetSigners()`) or the EVM **`from`**. That is the
right primitive, and the authz/feegrant coverage is thoughtful. But state a compliance
posture honestly, because ante-level sanctions cannot do the following:

- **Receiving is not blocked.** Anyone can send funds *to* a blacklisted address.
- **Funds are not frozen.** A blacklisted EOA that previously called `approve` on an
  ERC-20 can have those funds moved by the (non-blacklisted) spender. A blacklisted
  address's assets sitting in a contract can be withdrawn by whatever the contract's
  logic permits.
- **Contract-mediated action is not blocked.** A blacklisted party who controls a
  CosmWasm or EVM contract can act through it if any non-blacklisted key can trigger it.
- **Existing delegations keep accruing rewards** — the blacklisted address just cannot
  sign the claim.

If your compliance requirement is "sanctioned addresses cannot transact," you have that.
If it is "sanctioned funds are frozen," you do not, and closing that gap means
state-level hooks (a bank send hook, an EVM `StateDB` transfer guard) with materially
higher blast radius and consensus risk. **Decide which one you are claiming, in writing,
before anyone asks you in an audit.**

Also: verify that blacklist entries imported via `InitGenesis` go through
`normalizeAddress`. The keeper's `AddToBlacklist` does; I did not trace the genesis path.
If it does not, a hex-format entry in a genesis file would be stored unnormalised and
would never match at ante time — a silently ineffective sanction.

### C. Operational hygiene — do this week

**C1 — A live GitHub personal access token is embedded in both repos' git remotes.**

```
origin  https://ghp_<redacted>@github.com/Worldstreet-Web-Services/ArkConstellation.git
origin  https://ghp_<redacted>@github.com/Worldstreet-Web-Services/evm.git
```

The good news: I searched the full history of both repos (`git log --all -S`) and the
token is **not committed** — which is why the value is redacted above rather than
quoted. It lives in `.git/config` only.

The bad news: `ghp_`-prefixed classic PATs are **not scoped to a repository**. This one
grants whatever the issuing account can do across the whole `Worldstreet-Web-Services`
org — which, for a chain organisation, plausibly includes pushing to the repo that
produces your release binaries. It is also almost certainly in shell history, and any
`git remote -v` pasted into a ticket, a CI log, or a chat leaks it permanently.

*Recommendation, today:* revoke it. Replace with either SSH keys or a fine-grained token
scoped to specific repos, and store it in the system keychain via a git credential
helper rather than the URL. Then audit the org's GitHub audit log for the token's usage
history, and check whether the same token appears in any CI secret, Dockerfile, or
`.env`. Since it is the *same token in both repos*, assume it is used elsewhere too.

**C2 — Faucet: rate limiting is decorative, and it will be drained.** (`ops/docker/faucet/`,
currently uncommitted.)

The service is otherwise reasonably built — address regex validation, `subprocess.run`
with a list (no shell injection), bound to `127.0.0.1` on the host, dynamic gas pricing.
But:

- **The 50 KASH/24h cap is per-address.** Generating a fresh address is free. There is no
  IP limit, no captcha, no proof-of-work, no origin check. The cap stops nothing.
- **The faucet key controls a genesis account funded with 10²⁶ esp = 100,000,000 KASH —
  10% of your entire 1B supply.** For a devnet that is merely alarming; as a *pattern*
  carried to testnet it is a disaster. A faucet should hold a top-up-able working
  balance, not a tenth of the supply.
- **The mnemonic is passed as a plain environment variable** to two services and loaded
  into a `test` keyring backend, which is unencrypted on disk. It is in your shell env,
  your `.env` file, `docker inspect` output, and any process listing.
- **`HTTPServer` is single-threaded and holds a global lock across a call that sleeps up
  to 15 seconds** waiting for on-chain confirmation. Maximum throughput is roughly four
  requests per minute, and any one request blocks every other. One slow confirmation
  stalls the whole service — a trivial DoS, and a bad user experience even without one.
- **`Access-Control-Allow-Origin: *`** on the API — any website can drive the faucet from
  a visitor's browser.
- **The state file grows without bound**, is fully read and rewritten on every request,
  and is never pruned of addresses (only of entries within an address). Disk and latency
  both grow linearly with every address ever served.

*Recommendation:* cap the faucet account at a small working balance with a scripted
top-up. Use `ThreadingHTTPServer` and move confirmation off the request path (return the
tx hash immediately). Add IP-based rate limiting and an origin allowlist. Prune the state
file. Move the mnemonic to a mounted secret file with `0400`, not an env var. And commit
this code — it is currently untracked, so it is not reviewed, not versioned, and not
backed up.

**C3 — Devnet secrets are defaults.** Grafana ships `admin / arkconstellation`
(documented in the compose header). The `init` container runs as `root`. Validator keys
use `keyring-backend test` (plaintext on disk) and `entrypoint.sh` copies `keyring-test`
between node homes. All acceptable for a laptop devnet; none of it should survive
contact with a hosted environment. Write that boundary down explicitly so nobody
"promotes" the devnet compose file to staging.

**C4 — Cosmos SDK fork is behind on two upstream fixes.** Your own audit found it: the
pin is `v0.53.6`, and `v0.53.7`/`v0.53.8` shipped a secp256k1 pubkey-tag validation fix
and a compact-bitarray bounds check. Neither is a published CVE against v0.53.6, so this
is not urgent — but it is a standing item for the next re-pin, and you depend on
**MANTRA** to produce that re-pin (§8.2).

### D. Worth a test, not a fix

- **EIP-7623 post-refund gas floor** — see §6.2. Highest-value fuzzing target in the tree.
- **Precompile coverage.** All 10 precompiles are recommended "Enable," with Staking and
  ICS20 flagged for chaos coverage. ICS20 is the one with a $7M exploit in its history.
  Confirm your chaos suite actually exercises reentrancy paths through it, not just
  happy-path transfers.
- **`_ = ` audit.** Grep both repos for discarded errors (`_ =`) and swallowed returns in
  non-test code. Each one is a place where a failure becomes invisible. The Ark-Evm patch
  introduced two.

---

## 8. The stack: what it buys you, what it costs you

### 8.1 Advantages

**Instant finality.** CometBFT commits are final. No reorgs, no confirmation depth. For
anything settlement-shaped — exchanges, bridges, RWA — this is a categorical advantage
over Ethereum's probabilistic finality, and it is the reason to be on Cosmos at all.

**Sovereignty over the full stack.** You control the validator set, the fee market, the
gas schedule, block time, and what the state machine does before and after every
transaction. Nothing in this document would be *possible* on an L2 — a rollup inherits
its sequencer's rules. Your circuit breaker, your sanction ante, your custom
`distrclaim` precompile: all of these exist because you own the state machine.

**EVM compatibility without EVM constraints.** Solidity contracts, MetaMask, Hardhat,
Foundry, Blockscout, and the whole tooling ecosystem work out of the box — while you
simultaneously get Cosmos modules, CosmWasm, and native IBC. Precompiles let Solidity
call into staking, gov, and IBC directly. That combination is genuinely hard to get
anywhere else.

**IBC.** Native, trust-minimised interoperability with every Cosmos chain, without a
bridge contract holding pooled funds. Historically the largest single source of loss in
crypto is bridges; IBC's light-client model removes that class of risk. You have
`x/ratelimit` middleware wired in front of it, which is the correct additional control.

**Modularity as a security property.** Removing `x/tax` and `x/tokenfactory` deleted
~13,000 lines of attack surface — including a module holding **Minter** permission. Every
module you do not ship is a module that cannot be exploited.

**Upgrade coordination is a solved problem.** `x/upgrade` gives you scheduled, governance-
gated, height-coordinated binary swaps with state migrations. Compare to Ethereum's
social-consensus fork process.

### 8.2 Limitations and risks

**Fork depth is your dominant structural risk.** You are four forks deep — SDK, wasmd,
connect, evm — and you own exactly one of them. For the other three you are consuming
pinned artifacts from MANTRA, an organisation with no contractual obligation to you.
Concretely, if MANTRA stops maintaining `MANTRA-Chain/cosmos-sdk`, then when the next
SDK security advisory lands you must either (a) rebase MANTRA's fork onto the patched
upstream yourself, without knowing why every patch exists, or (b) migrate to vanilla
SDK, which may not be drop-in.

*Mitigation, in priority order:* (1) write down, for each of the three unowned forks,
what it changes vs upstream and why — you have already done this for the SDK and the
EVM, so wasmd and connect are the gap; (2) subscribe to the Cosmos SDK security
advisory feed directly rather than waiting for MANTRA; (3) budget a "de-fork" spike that
evaluates running vanilla `cosmos-sdk` + vanilla `wasmd`, and know the cost before you
need the answer.

**Validator set concentration.** 10 validators at genesis means 4 of them can halt the
chain and 7 can rewrite it. Cosmos chains halt rather than fork under Byzantine
conditions, so a 4-validator outage is a *full network stop*, not degraded service. Plan
validator diversity — jurisdiction, cloud provider, client version — as a security
control, not a marketing exercise.

**Chain halt is a real, regular operational mode.** Any consensus-affecting bug — a
non-deterministic state machine, a panic in a BeginBlocker, an ordering divergence —
stops the chain until every validator upgrades. Your hard-reboot chaos simulation exists
for exactly this and that is the right instinct. Rehearse it with real operators, not
just scripts.

**Non-determinism is the ambient hazard.** Every node must produce byte-identical state
from identical input. Go map iteration order is randomised; wall-clock time, floating
point, and goroutine scheduling are all forbidden in consensus paths. This is why you
will see `sort.Strings` calls that look pointless — they are not. Any code that touches
block execution needs review specifically through this lens.

**EVM-on-Cosmos is an impedance mismatch.** Two account models, two address encodings,
two gas systems, and two fee mechanisms, glued together. Most of the interesting bugs in
`cosmos/evm`'s history live at that seam — balance translation, precompile reentrancy,
gas estimation divergence. The MANTRA patch set is *disproportionately* about this seam,
which tells you where to focus review effort.

**Genesis is unforgiving.** Chain ID, denom, prefix, proto namespace, initial supply, and
allocation are all effectively immutable after block 1. Everything in §7-A is in that
category. Your gentx rehearsal tooling with deliberately-bad fixtures is genuinely good
practice — extend the same rigour to the parameters themselves.

**Ecosystem is smaller than Ethereum's.** Fewer auditors have Cosmos SDK depth, fewer
tools understand a chain that is EVM-compatible-but-not-Ethereum, and some infrastructure
(certain oracles, indexers, wallet features) will need custom work. Blockscout works;
assume anything more specialised needs validation.

---

## 9. Practical guides

### 9.1 Build and run a devnet

```bash
# Build the binary (output: build/arkd)
make build

# Docker devnet: 2 validators + 2 sentries + monitoring
# Requires ops/docker/.env with FAUCET_MNEMONIC set
docker compose -f ops/docker/docker-compose.devnet.yml up -d

# pystarport-based devnet (the networks/devnet path)
make devnet-up
make devnet-info        # node status + validator info
make devnet-verify      # block production checks
make devnet-log
make devnet-down
make devnet-clean

# Block explorer
make blockscout-up      # then http://localhost:4000
make blockscout-status
```

Endpoints once up:

| Service | URL |
|---|---|
| CometBFT RPC | `http://localhost:26657` |
| Cosmos LCD/REST | `http://localhost:1317` |
| EVM JSON-RPC | `http://localhost:8545` |
| EVM WebSocket | `ws://localhost:8546` |
| Grafana | `http://localhost:3000` (`admin` / `arkconstellation`) |
| Prometheus | `http://localhost:9092` |
| Alertmanager | `http://localhost:9093` |
| Blockscout | `http://localhost:4000` |
| Faucet | `http://localhost:8088` |

### 9.2 First five commands to sanity-check a running node

```bash
# 1. Is it alive and what height?
curl -s localhost:26657/status | jq '.result.sync_info'

# 2. Is the EVM layer alive? (this is the check your alerting may be missing — §7-B2)
curl -s -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'

# 3. Is the EVM chain ID what you think it is?
curl -s -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}'

# 4. What is the current base fee? (drives ALL tx pricing on this chain)
arkd query feemarket base-fee --node http://localhost:26657 -o json

# 5. Who is validating, and is anyone jailed?
arkd query staking validators -o json | jq '.validators[] | {moniker:.description.moniker, jailed, tokens}'
```

### 9.3 Exercising the emergency brake

The circuit breaker is your most important control. Rehearse it, do not just trust the
proof log.

```bash
# Disable a message type chain-wide (requires circuit authority)
arkd tx circuit disable "/cosmos.bank.v1beta1.MsgSend" --from <authority> ...

# Confirm it is tripped
arkd query circuit disabled-list

# ...attempt a MsgSend, observe rejection at the ante stage...

# Re-enable
arkd tx circuit reset "/cosmos.bank.v1beta1.MsgSend" --from <authority> ...
```

Rehearse with the *real* authority key and the *real* humans, timed. Ask: from the moment
someone notices an exploit, how many minutes until the message type is disabled? If the
answer involves a 7-day governance vote, fix §7-A3 first.

### 9.4 Running the chaos suites

```bash
pip install -r scripts/chaos/requirements.txt
./scripts/chaos/circuit-breaker-test.sh
./scripts/chaos/mempool-flood.sh
./scripts/chaos/rate-limit-test.sh
./scripts/chaos/hard-reboot-sim.sh
./scripts/chaos/validator-failure-sim.sh
# results land in scripts/chaos/reports/
```

### 9.5 Genesis ceremony

```bash
# Validate collected gentxs (boots a throwaway node per file — ~5-8s each)
./scripts/genesis/collect-gentx.sh

# Produce the canonical hash every validator must independently reproduce
./scripts/genesis/hash-genesis.sh
```

The rehearsal fixtures in `scripts/genesis/rehearsal/gentx/` include a tampered signature
and an overclaimed stake, both verified to be rejected. Before the real ceremony, run the
rehearsal end to end with the actual validator cohort, on their own machines — the point
is to test the *humans and their tooling*, not the script.

### 9.6 Working on the Go code

```bash
make test-unit                    # fast
make test-cover
make test-e2e                     # ~35min, builds a docker image first
golangci-lint run                 # uses .golangci.yml — gofumpt/gci will catch import order
make mocks                        # regenerate mocks after changing an interface
```

For the EVM fork, iterate locally without pushing tags:

```bash
# in ArkConstellation/go.mod, temporarily:
replace github.com/cosmos/evm => ../Ark-Evm
```

Remember to revert before committing.

### 9.7 Where to change what

| I want to... | Go to |
|---|---|
| Add or remove a module | `app/app.go` — and *all* of: `maccPerms`, store keys, the four orderings, `app/wasm.go`, `app/queries/queries.go` |
| Add a transaction-level control | `app/ante/cosmos.go` **and** `app/ante/evm.go` — both, always |
| Change a denom or prefix | `app/params/config.go` + `cmd/arkd/main.go` (both — §7-B4) |
| Change chain ID mapping | `app/config.go` `EVMChainIDMap` |
| Change genesis parameters | `networks/*/genesis-params.json` — **not** the binary |
| Expose a Cosmos module to Solidity | `app/precompiles/` — model on `distrclaim` |
| Change EVM execution semantics | the **Ark-Evm** repo, then retag and bump `go.mod`'s replace |

---

## 10. The decision queue

Ordered by how expensive they get if deferred.

| # | Decision | Cost if deferred past genesis | Status |
|---|---|---|---|
| 1 | Proto namespace: keep `mantrachain.*` or rename to `ark.*`? | **Hard fork + every client breaks** | Open (§7-A1) |
| 2 | EVM chain ID uniqueness; delete `mantra-*` and one of the two 11199 entries | **Cross-chain replay** | Open (§7-A2) |
| 3 | Circuit-breaker authority: who, how fast, rehearsed? | Cannot respond to an incident | ⏳ Pending (decision #17) |
| 4 | Sanction posture: "cannot transact" or "funds frozen"? | Compliance claim you cannot back | Undocumented (§7-B5) |
| 5 | Revoke the leaked PAT | Compromise window stays open | **Today** (§7-C1) |
| 6 | Revert/replace the Ark-Evm JSON-RPC commit | Silent RPC outages in production | Open (§6.2) |
| 7 | De-fork strategy for wasmd + connect (audit or replace) | Stranded on unmaintained deps | Not started (§8.2) |
| 8 | Faucet: cap the account, fix rate limiting, **commit the code** | Testnet drained; unreviewed code | Uncommitted (§7-C2) |
| 9 | Genesis allocation & vesting proposal → locked | Supply is immutable after block 1 | Drafted, needs sign-off |
| 10 | Precompile enablement — sign off, do not rubber-stamp | Each one is permanent attack surface | Recommended, not approved |
| 11 | Governance timelock duration (decision #14) | — | 🔒 Required, duration TBD |
| 12 | Independent external audit of the EIP-7623 change | Consensus bug in the gas path | Not scheduled (§6.2) |

**If you do only three things this week:** revoke the token (§7-C1), settle the proto
namespace question (§7-A1), and decide the circuit-breaker authority (§7-A3). Everything
else has a longer fuse.

---

## Appendix A — Command reference used to derive this document

Reproduce or extend the analysis:

```bash
# ArkConstellation: your commits vs upstream MANTRA
git log --oneline --no-merges main..base-genesis
git diff --stat main...base-genesis

# Ark-Evm: your commit vs MANTRA's fork
cd ../Ark-Evm
git diff mantra/v0.6.x...base-genesis

# Ark-Evm: MANTRA's fork vs cosmos/evm upstream
git log --oneline --no-merges origin/main..mantra/v0.6.x

# What is actually compiled (replace directives override require lines)
grep -A40 '^replace' go.mod

# Secret sweep
git log --all -S'ghp_' --oneline
grep -rn "mantra1[a-z0-9]\{30,\}" .     # residual MANTRA production addresses
grep -rn "_ = " --include="*.go" app/ x/ | grep -v _test.go   # swallowed errors
```

## Appendix B — Your team's own documents, and how much to trust them

These are good and you should read them; this document deliberately does not duplicate
them.

| Document | What it is | Trust |
|---|---|---|
| `docs/proof/fork-audit-cosmos-evm.md` | 3-way diff of cosmos/evm v0.6.0 → v0.6.2 → MANTRA fork, categorised | **High.** Methodologically sound, verifies claims against source. Only stale detail: written against the `v8-mantra-1` tag, you now pin `ark-1`. |
| `docs/proof/fork-audit-cosmos-sdk.md` | Same treatment for the SDK fork | High |
| `docs/decisions/module-and-config-decisions.md` | Authoritative locked-parameter record | **High, and it self-corrects** — it caught its own 100× slashing-fraction error by checking the vendored source. That is the behaviour you want. |
| `GAPS.md` | Honest punch list | High. Notably refuses to fabricate a sign-off it does not have. Under-rates the proto-namespace item (§7-A1). |
| `STATUS.md` | Narrative of what was done and verified | High. One reconciliation needed: it states `MinGasPrices` default `0esp`, ops uses `0.01esp` (§7-B1). |
| `networks/*/README.md`, `RUNBOOK.md` | Operational procedures | Verified against real runs with committed evidence logs |
| `scripts/chaos/reports/` | Raw gosec/semgrep/slither output + run results | Raw tool output — triage still required; findings are not pre-filtered for false positives |

The general standard of documentation here is above average for a chain at this stage,
and the habit of committing *evidence* (proof logs, transcripts, rejected-gentx fixtures)
rather than assertions is the single best process signal in the repo. Keep it.
