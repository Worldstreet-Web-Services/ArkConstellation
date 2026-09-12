package types

import (
	"time"

	"cosmossdk.io/collections"
)

const (
	ModuleName = "govtimelock"

	// MinimumDelay is the shortest execution delay a production chain may
	// configure; GenesisState.Validate enforces it.
	MinimumDelay = 48 * time.Hour
)

// Prefixes 64-66 are reserved by Ark inside the existing x/gov store.
// Upstream v0.53.x currently uses 0-4, 16, 32, and 48-49.
//
// x/govtimelock deliberately has no StoreKey of its own and shares x/gov's
// KVStore, so these prefixes must never collide with a prefix that x/gov
// itself claims. That invariant is asserted by TestPrefixesDoNotCollideWithGov
// so an SDK bump that claims either value fails a test instead of silently
// corrupting governance state.
var (
	ScheduledProposalPrefix = collections.NewPrefix(64)
	ActivationHeightPrefix  = collections.NewPrefix(65)
	ExecutionDelayPrefix    = collections.NewPrefix(66)
)

// ReservedPrefixes lists every prefix govtimelock claims in the shared x/gov
// store, for the collision test.
var ReservedPrefixes = [][]byte{
	ScheduledProposalPrefix.Bytes(),
	ActivationHeightPrefix.Bytes(),
	ExecutionDelayPrefix.Bytes(),
}
