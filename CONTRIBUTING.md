# Contributing to ArkConstellation

This document covers the working rules for the 72-hour launch sprint. All four engineers must read this before touching the repository.

---

## Repository Structure

```
.
├── app/                  # State machine (Eng 1)
├── cmd/                  # CLI & binary entrypoints (Eng 1)
├── x/                    # Custom Cosmos modules (Eng 1)
├── networks/
│   ├── devnet/           # pystarport configs, peer seeds (Eng 2)
│   └── mainnet/          # genesis.json, gentx submissions (Eng 2)
├── scripts/
│   ├── chaos/            # RPC fuzzer, attack & partition scripts (Eng 3)
│   └── genesis/          # gentx validation & genesis assembly (Eng 2)
└── ops/
    ├── docker/           # Validator / sentry container definitions (Eng 4)
    ├── monitoring/       # Prometheus configs & Grafana dashboards (Eng 4)
    └── runbooks/         # Halts, recovery, key ceremony procedures (Eng 4)
```

---

## Branch Rules

### Integration Branch

`base-genesis` is the integration branch for this sprint. It is pinned to the `v1.0.1` release of the upstream `mantrachain` repo and acts as the source of truth for all track branches.

**Do not push directly to `base-genesis` or `main`.**

### Track Branches

Each engineer owns exactly one track branch, cut from `base-genesis`:

| Branch | Owner | Workspace |
|--------|-------|-----------|
| `track/1-state-machine` | Eng 1 | `app/`, `cmd/`, `x/` |
| `track/2-consensus-genesis` | Eng 2 | `networks/`, `scripts/genesis/` |
| `track/3-security-chaos` | Eng 3 | `scripts/chaos/` |
| `track/4-infra-observability` | Eng 4 | `ops/` |

### Setting Up Your Branch

```bash
git clone https://github.com/Worldstreet-Web-Services/ArkConstellation.git
cd ArkConstellation
git checkout base-genesis

# Replace N and description with your track
git checkout -b track/N-description
git push origin track/N-description
```

### Merging Back

- Open a pull request targeting `base-genesis` (not `main`)
- The CI `build` job must pass before merging
- The track lead must sign off before merge

---

## CI Pipeline

Every push and pull request triggers two automated jobs:

1. **`build`** — Compiles `mantrachaind` using Go 1.25. Uploads the binary as a downloadable artifact. If this fails, the binary is broken.

2. **`validate-genesis`** — Runs after `build`. Downloads the compiled binary and validates any `genesis.json` files found under `networks/`. Skips cleanly if no genesis files exist yet.

**Engineers 2, 3, and 4** do not need Go installed locally. Download the pre-compiled `mantrachaind` binary directly from the GitHub Actions artifact on any passing `build` run:

> GitHub → Actions → most recent passing run → Artifacts → `chain-binary`

---

## Handoff Protocol

All dependencies between engineers are passed as either:

- **Tagged binary releases** (`v0.1.0-alpha`, `v1.0.0-rc1`, `v1.0.0`) — compiled by Eng 1, uploaded automatically by CI
- **Committed files in dedicated directories** (`networks/`, `scripts/`, `ops/`) — no waiting on incomplete local branches

### Dependency Order

```
[Eng 1: v0.1.0-alpha] ──► Eng 2 devnet spinup
                      ──► Eng 3 static analysis begins

[Eng 3: bug reports]  ──► Eng 1 fixes

[Eng 1: v1.0.0-rc1]  ──► Eng 2 multi-validator devnet

[Eng 2: devnet live]  ──► Eng 4 Blockscout deployment
                      ──► Eng 3 load testing

[Eng 2: genesis.json] ──► Eng 4 multisig ceremony
                      ──► All: mainnet genesis
```

---

## Versioning

Eng 1 tags releases. All other engineers pull from CI artifacts — do not build from source unless debugging a specific issue.

| Tag | When | What it means |
|-----|------|---------------|
| `v0.1.0-alpha` | End of Day 1 | First compiling build, modules stripped |
| `v1.0.0-rc1` | End of Day 2 | EVM wiring complete, suitable for multi-validator testnet |
| `v1.0.0` | Day 3 morning | Final frozen binary, reproducible build hash verified |

---

## Security

- **Do not share private keys, mnemonics, or tokens in this repository or in PR comments.**
- Static analysis (Semgrep, GoSec, Slither) must pass before `v1.0.0-rc1` is tagged.
- The Eng 3 chaos sign-off is required before mainnet genesis proceeds.
- See [SECURITY.md](SECURITY.md) for vulnerability reporting.

---

## Key Decisions (Must Be Locked by End of Day 1)

The following decisions block genesis creation. If they slip past Day 1, the timeline slips with them:

- [ ] `x/sanction` — keep or strip?
- [ ] `x/tokenfactory` — keep or strip?
- [ ] `x/tax` — read the module source, then decide
- [ ] Chain ID (Cosmos) and EVM chain ID
- [ ] IBC enabled at genesis?
- [ ] Native token name, symbol, and initial allocations
- [ ] Initial validator set identities (3–5 entities, geographically diverse)
