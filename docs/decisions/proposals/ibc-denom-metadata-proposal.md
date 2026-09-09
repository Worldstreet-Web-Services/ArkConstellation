# PROPOSAL: How IBC assets get correct decimals, given regular onboarding

**Status:** ✅ DECIDED — B. Implemented as a governance-gated `MsgSetDenomMetadata` on `track/1-denommetadata` (commit `ecc6b660`, 2026-09-09). This document records the decision and the evidence behind it; it is not an open question. **Option A tested and eliminated 2026-09-07** — see below.

**Author:** Drafted for review, 2026-09-05.

**Scope:** One question — **how denom metadata gets set for IBC-derived assets**, now that the chain expects to onboard them regularly. It does **not** propose reversing decision #2, and it does **not** propose a Hyperlane-side fix or mitigation — the Hyperlane impact discussed below is cited only as motivation for urgency, not as something this decision resolves.

---

## The defect, reproduced

MANTRA's OM was transferred to `arkdevnet_9000-1` over IBC on 2026-09-04. The
`erc20` middleware created a token pair automatically — that part works, no
registration or governance needed. The resulting ERC-20 is queryable through the
EVM and reports:

```
0xE6E6E1c7eEc542b44566e7626E8e4a35BC66cc7E
  name         "transfer/channel-0/amantra IBC token"
  symbol       AMANTRA
  totalSupply  2e18
  decimals     0        <-- the underlying token is 18-decimal
```

## Why

ICS-20 does not carry denom metadata across the wire, so the receiving chain
synthesises it on first receipt. What it produces is internally inconsistent:

```json
display:     "transfer/channel-0/amantra"
denom_units: [ { "denom": "amantra", "exponent": 0 } ]
```

`decimals` matches the last `/`-separated segment of `display` against the
denom units list; here `amantra` *does* match the single unit in `denom_units` —
there is no "unit missing" fallback involved. The match succeeds, but the unit it
matches carries exponent 0, because the correct exponent (18) never crossed the
wire in the first place.

The native denom demonstrates the mechanism working: `esp` has `display: KASH`
with `KASH` present at exponent 18, and resolves correctly. MANTRA publishes
correct metadata for `amantra` on its own chain — `mantra` at exponent 18 — and
that information simply never crosses.

This is a **known, general problem** with IBC assets on `cosmos/evm` chains, not
something specific to Ark or MANTRA.

## Why it cannot be fixed today

`x/bank` exposes no message for setting denom metadata. `x/tokenfactory`, which
provides one, was removed in decision #2 — part of a combined ~13,000-line removal
alongside `x/tax` (decision #3) that included a module holding **Minter**
permission — a decision this proposal does not dispute.

The only route currently available is an **upgrade handler**: `app/upgrades/`
exists and the app holds a `BankKeeper`, so a chain upgrade can call
`SetDenomMetaData` directly.

**Genesis pre-registration does not work**, and is worth ruling out explicitly.
The denom hash depends on the channel:

```
transfer/channel-0/amantra -> ibc/784D2618...
transfer/channel-1/amantra -> ibc/5F4B5CA9...
transfer/channel-7/amantra -> ibc/D5B08C0A...
```

Channel IDs are assigned when channels open, which is after genesis. The hash to
pre-register generally cannot be known in advance.

## Why the cadence changes the answer

An upgrade handler is a reasonable fix for a rare event. It is the wrong shape for
a recurring one: **onboarding a token becomes a chain upgrade** — validator
coordination, a scheduled height, a restart. At the stated cadence of regular
onboarding, that cost recurs indefinitely and every asset ships broken until the
next upgrade window.

## Options

**A — Upgrade `cosmos/evm`. ❌ TESTED AND RULED OUT (2026-09-07).**

MANTRA Dukong runs `v8.5.0-pre.1`, one minor version ahead of Ark, on the same
`cosmos/evm` lineage. It holds two IBC-derived token pairs. Both report the same
defect:

| MANTRA ERC-20 | Source | `decimals()` | Correct |
|---|---|---|---|
| `0x17735FA6…` | our `esp` over IBC | **0** | 18 |
| `0xaF43A2dA…` | `uosmo` from Osmosis | **0** | 6 |
| `0x88B60172…` | native, via `x/tokenfactory` | **6** | 6 ✅ |

The contrast in the third row is the whole answer. A version one minor ahead
behaves identically for IBC assets, while its *native* denoms are correct —
because `x/tokenfactory` writes proper metadata at creation time.

**This is not a code defect and no upstream release can fix it.** The exponent
never crosses the wire, so the receiving chain has nothing to derive from; the
synthesised metadata carries a single unit at exponent 0, and "use the highest
denom unit" still yields 0 because there is only one. Recovering the value
requires supplying it on the destination chain, out of band.

The upstream fixes found in the changelog address adjacent symptoms — a query
reverting, a panic on missing coin info — not the absence of the data itself.

**B — A minimal governance-gated metadata setter.** One message,
`MsgSetDenomMetadata`, authority-gated to gov. Roughly a hundred lines. It cannot
mint, cannot create denoms, cannot do anything but relabel existing ones — a small
fraction of the surface decision #2 removed. Onboarding becomes a governance
proposal: minutes, no downtime, no validator coordination.

**C — Upgrade handler per batch.** Works today, no new code. Correct if onboarding
turns out to be rarer than expected.

**D — Document and accept.** Consumers must never trust `decimals()` on an
IBC-derived ERC-20. Already written into the interchain repo's runbook. This is
necessary regardless of which option is chosen, because it protects assets that
arrive before any fix lands.

## Recommendation

**B.** A was tested and ruled out; C does not fit the stated cadence. **D
unconditionally and immediately**, since it protects every asset arriving before
B lands.

Note what the evidence implies about decision #2: MANTRA gets correct decimals on
its native denoms *because it kept `x/tokenfactory`*. Ark removed it and now has
no route at all. That does not make removing ~13,000 lines
(`x/tax` + `x/tokenfactory` combined) and a Minter permission wrong — but it does mean the specific capability of writing denom metadata needs
replacing, and B is the smallest possible replacement.

## The consequence that makes this more than cosmetic

Balances are correct — `2e18` is a true base-unit amount. The defect is
interpretive, and a wallet rendering two quintillion OM is merely embarrassing.

**But a Hyperlane Warp Route derives its scale factor from token decimals.**
Bridging an IBC-derived asset onward to Base while trusting `decimals()` over the
real 18 is exactly the decimals mismatch documented as the most common way bridges
lose money — on precisely the IBC → Ark → Base path the interchain work exists to
enable. Until this is fixed, warp route decimals must be read from the source
chain's metadata, never from the IBC-derived contract.
