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
