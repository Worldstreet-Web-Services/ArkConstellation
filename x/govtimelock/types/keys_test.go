package types_test

import (
	"testing"

	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"
)

// TestPrefixesDoNotCollideWithGov guards the invariant that x/govtimelock
// reuses x/gov's KVStore rather than owning a StoreKey: its prefixes must never
// overlap one x/gov claims. The two modules build their collections.Schemas
// independently, so nothing else catches an SDK bump that starts using prefix
// 64 or 65 — without this test that collision would silently corrupt
// governance state.
func TestPrefixesDoNotCollideWithGov(t *testing.T) {
	govPrefixes := map[string]struct {
		name  string
		bytes []byte
	}{
		"proposals":      {"ProposalsKeyPrefix", govtypes.ProposalsKeyPrefix.Bytes()},
		"active_queue":   {"ActiveProposalQueuePrefix", govtypes.ActiveProposalQueuePrefix.Bytes()},
		"inactive_queue": {"InactiveProposalQueuePrefix", govtypes.InactiveProposalQueuePrefix.Bytes()},
		"proposal_id":    {"ProposalIDKey", govtypes.ProposalIDKey.Bytes()},
		"voting_period":  {"VotingPeriodProposalKeyPrefix", govtypes.VotingPeriodProposalKeyPrefix.Bytes()},
		"deposits":       {"DepositsKeyPrefix", govtypes.DepositsKeyPrefix.Bytes()},
		"votes":          {"VotesKeyPrefix", govtypes.VotesKeyPrefix.Bytes()},
		"params":         {"ParamsKey", govtypes.ParamsKey.Bytes()},
		"constitution":   {"ConstitutionKey", govtypes.ConstitutionKey.Bytes()},
	}

	for _, reserved := range types.ReservedPrefixes {
		for _, gov := range govPrefixes {
			require.NotEqual(t, gov.bytes, reserved,
				"govtimelock prefix %v collides with x/gov %s; pick a free prefix or give govtimelock its own StoreKey",
				reserved, gov.name)
		}
	}
}

// TestReservedPrefixesAreDistinct ensures the module's own prefixes do not
// collide with each other.
func TestReservedPrefixesAreDistinct(t *testing.T) {
	seen := make(map[string]struct{}, len(types.ReservedPrefixes))
	for _, prefix := range types.ReservedPrefixes {
		_, dup := seen[string(prefix)]
		require.False(t, dup, "duplicate govtimelock prefix %v", prefix)
		seen[string(prefix)] = struct{}{}
	}
}
