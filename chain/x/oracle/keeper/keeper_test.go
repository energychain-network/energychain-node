package keeper_test

import (
	"testing"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/keeper"
	"energychain/x/oracle/types"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), "authority")
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())

	return k, ctx
}

func TestSubmitAndGetData(t *testing.T) {
	k, ctx := setupKeeper(t)

	data := types.OracleData{
		Category:    "spot_price",
		Value:       "35000",
		Metadata:    `{"unit":"CNY/MWh","region":"east"}`,
		Timestamp:   1000,
		Submitter:   "oracle1",
		BlockHeight: 1,
	}

	k.SetOracleData(ctx, data)

	latest, found := k.GetLatestData(ctx, "spot_price")
	if !found {
		t.Fatal("expected to find latest data")
	}
	if latest.Value != "35000" {
		t.Errorf("value: want 35000, got %s", latest.Value)
	}
	if latest.Category != "spot_price" {
		t.Errorf("category: want spot_price, got %s", latest.Category)
	}
}

func TestGetDataHistory(t *testing.T) {
	k, ctx := setupKeeper(t)

	for i := int64(1); i <= 3; i++ {
		k.SetOracleData(ctx, types.OracleData{
			Category:  "spot_price",
			Value:     "30000",
			Timestamp: i * 100,
			Submitter: "oracle1",
		})
	}

	history := k.GetDataHistory(ctx, "spot_price", 0, 400)
	if len(history) != 3 {
		t.Errorf("expected 3 data entries, got %d", len(history))
	}

	partial := k.GetDataHistory(ctx, "spot_price", 200, 400)
	if len(partial) != 2 {
		t.Errorf("expected 2 data entries in range [200,400), got %d", len(partial))
	}
}

func TestOracleManagement(t *testing.T) {
	k, ctx := setupKeeper(t)

	oracle := types.OracleInfo{
		Address:              "oracle1addr",
		Name:                 "TestOracle",
		Active:               true,
		AuthorizedCategories: []string{"spot_price", "carbon_price"},
	}

	k.AddOracle(ctx, oracle)

	got, found := k.GetOracle(ctx, "oracle1addr")
	if !found {
		t.Fatal("expected to find oracle")
	}
	if got.Name != "TestOracle" {
		t.Errorf("oracle name: want TestOracle, got %s", got.Name)
	}
	if !got.IsAuthorizedFor("spot_price") {
		t.Error("oracle should be authorized for spot_price")
	}
	if got.IsAuthorizedFor("weather") {
		t.Error("oracle should not be authorized for weather")
	}

	k.RemoveOracle(ctx, "oracle1addr")

	_, found = k.GetOracle(ctx, "oracle1addr")
	if found {
		t.Error("oracle should have been removed")
	}
}

func TestOracleAuthorizedForAll(t *testing.T) {
	k, ctx := setupKeeper(t)

	oracle := types.OracleInfo{
		Address:              "oracle_all",
		Name:                 "AllOracle",
		Active:               true,
		AuthorizedCategories: []string{},
	}
	k.AddOracle(ctx, oracle)

	if !k.IsAuthorizedOracle(ctx, "oracle_all", "any_category") {
		t.Error("oracle with empty AuthorizedCategories should be authorized for any category")
	}
}

func TestIsAuthorizedOracle(t *testing.T) {
	k, ctx := setupKeeper(t)

	if k.IsAuthorizedOracle(ctx, "nobody", "spot_price") {
		t.Error("unregistered address should not be authorized")
	}

	k.AddOracle(ctx, types.OracleInfo{
		Address:              "oracle1",
		Name:                 "O1",
		Active:               true,
		AuthorizedCategories: []string{"spot_price"},
	})

	if !k.IsAuthorizedOracle(ctx, "oracle1", "spot_price") {
		t.Error("registered oracle should be authorized for spot_price")
	}
	if k.IsAuthorizedOracle(ctx, "oracle1", "carbon_price") {
		t.Error("oracle should not be authorized for carbon_price")
	}
}

func TestGenesisImportExport(t *testing.T) {
	k, ctx := setupKeeper(t)

	gs := types.GenesisState{
		Params: types.DefaultParams(),
		Data: []types.OracleData{
			{Category: "spot_price", Value: "45000", Timestamp: 500, Submitter: "o1"},
		},
		Oracles: []types.OracleInfo{
			{Address: "o1", Name: "Oracle1", Active: true, AuthorizedCategories: []string{"spot_price"}},
		},
	}

	k.InitGenesis(ctx, gs)

	exported := k.ExportGenesis(ctx)
	if len(exported.Oracles) != 1 {
		t.Errorf("exported oracles: want 1, got %d", len(exported.Oracles))
	}
	if len(exported.Data) == 0 {
		t.Error("expected at least 1 exported data entry")
	}
}
