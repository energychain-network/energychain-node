package oracle

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

	"energychain/x/oracle/keeper"
	"energychain/x/oracle/types"
)

var (
	_ module.AppModuleBasic   = AppModuleBasic{}
	_ module.AppModule        = AppModule{}
	_ module.HasGenesis       = AppModule{}
	_ appmodule.HasEndBlocker = AppModule{}
)

// ---------------------------------------------------------------------------
// AppModuleBasic
// ---------------------------------------------------------------------------

type AppModuleBasic struct{}

func (AppModuleBasic) Name() string { return types.ModuleName }

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterCodec(cdc)
}

func (AppModuleBasic) RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)
}

func (AppModuleBasic) DefaultGenesis(_ codec.JSONCodec) json.RawMessage {
	gs := types.DefaultGenesis()
	bz, err := json.Marshal(gs)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %s default genesis: %v", types.ModuleName, err))
	}
	return bz
}

func (AppModuleBasic) ValidateGenesis(_ codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var gs types.GenesisState
	if err := json.Unmarshal(bz, &gs); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return gs.Validate()
}

func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	if err := types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx)); err != nil {
		panic(err)
	}
}

// ---------------------------------------------------------------------------
// AppModule
// ---------------------------------------------------------------------------

type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

func NewAppModule(_ codec.Codec, keeper keeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         keeper,
	}
}

func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), keeper.NewQueryServerImpl(am.keeper))
}

func (am AppModule) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) {
	var gs types.GenesisState
	if err := json.Unmarshal(data, &gs); err != nil {
		panic(fmt.Sprintf("failed to unmarshal %s genesis: %v", types.ModuleName, err))
	}
	if err := am.keeper.InitGenesis(ctx, gs); err != nil {
		panic(fmt.Sprintf("failed to init %s genesis: %v", types.ModuleName, err))
	}
}

func (am AppModule) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) json.RawMessage {
	gs := am.keeper.ExportGenesis(ctx)
	bz, err := json.Marshal(gs)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal %s genesis: %v", types.ModuleName, err))
	}
	return bz
}

// ConsensusVersion is bumped to 2 with the M1 rewrite. The new schema is
// completely different from the legacy oracle (Topics + Providers +
// Submissions + Aggregated + ReserveAttestations replace the M0
// Categories + OracleData + OracleInfo trio). Migration is intentionally
// destructive: testnets must clear the oracle prefix when upgrading.
func (AppModule) ConsensusVersion() uint64 { return 2 }

func (am AppModule) IsOnePerModuleType() {}
func (am AppModule) IsAppModule()        {}

// EndBlock runs the deterministic per-block work: aggregate fresh
// submissions into AggregatedValue rows, sweep matured bond-release
// queue entries, and flip providers whose suspension has expired back to
// ACTIVE. Errors are logged so a downstream module bug cannot brick the
// chain — the next block retries.
func (am AppModule) EndBlock(goCtx context.Context) error {
	ctx := sdk.UnwrapSDKContext(goCtx)
	if n, err := am.keeper.AggregateAll(ctx); err != nil {
		ctx.Logger().Error("oracle: aggregate", "err", err)
	} else if n > 0 {
		ctx.Logger().Info("oracle: aggregated topics", "count", n)
	}
	if _, err := am.keeper.SweepBondReleases(ctx); err != nil {
		ctx.Logger().Error("oracle: bond sweep", "err", err)
	}
	if _, err := am.keeper.SweepExpiredSuspensions(ctx); err != nil {
		ctx.Logger().Error("oracle: suspension sweep", "err", err)
	}
	return nil
}
