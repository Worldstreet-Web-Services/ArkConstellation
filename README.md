# ArkConstellation

A sovereign, EVM-compatible L1 blockchain forked from [`MANTRA-Chain/mantrachain`](https://github.com/MANTRA-Chain/mantrachain). Built on Cosmos SDK + CometBFT + `cosmos/evm`.

## Overview

ArkConstellation is a permissioned, EVM-compatible Layer 1 chain targeting production genesis within a 3-day build sprint. It inherits MANTRA's battle-tested integration of Cosmos SDK with EVM execution via `cosmos/evm`, with a module set and parameter set explicitly tuned for this chain's use case rather than MANTRA's regulated RWA positioning.

## Stack

| Component | Version | Source |
|-----------|---------|--------|
| CometBFT | `v0.38.23` | Upstream, unmodified |
| Cosmos SDK | `v0.53.8-v8-mantra-1` | MANTRA fork (audited against upstream) |
| `cosmos/evm` | `v0.6.2-v8-mantra-1` | MANTRA fork (diff required) |
| IBC-go | `v10.5.1` | Upstream, unmodified |
| Go toolchain | `1.25.0` | Per `go.mod` |

## Getting Started

### Prerequisites

- Go `1.25.0` or later
- `make`
- `gcc` (required for ledger support)

### Build

```bash
git clone https://github.com/Worldstreet-Web-Services/ArkConstellation.git
cd ArkConstellation
make build
```

The compiled binary is output to `./build/mantrachaind`.

### Pre-built Binaries

Pre-compiled binaries are published automatically by CI on every push to `base-genesis` and on every version tag. Download them from the **Actions** tab → most recent passing run → **Artifacts** → `chain-binary`. SHA-256 checksums are included.

### Run Tests

```bash
make test-unit
```

## Repository Layout

```
.
├── app/                  # State machine wiring (app.go, module manager)
├── cmd/                  # Binary entrypoints
├── x/                    # Custom Cosmos modules
├── networks/
│   ├── devnet/           # pystarport local testnet configs
│   └── mainnet/          # Production genesis.json and gentx submissions
├── scripts/
│   ├── chaos/            # RPC fuzzer and adversarial test scripts
│   └── genesis/          # gentx validation and genesis assembly
└── ops/
    ├── docker/           # Validator and sentry node container definitions
    ├── monitoring/       # Prometheus configs and Grafana dashboards
    └── runbooks/         # Incident response procedures
```

## Architecture

- **Consensus:** CometBFT — unmodified, do not touch
- **EVM execution:** `cosmos/evm` module, targeting 1–2s block finality
- **Gas token:** 18-decimal denomination (hard requirement of `cosmos/evm`)
- **Validator set:** Permissioned at genesis; progressive decentralization post-launch

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for branch rules, CI pipeline details, the engineer track structure, and the handoff protocol for the 72-hour build sprint.

## Security

See [SECURITY.md](SECURITY.md) for vulnerability reporting.

---

*Base fork: `MANTRA-Chain/mantrachain` @ `v8.4.0`*
