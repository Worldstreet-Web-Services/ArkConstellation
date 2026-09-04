# GAPS — Eng 1 (State Machine) track

What's still open, blocking, or needs a follow-up decision. See `STATUS.md` for the full narrative of what was done and verified; this file is the punch list.

## Blocking Day 2/3 progression

- **Eng 3 has published nothing yet** (checked directly: no `track/3-security-chaos` branch, no PR). `ark-v1.0.0-rc1` should not be cut with a fabricated or simulated chaos sign-off — wait for real input, even a preliminary "nothing found yet" note.
- **`ark-v1.0.0` (final) cannot honestly be cut** until the above closes AND `rc1` itself is tagged. Day 3's reproducible-build/checksum work can start independently of this (it doesn't depend on Eng 3), but the *tag* should wait.

## Decisions made this session that need explicit sign-off, not silent adoption

- ~~Mainnet EVM chain-id proposed as `ark_9001-1`~~ — **superseded.** `docs/decisions/module-and-config-decisions.md` #7 locks mainnet EVM chain-id to `11199` (Cosmos chain-id `arkconstellation-1`, #6). This is already correctly wired in code, not just documented: `app/config.go`'s `EVMChainIDMap["arkconstellation-1"] = 11199`, resolved automatically at node startup from the genesis chain-id (`init()` in that file) — no manual `app.toml` step needed for a standard mainnet node. Devnet's `arkdevnet_9000-1` maps to EVM chain-id `9000`, also already present in that same map.
- **Precompile audit recommends enabling all 10** (see `STATUS.md`'s table) — reasoned, but review-and-agree, not rubber-stamp, especially Staking and ICS20 (flagged for Eng 3 chaos coverage specifically).
- ~~**`skip-mev/feemarket` config kept at vendored defaults** rather than tuned — reasoned against the gasless/Paymaster goal, but never load-tested against real Paymaster relay traffic patterns.~~ **RESOLVED**: Paymaster infrastructure now implemented on branch `account-abstraction` with minimal EntryPoint (6628 bytes) and SimplePaymaster. See STATUS.md "Gas Sponsorship (Paymaster) Infrastructure" section.

## Explicitly flagged, not fixed (out of this track's asked-for scope)

- **`app/token_pair.go`'s `WTokenContractMainnet`** — resolved (SEC-01). Upstream MANTRA mainnet address (`0xD494...`) replaced with zero address (`0x0000...`) to ensure no legacy addresses persist in test fixtures or DefaultGenesis paths.
- **Full README/docs rebrand** — only the `README.md` "Modules" section was corrected (it described the three now-removed modules as present, which would have been actively misleading). No attempt at a full MANTRA→Ark rebrand of the rest of `README.md`/`CONTRIBUTING.md`, since this branch was cut from `main` (pure upstream MANTRA content, no Ark branding at all yet) rather than `base-genesis` (which already has an Ark-rebranded README from a separate commit) — reconciling these when this branch merges is a real integration step, not automatic.
- **Go module path / binary name / proto namespace** (`github.com/MANTRA-Chain/mantrachain`, `mantrachaind`, `mantrachain.*` proto packages) — deliberately not renamed alongside bech32/denom. Same "MANTRA branding visible in Ark's own identity" category, but a much larger, more disruptive change (touches every import statement, every deployed script/service assuming the binary name) that wasn't explicitly asked for. Flag as a separate decision if wanted — don't assume it's implied by the bech32/denom rename.

## Real findings from this session worth carrying forward

- **`app/genesis.go`'s dead `NewDefaultGenesisState()`** was removed rather than fixed-and-wired-in — see `STATUS.md` for the full reasoning (it was also broken, would have panicked on an empty erc20 TokenPairs slice). If a future session wants genesis customization baked into the binary itself rather than the deployment-time patch process, that's a deliberate re-decision, not a revert of a bug fix.
- **Hardcoded real MANTRA mainnet address in `x/tax` eliminated**: `x/tax`'s `DefaultMcaAddress` was removed with the module removal, and legacy incident-response upgrade scheduling in `PreBlocker` was cleaned up. Genuine production MANTRA data has no place in Ark's codebase. Worth a final grep sweep (`grep -rn "mantra1[a-z0-9]\{30,\}"`) before any real mainnet genesis day, in case another instance exists somewhere not touched by this track's specific module-removal work.
- **Cosmos SDK fork is measurably behind upstream on two real fixes** (secp256k1 pubkey-tag validation, compact-bitarray bounds check — both landed in v0.53.7/v0.53.8, past the fork's v0.53.6 pin). Not urgent (neither is a published CVE against the pinned version), but worth tracking for the next SDK re-pin. Full detail: `docs/proof/fork-audit-cosmos-sdk.md`.
- **cosmos/evm fork's EIP-7623 post-refund gas-floor change appears MANTRA-original and unreviewed by upstream** (not found in upstream through v1.0.0-rc2). Everything else in that fork's diff is either a verified upstream backport or config/test-only. This one specific change is the highest-value target for independent testing (refund-heavy, large-calldata Prague-path transactions) before trusting it in production. Full detail: `docs/proof/fork-audit-cosmos-evm.md`.
- **`go.mod`'s `cosmos/evm` require line was stale** (`v0.6.0` vs the actually-compiled `v0.6.2` base) — fixed this session (cosmetic, since the `replace` directive already governed real compilation, but was misleading to tooling/auditors). Worth checking whether any other `require` lines have similarly drifted from their `replace` targets.

---

# GAPS — what changes between this rehearsal and real mainnet day

Everything in `networks/devnet/`, `networks/mainnet/genesis-params.json`,
and `scripts/genesis/` was built and *tested* this session — real binary,
real 4-node devnet, real block production, real remote signing, real
(dummy) gentx rejection — against `base-genesis` as it stood at the time.
**Mid-session, `origin/base-genesis` itself was reset** from a `v1.0.1` pin
to a `v8.4.0` pin (see `git log origin/base-genesis`); this file, and every
script/config in this PR, reflects the post-reset state (`v8.4.0-1-g026e64c8`
at time of writing). This is the honest list of what's still different on
the day it counts for real.

**Reconciliation note (2026-08-22, post track/1-state-machine merge):**
`track/1-state-machine` merged to `base-genesis` and brought in
`docs/decisions/module-and-config-decisions.md`, an authoritative, evolving
team decision record that refines several values this track had used —
notably `x/sanction` is now **kept** (not stripped, superseding this
track's original brief), the denom scheme is the three-tier
`esp`/`espees`/`KASH` (base/intermediate/display, not just `esp`/`KASH`),
and several genesis parameters got real locked numbers for the first time
(21-day unbonding, 7-day voting period, 10 validators, SDK-default slashing
fractions). This session merged that base into `track/2-consensus-genesis`,
rebuilt the binary, and reconciled every genesis file/script/doc in this
track against the locked decisions — see the diff on `networks/devnet/
genesis-template.json`, `networks/devnet/pystarport.json`,
`networks/mainnet/genesis-params.json`, `networks/mainnet/RUNBOOK.md`, and
`networks/devnet/README.md`'s parameter table for the specifics. Two real
findings from that pass:

- **`docs/decisions/module-and-config-decisions.md` #11 mislabeled the real
  cosmos-sdk downtime slash default as "0.01%"** — verified against the
  vendored `x/slashing/types/params.go`, `DefaultSlashFractionDowntime =
  1/100 = 0.01` as a raw decimal fraction, which is **1%**, not 0.01%. The
  doc and every genesis file in this repo now use the code-verified value
  (1%). Worth a sanity-check by whoever wrote that table originally, in
  case the same off-by-100x slipped in anywhere else it was copied from.
- **`scripts/genesis/collect-gentx.sh` hardcoded `"ark"` as the target
  bech32 prefix** when re-encoding a validator's `valoper` address to its
  `acc` equivalent for balance lookup. Harmless only by coincidence (it
  happened to match the prefix in use at the time it was written) — any
  future prefix change would have silently produced wrong addresses with
  no error, since `bech32_reencode.py` will re-encode into *any* HRP it's
  told to and has no way to detect it's the wrong one. Fixed to derive the
  prefix from an address already present in the base genesis file instead
  (`ACCOUNT_PREFIX="${ACCOUNT_ADDR_SAMPLE%1*}"`) — round-trip tested against
  a real devnet genesis address (`ark1rvl2cva8w...` → derives `ark` →
  round-trips a synthetic `arkvaloper1...` back to the original correctly).

**Re-verification note (2026-08-22, fresh session):** everything below was
re-checked from scratch against the `ark-v0.1.0-alpha` binary rather than
trusted from the prior session. Binary from the Eng 1 tag, gentx fixtures
regenerated and re-run (tampered sig + overclaim still rejected, canonical
hash reproduced), devnet re-deployed with `ark` prefix and `KASH`/`esp`
denom (`make devnet-up` with `DEVNET_BIN` set to the alpha artifact), sentry
isolation re-verified against the *running* network via `net_info`
(validators: exactly 1 peer = own sentry, `pex=false`; new evidence in
`networks/devnet/proof/sentry-isolation.log`), TMKMS remote signing
re-integrated live for validator-0 with the updated `arkvalconspub` key format
(see `remote-signing/proof/live-signing-evidence.log`), block time
re-measured at 1.93-1.99s avg with the same `RoundStepPropose`-contention
root cause (load avg ~7.6 at capture). The `ark-v0.1.0-alpha` tag is now the
real Eng 1 release; the `v1.0.0-rc1` git tag visible in this repo remains
MANTRA's inherited 2024-10-07 upstream tag. Downstream handoff marker at
`networks/devnet/STATUS.md` updated.

## Blocks mainnet genesis outright

- **`ark-v0.1.0-alpha` is now the real Eng 1 release** and is the binary
  this track's devnet was re-verified against. The `v1.0.0-rc1` git tag
  visible in this repo is still MANTRA's inherited 2024-10-07 upstream tag
  and must not be used. Day-1 state-machine decisions taken by Eng 1 and
  since locked in `docs/decisions/module-and-config-decisions.md`: bech32
  prefix `ark`, three-tier denom `esp` (base, 1 wei-equiv.) / `espees`
  (intermediate, 10^9 esp, 1 gwei-equiv., gas-price display only) / `KASH`
  (display, 10^18 esp, symbol `KASH`), `x/tax` and `x/tokenfactory`
  stripped, and `x/sanction` retained. The EVM module's `LoadEvmCoinInfo`
  looks up the denom metadata for `evm_denom` (`esp`) and then uses the
  `denom_unit` matching `display` (`KASH`) to set its 18 decimals, so the
  base can remain `esp` while the public display name is `KASH`. Verified
  directly against `x/vm/keeper/coin_info.go`: because our display denom is
  exactly 18 decimals, `evm.params.extended_denom_options.extended_denom`
  is never actually read at runtime (only relevant for non-18-decimal
  chains) — it's set to `espees` in every genesis file purely for accurate
  self-documentation, not because anything consumes it today.
- **`networks/mainnet/genesis-params.json` no longer carries `x/tax` or
  `x/tokenfactory` overrides** because those modules were removed by Eng 1.
- **Only rehearsed against 2-4 dummy gentx files.** `collect-gentx.sh`'s
  per-file check boots a throwaway node (~5-8s each) — for a real N-large
  validator cohort this is still fine (a few minutes, one-time, high
  stakes) but hasn't been load-tested at real scale.

## A real, load-bearing codebase bug discovered this session (Eng 1's domain)

**`app/genesis.go`'s `NewDefaultGenesisState()` is never called anywhere in
`cmd/`** — verified by grepping the whole tree for its name; only the
function's own definition matches. It looks like it's meant to wire
`evm.params.evm_denom`, a native `erc20` token pair, and the EVM
feemarket's base fee to `amantra` automatically whenever `mantrachaind
init` runs. It doesn't run at all — `init` goes through the plain SDK
basic-module-manager path instead, so the raw default genesis has
`evm.params.evm_denom: "aatom"`, `mint`/`staking` denom `"stake"`, and an
**empty** `erc20.token_pairs` list. Worse: if this function ever *were*
wired in, `erc20State.TokenPairs[0].Denom = FeeDenom` would panic on index
0 of an empty slice, since `erc20types.DefaultGenesisState()` always
returns `TokenPairs: []TokenPair{}` (verified in the vendored `cosmos/evm`
fork's `x/erc20/types/genesis.go`). This repo's `genesis-template.json`
overrides the denom fields directly instead of relying on that function,
and deliberately does **not** attempt to construct a native ERC20 token
pair — that needs a real precompile/contract address, which is squarely
Eng 1's `app`/`x` domain to wire correctly, not something to improvise a
value for from the genesis/consensus track. **Recommend Eng 1 either fix
`NewDefaultGenesisState()` and wire it into `cmd/`, or delete it if it's
dead/superseded — as written it's a landmine for the next person who
assumes `mantrachaind init` does what that function's name implies.**

## A second discovered requirement: `bank.denom_metadata` is not cosmetic

The EVM module's `InitGenesis` panics at node startup —
`"error initializing evm coin info: denom metadata esp could not be
found"` — if `app_state.bank.denom_metadata` has no entry for the
bond/mint/evm denom. This isn't caught by `mantrachaind genesis
validate-genesis` (a structural/proto check only) — it only surfaces when
a node actually boots. `genesis-template.json` and `genesis-params.json`
both include a correct `denom_metadata` entry now (`esp` base, `espees`
intermediate, `KASH` display/symbol, with `wei`/`gwei`/`ether` aliases per
decision #8's exact JSON), and `networks/mainnet/RUNBOOK.md`'s assembly
script now relies on `genesis-params.json` as the single source of truth
for this value instead of re-declaring a second, driftable copy inline.

## Needs real infrastructure, not just config

- **Sentry topology is config-only on localhost.** `persistent_peers`/
  `pex`/`private_peer_ids` are correctly wired and *tested* (see
  `networks/devnet/README.md`'s sentry section) but all 4 processes share
  one loopback interface — nothing here provides actual network isolation.
  Real infra needs validators on a private subnet with no public IP and
  no inbound route at all, firewalled to accept only from their sentry's
  IP.
- **Every devnet node gets a local `priv_validator_key.json`**, including
  the sentries — a harmless side effect of `pystarport init`'s uniform
  process (sentries are never bonded so it's never used), but not
  representative of real sentry hardware, which shouldn't hold validator
  key material at all.
- **Only 2 bonded validators in the devnet** (by design, to fit "4 nodes"
  while demonstrating real sentry pairing) — losing either one halts the
  chain. Real mainnet needs the actual validator cohort assembled via
  `collect-gentx.sh` (Phase 4, already tested) with enough independent
  bonded validators that losing one still leaves ≥2/3 voting power.

## Remote signing — single-instance today, threshold needed for real mainnet

- **TMKMS softsign, single instance, no HSM.** Key material sits in a
  plain file, protected only by OS file permissions. Real mainnet needs
  either TMKMS with a YubiHSM2/Ledger backend, or a move to threshold
  signing (Horcrux) so no single machine holds a complete usable key. See
  `networks/devnet/remote-signing/README.md`'s comparison table.
- **TCP transport for the remote signer doesn't work as configured** —
  traced to an upstream CometBFT gap (unpersisted ephemeral
  SecretConnection key on every listener restart, confirmed by source:
  `privval/utils.go`'s own `// TODO: persist this key...` comment — still
  present in the `v0.38.23`-family version this repo currently vendors).
  Worked around with a Unix socket, which only works when the signer is
  co-located with the validator on one host. **A genuinely separate-host
  signer setup (the real target for production) needs one of**: a
  CometBFT patch that persists the listener key, a trusted network layer
  that doesn't depend on the ephemeral key for authentication, or
  confirming whether a TMKMS release newer than 0.15.0 (what this session
  installed) has grown a way to tolerate this. Not checked this session.
- **Double-sign protection state** lives on the same disk as everything
  else in this rehearsal. Real mainnet needs this on durable, ideally
  replicated storage — losing it and restarting is exactly the failure
  mode that causes double-signing.

## Still open per docs/decisions/module-and-config-decisions.md — genuine gaps, not faked

- **Governance timelock (decision #14, locked, minimum 48h) is not
  implemented anywhere in this codebase.** Stock cosmos-sdk `x/gov` has no
  timelock field, hook, or config knob — a passed proposal executes
  immediately in the same block its voting period ends. Implementing this
  needs either a custom module wrapping `MsgExecLegacyContent`/proposal
  execution, or a fork-level change to `x/gov`'s keeper. Not started. Do
  not assume any genesis parameter in this track's files provides this —
  none does, because none can.
- **`gov.min_deposit`/`expedited_min_deposit` in
  `networks/mainnet/genesis-params.json` are explicit placeholders**, not
  reviewed values — decision #15 blocks the real figure on a total supply
  decision that has not been made. Flagged prominently in that file's own
  header comment and in `networks/mainnet/RUNBOOK.md`'s pre-flight
  checklist so this can't be missed at genesis-day time.
- **EVM precompile decisions (decision doc's table) are still all `⏳`
  pending** in `docs/decisions/module-and-config-decisions.md`, despite
  `STATUS.md` (Eng 1's track) already containing a full precompile audit
  with reasoned recommendations. These two documents are currently out of
  sync — worth whoever owns the decisions doc copying Eng 1's findings in
  and getting real sign-off, rather than treating `STATUS.md`'s audit as
  itself the decision.
- **Admin/upgrade multisig (decision #17) is pending Eng 4's Day 3 key
  ceremony** — out of this track's scope, noted here only so it isn't
  mistaken for something this track's genesis files already handle. No
  multisig address appears anywhere in `networks/mainnet/genesis-params.json`
  because none has been decided yet.

## Lower priority / already mitigated, worth knowing about

- **Observed devnet block time is now ~2.97s average, above the 1-2s
  target band** (`networks/devnet/proof/block-times.log`, captured
  2026-08-22 against the freshly rebuilt `track/2-consensus-genesis`
  binary after correcting `timeout_commit` from `1s` to the
  decision-#12-locked `2s`). `verify-blocks.sh`'s own WARN check caught
  this — it is not silently passing. Root cause is very likely the other
  three CometBFT round timeouts (`timeout_propose`/`timeout_prevote`/
  `timeout_precommit`, all still `1s` each in `pystarport.json`) adding
  real overhead on top of `timeout_commit`, rather than the CPU-contention
  explanation an earlier measurement on this same file gave for a
  different, smaller overshoot — not reduced further this session, per
  decision #12's own explicit "do not chase sub-second block times on a
  3-day build" guidance. If the 1-2s band needs to be hit precisely, the
  other three timeout_* values are the ones to shrink next, not
  `timeout_commit` (that one is the one the decision doc explicitly pins).
  Re-run `make devnet-verify` after any such change and on real target
  hardware before trusting either number as representative.
- **This dev machine had no Go, `pystarport`, or `tmkms` installed at
  all** — all three were installed fresh this session. Two Go-toolchain
  nuances worth knowing: (1) `go.mod` specifies `go 1.25.0`; a newer
  Homebrew `go` (1.27, "latest") builds fine but a transitive dependency
  (`sonic`, a fast-JSON library) prints an environment-compatibility
  warning to **stdout** on every binary invocation outside its tested Go
  range, which broke `pystarport`'s JSON parsing until pinned to
  `go@1.25` specifically. (2) CI's `chain-binary` artifact is a Linux
  x86_64 binary that can't run natively on macOS anyway, so
  `CONTRIBUTING.md`'s "Eng 2-4 don't need Go" guidance doesn't hold here
  without also running inside a Linux container.
- **`pystarport` 0.2.5 (latest on PyPI) needed three independent
  compatibility patches**, all applied automatically and idempotently by
  `networks/devnet/patch-pystarport-cli.py` (fails loudly if pystarport's
  source ever stops matching what it expects): (1) Cosmos SDK v0.50+'s
  `genesis` subcommand nesting, (2) this binary's CLI renaming
  `tendermint show-node-id` to `comet show-node-id` (upstream CometBFT's
  own rename), (3) `interact()`'s default `stderr=subprocess.STDOUT`
  merging the `sonic` warning above into stdout and corrupting every
  `--output json` parse - patched to capture streams separately.
- **`collect-gentx.sh`'s valoper→account address resolution can't use
  `mantrachaind debug addr`** — its output format changed between the
  pre-reset and post-reset binary (from labeled `Bech32 Acc:`/`Bech32
  Val:` lines to raw EVM hex, since this app now uses `ethsecp256k1`
  keys). Replaced with a small self-contained bech32 re-encoder
  (`scripts/genesis/bech32_reencode.py`, standard BIP-173 algorithm, no
  external dependency) rather than depending on CLI output shape that's
  already proven unstable across versions of this same binary.
- **The root `Makefile` has a stale dependency reference**: line ~162
  computes `COSMWASM_VERSION := $(shell go list -m
  github.com/CosmWasm/wasmvm/v2 | sed ...)`, but the post-reset `go.mod`
  is on `github.com/CosmWasm/wasmvm/v3`. `go list -m` for the v2 path
  fails ("not a known dependency"), and since this is a top-level
  `$(shell ...)` assignment, the error prints on **every** `make`
  invocation regardless of target - cosmetic (nothing observed to
  actually break because of it) but noisy, and worth a one-line fix by
  whoever owns the root `Makefile`. Not fixed in this PR — out of this
  track's scope (`networks/`, `scripts/genesis/`) and not
  genesis/consensus related.
- **`x/tax`'s `Params` proto schema changed** between the pre-reset and
  post-reset codebase — the `max_mca_tax` field this session originally
  included in the devnet override no longer exists (`x/tax/types/
  params.pb.go` now only has `mca_tax`/`mca_address`), and including it
  fails genesis with `unknown field "max_mca_tax" in types.Params`. Fixed
  in the current `genesis-template.json`; noted here since it's a good
  example of exactly the kind of drift this whole pipeline exists to
  catch before it becomes a real-genesis-day surprise.
- **The CI `validate-genesis` job was broken before this branch touched
  it** (missing `libwasmvm.x86_64.so` on the runner, so it errored before
  ever reaching genesis content) — fixed in `.github/workflows/build.yml`
  as part of this PR.
- **`x/tax`'s module-level Go code hardcodes a real MANTRA mainnet address
  as its default `mca_address`** (`mantra15m77x4pe6w9vtpuqm22qxu0ds7vn4ehzwx8pls`)
  — inherited from upstream, not introduced by this branch. Every devnet
  genesis in this repo overrides it to a dedicated devnet-only test
  address specifically to avoid ever accidentally carrying that real
  address into a rehearsal artifact.

---

## Re-verification — `track/2-add-devnet-info` (2026-08-23)

Re-ran the Elisha handoff pipeline from a clean working state to confirm the genesis assembly tooling and devnet observability targets still work after the `devnet-info`/`devnet-log`/`devnet-explore` changes:

- `make devnet-info` works against the running 4-node devnet (validators 1 peer, sentries 2 peers, latest block heights match).
- `make devnet-log` tails the combined `devnet.log`.
- `make devnet-explore` prints live EVM blocks via `networks/devnet/explorer.sh`.
- `./scripts/genesis/collect-gentx.sh scripts/genesis/rehearsal/gentx scripts/genesis/rehearsal/base-genesis.json /tmp/rehearsal-genesis-2026-08-23.json` still accepts `valid-0.json` and `valid-1.json`, rejects `bad-overclaim.json` and `bad-tampered.json`.
- `./scripts/genesis/hash-genesis.sh /tmp/rehearsal-genesis-2026-08-23.json` reproduces the same canonical SHA-256 as before: `d0e283f6595fb07cb6fd973f68762f78b783d2ad7a63e074782ea5ee96316d61`.
- Binary used for the re-run reports version `track-2-consensus-genesis-7e2a522b` (the `ark-v0.1.0-alpha` / post-`track/1-state-machine` build); it does not include the Makefile-only `track/2-add-devnet-info` commits, which is correct for genesis validation because those changes do not affect state-machine output.

No new blockers surfaced. Elisha Day 3 remains blocked on the same external inputs: real validator `gentx` files, total token supply, governance deposit minima, admin/upgrade multisig, and governance timelock sign-off.
