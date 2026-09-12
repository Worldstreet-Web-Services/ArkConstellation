# MANTRA Cosmos SDK Fork Audit vs Upstream v0.53.x

## Methodology
- The original audit confirmed ArkConstellation's then-current pin as `github.com/cosmos/cosmos-sdk => github.com/MANTRA-Chain/cosmos-sdk v0.53.6-v8-mantra-1`, commit `eca3f45af56019d6e184b795b8216c447040f84c`, branch `mantra/v0.53.6`. See the remediation record below for the current pin.
- Shallow-cloned `MANTRA-Chain/cosmos-sdk` at tag `v0.53.6-v8-mantra-1` and `cosmos/cosmos-sdk` at tag `v0.53.6` (upstream has an **exact** matching tag, so this is a precise diff, not an approximation).
- Ran a real recursive `diff -rq` across both trees, then full unified diffs on every differing file.
- Separately diffed upstream `v0.53.6..v0.53.8` (latest released v0.53.x) and `v0.53.8..origin/release/v0.53.x` (unreleased tip) to answer the drift/security question.

### Re-audit at the remediated pin (`v0.53.8-v8-mantra-1`)
After the advisory remediation re-pinned the SDK, the diff was **re-run against the new tag** so the audit covers the artifact that is actually built:
- Shallow-cloned `MANTRA-Chain/cosmos-sdk` at tag `v0.53.8-v8-mantra-1` and `cosmos/cosmos-sdk` at tag `v0.53.8` (again an exact matching upstream tag, so a precise diff).
- Confirmed the fork tag resolves to commit `58cc2eb4a66fcab2359a69d262b984958c8d9273`, matching both the `go.mod` replace directive and the `cosmossdk.io/api` pseudo-version `v0.0.0-20260729035606-58cc2eb4a66f`.
- Ran a full recursive `diff -rq` across both trees: **44 differing entries**, of which the non-generated Go source differences are exactly `x/bank/keeper/{keeper,send}.go`, `x/bank/types/expected_keepers.go`, `x/mint/{keeper/migrator.go,keeper/mint.go,module.go,simulation/genesis.go,types/expected_keepers.go,types/params.go}`, `x/auth/tx/query.go`, `x/gov/types/config.go`, and `tests/e2e/mint/grpc.go`.
- **The divergence set is unchanged from the v0.53.6 audit** — the same two functional features plus the same query-path change and typo fix. No new divergence was introduced by the fork's rebase onto v0.53.8.
- Confirmed **no divergence** in `crypto/`, ante handlers, `baseapp`, `x/staking`, `x/distribution`, `x/slashing`, or `x/evidence` — the consensus-critical core outside bank/mint remains untouched at the new tag.
- Verified the MANTRA fork features survive the bump: `x/bank/types/hooks.go`, `SendCoinsWithoutBlockHook` (`x/bank/keeper/send.go:251`), `x/mint` `MaxSupply` (`x/mint/types/params.go`), and `RegisterMigration(..., 2, m.Migrate2to3)` (`x/mint/module.go:131`) are all present.
- Re-confirmed `x/auth/tx/query.go`'s `formatTxResults` still carries the silent-drop behavior described in §2 (upstream returns an error; the fork appends only successfully-decoded results).

### CometBFT bump carried by this re-pin
The SDK re-pin also moved CometBFT from the pseudo-version `v0.38.23-0.20260422215035-4928b26fd5ba` to the released tag `v0.38.23`. Verified this is a **strict fast-forward, not a downgrade**: commit `4928b26fd5ba` is an ancestor of `v0.38.23` (5 commits ahead, 0 behind). The intervening commits are:
- `20818d7d2` — `fix(light): stop witness comparison after divergence checks` (light-client detector).
- `5acce5a40` — `fix(abci): prevent panic on unlock in socket server panic recovery`.
- `a82d977a1` — test-only flaky-test fix; plus two changelog commits.

Both functional commits are bug fixes in the safe direction for a consensus node; no regression.

## Scale
The fork touches **~40 files total**, and it is *not* a sprawling fork — it clusters into exactly three functional changes plus repo/CI housekeeping:
1. A new **bank module "BeforeSend hooks"** system (Osmosis-style `TrackBeforeSend`/`BlockBeforeSend`) — 9 files.
2. A new **mint module `MaxSupply` param** with a real v2→v3 state migration — 17 files (incl. proto + generated pb code + tests).
3. A **non-consensus query-path behavior change** in `x/auth/tx/query.go`'s `formatTxResults` (see §2) — 1 file.
4. Everything else (~13 files) is CI/GitHub metadata, one comment typo, and mechanically-generated diffs from the above features (go.sum, api pulsar code, test proto reformatting).

No changes were found in staking, slashing, distribution, gov, evidence, upgrade, ante handlers, or crypto — the consensus-critical core outside bank/mint is untouched.

---

## 1. SAFE TO KEEP
- **`x/bank` hooks are opt-in/no-op by default** — `k.hooks == nil` short-circuits `TrackBeforeSend`/`BlockBeforeSend` to no-ops in `x/bank/keeper/hooks.go`; `SetHooks` panics if called twice, so nothing is silently double-wired.
- `x/bank/testutil/expected_keepers_mocks.go`, `x/mint/testutil/expected_keepers_mocks.go` — mechanically-generated mock additions matching the interface changes.
- `x/mint/keeper/migrator.go` + new `x/mint/migrations/v3/migrate.go` (+ test) — a correctly-implemented consensus-version migration (bumps `ConsensusVersion` 2→3, registers `Migrate2to3`, sets `MaxSupply=0` on old state) — this is the right way to introduce a new param, not a shortcut.
- `x/gov/types/config.go` — pure typo fix (`intialising`→`initializing`), zero behavior change.
- `.goreleaser.yml` goreleaser-action bump v3→v6 in `release.yml` — generic CI hygiene, harmless.
- `x/mint/README.md`, `tools/benchmark/CHANGELOG.md` — documentation of the real `MaxSupply` feature (not branding).
- `x/bank/keeper_test.go` — single trailing-comma gofmt-style diff, no semantic change.
- `go.mod`/`go.sum`/`tests/go.mod`/`tests/go.sum` — add a `cosmossdk.io/api => ./api` local replace so the fork can build its own modified `api` submodule; mechanically necessary, not a landmine by itself (see caveat below).

## 2. NEEDS REVIEW
- **`x/mint` `MaxSupply` param** (`x/mint/types/params.go`, `mint.go`, `mint.proto`, `mint.pb.go`, `module.go`): adds a 7th proto field and caps `BeginBlocker` minting once total supply would exceed `MaxSupply` (0 = infinite, so default-safe). This is a genuine **consensus-rule and wire-format change** to a stock module — a different chain built on this fork inherits a non-standard `x/mint` proto/state layout that diverges from vanilla cosmos-sdk. Any tooling/explorer that assumes the canonical 6-field mint params, or genesis-migration scripts written against stock SDK, needs to account for this field. ArkConstellation must decide whether it wants this feature and must ensure its own module manager correctly wires `RegisterMigration(..., 2, m.Migrate2to3)` if upgrading from an older mint state.
- **`x/bank` BeforeSend hooks architecture** (`x/bank/keeper/send.go`, `keeper.go`, `types/hooks.go`): inserts `BlockBeforeSend`/`TrackBeforeSend` calls into the hot path of `SendCoins`, `DelegateCoins`, `UndelegateCoins`, `InputOutputCoins`. Enforcement is **asymmetric by design**: `SendCoinsFromModuleToModule` explicitly bypasses `BlockBeforeSend` via a new `SendCoinsWithoutBlockHook`, while account↔module and account↔account paths always enforce it (confirmed against the new `TestHooks` test in `x/bank/app_test.go`). This is exactly the plumbing needed for a KYC/AML-style transfer blocklist (consistent with MANTRA's "regulatory chain" positioning) — inert today, but it's new attack surface/extension point in the money-movement code that a different chain's own modules could hook into, intentionally or by mistake. Worth an explicit check that ArkConstellation's own `app.go`/module wiring never calls `SetHooks` unless intended.
- **`x/auth/tx/query.go` `formatTxResults`**: upstream returns an error and aborts the whole query if *any* indexed tx fails to decode. The fork instead **silently drops** the undecodable tx from the result slice and returns the rest with no error. This is a real API-behavior change to `GetTxsEvent`/tx-search RPC — result-set length no longer 1:1 with matched txs, and decode failures (which could indicate indexing corruption) are now swallowed rather than surfaced. Not consensus-breaking (query-path only) but worth a deliberate decision, not an accident.
- **Two-module lockstep risk**: the fork's `go.mod` adds `replace cosmossdk.io/api => ./api` (only takes effect when cosmos-sdk itself is the main module). ArkConstellation's root and interchain modules now pin `cosmossdk.io/api` to `v0.0.0-20260729035606-58cc2eb4a66f`, matching the SDK tag's commit. Any future SDK re-pin must update both API replacements too, or generated pulsar code (mint, auth module config) can silently drift out of sync with the SDK proto. `scripts/check-sdk-api-lockstep.sh` (wired into `make lint-all` and the `lint` CI workflow) fails the build if the two pins ever diverge.

## 3. CLEARLY MANTRA-SPECIFIC AND REMOVABLE
- `.github/CODEOWNERS` — fully rewritten to MANTRA team handles (`@devops-admins-team`, `@development-team-blockchain`, etc.).
- `.github/dependabot.yml` — adds an npm/docs watcher assigned to `g-mantra` (a MANTRA individual).
- `.github/workflows/release.yml` — tag-matching regex hardcoded to `v[0-9]+.[0-9]+.[0-9]+-v[0-9]+-mantra-1`; Slack bot/channel branding genericized via `github.event.repository.name` (fine to keep the genericization, but the workflow itself only matters for cutting MANTRA's own GitHub releases of this repo).
- `.goreleaser.yml` — `owner: cosmos` → `owner: MANTRA-Chain` (GitHub release target).
- `.github/ISSUE_TEMPLATE/*` — upstream's `bug-report.md`, `epics.md`, `feature-request.md`, `module-readiness-checklist.md` deleted; replaced with MANTRA's own `standard-issue.yml`. Pure repo hygiene, zero code impact.
- Reworded Slack `SLACK_TITLE`/`SLACK_MESSAGE` strings referencing MANTRA infra — cosmetic only.

*(Noise, not really a category: `client/v2/internal/testpbgogo|testpbpulsar/msg.proto` and the `auth/module.pulsar.go` comment diff are trivial formatting/wording drift from whatever `buf format`/proto-gen version MANTRA's CI used — no functional content.)*

---

## Version drift / missing security fixes — REMEDIATED

**Remediated 2026-09-01.** ArkConstellation now pins the immutable
`github.com/MANTRA-Chain/cosmos-sdk v0.53.8-v8-mantra-1` tag at commit
`58cc2eb4a66fcab2359a69d262b984958c8d9273`. The matching
`cosmossdk.io/api` submodule is pinned in lockstep to
`v0.0.0-20260729035606-58cc2eb4a66f`, whose origin metadata resolves to the
same commit and the `api` subdirectory.

The fork update was merged upstream in
[`MANTRA-Chain/cosmos-sdk#357`](https://github.com/MANTRA-Chain/cosmos-sdk/pull/357).
Comparing `v0.53.6-v8-mantra-1...v0.53.8-v8-mantra-1` shows the complete
upstream v0.53.7/v0.53.8 backport sequence followed by the two MANTRA feature
commits. This preserves the bank pre-send hooks and mint `MaxSupply` behavior
while incorporating all fixes listed below.

### Historical finding

Upstream has **shipped two more v0.53.x patch releases past the fork's pin** (v0.53.7 on 2026-04-14, v0.53.8 on 2026-07-27 — 32 commits total), and the `release/v0.53.x` branch has additional unreleased commits beyond that. I confirmed directly (byte-diff) that the fork's `crypto/keys/secp256k1/secp256k1.go` and `x/auth/tx/sigs.go` are **identical to the v0.53.6 baseline** — none of these fixes were cherry-picked in independently.

Security/robustness fixes the fork was missing (all landed after v0.53.6 and now included by the v0.53.8 fork tag):
- **`fix(crypto): validate secp256k1 pubkey SEC1 tag byte`** (#26529/#26665) — previously a malformed/invalid compressed pubkey with a bad leading byte passed the length-only check.
- **`fix: bound compact bit array index by elems length`** (#26509/#26662) — out-of-range panic on malformed multisig bit arrays.
- **`fix: bound multisig signature and pubkey indexing by slice lengths`** (#26515) — `ConsumeMultisignatureVerificationGas`/`VerifyMultisignature` could panic with index-out-of-range on a crafted multisig.
- **`fix(x/auth/ante): reject tx with extra SignerInfos in SetPubKeyDecorator`** (#26573).
- **`fix: reject tx with mismatched signer info and signature counts`** (#26517) — decode error instead of panic when `SignerInfos`/`Signatures` counts disagree.
- **`fix(x/auth/tx): avoid nil pointer panic in GetSigningTxData`** (#26527, #26571) — nil `PublicKey`/nil multisig `Bitarray` crashed nodes.
- **`fix(x/distribution): return errors for missing historical rewards internally`** (#26518) — previously silently decoded absent records as zero, risking incorrect reward math.
- **`fix(x/distribution): fallback paths for withdrawing to blocked addresses`** (#26406) — prevents `Begin/EndBlocker` halting on a blocked-address edge case.
- **`fix(x/staking): handle redelegation when unbonded source validator is removed`** (#26408).
- **`fix(store/cachemulti): isolate traceContext map across branched stores`** (#25841) — concurrent map-write race.
- **`chore(v53 backport): delete proposal using proposal key`** (#26034, landed pre-v0.53.7) — fixes a real gov `EndBlocker` bug where the original code `return nil`'d out of the inactive-proposals loop after the *first* encoding-error proposal, silently skipping processing of every other proposal in that block.
- CometBFT bumped 0.38.21 → 0.38.23 in the v0.53.7/8 line, and further to 0.38.26 on the unreleased `release/v0.53.x` tip — worth checking CometBFT's own changelog for what those cover.

Most of these are **panic/DoS-class bugs reachable via a crafted transaction or malformed signature data** — exactly the class of issue that matters most for a new chain's validator set. I checked the GitHub Security Advisories page for `cosmos/cosmos-sdk` directly: none of these specific fixes have a published formal GHSA/ISA/ASA advisory number (they appear to be proactive backports, likely from fuzzing/audit work, not full public disclosures), and the one formal advisory in range (**ISA-2025-005, Integer Overflow, GHSA-p22h-3m2v-cmgh**, affecting ≤v0.50.13 and ≤v0.53.2) is **already patched** in v0.53.6, so that one is not a gap. The unpublished-advisory backports above were real gaps **at the pre-remediation v0.53.6 pin** — the v0.53.8 fork tag now pinned (see the REMEDIATED section above) includes all of them.

---

## Verdict

This fork is well-scoped: its functional differences from upstream are confined to three documented changes — two tested consensus-path features (bank pre-send hooks that are a no-op unless wired, and a mint max-supply cap with a proper migration) plus one **non-consensus query-path** behavior change (`x/auth/tx/query.go`'s `formatTxResults` silently drops undecodable txs where upstream returns an error; see §2) — plus repository/CI customization. ArkConstellation has now completed the most important original action item by moving to the v0.53.8 fork tag containing the upstream panic/DoS and correctness fixes. The remaining governance items are to strip or replace MANTRA-specific repository metadata and to keep the `MaxSupply` and bank-hook semantics as explicit product decisions.
