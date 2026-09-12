# Governance execution timelock

This Ark-local module enforces decision #14: every passed governance proposal,
including an expedited proposal, waits at least 48 hours before its messages
execute.

The app wraps the stock Cosmos SDK `x/gov` end blocker. At the end of voting it
performs the normal tally and deposit handling, marks a successful proposal as
passed, and adds it to a time-ordered execution queue. A later end blocker
executes due proposals atomically. If any message fails or panics, none of that
proposal's message writes are committed and the proposal is marked failed.

The queue uses prefix 64 in the existing `x/gov` KV store. The SDK v0.53.x
governance module uses prefixes 0-4, 16, 32, and 48-49. Reusing the store avoids
a new store-key migration while keeping the state in the application export and
import lifecycle under the `govtimelock` genesis key.

Passed proposals retain the standard `PROPOSAL_STATUS_PASSED` status while they
wait because the upstream enum has no scheduled state. Indexers can observe the
`proposal_scheduled` event and its `execution_time` attribute. Completion emits
`proposal_executed`; a failed execution also records `failed_reason` on the
proposal.
