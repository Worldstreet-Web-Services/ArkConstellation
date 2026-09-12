package keeper

import (
	"context"
	"errors"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Keeper struct {
	Schema collections.Schema
	// ScheduledProposals is keyed by (executionTime, proposalID); the
	// proposal ID lives entirely in the key, so this is a set rather than a
	// map to a value that would just duplicate it.
	ScheduledProposals collections.KeySet[collections.Pair[time.Time, uint64]]
	// ActivationHeight records the block height at which the governance
	// execution timelock became active. It is unset until the coordinated
	// upgrade handler runs, and the EndBlocker keeps stock x/gov execution
	// semantics until then so a binary swap alone cannot fork the chain.
	ActivationHeight collections.Item[int64]
	// ExecutionDelay is the configured delay; zero means use the module default.
	ExecutionDelay collections.Item[int64]
}

func NewKeeper(storeService store.KVStoreService) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		ScheduledProposals: collections.NewKeySet(
			sb,
			types.ScheduledProposalPrefix,
			"scheduled_governance_proposals",
			collections.PairKeyCodec(sdk.TimeKey, collections.Uint64Key), //nolint:staticcheck // retain x/gov time encoding
		),
		ActivationHeight: collections.NewItem(
			sb,
			types.ActivationHeightPrefix,
			"govtimelock_activation_height",
			collections.Int64Value,
		),
		ExecutionDelay: collections.NewItem(
			sb,
			types.ExecutionDelayPrefix,
			"govtimelock_execution_delay",
			collections.Int64Value,
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
	return k.ScheduledProposals.Set(ctx, collections.Join(executionTime, proposalID))
}

func (k Keeper) Remove(ctx context.Context, executionTime time.Time, proposalID uint64) error {
	return k.ScheduledProposals.Remove(ctx, collections.Join(executionTime, proposalID))
}

// ClearSchedule drops every scheduled entry. Used when a chain forks or
// resets from exported state (see App.prepForZeroHeightGenesis): a scheduled
// entry's ExecutionTime is an absolute wall-clock time carried over from the
// source chain, so on a fork it may already be in the past — re-adjusting it
// to a new reference time would be an arbitrary guess, so proposals that were
// mid-flight are dropped and must be resubmitted on the new chain instead.
func (k Keeper) ClearSchedule(ctx context.Context) error {
	return k.ScheduledProposals.Clear(ctx, nil)
}

// SetActivationHeight records the height at which the timelock takes effect.
// Called once, from the coordinated upgrade handler.
func (k Keeper) SetActivationHeight(ctx context.Context, height int64) error {
	return k.ActivationHeight.Set(ctx, height)
}

// GetActivationHeight returns the recorded activation height, or 0 if the
// timelock has not been activated.
func (k Keeper) GetActivationHeight(ctx context.Context) (int64, error) {
	height, err := k.ActivationHeight.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return height, nil
}

// SetExecutionDelay records the configured execution delay.
func (k Keeper) SetExecutionDelay(ctx context.Context, delay time.Duration) error {
	return k.ExecutionDelay.Set(ctx, int64(delay))
}

// GetExecutionDelay returns the configured delay, or fallback if none is set.
func (k Keeper) GetExecutionDelay(ctx context.Context, fallback time.Duration) (time.Duration, error) {
	stored, err := k.ExecutionDelay.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return fallback, nil
		}
		return 0, err
	}
	if stored <= 0 {
		return fallback, nil
	}
	return time.Duration(stored), nil
}

// IsActive reports whether the execution timelock governs proposal execution at
// the given height. It is false until the upgrade handler sets the activation
// height, which keeps old and new binaries in agreement before the upgrade.
func (k Keeper) IsActive(ctx context.Context, height int64) (bool, error) {
	activationHeight, err := k.ActivationHeight.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return height >= activationHeight, nil
}

// Due returns the entries scheduled at or before now.
func (k Keeper) Due(ctx context.Context, now time.Time) ([]types.ScheduledProposal, error) {
	return k.collect(ctx, collections.NewPrefixUntilPairRange[time.Time, uint64](now))
}

// Export returns every scheduled entry, for genesis export.
func (k Keeper) Export(ctx context.Context) ([]types.ScheduledProposal, error) {
	return k.collect(ctx, nil)
}

// collect gathers the scheduled entries matching rng; a nil rng means all.
func (k Keeper) collect(ctx context.Context, rng collections.Ranger[collections.Pair[time.Time, uint64]]) ([]types.ScheduledProposal, error) {
	iter, err := k.ScheduledProposals.Iterate(ctx, rng)
	if err != nil {
		return nil, err
	}

	keys, err := iter.Keys()
	if err != nil {
		return nil, err
	}

	scheduled := make([]types.ScheduledProposal, 0, len(keys))
	for _, key := range keys {
		scheduled = append(scheduled, types.ScheduledProposal{
			ProposalID:    key.K2(),
			ExecutionTime: key.K1(),
		})
	}
	return scheduled, nil
}
