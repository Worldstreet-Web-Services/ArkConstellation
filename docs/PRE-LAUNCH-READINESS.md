# ArkConstellation — Pre-Launch Readiness (Testnet/Mainnet)

Comprehensive punch list of what a modern general-purpose EVM chain typically needs, checked against the actual state of `ArkConstellation` and `Ark-Evm` (`base-genesis`, as of 2026-08-31). Status tags:

- ✅ **Done** — verified present and wired in.
- ⚠️ **Partial** — exists but incomplete, unwired, or unvalidated.
- ❌ **Missing** — not found anywhere in either repo.
- 🤔 **Decision needed** — a real choice to make, not just an implementation task.

Every "verified" line below was checked directly against the code/config in this session, not assumed. Items marked "industry-standard, unverified" are general knowledge about what modern chains do, not something I confirmed is planned — treat those as prompts to decide, not confirmed gaps.

Related tracking: GitHub issues [#16–#24](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues) on `ArkConstellation`, and 5 private security advisories on `Ark-Evm` (see the repo's Security tab).

---

## 1. Execution-layer UX & account infrastructure

| Item | Status | Detail |
|---|---|---|
| EIP-7702 account abstraction | ✅ Done | Prague active by default; real ante-handler + mempool wiring; 796-line dedicated test suite (`Ark-Evm` `tests/systemtests/accountabstraction/`). |
| Standard preinstalls (Create2, Multicall3, Permit2, Safe factory, EIP-2935) | ⚠️ Partial | Code exists in `Ark-Evm`'s `DefaultPreinstalls`; **not deployed** — `app_state.vm.preinstalls` is empty in ArkConstellation's genesis. Tracked: [#21](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/21). |
| ERC-4337 (EntryPoint/bundler/UserOperation) | 🤔 Decision needed | Not present anywhere. EIP-7702 covers native AA; ERC-4337 is a separate, older model some wallet tooling still assumes. Tracked: [#24](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/24). |
| Gas sponsorship / Paymaster | 🤔 Decision needed | Only a fee-market config choice (`min_gas_price: 0`) exists, no actual sponsorship mechanism (no `x/feegrant` confirmed wired for EVM txs, no paymaster contract/precompile). Tracked: [#22](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/22). |
| EIP-3009 / EIP-2612 gasless signed transfers | ❌ Missing | No token here implements `transferWithAuthorization` or `permit`. Relevant to any x402-style payment flow. Tracked: [#23](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/23). |
| EIP-712 typed signing | ✅ Done | Standard `eth_signTypedData_v4` — works via any go-ethereum-based JSON-RPC, nothing chain-specific needed. |
| Latest Ethereum hardfork parity (Fusaka/Osaka) | ⚠️ Partial | Prague/Pectra active by default; `OsakaTime` field exists end-to-end (proto, chain config, blob defaults) but is `nil` — not activated, no commits/tests targeting it. Ethereum mainnet has been on Fusaka since Dec 2025. Not urgent (most Fusaka changes are L1 DA-layer specific), but should be a tracked, deliberate decision rather than silent staleness. |

## 2. Precompiles

| Item | Status | Detail |
|---|---|---|
| Core precompile set (Bech32, Bank, Staking, Distribution, ICS20, Gov, Vesting, Slashing, P256, `distrclaim`) | ⚠️ Partial | Audited and recommended-enable by Eng 1 (`STATUS.md`'s table), but `docs/decisions/module-and-config-decisions.md`'s own precompile table is still all `⏳ pending` — the audit and the actual decision record are out of sync. Needs real sign-off, especially Staking and ICS20 (flagged for chaos-testing). |
| `distrclaim` custom precompile (Ark-original, address `0x0a01`) | ⚠️ Partial | Not part of upstream `cosmos/evm` — being Ark's own addition, it hasn't had the same fork-audit scrutiny as the rest of the precompile set. Worth an explicit security review before mainnet, same rigor as the ICS20/statedb items already audited. |
| RIP-7212 (P256/WebAuthn passkey verification) | ✅ Done | Present and recommended-enable. |
| BLS12-381 (EIP-2537) | ✅ Done | Part of the Prague precompile set already active (`WithPraguePrecompiles()`). |

## 3. Interoperability / bridging

| Item | Status | Detail |
|---|---|---|
| IBC token transfer (ICS20) | ✅ Done (with caveats) | Present, and carries the critical nested-forwarding security guard — but see the private `Ark-Evm` advisories on the v2 middleware's dropped ack-validation logic and the ICS20 EVM precompile's own review status. |
| Interchain Accounts (ICA host + controller) | ✅ Done | Both `icahostkeeper` and `icacontrollerkeeper` are wired in `app/app.go`. |
| CosmWasm | ✅ Done | `wasmkeeper.Keeper` present in `app/app.go`. |
| General cross-chain messaging (Axelar, LayerZero, Wormhole, Hyperlane, etc.) | ❌ Missing (industry-standard, unverified need) | No integration found. Whether this is needed depends on whether ArkConstellation wants asset/message bridging beyond the IBC ecosystem — worth an explicit product decision, not an oversight to fix reflexively. |
| Canonical bridged-asset registry / token list process | ❌ Missing (industry-standard, unverified need) | No process found for how a bridged asset gets a canonical address/denom mapping recognized by wallets and explorers. Needed before external bridges integrate. |

## 4. Oracles / price feeds

| Item | Status | Detail |
|---|---|---|
| Skip Connect (Slinky oracle module) | ⚠️ Partial | `github.com/skip-mev/connect/v2` (MANTRA's fork) is a real dependency, and `oracletypes.RegisterInterfaces` is called in `app/app.go` — but there is **no `OracleKeeper` field, no module wiring, no live oracle module registered**. This is a dependency, not a running feature. **No functioning on-chain price oracle exists today.** |
| Why it matters | — | Any DeFi primitive that needs a trusted price (liquidations, oracle-priced AMMs, lending protocols) has nothing to build on until this is actually wired up, not just imported. |

## 5. Explorer, indexing, developer tooling

| Item | Status | Detail |
|---|---|---|
| Block explorer (Blockscout) | ✅ Done | `feat/blockscout-explorer` is merged into `base-genesis` — local Blockscout + multi-cloud sentry topology docker setup exists. Verify it's production-deployable, not just devnet-docker-compose. |
| Faucet | ⚠️ Partial | `ops/docker/faucet/` (Dockerfile, entrypoint, faucet.py) exists locally but is **untracked** — not yet committed/merged into `base-genesis`. Needed before public testnet. |
| Dedicated indexer (subgraph-equivalent / Cosmos-EVM-aware indexing service) | ❌ Missing (industry-standard, unverified need) | No integration found beyond Blockscout's own indexing. Worth deciding whether third-party indexers (Ponder, Subsquid, Goldsky, etc.) need explicit chain-config/docs support. |
| Public RPC/API documentation for external builders | ❌ Missing (industry-standard, unverified need) | Internal runbooks (`networks/mainnet/RUNBOOK.md`, `ops/runbooks/`) exist; no evidence of external-facing developer docs (RPC endpoints, chain ID, supported methods, rate limits). |

## 6. Validator / infrastructure readiness

All items below are already tracked in the team's own `GAPS.md` — restated here for completeness of the pre-launch picture, not re-discovered:

| Item | Status | Detail |
|---|---|---|
| Sentry topology | ⚠️ Partial | Config-correct and tested on localhost; real network isolation (private subnet, no public IP on validators) not yet built. |
| Remote signing (TMKMS) | ⚠️ Partial | Single-instance softsign only, no HSM (YubiHSM2/Ledger) or threshold signing (Horcrux). Key material sits in a plain file. |
| Remote-signer transport | ⚠️ Partial | TCP transport doesn't work as configured (upstream CometBFT gap — unpersisted ephemeral key on listener restart); Unix socket workaround only works when signer is co-located with the validator on one host. A genuinely separate-host signer setup needs a CometBFT patch, a trust layer that doesn't depend on the ephemeral key, or a newer TMKMS release. |
| Double-sign protection storage | ⚠️ Partial | Lives on the same disk as everything else in the rehearsal; real mainnet needs durable, ideally replicated storage. |
| Validator cohort scale | ⚠️ Partial | Only rehearsed against 2-4 dummy gentx files / 2 bonded devnet validators. Real mainnet needs the actual validator cohort assembled and tested at real scale via `collect-gentx.sh`. |
| Admin/upgrade multisig | ❌ Missing | Decision #17, pending Eng 4's key ceremony. No multisig address exists anywhere in `networks/mainnet/genesis-params.json` yet. |
| Block time tuning | ⚠️ Partial | Observed ~2.97s average, above the 1-2s target band. Root cause likely the three untuned CometBFT round timeouts (`timeout_propose`/`timeout_prevote`/`timeout_precommit`), not `timeout_commit` (which is correctly locked at the decision-#12 value). |

## 7. Governance & compliance

| Item | Status | Detail |
|---|---|---|
| Governance timelock (decision #14, 48h minimum) | ❌ Missing | Not implemented anywhere — stock `x/gov` executes a passed proposal immediately. Tracked as a private advisory ([GHSA-226m-7q8f-gj3p](https://github.com/Worldstreet-Web-Services/ArkConstellation/security/advisories/GHSA-226m-7q8f-gj3p)). |
| Governance deposit minimums | ❌ Missing | `gov.min_deposit`/`expedited_min_deposit` are explicit placeholders, blocked on an undecided total-supply figure. Tracked: [#19](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/19). |
| `x/sanction` compliance module | ✅ Done | Retained (not stripped), wired into both Cosmos and EVM ante paths, unit + e2e tests passing. |
| Circuit breaker | ✅ Done | Wired across both Cosmos and EVM JSON-RPC ante paths, verified live. |

## 8. Consensus / core protocol currency

All previously identified and already tracked as issues/advisories — restated for completeness:

| Item | Status | Detail |
|---|---|---|
| `cosmos-sdk` fork missing ~10 upstream panic/DoS fixes (v0.53.7/v0.53.8) | ❌ Missing | Reachable via crafted/malformed tx against a live validator. Private advisory: [GHSA-93g8-x7xx-r6w2](https://github.com/Worldstreet-Web-Services/ArkConstellation/security/advisories/GHSA-93g8-x7xx-r6w2). |
| `Ark-Evm` `x/erc20/v2/ibc_middleware.go` dropped ack-validation | ❌ Missing | Private advisory: [GHSA-v4q3-rf62-9p72](https://github.com/Worldstreet-Web-Services/evm/security/advisories/GHSA-v4q3-rf62-9p72). |
| Locked-balance-snapshot accounting removed vs. MANTRA parent | 🤔 Decision needed | Private advisory: [GHSA-x792-4cmc-h4rr](https://github.com/Worldstreet-Web-Services/evm/security/advisories/GHSA-x792-4cmc-h4rr). |
| EIP-7623 post-refund gas-floor (Ark-original, unreviewed upstream) | ⚠️ Partial | Private advisory: [GHSA-356v-8xq9-q39f](https://github.com/Worldstreet-Web-Services/evm/security/advisories/GHSA-356v-8xq9-q39f). |
| ICS20 nested-forwarding guard as required CI gate | ⚠️ Partial | Private advisory: [GHSA-hr5c-gvqp-3m6j](https://github.com/Worldstreet-Web-Services/evm/security/advisories/GHSA-hr5c-gvqp-3m6j). |
| Fork-audit doc for the Ark-specific diff layer | ❌ Missing | Private advisory: [GHSA-33x5-wq5v-h6p5](https://github.com/Worldstreet-Web-Services/evm/security/advisories/GHSA-33x5-wq5v-h6p5). |

## 9. Security process

| Item | Status | Detail |
|---|---|---|
| Real chaos/security testing (Eng 3) | ❌ Missing | No `track/3-security-chaos` branch or PR exists. Blocks `ark-v1.0.0-rc1`. Tracked: [#16](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/16). |
| `SECURITY.md` contact | ❌ Missing | Still points to `security@mantrachain.io`. Tracked: [#20](https://github.com/Worldstreet-Web-Services/ArkConstellation/issues/20). |
| External/third-party security audit | ❌ Missing (industry-standard, unverified need) | No evidence of an engaged external auditor found in either repo. Standard for a mainnet launch carrying real value. |
| Public bug bounty program | ❌ Missing (industry-standard, unverified need) | No bug-bounty program content found (Immunefi or equivalent) — MANTRA itself had one (referenced in the Moonbeam/MANTRA research earlier this session as the industry norm). |
| Repo visibility / credential hygiene | ⚠️ Partial | Both `ArkConstellation` and `Ark-Evm` are **public** repos, pre-launch. Both had a live GitHub PAT embedded in plaintext in local `.git/config` (should be rotated regardless of repo visibility). Worth a deliberate decision on whether either repo should be private until closer to launch. |

## 10. Release engineering

| Item | Status | Detail |
|---|---|---|
| Reproducible builds / checksums | ⚠️ Partial | Named as Day 3 work in `GAPS.md`, can proceed independently of the Eng 3 blocker, not confirmed complete. |
| CI `validate-genesis` job | ✅ Done | Was broken (missing `libwasmvm.x86_64.so` on the runner), fixed this cycle per `GAPS.md`. |
| Root `Makefile` stale `wasmvm/v2` reference | ⚠️ Partial | Cosmetic — prints an error on every `make` invocation since `go.mod` moved to `wasmvm/v3`. Noted but not fixed (out of the track that found it). |

## 11. Rebranding / identity hygiene

| Item | Status | Detail |
|---|---|---|
| Bech32 prefix, denom hierarchy, chain-id | ✅ Done | `ark` prefix, `esp`/`espees`/`KASH` three-tier denom, chain-ids locked and wired in `app/config.go`. |
| Full README/CONTRIBUTING rebrand | ⚠️ Partial | Only the actively-misleading "Modules" section of `README.md` was corrected; the rest is unaddressed. |
| Go module path / binary name / proto namespace (`MANTRA-Chain/mantrachain`, `mantrachaind`) | 🤔 Decision needed | Deliberately not renamed — large, disruptive change touching every import and deployed script. Needs an explicit go/no-go, not silent inheritance. |
| Legacy MANTRA mainnet addresses in code | ⚠️ Partial | Known instances resolved (`WTokenContractMainnet` zeroed, `x/tax`'s hardcoded address removed with the module). `GAPS.md` recommends a final sweep (`grep -rn "mantra1[a-z0-9]\{30,\}"`) before real mainnet genesis day — not confirmed done. |

## 12. Load / scale testing

| Item | Status | Detail |
|---|---|---|
| Gentx collection at real validator-cohort scale | ⚠️ Partial | Only rehearsed against 2-4 dummy files; real N-large cohort untested. |
| Paymaster/relayer traffic patterns | ❌ Missing | Explicitly never load-tested, per `GAPS.md` — moot until the Paymaster infra itself exists (see section 1). |
| Block-time / consensus tuning under real load | ⚠️ Partial | Only measured on a single dev machine under local CPU contention; not representative of real target hardware. |

---

## How to use this document

This is a snapshot, not a living tracker — treat the linked GitHub issues and security advisories as the source of truth for status going forward, and update this file (or replace it with a proper project-board view) as items close. The "🤔 Decision needed" items are the ones most worth a team discussion before more engineering time goes into them, since implementation without a decision risks building the wrong thing.
