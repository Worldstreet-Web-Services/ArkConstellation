# Governance execution timelock (`x/govtimelock`)

## What it does

Passed governance proposals no longer execute in the block their voting period ends. They are recorded with an execution time of `block_time + delay` (48h in production) and their messages run atomically in the first EndBlock at or after that time. The delay gives the chain a window to react — via a follow-up proposal, a coordinated halt, or an emergency upgrade — before a malicious or mistaken proposal takes effect.

## Activation is gated on a coordinated upgrade height

Deferring execution changes what EndBlock does, which makes it a consensus-behavior change rather than a local one. If it took effect the moment an operator swapped binaries, a node on the new binary would schedule a passed proposal while a node still on the old binary executed it immediately in the same block — an AppHash divergence the first time any proposal passed after the swap.

The timelock is therefore inert until an activation height is recorded in state:

- **Existing chains** activate through the `v8.5.0` upgrade handler (`app/upgrades/v8_5`), which records the height at which the upgrade ran. Every node reaches that height at the same block regardless of when its operator installed the binary, so old and new binaries agree on behavior right up to the activation point.
- **New chains** activate at height 1 via `DefaultGenesis`, so a fresh network runs with the timelock from its first block rather than silently running without it until somebody remembers to schedule an upgrade.

`Keeper.IsActive` reports the gate, and before activation the EndBlocker runs stock `x/gov` execution semantics — passed proposals execute immediately, and no scheduled entries are consumed.

## Failure handling: a bad entry must never halt the chain

`executeMaturedProposals` runs inside EndBlock, and the SDK module manager (`types/module/module.go`) treats any error returned from EndBlock as fatal for the block with no per-module recovery. A scheduled entry whose proposal cannot be loaded, or whose status is no longer `PROPOSAL_STATUS_PASSED`, is therefore **dropped rather than returned as an error**: it is removed from the schedule, logged, and reported through a `proposal_executed` event with a failed result.

This matters more than the usual "handle the error" case. The scheduled entry is only removed after a successful execution pass, so returning an error would leave the entry due again on the next block, and the next — a permanent halt from that height forward rather than one bad block. The sibling loops in the same file (`processInactiveProposals`, `processEndedVotingPeriods`) already use per-proposal recovery for exactly this reason; the execution loop now matches them.

Genesis is validated against `x/gov` at `InitGenesis` for the same reason. `GenesisState.Validate` only sees this module's own state, so it structurally cannot catch a scheduled entry naming a proposal `x/gov` does not have (the entry that walks into the halt path above), or a `PROPOSAL_STATUS_PASSED` proposal with no schedule (a proposal stranded forever, its messages never executing). Both cross-module invariants are enforced where both stores are available.

## Known semantic caveat: `PROPOSAL_STATUS_PASSED` covers the pending window

A scheduled proposal is set to `PROPOSAL_STATUS_PASSED` at tally time, and the SDK documents that value as "passed and successfully executed". Under the timelock it additionally covers the pending-execution window — up to 48 hours in which the proposal has passed but its messages have **not** run.

Consequences, accepted deliberately:

- Indexers, explorers, wallets, and other chains reading v1 status over IBC/ICS will report such a proposal as executed for the whole delay window.
- The `EventTypeActiveProposal` emitted at tally time is byte-identical to stock's post-execution event.
- Only the module's own `proposal_scheduled` and `proposal_executed` events distinguish the two states. Consumers that need the distinction must watch those.

Adding a distinct status value would be the cleaner fix, but `govv1.ProposalStatus` is part of the v1 wire format: a new enum value changes what every existing client sees and breaks cross-chain consumers that switch exhaustively on the current set. That is a larger, separately-reviewed change than this PR should carry, so the reuse is documented rather than fixed here.

## Hook ordering

`AfterProposalVotingPeriodEnded` fires **after** a proposal's messages execute, matching stock `x/gov`. For a timelocked proposal that means the hook is deferred to `executeMaturedProposals` rather than firing at tally time up to 48h earlier. The only registered consumer today (the ICS provider keeper) has a no-op implementation, but preserving the ordering keeps the trap from being sprung on the next consumer.

## Store layout

`x/govtimelock` has no `StoreKey` of its own; it shares `x/gov`'s KVStore under prefixes 64-66 (`ScheduledProposalPrefix`, `ActivationHeightPrefix`, `ExecutionDelayPrefix`). Upstream `x/gov` in the pinned SDK uses 0-4, 16, 32 and 48-49, so the range is free today. Because the two modules build their `collections.Schema`s independently, nothing at runtime would catch an SDK bump that started using one of these prefixes — the collision would silently corrupt governance state. `TestPrefixesDoNotCollideWithGov` asserts the invariant so such a bump fails a test instead.

## Configurable delay

The 48h delay is the default (`MinimumDelay`), not a hard-coded constant in the execution path. `GenesisState.ExecutionDelay` overrides it so test networks can use a short delay; production genesis leaves it unset. This is what allows the e2e and interchain governance suites — which assert post-execution effects within seconds — to keep working.
