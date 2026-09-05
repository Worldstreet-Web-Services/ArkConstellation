# DECISION: Raise the devnet's unbonding period to 21 days for IBC interop

**Status:** 🟢 DECIDED 2026-09-03, **APPLIED 2026-09-04**. Genesis sources carry `1814400s`, so a devnet built from scratch is mainnet-shaped from block zero. The running `arkdevnet_9000-1` was migrated by governance proposal **#3** (passed 180,000,000 KASH yes / 0 no; `unbonding_time` now `1814400s`, every other staking param unchanged). Any *other* pre-existing devnet still runs `300s` — confirm with `arkd query staking params` rather than assuming.

**Author:** Engineering. Drafted 2026-09-03; restructured 2026-09-04 to record the decision rather than the proposal.

**Scope:** One question — **which chain we open IBC channels against**, given that `arkdevnet_9000-1` was configured with a 300s unbonding period. It does **not** change mainnet's locked 21-day figure (decision #10), which is correct as it stands. It does **not** cover relayer operations. The gRPC gap it originally listed as blocking was resolved on 2026-09-04 and is no longer open.

---

## Decision

> ## Make `arkdevnet_9000-1` **mainnet-shaped at 21 days** (`1814400s`), matching locked decision #10.

The devnet's fast unbonding period and IBC interoperability are mutually exclusive on
the same chain. Both were legitimately wanted, so the drafted recommendation was to run
a second network and keep each capability on its own chain.

**That was rejected.** The party operating both devnet validators is the same one
driving interop, and the launch-readiness work that motivated in-session unbonding
rehearsals is complete. With no remaining need to exercise unbonding on *this* chain, a
second network is pure overhead — and a devnet matching mainnet's staking parameters is
strictly more representative for everything else tested against it. If unbonding
scenarios resurface, they get a throwaway chain.

### What this costs

In-session unbonding, redelegation and slashing-during-unbonding rehearsals — the
documented reason the fast value existed. Anyone who needs them should expect to spin a
separate chain rather than reverting this.

## The constraint

An IBC light client's **trusting period must be shorter than the counterparty's unbonding period** — conventionally two-thirds of it. That is not a relayer setting we can tune around; it is what makes the light client's security argument work. A validator set that has fully unbonded can no longer be slashed, so a client must never accept headers signed by one.

With the parameters as they stood before this change:

| | Value | Resulting trusting period |
|---|---|---|
| `arkdevnet_9000-1` unbonding | `300s` (5 min) | **~200 seconds** |
| `mantra-dukong-1` unbonding | `691200s` (8 days) | ~5.3 days |
| ArkConstellation mainnet (locked, #10) | `1814400s` (21 days) | ~14 days |

A client on MANTRA tracking Ark would therefore expire unless updated **more often than every ~200 seconds**. An expired client cannot be revived by a relayer — recovery requires a governance proposal on the *counterparty* chain. Any relayer restart, RPC interruption, or operator machine sleeping past three minutes would permanently kill the channel and leave MANTRA holding the cleanup.

Independently of the mechanics: no counterparty's governance would approve creating a client with a three-minute trusting period, and asking them to is not a good first impression.

## On making the period longer still — months

Raised during review: could the unbonding period be set in months, to buy more relayer-downtime tolerance?

**It could, and it should not.** Three reasons:

1. **21 days already provides ~14 days of trusting period.** That is not a tight margin, it is an enormous one. A relayer that has been down for two weeks is not a configuration problem.
2. **The trusting period is a security parameter for the counterparty, not a convenience for us.** A long trusting period means their light client will accept headers from a validator set it last verified a long time ago — the weak-subjectivity window. Counterparty governance reviews this figure, and an unusual one reads as a red flag rather than as caution.
3. **Stakers cannot withdraw for the whole period.** A months-long unbonding period is a material cost to holders and to exchange integrations, paid for a benefit that redundant relayers deliver for free.

The correct remedy for relayer downtime is **redundant relayers and alerting on client age**, not an inflated trusting period. 21 days is the Cosmos Hub standard, exceeds MANTRA's 8 days, and needs no change.

## Rejected alternative: a separate `ark-testnet-1`

Kept for the record, since it was the drafted recommendation.

Stand up a second network with mainnet-shaped parameters for interop and leave
`arkdevnet_9000-1` at `300s`, preserving both capabilities. Rejected for the reasons in
**Decision** above: nobody needs the fast-unbonding capability on this chain any more,
so the second network would be overhead without a beneficiary.

Its requirements, if it is ever revisited: `unbonding_time` at `1814400s`,
`historical_entries` at `10000` (already correct), a short `gov.voting_period` for
iteration, and a public gRPC endpoint.

## Applying it

The change lands in two places, and both are needed:

- **Genesis** — `networks/devnet/genesis-template.json` and `pystarport.json` now carry
  `1814400s`, so any devnet built from scratch is mainnet-shaped from block zero.
- **The running chain** — moves only by governance. Submit
  `networks/devnet/proposals/staking-unbonding-21d.json`. With a 120s voting period and
  a 1 KASH deposit it lands in about four minutes, and both validators are operated by
  the same party, so no external coordination is needed. Done for
  `arkdevnet_9000-1` on 2026-09-04 as proposal #3.

  Two things that trip up the submission, neither documented elsewhere:

  - **Fees.** The compose sets `MIN_GAS_PRICES: "0.01esp"`, but `cosmos/evm`'s
    `MinGasPriceDecorator` enforces a *global* minimum of 1 gwei per gas unit. Use
    `--gas-prices 1000000000esp`; `0.01esp` is rejected with a stack trace ending
    `provided fee < minimum global fee`.
  - **The node image's entrypoint requires `NODE_ROLE`**, so a one-off `arkd`
    invocation needs `--entrypoint arkd` to bypass it.

Verify with `arkd query staking params` rather than assuming either path was taken.

## What this does not resolve

- **Relayer operations.** Who runs it, funded accounts on both chains, and monitoring
  for client expiry — the failure that actually kills Cosmos connections.
- **Rate-limit quotas** on the IBC transfer stack. The middleware is already wired; the
  numbers are a separate decision.

**Resolved since drafting:** the gRPC gap. Hermes and the Go relayer both require gRPC,
and the devnet exposed only RPC and LCD. The node was already serving gRPC on 9095 —
`entrypoint.sh` moves it there to leave 9090 to CometBFT's Prometheus endpoint — but the
port was never published and there was no Caddy route. Both were fixed on 2026-09-04 and
`grpc.34.60.137.196.sslip.io` now answers.

That infra change is **not in this PR's diff**, and two things about it are separate:
it is **applied and live on the host**, verified by a real gRPC call returning
`grpc-status: 0` through Caddy — but it is **not yet in this repo**. The `ops/` change
describing it is **#30**. Until that merges, this record cites a Caddy route the tree
does not contain — check #30's status rather than inferring it from this sentence.
Review the route there, not from these files.

With both this and gRPC applied, **nothing further blocks opening a channel.**
