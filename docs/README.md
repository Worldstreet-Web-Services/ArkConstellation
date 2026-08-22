# docs/

This directory contains all project documentation for the 72-hour launch sprint.

## Structure

```
docs/
├── PRD.md                              # Full product requirements document
├── tasks/
│   ├── track-1-state-machine.md        # Eng 1 task checklist (State Machine Lead)
│   ├── track-2-consensus-genesis.md    # Eng 2 task checklist (Consensus & Genesis Ops)
│   ├── track-3-security-chaos.md       # Eng 3 task checklist (Security & Chaos)
│   └── track-4-infra-observability.md  # Eng 4 task checklist (Infra & Observability)
└── decisions/
    └── module-and-config-decisions.md  # Day 1 module and chain config decision log
```

## Quick Links

| Document | Purpose |
|----------|---------|
| [PRD.md](PRD.md) | Full product requirements, risks, and launch checklist |
| [Track 1 Tasks](tasks/track-1-state-machine.md) | Eng 1: upstream diffs, module pruning, EVM wiring, binary releases |
| [Track 2 Tasks](tasks/track-2-consensus-genesis.md) | Eng 2: genesis params, pystarport devnet, gentx collection |
| [Track 3 Tasks](tasks/track-3-security-chaos.md) | Eng 3: static analysis, chaos testing, deposit-cap contracts |
| [Track 4 Tasks](tasks/track-4-infra-observability.md) | Eng 4: infra, monitoring, Blockscout, key ceremony |
| [Module & Config Decisions](decisions/module-and-config-decisions.md) | Day 1 decision log — must be locked before EOD 1 |

## Rules

- The decisions log must be finalized by **end of Day 1**. If it slips, the timeline slips.
- All engineers check the decisions log before writing any code that depends on module presence or chain config.
- Task files use `[ ]` for pending and `[x]` for completed items. Update your own task file as you go.
