package app

import (
	"testing"
	"time"

	govtimelock "github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/stretchr/testify/require"
)

func TestGovernanceTimelockExecutesOnlyAtBoundary(t *testing.T) {
	require.Equal(t, 48*time.Hour, govtimelock.MinimumDelay)

	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	original := banktypes.DefaultParams()
	require.NoError(t, chain.BankKeeper.SetParams(ctx, original))
	updated := original
	updated.DefaultSendEnabled = !original.DefaultSendEnabled
	proposal := makePassedProposal(t, &banktypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    updated,
	}, 77, start)
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, proposal.Id, start.Add(govtimelock.MinimumDelay)))

	beforeBoundary := ctx.WithBlockTime(start.Add(govtimelock.MinimumDelay - time.Nanosecond))
	require.NoError(t, govtimelock.EndBlocker(beforeBoundary, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))
	require.Equal(t, original.DefaultSendEnabled, chain.BankKeeper.GetParams(beforeBoundary).DefaultSendEnabled)

	atBoundary := ctx.WithBlockTime(start.Add(govtimelock.MinimumDelay))
	require.NoError(t, govtimelock.EndBlocker(atBoundary, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))
	require.Equal(t, updated.DefaultSendEnabled, chain.BankKeeper.GetParams(atBoundary).DefaultSendEnabled)

	due, err := chain.GovTimelockKeeper.Due(atBoundary, atBoundary.BlockTime())
	require.NoError(t, err)
	require.Empty(t, due)
}

func TestGovernanceTimelockPreservesAtomicExecution(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	original := banktypes.DefaultParams()
	require.NoError(t, chain.BankKeeper.SetParams(ctx, original))
	updated := original
	updated.DefaultSendEnabled = !original.DefaultSendEnabled
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	proposal := makePassedProposalWithMessages(t, []sdk.Msg{
		&banktypes.MsgUpdateParams{Authority: authority, Params: updated},
		&banktypes.MsgUpdateParams{Authority: sdk.AccAddress("not-governance").String(), Params: original},
	}, 78, start)
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, proposal.Id, start))

	require.NoError(t, govtimelock.EndBlocker(ctx, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))
	require.Equal(t, original.DefaultSendEnabled, chain.BankKeeper.GetParams(ctx).DefaultSendEnabled)

	stored, err := chain.GovKeeper.Proposals.Get(ctx, proposal.Id)
	require.NoError(t, err)
	require.Equal(t, govv1.StatusFailed, stored.Status)
	require.NotEmpty(t, stored.FailedReason)
}

func makePassedProposal(t *testing.T, msg sdk.Msg, id uint64, now time.Time) govv1.Proposal {
	t.Helper()
	return makePassedProposalWithMessages(t, []sdk.Msg{msg}, id, now)
}

func makePassedProposalWithMessages(t *testing.T, msgs []sdk.Msg, id uint64, now time.Time) govv1.Proposal {
	t.Helper()
	proposal, err := govv1.NewProposal(msgs, id, now, now, "", "timelock test", "timelock test", sdk.AccAddress("proposer"), false)
	require.NoError(t, err)
	proposal.Status = govv1.StatusPassed
	return proposal
}
