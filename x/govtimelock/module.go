package govtimelock

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cosmossdk.io/core/appmodule"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/keeper"
	"github.com/MANTRA-Chain/mantrachain/v8/x/govtimelock/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	gov "github.com/cosmos/cosmos-sdk/x/gov"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

const (
	ConsensusVersion = 1
	// MinimumDelay re-exports types.MinimumDelay for callers that already
	// import this package.
	MinimumDelay = types.MinimumDelay
)

var (
	_ module.AppModuleBasic      = AppModuleBasic{}
	_ module.HasGenesis          = AppModule{}
	_ module.HasConsensusVersion = AppModule{}
	_ appmodule.AppModule        = AppModule{}
	_ appmodule.HasEndBlocker    = GovAppModule{}
)

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string                                                { return types.ModuleName }
func (AppModuleBasic) RegisterLegacyAminoCodec(*codec.LegacyAmino)                 {}
func (AppModuleBasic) RegisterInterfaces(cdctypes.InterfaceRegistry)               {}
func (AppModuleBasic) RegisterGRPCGatewayRoutes(client.Context, *runtime.ServeMux) {}

func (AppModuleBasic) DefaultGenesis(codec.JSONCodec) json.RawMessage {
	bz, err := json.Marshal(types.DefaultGenesis())
	if err != nil {
		panic(err)
	}
	return bz
}

func (AppModuleBasic) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var state types.GenesisState
	if err := json.Unmarshal(bz, &state); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return state.Validate()
}

type AppModule struct {
	AppModuleBasic
	keeper    keeper.Keeper
	govKeeper *govkeeper.Keeper
}

func NewAppModule(k keeper.Keeper, govKeeper *govkeeper.Keeper) AppModule {
	return AppModule{keeper: k, govKeeper: govKeeper}
}

func (AppModule) IsOnePerModuleType()      {}
func (AppModule) IsAppModule()             {}
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, bz json.RawMessage) {
	var state types.GenesisState
	if err := json.Unmarshal(bz, &state); err != nil {
		panic(err)
	}
	// AppModuleBasic.ValidateGenesis wires Validate into the separate,
	// optional `genesis validate-genesis` CLI flow; it is not otherwise called
	// on the path a node actually takes at startup. Call it here too so a
	// self-contained invariant (e.g. the 48h minimum execution delay) can't
	// reach a running chain just because it skipped that CLI step.
	if err := state.Validate(); err != nil {
		panic(err)
	}
	// GenesisState.Validate cannot see x/gov's state, so the cross-module
	// invariants are checked here where both stores are available. Failing at
	// InitGenesis is the right place for this: a mismatch that slipped through
	// would otherwise surface as a stranded proposal or a dropped schedule at
	// EndBlock, long after the operator could act on it.
	if am.govKeeper != nil {
		if err := am.validateAgainstGov(ctx, state); err != nil {
			panic(err)
		}
	}
	for _, scheduled := range state.ScheduledProposals {
		if err := am.keeper.Schedule(ctx, scheduled.ProposalID, scheduled.ExecutionTime); err != nil {
			panic(err)
		}
	}
	// Height 0 means "not activated at genesis" — an existing chain that will
	// activate via the coordinated upgrade handler instead.
	if state.ActivationHeight > 0 {
		if err := am.keeper.SetActivationHeight(ctx, state.ActivationHeight); err != nil {
			panic(err)
		}
	}
	if state.ExecutionDelay > 0 {
		if err := am.keeper.SetExecutionDelay(ctx, state.ExecutionDelay); err != nil {
			panic(err)
		}
	}
}

// validateAgainstGov enforces the two cross-module invariants that
// types.GenesisState.Validate structurally cannot: every scheduled entry must
// name a proposal that exists in x/gov and is StatusPassed, and every passed
// proposal in x/gov must have a corresponding scheduled entry (otherwise its
// messages would never execute).
func (am AppModule) validateAgainstGov(ctx sdk.Context, state types.GenesisState) error {
	scheduledIDs := make(map[uint64]struct{}, len(state.ScheduledProposals))
	for _, scheduled := range state.ScheduledProposals {
		scheduledIDs[scheduled.ProposalID] = struct{}{}

		proposal, err := am.govKeeper.Proposals.Get(ctx, scheduled.ProposalID)
		if err != nil {
			return fmt.Errorf(
				"%s genesis schedules proposal %d, which does not exist in x/gov: %w",
				types.ModuleName, scheduled.ProposalID, err,
			)
		}
		if proposal.Status != govv1.StatusPassed {
			return fmt.Errorf(
				"%s genesis schedules proposal %d with status %s, expected %s",
				types.ModuleName, scheduled.ProposalID, proposal.Status.String(), govv1.StatusPassed.String(),
			)
		}
	}

	err := am.govKeeper.Proposals.Walk(ctx, nil, func(id uint64, proposal govv1.Proposal) (bool, error) {
		if proposal.Status != govv1.StatusPassed {
			return false, nil
		}
		if _, ok := scheduledIDs[id]; !ok {
			return true, fmt.Errorf(
				"x/gov proposal %d is %s but has no %s schedule; its messages would never execute",
				id, proposal.Status.String(), types.ModuleName,
			)
		}
		return false, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	scheduled, err := am.keeper.Export(ctx)
	if err != nil {
		panic(err)
	}
	activationHeight, err := am.keeper.GetActivationHeight(ctx)
	if err != nil {
		panic(err)
	}
	executionDelay, err := am.keeper.GetExecutionDelay(ctx, MinimumDelay)
	if err != nil {
		panic(err)
	}
	bz, err := json.Marshal(types.GenesisState{
		ScheduledProposals: scheduled,
		ActivationHeight:   activationHeight,
		ExecutionDelay:     executionDelay,
	})
	if err != nil {
		panic(err)
	}
	return bz
}

// GovAppModule preserves all stock x/gov services, migrations, genesis, and
// simulation behavior while replacing only its EndBlock execution path.
type GovAppModule struct {
	gov.AppModule
	govKeeper      *govkeeper.Keeper
	timelockKeeper keeper.Keeper
	delay          time.Duration
}

func NewGovAppModule(base gov.AppModule, govKeeper *govkeeper.Keeper, timelockKeeper keeper.Keeper) GovAppModule {
	return GovAppModule{
		AppModule:      base,
		govKeeper:      govKeeper,
		timelockKeeper: timelockKeeper,
		delay:          MinimumDelay,
	}
}

func (am GovAppModule) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	return EndBlocker(sdkCtx, am.govKeeper, am.timelockKeeper, am.delay)
}
