# Mainnet genesis day — operator runbook

One-shot, in order. This pipeline runs once for real — if a step's
prerequisite isn't met, stop and fix it before proceeding; every script
here fails loudly rather than silently on bad input, so a hard stop is the
expected/safe outcome of a real problem, not something to route around.

**Before starting**, confirm all of these are actually true — do not take
any on faith:

- [ ] Eng 1 has tagged the frozen binary (`v1.0.0` per `CONTRIBUTING.md`'s
      versioning table) and its CI-published SHA-256 is recorded somewhere
      independent of this repo (e.g. pinned in the genesis coordination
      channel).
- [ ] `CONTRIBUTING.md`'s "Key Decisions" checklist is fully resolved —
      chain-id, native token allocations, `x/tax`/`x/tokenfactory`/
      `x/sanction` keep-or-strip, initial validator identities. If any box
      is still open, stop — do not improvise a value here.
- [ ] Eng 3's chaos/security sign-off has actually landed (required before
      mainnet genesis per `CONTRIBUTING.md`).
- [ ] Every prospective validator has been sent
      `networks/mainnet/genesis-params.json` and the account-allocation
      list, and has confirmed their intended self-delegation amount and
      moniker.
- [ ] `gov.min_deposit`/`expedited_min_deposit` in `genesis-params.json` are
      NOT the reviewed final numbers as of this writing - decision #15 in
      `docs/decisions/module-and-config-decisions.md` blocks them on a total
      supply figure that hasn't been set. Confirm this has since been
      resolved and the file updated; do not launch mainnet on the
      placeholder figures currently in that file.
- [ ] Governance timelock (decision #14, locked, minimum 48h) is **not
      implemented anywhere in this codebase** - stock cosmos-sdk `x/gov` has
      no timelock field or hook. If launch is proceeding without it, that is
      a conscious, documented risk acceptance, not an oversight; confirm
      someone has actually made that call rather than assuming it's handled.
- [ ] `evm.params.active_static_precompiles` in the assembled genesis matches
      `ArkActiveStaticPrecompiles()` in `app/precompiles.go` **as compiled into
      the tagged binary** - 9 addresses, sorted, lowercase. `validate-genesis`
      does NOT check this: an empty list boots fine and silently makes every
      Cosmos module unreachable from Solidity (this is how the array shipped as
      `[]` through an entire audit cycle - issue #37), and an address that is
      active in params but not registered in the binary passes validation and
      then **panics on first call**, reachable over unauthenticated `eth_call`.
      `go test ./app/ -run Precompile` against the tagged commit is the check;
      `0x...0803` must NOT appear. See
      `docs/decisions/proposals/precompile-enablement-proposal.md`.
- [ ] Eng 3 has actually signed off on Staking (`0x...0800`), ICS20
      (`0x...0802`) and `distrclaim` (`0x...0a01`) specifically. These three are
      recorded as blocking `v1.0.0` in the precompile proposal's sign-off table
      and were NOT covered by the Day 1 static analysis, whose precompile table
      was wrong in eight of ten rows (since corrected). If the reviews have not
      landed, remove those addresses from the genesis list rather than launching
      and reviewing afterwards.

## Step 1 — Assemble the base genesis

```bash
BIN=./build/arkd          # the tagged binary, not a local rebuild
CHAIN_ID=arkconstellation-1   # locked, decision #6 in docs/decisions/module-and-config-decisions.md
                               # cannot change after genesis without a full chain migration

$BIN init <moniker> --chain-id "$CHAIN_ID" --home /tmp/mainnet-genesis

# Add every real, decided allocation - the exchange/foundation/team/
# ecosystem accounts from the locked allocation list, AND every
# prospective validator's self-delegation funding account. A validator
# whose account isn't funded here will be rejected in Step 2, correctly.
$BIN genesis add-genesis-account <address> <amount>esp --home /tmp/mainnet-genesis
# esp is the base unit (1 wei-equivalent), espees the intermediate unit
# (10^9 esp, 1 gwei-equivalent, gas-price display only), KASH the
# 18-decimal display unit (1 KASH = 10^18 esp = 10^9 espees). Locked,
# decision #8 - do not use amounts in any other denom here.
# ... repeat for every account ...

# EVM chain-id (decision #7, locked to 11199) does NOT need a separate
# app.toml setting for mainnet: app/config.go's EVMChainIDMap already maps
# "arkconstellation-1" -> 11199 as a package-level default, and its init()
# reads the Cosmos chain-id straight out of genesis.json at node startup to
# resolve it - verified by reading that source directly, not assumed. Only
# an operator running a NON-standard chain-id would ever need to set
# `evm.evm-chain-id` in app.toml manually.

python3 -c "
import json
g = json.load(open('/tmp/mainnet-genesis/config/genesis.json'))
patch = json.load(open('networks/mainnet/genesis-params.json'))
patch.pop('_comment', None)
# bank.denom_metadata is REQUIRED, not optional: the evm module's
# InitGenesis panics at node startup without an entry for the bond/mint/evm
# denom ('error initializing evm coin info: denom metadata <denom> could
# not be found') - discovered empirically standing up the devnet, see
# networks/devnet/genesis-template.json's _comment for the full story.
# genesis-params.json above already carries the correct, current
# denom_metadata (esp/espees/KASH with aliases) - it is NOT redefined here.
# Do not hand-roll a second copy of it in this script: two copies of the
# same value drift, and only genesis-params.json is reviewed as the source
# of truth for the denom scheme.
def merge(a, b):
    for k, v in b.items():
        if isinstance(v, dict) and isinstance(a.get(k), dict): merge(a[k], v)
        else: a[k] = v
merge(g, patch)
json.dump(g, open('/tmp/mainnet-genesis/config/genesis.json', 'w'))
"
$BIN genesis validate-genesis /tmp/mainnet-genesis/config/genesis.json
cp /tmp/mainnet-genesis/config/genesis.json networks/mainnet/base-genesis.json
```

`validate-genesis` only checks structural/proto validity - it will NOT catch
a missing `denom_metadata` entry (that's a runtime panic during `InitChain`,
not a static validation error). Before trusting this base genesis, do a
throwaway single-node boot smoke test and confirm it reaches height 1
without panicking:

```bash
rm -rf /tmp/smoke-test
$BIN init smoketest --chain-id "$CHAIN_ID" --home /tmp/smoke-test >/dev/null
cp networks/mainnet/base-genesis.json /tmp/smoke-test/config/genesis.json
timeout 10 $BIN start --home /tmp/smoke-test --minimum-gas-prices 0esp || true
# look for "committed state" / height advancing in the output above, not a panic
```

If `genesis-params.json` needs a real value for something left as a
devnet-only placeholder in this repo (`x/tax.mca_address`,
`x/tokenfactory.fee_collector_address` — see that file's header comment),
resolve it now, before continuing. Do not carry a devnet placeholder
address into this file.

## Step 2 — Collect and validate gentxs

Each validator submits their `gentx-<moniker>.json` through the agreed
secure channel (not a public PR — see `SECURITY.md`) into a single
directory.

```bash
MANTRACHAIND_BIN=$BIN ./scripts/genesis/collect-gentx.sh \
  <gentx-submissions-dir> \
  networks/mainnet/base-genesis.json \
  networks/mainnet/genesis-template.json
```

Read the output carefully. Every `REJECT` line names the file and the
specific reason. **Do not proceed with a partial validator set** — resolve
each rejection with its submitter (resubmit, or explicitly agree to
exclude them) and re-run this step from scratch until it exits 0.

## Step 3 — Hash and cross-publish

```bash
./scripts/genesis/hash-genesis.sh networks/mainnet/genesis-template.json
```

Publish the **canonical** hash (not the raw-bytes one — see the script's
output for why) to every validator through the agreed channel. Every
validator independently runs this same command against the file you send
them and confirms the hash matches before going further. A single
mismatch means stop and find out why before any node starts.

## Step 4 — Commit and tag

```bash
cp networks/mainnet/genesis-template.json networks/mainnet/genesis.json
git add networks/mainnet/genesis.json
git commit -m "genesis: canonical mainnet genesis, hash <canonical hash from step 3>"
```

Open a PR per `CONTRIBUTING.md`'s branch rules (never push directly to
`base-genesis` or `main`) even under time pressure — the "track lead sign
off before merge" requirement exists specifically for this file.

## Step 5 — Coordinated start

All validators start their nodes at the agreed genesis time
(`genesis_time` in the file — everyone is starting the *same* file, so
this is already fixed). Confirm with each validator that
`mantrachaind genesis validate-genesis` passed locally on their copy
before they start.

## If something goes wrong after nodes start

This runbook covers assembly, not incident response — halts, recovery,
and the key ceremony process live in `ops/runbooks/` (Eng 4's track). If
the chain fails to reach block 1, the most likely causes, in order of
probability based on what this pipeline already checked: a validator
started with a genesis file that doesn't match the published hash (go
back to Step 3), or a gentx that passed `collect-gentx.sh`'s checks here
but still fails differently under the *tagged binary* if it differs from
what was used to build `networks/mainnet/base-genesis.json` in Step 1
(always use the same tagged binary for every step of this runbook).
