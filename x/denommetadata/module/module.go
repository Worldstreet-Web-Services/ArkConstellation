package module

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	"github.com/MANTRA-Chain/mantrachain/v8/x/denommetadata/keeper"
	"github.com/MANTRA-Chain/mantrachain/v8/x/denommetadata/types"
)

var (
	_ module.AppModuleBasic = AppModuleBasic{}
	_ appmodule.AppModule   = AppModule{}
)

// AppModuleBasic implements the AppModuleBasic interface.
type AppModuleBasic struct{ cdc codec.BinaryCodec }

func NewAppModuleBasic(cdc codec.BinaryCodec) AppModuleBasic {
	return AppModuleBasic{cdc: cdc}
}

func (AppModuleBasic) Name() string { return types.ModuleName }

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

func (AppModuleBasic) RegisterInterfaces(reg cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(reg)
}

// DefaultGenesis returns nil: this module owns no state. x/bank remains the
// single source of truth for denom metadata.
func (AppModuleBasic) DefaultGenesis(codec.JSONCodec) json.RawMessage { return nil }

func (AppModuleBasic) ValidateGenesis(codec.JSONCodec, client.TxEncodingConfig, json.RawMessage) error {
	return nil
}

func (AppModuleBasic) RegisterGRPCGatewayRoutes(client.Context, *runtime.ServeMux) {}

// GetTxCmd returns nil: this message is governance-only, so it is submitted
// through `tx gov submit-proposal`, not a bespoke CLI command.
func (AppModuleBasic) GetTxCmd() *cobra.Command { return nil }

func (AppModuleBasic) GetQueryCmd() *cobra.Command { return nil }

// AppModule implements the AppModule interface.
type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

func NewAppModule(cdc codec.Codec, k keeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: NewAppModuleBasic(cdc),
		keeper:         k,
	}
}

func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
}

func (AppModule) InitGenesis(context.Context, codec.JSONCodec, json.RawMessage) {}

func (AppModule) ExportGenesis(context.Context, codec.JSONCodec) json.RawMessage { return nil }

// ConsensusVersion implements AppModule/ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return 1 }

func (am AppModule) IsOnePerModuleType() {}
func (am AppModule) IsAppModule()        {}

var _ = fmt.Sprintf
