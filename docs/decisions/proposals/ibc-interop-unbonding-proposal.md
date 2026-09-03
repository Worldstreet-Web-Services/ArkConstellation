# PROPOSAL: Where IBC interop gets tested, given the devnet's 300s unbonding period

**Status:** 🟢 DECIDED 2026-09-03 — take the *alternative* path recorded below: make `arkdevnet_9000-1` mainnet-shaped at 21 days rather than standing up a second network. `networks/devnet/proposals/staking-unbonding-21d.json` is the change to submit. The devnet deviation table in `networks/devnet/README.md` must be updated in the same change.

**Author:** Drafted for review, 2026-09-03.

**Scope:** This proposal addresses exactly one question — **which chain we open IBC channels against for interop testing**, given that `arkdevnet_9000-1` deliberately runs a 300s unbonding period. It does **not** propose changing mainnet's locked 21-day figure (decision #10), which is correct as it stands and needs no revision. It does **not** cover relayer operations or the gRPC endpoint gap, both of which are separate and still open.

---

## Recommendation as drafted (superseded — see Status)

> ## Stand up a separate **`ark-testnet-1`** with mainnet-shaped parameters for interop testing. Leave `arkdevnet_9000-1` at 300s.

The devnet's fast unbonding period and IBC interoperability are **mutually exclusive on the same chain**, and both are legitimately wanted. Running two networks is cheaper than losing either capability.

### Why the decision went the other way

The operator of both devnet validators is the same party driving interop, and the
launch-readiness work that motivated fast unbonding tests is complete. With no
remaining need to exercise unbonding on *this* chain, a second network is pure
overhead — and a devnet that matches mainnet's staking parameters is strictly
more representative for everything else being tested against it. Unbonding
scenarios, if they resurface, get a throwaway chain.

---

## The constraint

An IBC light client's **trusting period must be shorter than the counterparty's unbonding period** — conventionally two-thirds of it. That is not a relayer setting we can tune around; it is what makes the light client's security argument work. A validator set that has fully unbonded can no longer be slashed, so a client must never accept headers signed by one.

With the devnet's current parameters:

| | Value | Resulting trusting period |
|---|---|---|
| `arkdevnet_9000-1` unbonding | `300s` (5 min) | **~200 seconds** |
| `mantra-dukong-1` unbonding | `691200s` (8 days) | ~5.3 days |
| ArkConstellation mainnet (locked, #10) | `1814400s` (21 days) | ~14 days |

A client on MANTRA tracking Ark would therefore expire unless updated **more often than every ~200 seconds**. An expired client cannot be revived by a relayer — recovery requires a governance proposal on the *counterparty* chain. Any relayer restart, RPC interruption, or operator machine sleeping past three minutes would permanently kill the channel and leave MANTRA holding the cleanup.

Independently of the mechanics: no counterparty's governance would approve creating a client with a three-minute trusting period, and asking them to is not a good first impression.

## Why not simply raise the devnet's unbonding period

Because the 300s figure is doing a job. Per `networks/devnet/README.md`, it exists so that unbonding, redelegation, and slashing-during-unbonding can be exercised inside a working session — scenarios that a 21-day period makes untestable locally. Raising it to an IBC-viable value would silently remove the capability the devnet was configured to provide.

The two requirements genuinely conflict:

| Requirement | Needs |
|---|---|
| Exercise unbonding/slashing in-session | Unbonding measured in **minutes** |
| Hold an IBC channel open | Unbonding measured in **days** |

A second network resolves this cleanly and matches the repo's existing pattern of distinguishing "devnet-fast" from "mainnet-shaped" configuration, rather than overloading one chain with both.

## On making the period longer still — months

Raised during review: could the unbonding period be set in months, to buy more relayer-downtime tolerance?

**It could, and it should not.** Three reasons:

1. **21 days already provides ~14 days of trusting period.** That is not a tight margin, it is an enormous one. A relayer that has been down for two weeks is not a configuration problem.
2. **The trusting period is a security parameter for the counterparty, not a convenience for us.** A long trusting period means their light client will accept headers from a validator set it last verified a long time ago — the weak-subjectivity window. Counterparty governance reviews this figure, and an unusual one reads as a red flag rather than as caution.
3. **Stakers cannot withdraw for the whole period.** A months-long unbonding period is a material cost to holders and to exchange integrations, paid for a benefit that redundant relayers deliver for free.

The correct remedy for relayer downtime is **redundant relayers and alerting on client age**, not an inflated trusting period. 21 days is the Cosmos Hub standard, exceeds MANTRA's 8 days, and needs no change.

## What `ark-testnet-1` needs

Mainnet-shaped where it matters for interop; devnet-convenient elsewhere.

| Param | Value | Reason |
|---|---|---|
| `staking.unbonding_time` | `1814400s` (21 days) | Matches mainnet-locked #10; yields a ~14-day trusting period |
| `staking.historical_entries` | `10000` | Required for IBC proofs; already correct on devnet |
| `gov.voting_period` | short (e.g. `120s`) | Keep iteration fast; not interop-relevant |
| Public **gRPC** endpoint | required | Hermes and the Go relayer both need it; the devnet exposes RPC and LCD only |

## What this does not resolve

- **The gRPC gap.** `grpc.34.60.137.196.sslip.io` accepts TCP (sslip.io resolves any subdomain) but nothing serves gRPC behind it, and 9090/9091 are firewalled. Any relayer needs this regardless of which network we point it at. Infrastructure item, not a parameter.
- **Relayer operations.** Who runs it, funded accounts on both chains, monitoring for client expiry.
- **Rate-limit quotas** on the IBC transfer stack. The middleware is already wired; the numbers are a separate decision.

## If the decision goes the other way

If standing up a second network is judged not worth it and the devnet is to be changed instead, `networks/devnet/proposals/staking-unbonding-21d.json` is a ready-to-submit parameter change. Submitting it **removes the devnet's ability to exercise unbonding in-session**, and `networks/devnet/README.md`'s deviation table must be updated in the same change so the two do not silently disagree.
