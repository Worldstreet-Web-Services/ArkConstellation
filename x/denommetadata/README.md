# `x/denommetadata`

A single governance-gated message, `MsgSetDenomMetadata`, for correcting bank
denom metadata on denominations that already exist.

## Why it exists

ICS-20 does not carry denom metadata across the wire. When an asset arrives over
IBC, the receiving chain writes a single denom unit at exponent 0 and never
learns the source chain's real precision:

```json
display:     "transfer/channel-0/amantra"
denom_units: [ { "denom": "amantra", "exponent": 0 } ]
```

The ERC-20 precompile's `Decimals()` (`precompiles/erc20/query.go` in
`cosmos/evm`) reads this metadata live on every call. For an `ibc/` base denom it
matches the **last `/`-separated segment of `display`** against a `denom_units`
entry — a real lookup, not a fallback; an unresolved display **reverts the
call**, it does not return 0. Here the last segment is `"amantra"`, which *does*
match the unit above, so the lookup succeeds and returns exactly what is stored:
exponent 0. **`decimals() = 0` because 0 is genuinely what was written**, not
because the lookup failed to find anything — so `x/erc20`'s ERC-20 for this
denom reports `decimals() = 0` regardless of the real precision.

(An earlier revision of this document described the mechanism as a
"display not found, falls back to 0" — verified against the actual precompile
source and corrected. There is no such fallback in that function; a genuine
non-match reverts.)

Measured on `arkdevnet_9000-1`, 2026-09-04: MANTRA's `amantra` (18 decimals)
arrived over `channel-0`, and its ERC-20 at
`0xE6E6E1c7eEc542b44566e7626E8e4a35BC66cc7E` reports `decimals() = 0`.

**No upstream release fixes this**, and that was tested rather than assumed.
MANTRA Dukong runs `cosmos/evm` one minor version ahead and shows the same
defect on both of its IBC-derived pairs, while its *native* denoms are correct —
because `x/tokenfactory` writes proper metadata at creation. The exponent never
crosses the wire, so there is nothing to derive from; the value has to be
supplied on the destination chain, out of band.

This chain removed `x/tokenfactory` (decision #2) and `x/bank` exposes no
metadata-setting message, so there was no way to supply it. This module is that
way, and nothing more.

## Why it is safe

Decision #2 removed ~13,000 lines including a module holding **Minter**
permission. This module reintroduces none of that surface:

- **No state of its own.** No store key, no genesis. `x/bank` remains the single
  source of truth; this is only a write path into it.
- **A deliberately narrow bank interface** — `GetDenomMetaData` and
  `SetDenomMetaData`. No balances, no minting, no burning. Even with a
  compromised authority it cannot move or create tokens; it can only relabel
  denominations that already exist.
- **Governance-only.** Metadata determines how every wallet, explorer and bridge
  interprets an amount, so a wrong exponent is a financial hazard, not a
  cosmetic one.
- **Corrects, never creates.** A denom with no existing metadata is rejected: it
  has never been received or created here, and describing it would invent a
  token that does not exist.
- **Refuses to write the bug it fixes.** `ValidateBasic` rejects metadata whose
  `display` is absent from `denom_units` — so a proposal that would recreate
  `decimals: 0` fails validation instead of silently passing.

## Usage

Governance-only, so there is no bespoke CLI command — submit it as a proposal:

```json
{
  "messages": [{
    "@type": "/mantrachain.denommetadata.v1.MsgSetDenomMetadata",
    "authority": "<gov module address>",
    "metadata": [{
      "base": "ibc/784D26186FFF0AFB5DD933EEE6E6E1C7EEC542B44566E7626E8E4A35BC66CC7E",
      "display": "mantra",
      "name": "MANTRA",
      "symbol": "OM",
      "denom_units": [
        { "denom": "ibc/784D26186FFF0AFB5DD933EEE6E6E1C7EEC542B44566E7626E8E4A35BC66CC7E",
          "exponent": 0, "aliases": ["amantra"] },
        { "denom": "mantra", "exponent": 18 }
      ]
    }]
  }],
  "deposit": "1000000000000000000esp",
  "title": "Correct denom metadata for IBC OM",
  "summary": "Sets the display unit and exponent so decimals() resolves to 18."
}
```

Take the exponent from the **source chain's** metadata, not from the
IBC-derived ERC-20 contract — the contract is what is wrong.

Two things `banktypes.Metadata.Validate()` enforces that catch people out — the
example above already follows both:

- **The first denom unit must be the base denom at exponent 0.** For an IBC
  asset that is the `ibc/<hash>`, not the source chain's base name. Writing
  `{"denom": "amantra", "exponent": 0}` first looks natural and is rejected; put
  the hash first and carry `amantra` as an alias instead — `amantra` is not
  otherwise needed, since `Decimals()` resolves through `display`, not through
  any alias.
- **A unit named by `display` must exist.** This is the rule whose absence causes
  `decimals() = 0`, and bank already enforces it — which is why this module adds
  no check of its own for it. An earlier revision did, and it was removed once
  bank's behaviour was tested rather than assumed.

Together these mean the metadata the chain synthesises on IBC receipt would
**not pass bank's own validation**. It exists only because the transfer module
writes it directly, bypassing `Validate()`.

## Until a proposal lands

Assets that arrived before their metadata was corrected still report
`decimals() = 0`. Anything deriving a scale factor from that value — a Hyperlane
Warp Route, for instance — must read decimals from the source chain instead.
See `SECURITY-MODEL.md` in the interchain repository.
