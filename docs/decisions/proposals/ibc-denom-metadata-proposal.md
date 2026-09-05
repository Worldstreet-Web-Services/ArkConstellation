# PROPOSAL: How IBC assets get correct decimals, given regular onboarding

**Status:** 🟠 PROPOSAL — not decided. Requires sign-off from whoever owns the module set (decision #2, which removed `x/tokenfactory`).

**Author:** Drafted for review, 2026-09-05.

**Scope:** One question — **how denom metadata gets set for IBC-derived assets**, now that the chain expects to onboard them regularly. It does **not** propose reversing decision #2, and it does **not** cover the Hyperlane path.

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

`decimals` resolves to the exponent of the unit named by `display`, and that unit
is absent from the list. It falls back to 0.

The native denom demonstrates the mechanism working: `esp` has `display: KASH`
with `KASH` present at exponent 18, and resolves correctly. MANTRA publishes
correct metadata for `amantra` on its own chain — `mantra` at exponent 18 — and
that information simply never crosses.

This is a **known, general problem** with IBC assets on `cosmos/evm` chains, not
something specific to Ark or MANTRA.

## Why it cannot be fixed today

`x/bank` exposes no message for setting denom metadata. `x/tokenfactory`, which
provides one, was removed in decision #2 along with ~13,000 lines including a
module holding **Minter** permission — a decision this proposal does not dispute.

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

**A — Check upstream first.** This is a known issue and fixes exist upstream
referencing exactly this failure ("decimals reverts when Display doesn't match
DenomUnit for IBC tokens", "use the highest denom unit when deploying an ERC20").
**I could not confirm which release contains them**, and Ark runs
`cosmos/evm v0.6.2-ark-1`, a fork. Confirming whether a later `cosmos/evm` — or
MANTRA's own `v8.5.0-pre.1`, which is one minor ahead — already fixes this is the
cheapest possible outcome and should happen before any code is written here.

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

**A, then B if A does not resolve it.** D unconditionally and immediately.

## The consequence that makes this more than cosmetic

Balances are correct — `2e18` is a true base-unit amount. The defect is
interpretive, and a wallet rendering two quintillion OM is merely embarrassing.

**But a Hyperlane Warp Route derives its scale factor from token decimals.**
Bridging an IBC-derived asset onward to Base while trusting `decimals()` over the
real 18 is exactly the decimals mismatch documented as the most common way bridges
lose money — on precisely the IBC → Ark → Base path the interchain work exists to
enable. Until this is fixed, warp route decimals must be read from the source
chain's metadata, never from the IBC-derived contract.
