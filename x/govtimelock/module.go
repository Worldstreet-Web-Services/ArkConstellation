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
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
)

const (
	ConsensusVersion = 1
	MinimumDelay     = 48 * time.Hour
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
	keeper keeper.Keeper
}

func NewAppModule(k keeper.Keeper) AppModule {
	return AppModule{keeper: k}
}

func (AppModule) IsOnePerModuleType()      {}
func (AppModule) IsAppModule()             {}
func (AppModule) ConsensusVersion() uint64 { return ConsensusVersion }

func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, bz json.RawMessage) {
	var state types.GenesisState
	if err := json.Unmarshal(bz, &state); err != nil {
		panic(err)
	}
	for _, scheduled := range state.ScheduledProposals {
		if err := am.keeper.Schedule(ctx, scheduled.ProposalID, scheduled.ExecutionTime); err != nil {
			panic(err)
		}
	}
}

func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	scheduled, err := am.keeper.Export(ctx)
	if err != nil {
		panic(err)
	}
	bz, err := json.Marshal(types.GenesisState{ScheduledProposals: scheduled})
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
	return EndBlocker(sdk.UnwrapSDKContext(ctx), am.govKeeper, am.timelockKeeper, am.delay)
}
