package keeper_test

import (
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/keeper"
	"github.com/cosmos/cosmos-sdk/runtime"
	testutil "github.com/cosmos/cosmos-sdk/testutil"
	"github.com/stretchr/testify/require"
)

func TestQueueOrderingAndExport(t *testing.T) {
	storeKey := storetypes.NewKVStoreKey("govtimelock-test")
	ctx := testutil.DefaultContext(storeKey, storetypes.NewTransientStoreKey("govtimelock-test-transient"))
	k := keeper.NewKeeper(runtime.NewKVStoreService(storeKey))

	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	require.NoError(t, k.Schedule(ctx, 2, start.Add(49*time.Hour)))
	require.NoError(t, k.Schedule(ctx, 1, start.Add(48*time.Hour)))

	due, err := k.Due(ctx, start.Add(48*time.Hour))
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, uint64(1), due[0].ProposalID)

	exported, err := k.Export(ctx)
	require.NoError(t, err)
	require.Len(t, exported, 2)
	require.Equal(t, uint64(1), exported[0].ProposalID)
	require.Equal(t, uint64(2), exported[1].ProposalID)
}
