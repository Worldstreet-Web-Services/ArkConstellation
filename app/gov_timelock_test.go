package app

import (
	"encoding/json"
	"testing"
	"time"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/MANTRA-Chain/mantrachain/v8/app/upgrades"
	v8_5 "github.com/MANTRA-Chain/mantrachain/v8/app/upgrades/v8_5"
	govtimelock "github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock"
	govtimelocktypes "github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
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
	activateTimelock(t, chain, ctx)

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
	activateTimelock(t, chain, ctx)

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

// activateTimelock marks the execution timelock active from the context's
// height, standing in for the coordinated v8.5.0 upgrade handler.
func activateTimelock(t *testing.T, chain *App, ctx sdk.Context) {
	t.Helper()
	require.NoError(t, chain.GovTimelockKeeper.SetActivationHeight(ctx, ctx.BlockHeight()))
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

// TestGovernanceTimelockDoesNotHaltOnMissingProposal covers the chain-halt
// class: a scheduled entry whose gov record is absent must not abort EndBlock.
// Before the per-entry recovery, this returned an error out of EndBlock — and
// because the entry was never removed it stayed due on every later block, so
// the halt was permanent rather than a single bad block.
func TestGovernanceTimelockDoesNotHaltOnMissingProposal(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})
	activateTimelock(t, chain, ctx)

	// Schedule a proposal that does not exist in x/gov.
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, 4242, start))

	require.NoError(t, govtimelock.EndBlocker(ctx, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))

	// The bad entry must be gone, so the next block is unaffected.
	due, err := chain.GovTimelockKeeper.Due(ctx, ctx.BlockTime())
	require.NoError(t, err)
	require.Empty(t, due, "bad entry must be dropped, otherwise it halts every subsequent block")

	next := ctx.WithBlockTime(start.Add(time.Minute))
	require.NoError(t, govtimelock.EndBlocker(next, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))
}

// TestGovernanceTimelockDoesNotHaltOnUnexpectedStatus covers the sibling case:
// a scheduled proposal whose status moved off StatusPassed during the delay
// window must be dropped, not executed, and must not abort EndBlock.
func TestGovernanceTimelockDoesNotHaltOnUnexpectedStatus(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})
	activateTimelock(t, chain, ctx)

	original := banktypes.DefaultParams()
	require.NoError(t, chain.BankKeeper.SetParams(ctx, original))
	updated := original
	updated.DefaultSendEnabled = !original.DefaultSendEnabled

	proposal := makePassedProposal(t, &banktypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    updated,
	}, 79, start)
	proposal.Status = govv1.StatusRejected
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, proposal.Id, start))

	require.NoError(t, govtimelock.EndBlocker(ctx, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))

	// Messages must NOT have run for a non-passed proposal.
	require.Equal(t, original.DefaultSendEnabled, chain.BankKeeper.GetParams(ctx).DefaultSendEnabled)

	due, err := chain.GovTimelockKeeper.Due(ctx, ctx.BlockTime())
	require.NoError(t, err)
	require.Empty(t, due)
}

// TestGovernanceTimelockInertBeforeActivation is the consensus-safety gate: on
// a chain that has not run the v8.5.0 upgrade, a passed proposal must execute
// immediately with stock x/gov semantics. Without this, a node running the new
// binary would defer execution while an un-upgraded peer executed at once —
// an immediate AppHash divergence.
func TestGovernanceTimelockInertBeforeActivation(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})
	// Deliberately no activateTimelock: this chain has not upgraded.

	active, err := chain.GovTimelockKeeper.IsActive(ctx, ctx.BlockHeight())
	require.NoError(t, err)
	require.False(t, active, "timelock must be inert until the upgrade handler runs")

	// A scheduled entry that is already due must be left untouched while
	// inactive, rather than executed by the new binary ahead of its peers.
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, 4243, start))
	require.NoError(t, govtimelock.EndBlocker(ctx, &chain.GovKeeper, chain.GovTimelockKeeper, govtimelock.MinimumDelay))

	due, err := chain.GovTimelockKeeper.Due(ctx, ctx.BlockTime())
	require.NoError(t, err)
	require.Len(t, due, 1, "pre-activation EndBlocker must not consume scheduled entries")
}

// TestGovernanceTimelockActivatesAtHeight confirms the gate flips exactly at
// the recorded activation height and not before.
func TestGovernanceTimelockActivatesAtHeight(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	require.NoError(t, chain.GovTimelockKeeper.SetActivationHeight(ctx, 100))

	active, err := chain.GovTimelockKeeper.IsActive(ctx, 99)
	require.NoError(t, err)
	require.False(t, active)

	active, err = chain.GovTimelockKeeper.IsActive(ctx, 100)
	require.NoError(t, err)
	require.True(t, active)

	active, err = chain.GovTimelockKeeper.IsActive(ctx, 101)
	require.NoError(t, err)
	require.True(t, active)
}

// TestGovTimelockGenesisRejectsUnscheduledPassedProposal covers the genesis
// cross-module invariant that types.GenesisState.Validate structurally cannot
// see: a PROPOSAL_STATUS_PASSED proposal in x/gov with no matching schedule
// would otherwise be stranded forever, its messages never executing.
func TestGovTimelockGenesisRejectsUnscheduledPassedProposal(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	proposal := makePassedProposal(t, &banktypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    banktypes.DefaultParams(),
	}, 91, start)
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))

	module := govtimelock.NewAppModule(chain.GovTimelockKeeper, &chain.GovKeeper)
	empty, err := json.Marshal(govtimelocktypes.DefaultGenesis())
	require.NoError(t, err)

	require.PanicsWithError(t,
		"x/gov proposal 91 is PROPOSAL_STATUS_PASSED but has no govtimelock schedule; its messages would never execute",
		func() { module.InitGenesis(ctx, chain.AppCodec(), empty) },
	)
}

// TestGovTimelockGenesisRejectsDanglingSchedule covers the inverse mismatch: a
// scheduled entry naming a proposal x/gov does not have. Left unchecked this is
// exactly the entry that reaches executeMaturedProposals and, before the
// per-entry recovery, halted the chain.
func TestGovTimelockGenesisRejectsDanglingSchedule(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	module := govtimelock.NewAppModule(chain.GovTimelockKeeper, &chain.GovKeeper)
	state, err := json.Marshal(govtimelocktypes.GenesisState{
		ActivationHeight:   1,
		ScheduledProposals: []govtimelocktypes.ScheduledProposal{{ProposalID: 92, ExecutionTime: start}},
	})
	require.NoError(t, err)

	require.Panics(t, func() { module.InitGenesis(ctx, chain.AppCodec(), state) })
}

// TestGovTimelockGenesisAcceptsMatchedPair confirms a consistent genesis still
// initializes cleanly.
func TestGovTimelockGenesisAcceptsMatchedPair(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	proposal := makePassedProposal(t, &banktypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    banktypes.DefaultParams(),
	}, 93, start)
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))

	module := govtimelock.NewAppModule(chain.GovTimelockKeeper, &chain.GovKeeper)
	state, err := json.Marshal(govtimelocktypes.GenesisState{
		ActivationHeight:   1,
		ScheduledProposals: []govtimelocktypes.ScheduledProposal{{ProposalID: 93, ExecutionTime: start.Add(48 * time.Hour)}},
	})
	require.NoError(t, err)

	require.NotPanics(t, func() { module.InitGenesis(ctx, chain.AppCodec(), state) })

	scheduled, err := chain.GovTimelockKeeper.Export(ctx)
	require.NoError(t, err)
	require.Len(t, scheduled, 1)
	require.Equal(t, uint64(93), scheduled[0].ProposalID)
}

// TestGovTimelockActiveOnFreshChain guards the case the activation gate could
// otherwise regress: a brand-new chain must have the timelock ON from genesis.
// Only the upgrade handler sets the activation height for an existing chain, so
// without DefaultGenesis activating at height 1 a fresh mainnet would silently
// run with no timelock at all.
func TestGovTimelockActiveOnFreshChain(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	module := govtimelock.NewAppModule(chain.GovTimelockKeeper, &chain.GovKeeper)
	module.InitGenesis(ctx, chain.AppCodec(), module.DefaultGenesis(chain.AppCodec()))

	active, err := chain.GovTimelockKeeper.IsActive(ctx, 1)
	require.NoError(t, err)
	require.True(t, active, "a chain starting from genesis must have the timelock active")
}

// TestGovTimelockValidateGenesisRejectsBelowMinimumDelay confirms the
// `genesis validate-genesis` CLI path (AppModuleBasic.ValidateGenesis, which
// scripts/genesis/collect-gentx.sh runs before any real genesis is finalized)
// enforces the 48h minimum. InitGenesis itself deliberately does not — see
// the comment on GenesisState.ExecutionDelay — so this is the actual
// enforcement point for a hand-edited or script-produced genesis.
func TestGovTimelockValidateGenesisRejectsBelowMinimumDelay(t *testing.T) {
	chain := SetupWithEmptyStore(t)

	basic := govtimelock.AppModuleBasic{}
	state, err := json.Marshal(govtimelocktypes.GenesisState{
		ActivationHeight: 1,
		ExecutionDelay:   time.Second,
	})
	require.NoError(t, err)

	err = basic.ValidateGenesis(chain.AppCodec(), nil, state)
	require.ErrorContains(t, err, "execution delay must be at least 48h0m0s, got 1s")
}

// TestExportForZeroHeightResetsGovTimelockActivationHeight covers the fork
// class: `--for-zero-height` resets staking/distribution/slashing state for a
// fresh start, and must do the same for govtimelock's activation height.
// Left as the source chain's (large) height, the forked chain — starting at
// height 1 — would silently run with no timelock until it grinds up to that
// height. Calls the unexported prepForZeroHeightGenesis directly (this test
// file is in package app) rather than the full ExportAppStateAndValidators,
// which needs a fully InitChain-ed app with EVM genesis coin info this
// package's bare test helpers don't set up.
func TestExportForZeroHeightResetsGovTimelockActivationHeight(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	ctx := chain.NewUncachedContext(false, tmproto.Header{})
	require.NoError(t, chain.GovTimelockKeeper.SetActivationHeight(ctx, 4_000_000))

	// A proposal mid-flight on the source chain, waiting out its 48h delay.
	// Its ExecutionTime is an absolute wall-clock time that would already be
	// in the past by the time a forked chain actually launches, letting it
	// execute in the new chain's very first block if carried over verbatim.
	require.NoError(t, chain.GovTimelockKeeper.Schedule(ctx, 42, time.Now().Add(48*time.Hour)))

	chain.prepForZeroHeightGenesis(ctx, nil)

	height, err := chain.GovTimelockKeeper.GetActivationHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), height,
		"a zero-height export must not carry over a stale activation height from the source chain")

	due, err := chain.GovTimelockKeeper.Due(ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, due, "a zero-height export must not carry over an in-flight schedule from the source chain")
}

// TestV8_5UpgradeHandlerActivatesOnExistingChainWithPassedProposals covers the
// upgrade-handler halt class: govtimelock is a brand-new module, so
// module.Manager.RunMigrations would otherwise call its InitGenesis with an
// empty DefaultGenesis(), and InitGenesis's cross-validation against x/gov
// panics on any proposal that has ever passed. Any real chain has such
// proposals, so the upgrade handler must activate the timelock without
// routing through InitGenesis at all.
func TestV8_5UpgradeHandlerActivatesOnExistingChainWithPassedProposals(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start, Height: 100})

	proposal := makePassedProposal(t, &banktypes.MsgUpdateParams{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Params:    banktypes.DefaultParams(),
	}, 94, start)
	require.NoError(t, chain.GovKeeper.SetProposal(ctx, proposal))

	// Simulate the real upgrade: govtimelock has never run migrations before,
	// so it is absent from the stored version map.
	fromVM := chain.ModuleManager.GetVersionMap()
	delete(fromVM, govtimelocktypes.ModuleName)

	handler := v8_5.CreateUpgradeHandler(
		chain.ModuleManager,
		chain.Configurator(),
		&upgrades.UpgradeKeepers{GovTimelockKeeper: &chain.GovTimelockKeeper},
		nil,
	)

	var toVM module.VersionMap
	require.NotPanics(t, func() {
		var err error
		toVM, err = handler(ctx, upgradetypes.Plan{}, fromVM)
		require.NoError(t, err)
	})
	require.Equal(t, uint64(govtimelock.ConsensusVersion), toVM[govtimelocktypes.ModuleName])

	active, err := chain.GovTimelockKeeper.IsActive(ctx, ctx.BlockHeight())
	require.NoError(t, err)
	require.True(t, active, "upgrade handler must activate the timelock at the upgrade height")

	// The pre-existing passed proposal must not have been scheduled by a stray
	// InitGenesis call — it predates the timelock and already executed under
	// stock semantics.
	due, err := chain.GovTimelockKeeper.Due(ctx, ctx.BlockTime().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, due)
}

// TestGovTimelockGenesisRoundTrip ensures the activation height survives an
// export/import cycle, so a state-exported chain does not silently lose it.
func TestGovTimelockGenesisRoundTrip(t *testing.T) {
	chain := SetupWithEmptyStore(t)
	start := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	ctx := chain.NewUncachedContext(false, tmproto.Header{Time: start})

	module := govtimelock.NewAppModule(chain.GovTimelockKeeper, &chain.GovKeeper)
	require.NoError(t, chain.GovTimelockKeeper.SetActivationHeight(ctx, 500))

	exported := module.ExportGenesis(ctx, chain.AppCodec())
	var state govtimelocktypes.GenesisState
	require.NoError(t, json.Unmarshal(exported, &state))
	require.Equal(t, int64(500), state.ActivationHeight)
	require.NoError(t, state.Validate())
}
