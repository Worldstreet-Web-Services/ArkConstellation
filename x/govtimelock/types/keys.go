package types

import "cosmossdk.io/collections"

const (
	ModuleName = "govtimelock"
)

// Prefix 64 is reserved by Ark inside the existing x/gov store. Upstream
// v0.53.x currently uses 0-4, 16, 32, and 48-49.
var ScheduledProposalPrefix = collections.NewPrefix(64)
