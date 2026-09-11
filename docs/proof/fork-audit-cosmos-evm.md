# Fork Audit: MANTRA-Chain/evm v0.6.2-v8-mantra-1 vs cosmos/evm

> ⚠️ **Stale header, re-pinned since this audit ran (noted 2026-09-11, issue #37).**
> `go.mod` no longer says what the two bullets below say. The current state is:
>
> - `go.mod:14`: `replace github.com/cosmos/evm => github.com/Worldstreet-Web-Services/evm v0.6.2-ark-1`
> - `go.mod:67`: `github.com/cosmos/evm v0.6.2` — the stale nominal `v0.6.0` flagged
>   below has since been corrected, so that preliminary finding is resolved.
>
> `Worldstreet-Web-Services/evm v0.6.2-ark-1` is Ark's re-tag of the same patch set
> this audit examined, so **the findings below still apply** — but the artifact named
> in the title is not the one Ark builds, and a future re-audit should diff against
> the `-ark-1` tag directly. That repo is public, so it can be read without fork
> access. The audit also predates the `0x…0803` vesting finding (#37), which is an
> inherited upstream defect rather than fork drift and so would not have shown up in
> a fork-vs-upstream diff at all.

## Setup verification

- ArkConstellation's `go.mod` line 67: `github.com/cosmos/evm v0.6.0` (the nominal/declared upstream version)
- ArkConstellation's `go.mod` line 14: `replace github.com/cosmos/evm => github.com/MANTRA-Chain/evm v0.6.2-v8-mantra-1` (what actually gets compiled)

**Important preliminary finding:** the `require` line is stale/misleading. The code ArkConstellation actually builds is not based on upstream `v0.6.0` — the replace tag `v0.6.2-v8-mantra-1` is built on top of upstream **v0.6.2** (confirmed: `git describe --tags` on the fork's `mantra/v8-mantra` line resolves cleanly against v0.6.2's tree with only MANTRA's own patches on top). This matters because most of the "drift" from v0.6.0 that a naive diff shows is just legitimate upstream improvement between v0.6.0→v0.6.2, not MANTRA authorship. I did a proper 3-way comparison to separate the two:

- **cosmos/evm@v0.6.0** (nominal version) — shallow-cloned, verified via `git describe`
- **cosmos/evm@v0.6.2** (real base) — shallow-cloned, verified via `git describe`
- **MANTRA-Chain/evm@v0.6.2-v8-mantra-1** (fork) — shallow-cloned, verified via `git describe`

Diff v0.6.0→v0.6.2: 34 files (13 real upstream commits, confirmed via GitHub compare API, `ahead_by:13, behind_by:0`) — all inherited "for free."
Diff v0.6.2→fork: **42 files, ~1,123 diff lines** — this is the true MANTRA-authored patch set, and what the categorization below is based on. (For reference, the naive fork-vs-v0.6.0 diff is 65 files / 7,277 lines — most of that bulk is just the v0.6.0→v0.6.2 upstream delta, not MANTRA's doing.)

---

## 1. SAFE TO KEEP

- `server/config/config.go`, `flags.go`, `toml.go`, `json_rpc.go`, `migration/v0.50-app.toml` — adds `JSONRPC.HTTPBodyLimit` (default 5MB) to cap JSON-RPC HTTP request body size, with validation, CLI flag, and TOML template support. A genuine DoS-hardening addition, not present upstream.
- `server/start.go` — deletes MANTRA/upstream's hand-rolled `startGrpcServer()` (~65 lines) and calls the SDK's own `server.StartGrpcServer` instead. Pure de-duplication, no behavior loss.
- `server/config/config.go` — propagates `GRPCConfig.HistoricalGRPCAddressBlockRange` from the SDK's own config loader into evm's `Config`, fixing a field that was silently dropped by evm's custom config loader.
- `evmd/tests/network/util.go` — test-network harness updated to use `servergrpc.NewGRPCServerAndContext` (matches the `server/start.go` cleanup above). Test-only.
- `x/vm/types/interfaces.go`, `x/vm/wrappers/testutil/mock.go`, `x/vm/types/mocks/BankKeeper.go`, `x/precisebank/types/mocks/MockBankKeeper.go` — add `BlockedAddr(addr)` to the `BankKeeper` interface + regenerate mocks. This is pure plumbing required by the statedb security fix below (category 2), not an independent change.
- `testutil/integration/params.go`, `evmd/testutil/eth_setup.go` — test-harness `ConsensusParams.Block.MaxGas` changed from `-1` (unlimited) to `80_000_000`, made realistic for test runs.
- `tests/integration/precompiles/{gov,werc20}/*`, `tests/integration/x/vm/test_call_evm.go` — test-only gas-limit adjustments (from absurd `1e9`–`1e12` down to `50_000_000`) to fit the new finite `MaxGas`, plus one new test-only `fromFn` helper param. No production impact.
- `encoding/address/address_codec_test.go` — trivial local-variable extraction, no behavior change.
- `precompiles/ics20/tx_callback_guard_test.go`, `x/ibc/callbacks/types/context_test.go`, `x/vm/keeper/grpc_query_test.go` — new test files exercising the fixes below.
- `go.mod`/`go.sum` (root, `evmd/`, `tests/systemtests/`) — routine transitive bump of `bytedance/sonic` v1.14.2→v1.15.0. Unrelated to any of the above.
- `CHANGELOG.md` — documents the ICS20 fix (references upstream PR #1061, see below).

**No branding/vanity glue found.** I specifically grepped every `.go` file in the fork for "mantra" — zero hits. Module path is untouched (`module github.com/cosmos/evm`, not renamed), so this is a clean drop-in replace-directive fork. The only "Mantra" text anywhere is in `README.md`, and that text is verbatim identical to upstream v0.6.2's own README (upstream's README credits Mantra, Ondo, Mezo etc. as contributors/adopters — it's not MANTRA-injected).

## 2. NEEDS REVIEW (consensus / security / EVM-semantics relevant)

1. **`precompiles/ics20/tx.go` + new `x/ibc/callbacks/types/context.go`** — adds a reentrancy guard: `Transfer()` on the ICS20 precompile now rejects calls when a context marker `IsSourceCallbackExecution` is set (set by `x/ibc/callbacks/keeper/keeper.go` around IBC-callback EVM execution). **This is the concrete follow-up hardening for CRITICAL advisory GHSA-54gx-3cgr-7mfm / ASA-2026-002** (nested/reentrant ICS20 precompile execution → the $7M Saga exploit, patched at v0.6.0). I traced this to real upstream PR **cosmos/evm#1061** ("fix(ibc): block nested ICS20 forwarding in src callbacks", merged to `main` 2026-04-27, PR body explicitly references MANTRA's own e2e test suite as validation). **Verified this fix is absent from v0.6.1, v0.6.2 (published 2026-08-19, "important security fixes"!), and even v0.7.0 — it only ships upstream starting at v0.7.1/v0.7.2/v1.0.0-rc.** MANTRA backported it into their v0.6.x-line fork themselves. Net effect: correctness-positive, but it's new attack-surface-reducing logic that should be regression-tested against any of ArkConstellation's own IBC-callback contracts that might legitimately chain a source-callback into a transfer.

2. **`x/vm/keeper/statedb.go` (`SetBalanceWithLocked`) + `rpc/types/utils.go` + `rpc/backend/comet_to_eth.go`** — a linked pair of changes. The balance-write guard that mints/burns bank coins to match EVM-computed balances previously only blocked writes to accounts already typed `ModuleAccountI`. MANTRA extends it to also block writes to any bank-`BlockedAddr` (unless the write is a no-op re-affirming the current balance), closing a gap where a reserved/blocked address without an instantiated account object could have funds force-set via a plain EVM value transfer. In lockstep, the RPC layer's special-case tolerance for the resulting `"failed to commit stateDB"` error (`StateDBCommitError`/`TxStateDBCommitError`, previously treated as an "expected failure" in block/tx-status scanning) was **removed**. These three files must be adopted or reverted together — cherry-picking the statedb hardening without the RPC change (or vice versa) will reintroduce confusing tx-status behavior.

3. **`x/vm/keeper/state_transition.go`** — three distinct changes bundled in one file:
   - **(a)** Removes a real duplicate-event-emission bug: `ApplyTransaction` was emitting `PostTxProcessing` hook events into `ctx.EventManager()` twice — once implicitly via `commitFn()` (cosmos-sdk's `CacheContext().writeCache` already does `ctx.EventManager().EmitEvents(cc.Events())`, confirmed by reading SDK v0.53.6 source), and once again via an explicit manual `events[eventsLen:]` copy. **This changes on-chain event output** for any tx that goes through PostTxProcessing hooks (e.g. ERC-20 conversion hooks) — indexers/explorers built against vanilla v0.6.x's double-emission behavior will see fewer/different events on MANTRA's chain. Correct fix, but flag it for anyone building event-dependent infra.
   - **(b)** State-override paths (`eth_call`/`eth_estimateGas` with overrides) now also register the chain's active *static* precompiles (`params.ActiveStaticPrecompiles`), not just go-ethereum's base set — otherwise overrides could silently disable custom Cosmos-EVM precompiles for that simulated call. **Verified byte-identical to upstream v0.7.2's fix** — a genuine backport.
   - **(c)** Adds EIP-7623 calldata-floor enforcement to the *post-refund gas actually charged* (`gasUsed = max(gasUsed, floorDataGas)` after refunds), on top of the pre-existing upfront `GasLimit < floorDataGas` intrinsic check. **This one I could NOT find in upstream v0.7.2 or v1.0.0-rc2 — it appears to be MANTRA-original**, unreviewed by upstream. It's a plausible/correct EIP-7623 compliance fix, but as a novel change to consensus-relevant gas accounting on the Prague path, it deserves the most independent scrutiny of anything in this diff — test refund-heavy, large-calldata transactions specifically.

4. **`x/vm/keeper/precompiles.go` (`GetPrecompileRecipientCallHook`)** — fixes a real bug: this hook is the *default* call-hook used for every normal transaction (`GetPrecompilesCallHook` is only used on the override path). Upstream v0.6.0/v0.6.2's version resolved dynamic (governance-registered) ERC20 precompiles reached via an internal `CALL` but never actually called `evm.WithPrecompiles(...)` to install them — meaning contract-to-contract calls into a dynamically-registered ERC20 precompile that wasn't already pre-loaded could silently fail to execute the precompile logic. **Verified identical (including the doc-comment) to upstream v0.7.2's fix** — another backport, this time of core EVM call-dispatch semantics. Worth regression-testing router/aggregator-style contracts that call into dynamic ERC20 precompiles.

5. **`x/vm/keeper/call_evm.go` (`CallEVM`/`CallEVMWithData`)** — the `gasCap *big.Int` parameter existed in the function signature already but was completely ignored; every internal EVM call (IBC-callback `approve`/calldata dispatch passing `remainingGas`, erc20 module's decimals/balanceOf/transfer calls) unconditionally got `config.DefaultGasCap` regardless of caller intent. Now honored as `min(gasCap, DefaultGasCap)`. Changes effective gas-limit semantics for module-internal calls, most visibly IBC-callback-triggered contract execution.

6. **`x/vm/keeper/block_proposer.go` (`GetCoinbaseAddress`)** — on failed validator lookup, changed from returning a wrapped error (which, since this runs inside `EVMConfig` called on essentially every tx, would hard-fail EVM processing for the block) to silently returning the zero address. A defensible liveness-over-strictness trade-off, but it converts a per-tx consensus-relevant hard failure into a silent one — should be a deliberate, acknowledged choice by ArkConstellation, not an inherited surprise.

7. **`x/vm/keeper/grpc_query.go` (`EthCall`)** — wraps context via `evmante.BuildEvmExecutionCtx(...)` before simulation, to align `eth_call`/`eth_estimateGas` KV-store gas accounting for precompile-internal writes with real `DeliverTx` behavior. Legitimate simulation-accuracy fix, but it changes gas-estimation output for txs touching native precompiles (previously over-estimated).

*(Also worth knowing, though it's inherited from upstream v0.6.2 rather than MANTRA-authored: `x/vm/statedb/state_object.go`'s `SubBalance` gained an explicit panic on balance underflow — upstream v0.6.0 would have silently wrapped a `uint256` subtraction that exceeds current balance, which is a classic mint-from-nothing bug class. MANTRA's fork has this protection purely by virtue of tracking v0.6.2 instead of bare v0.6.0.)*

## 3. CLEARLY MANTRA-SPECIFIC AND REMOVABLE

**Essentially empty.** After walking every one of the 42 changed files in the true (v0.6.2-based) diff, I found no MANTRA-only branding, hardcoded chain-id checks, dead glue code, or vanity modifications that would need stripping before another chain builds on this. Everything in the diff is either a genuine bug/security fix, a config/ops addition, or test-scaffolding. This is a positive signal about the fork's hygiene — but also means there's no easy "delete the MANTRA cruft" step; every change needs the review treatment above rather than a blanket removal.

---

## Is the fork ahead or behind on security fixes?

**On every *disclosed* advisory: not behind.** GitHub's security-advisories API lists exactly 3 published advisories for cosmos/evm, all with patched versions ≤ v0.6.0 (GHSA-54gx-3cgr-7mfm/ASA-2026-002 critical ICS20 nested-execution bug patched at v0.6.0; GHSA-8pfh-j44r-f654 patched at v0.3.2/v0.4.2/v0.5.0; GHSA-mjfq-3qr2-6g84 patched pre-v0.6.0 evmos-era). The fork's v0.6.2 base postdates all of them.

**On top of that, the fork is measurably *ahead*** of both nominal v0.6.0 and its actual v0.6.2 base in a security-relevant, verifiable way: it backports the ICS20 source-callback reentrancy guard (upstream PR #1061) that upstream itself has **only shipped starting at v0.7.1/v1.0.0-rc — not in v0.6.2**, which was published just 3 days before this audit as a dedicated "important security fixes" release for the v0.6.x line and still doesn't contain it. It also backports two other genuine upstream v0.7.2 correctness fixes (state-override precompile registration, and the default-path dynamic-precompile dispatch bug) that likewise never made it into any v0.6.x point release. One further change (the EIP-7623 post-refund floor) appears to be MANTRA-original and not present anywhere upstream through v1.0.0-rc2 — this is the one item in the whole diff that hasn't had the benefit of upstream's own review, and is the single item I'd most want independently tested before trusting it in production.

**Where the fork is "behind":** only in the trivial, expected sense that it doesn't carry the unrelated v0.7.x/v1.0.0 feature and refactor work (SDK/ibc-go v10 migration, module renames, tracing instrumentation, `x/precisebank` improvements beyond what v0.6.2 has, etc.) — none of which is advisory-driven, and adopting it would mean a major, state-breaking migration off the v0.6.x line entirely. That's a roadmap decision, not a security gap.

## Verdict

This fork is a lean, well-targeted patch set on top of a real, current upstream base (v0.6.2, not the stale v0.6.0 that go.mod's `require` line nominally claims — that require line should be corrected to avoid confusing tooling/auditors) — it contains no MANTRA branding or throwaway glue code, and every substantive change I found is either a config/ops hardening addition or a genuine bug fix, several of which are verifiably backports of upstream's own later fixes (including the specific reentrancy hardening for the critical ICS20 advisory that struck Saga, which upstream still hasn't backported to its own v0.6.x release line as of days ago). It is safe to build a different chain's EVM execution on top of, with three concrete caveats before doing so: (1) treat the EIP-7623 post-refund gas-floor change as unreviewed by upstream and test it specifically against Prague-rules refund-heavy transactions; (2) understand that on-chain event output, `eth_call`/`eth_estimateGas` gas estimates, and behavior toward "blocked" addresses have all changed versus both vanilla v0.6.0 and v0.6.2 — anything (indexers, wallets, bridges) built assuming vanilla behavior needs to be re-validated against these specific diffs rather than assumed compatible; and (3) fix the go.mod `require` line's stale version number so it doesn't mislead future auditors or tooling about what's actually running.