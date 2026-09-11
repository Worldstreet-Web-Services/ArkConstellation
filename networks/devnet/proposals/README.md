# Devnet governance proposals

Submittable `MsgUpdateParams`-style proposal JSON for `arkdevnet_9000-1`. This is
the repo's first proposal directory, so the conventions below are being set here.

## What belongs here

Parameter changes to a **running** devnet. A parameter that should hold for every
future devnet belongs in `../genesis-template.json` and `../pystarport.json`
instead — usually both, since a change made only by governance silently reverts
the next time someone runs `make devnet-up`.

Most changes need both: genesis for chains not yet built, a proposal for the one
already running.

## Submitted

| File | Proposal | Status |
|---|---|---|
| `staking-unbonding-21d.json` | #3 | ✅ Passed 2026-09-04 — 180,000,000 KASH yes / 0 no. `unbonding_time` 300s → 1814400s |
| `evm-active-precompiles.json` | — | 📝 Drafted, not submitted — activates the 9 registered EVM static precompiles (issue #37) |

`evm-active-precompiles.json` carries one hazard worth repeating outside the file:
**do not add `0x…0803` (Vesting) to that list.** The fork advertises the address in
`evmtypes.AvailableStaticPrecompiles` but ships no implementation, and activating
an address the keeper has no contract for makes `GetStaticPrecompileInstance`
panic — reachable by any caller over `eth_call`. `ValidatePrecompiles` accepts it
happily, so nothing upstream of the call site will stop you. `app/precompiles.go`
carries the full explanation and a test that fails if the exclusion is undone.

Before submitting it, run `arkd query evm params` and diff against the `params`
block in the file. `MsgUpdateParams` replaces the whole struct, so any field where
the live chain disagrees with the file gets silently reset to the file's value.

After it passes, confirming the param changed is not the same as confirming the
precompiles work — verify dispatch, not just state:

```bash
scripts/chaos/precompile-verify.sh --rpc http://sentry-0:8545 --api http://sentry-0:1317
```

## Submitting

```bash
arkd tx gov submit-proposal networks/devnet/proposals/<file>.json \
  --from <key> --home <keyring home> --keyring-backend test \
  --chain-id arkdevnet_9000-1 --node http://sentry-0:26657 \
  --gas auto --gas-adjustment 1.5 --gas-prices 1000000000esp --yes
```

Two things that are easy to get wrong, both of which cost a failed transaction:

- **`--gas-prices 1000000000esp` is required**, and it is *not* what the node
  advertises. `cosmos/evm` enforces a chain-wide 1 gwei per gas unit through
  feemarket's `min_gas_price`, while `/cosmos/base/node/v1beta1/config` reports
  `0.01esp` from the node's local `minimum-gas-prices`. The local value is
  advisory and CheckTx-only; the chain-wide one is consensus. Using the
  advertised value fails with a long ante-handler stack trace ending
  `provided fee < minimum global fee`.
- **The node image's entrypoint requires `NODE_ROLE`**, so a one-off `arkd`
  invocation in a container needs `--entrypoint arkd` to bypass it.

Voting is 120s with a 1 KASH deposit, so a proposal resolves in about four
minutes. Both devnet validators are operated by the same party — vote from each,
then confirm the effect directly rather than trusting the proposal status:

```bash
arkd query staking params
```

## Rolling back is asymmetric

A passed proposal and the files that describe it revert differently, and this
catches people out.

`git revert` restores the JSON and the docs. It does **not** restore the chain —
`arkdevnet_9000-1` is at 21 days by passed governance and only moves by another
proposal, roughly four minutes. Reverting the commit alone leaves the repo saying
`300s` while the chain runs `1814400s`: the same contradiction this directory
exists to prevent, pointing the other way.

To actually roll back, do both: submit the reversing proposal, confirm with
`arkd query staking params`, then revert the files.

## After a proposal lands

Update the row above, and check whether `../README.md`'s deviation table still
describes reality. A passed proposal that leaves the table stale is how the docs
and the chain drift apart.
