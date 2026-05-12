package device

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"energychain/x/device/keeper"
	"energychain/x/device/types"
)

var (
	_ module.AppModuleBasic   = AppModuleBasic{}
	_ module.AppModule        = AppModule{}
	_ module.HasGenesis       = AppModule{}
	_ appmodule.HasEndBlocker = AppModule{}
)

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string                                      { return types.ModuleName }
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino)   { types.RegisterCodec(cdc) }
func (AppModuleBasic) RegisterInterfaces(reg cdctypes.InterfaceRegistry) { types.RegisterInterfaces(reg) }
func (AppModuleBasic) DefaultGenesis(_ codec.JSONCodec) json.RawMessage {
	bz, err := json.Marshal(types.DefaultGenesis())
	if err != nil {
		panic(err)
	}
	return bz
}
func (AppModuleBasic) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var gs types.GenesisState
	if err := json.Unmarshal(bz, &gs); err != nil {
		return fmt.Errorf("unmarshal device genesis: %w", err)
	}
	return gs.Validate()
}
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

func NewAppModule(_ codec.Codec, k keeper.Keeper) AppModule { return AppModule{keeper: k} }

func (am AppModule) Name() string { return types.ModuleName }

func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), keeper.NewQueryServerImpl(am.keeper))
}

func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) {
	var gs types.GenesisState
	if err := json.Unmarshal(data, &gs); err != nil {
		panic(fmt.Sprintf("unmarshal device genesis: %v", err))
	}
	if err := am.keeper.InitGenesis(ctx, gs); err != nil {
		panic(fmt.Sprintf("init device genesis: %v", err))
	}
}

func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	bz, err := json.Marshal(am.keeper.ExportGenesis(ctx))
	if err != nil {
		panic(err)
	}
	return bz
}

// EndBlock auto-flips ATTESTED → SUSPECT for devices whose attestation
// expired during the block. Bounded per-block work.
func (am AppModule) EndBlock(goCtx context.Context) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, err := am.keeper.ExpireAttestations(ctx); err != nil {
		ctx.Logger().Error("device: expire attestations", "err", err)
	}
	return nil
}

func (am AppModule) ConsensusVersion() uint64 { return 1 }
func (am AppModule) IsOnePerModuleType()      {}
func (am AppModule) IsAppModule()             {}
