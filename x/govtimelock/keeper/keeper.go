package keeper

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Keeper struct {
	Schema             collections.Schema
	ScheduledProposals collections.Map[collections.Pair[time.Time, uint64], uint64]
}

func NewKeeper(storeService store.KVStoreService) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		ScheduledProposals: collections.NewMap(
			sb,
			types.ScheduledProposalPrefix,
			"scheduled_governance_proposals",
			collections.PairKeyCodec(sdk.TimeKey, collections.Uint64Key), //nolint:staticcheck // retain x/gov time encoding
			collections.Uint64Value,
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema
	return k
}

func (k Keeper) Schedule(ctx context.Context, proposalID uint64, executionTime time.Time) error {
	return k.ScheduledProposals.Set(ctx, collections.Join(executionTime, proposalID), proposalID)
}

func (k Keeper) Remove(ctx context.Context, executionTime time.Time, proposalID uint64) error {
	return k.ScheduledProposals.Remove(ctx, collections.Join(executionTime, proposalID))
}

func (k Keeper) Due(ctx context.Context, now time.Time) ([]types.ScheduledProposal, error) {
	iter, err := k.ScheduledProposals.Iterate(ctx, collections.NewPrefixUntilPairRange[time.Time, uint64](now))
	if err != nil {
		return nil, err
	}

	entries, err := iter.KeyValues()
	if err != nil {
		return nil, err
	}

	due := make([]types.ScheduledProposal, 0, len(entries))
	for _, entry := range entries {
		due = append(due, types.ScheduledProposal{
			ProposalID:    entry.Value,
			ExecutionTime: entry.Key.K1(),
		})
	}
	return due, nil
}

func (k Keeper) Export(ctx context.Context) ([]types.ScheduledProposal, error) {
	iter, err := k.ScheduledProposals.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}

	entries, err := iter.KeyValues()
	if err != nil {
		return nil, err
	}

	scheduled := make([]types.ScheduledProposal, 0, len(entries))
	for _, entry := range entries {
		scheduled = append(scheduled, types.ScheduledProposal{
			ProposalID:    entry.Value,
			ExecutionTime: entry.Key.K1(),
		})
	}
	return scheduled, nil
}
