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

	"energychain/x/audit/keeper"
	"energychain/x/audit/types"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	tStoreKey := storetypes.NewTransientStoreKey(types.TStoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	stateStore.MountStoreWithDB(tStoreKey, storetypes.StoreTypeTransient, db)
	if err := stateStore.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), tStoreKey, "authority", nil)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger())

	return k, ctx
}

func TestRecordAndGetAuditLog(t *testing.T) {
	k, ctx := setupKeeper(t)

	auditLog := types.AuditLog{
		ID:          1,
		EventType:   "contract_deploy",
		Actor:       "energy1alice",
		Target:      "0xContractAddr",
		Action:      "deploy",
		Data:        `{"contract":"EnergyDataAttestation"}`,
		BlockHeight: 100,
		Timestamp:   1000,
		TxHash:      "0xTxHash",
	}

	k.RecordAuditLog(ctx, auditLog)

	got, found := k.GetAuditLog(ctx, 1)
	if !found {
		t.Fatal("expected to find audit log")
	}
	if got.Actor != "energy1alice" {
		t.Errorf("actor: want energy1alice, got %s", got.Actor)
	}
	if got.Action != "deploy" {
		t.Errorf("action: want deploy, got %s", got.Action)
	}
	if got.EventType != "contract_deploy" {
		t.Errorf("event_type: want contract_deploy, got %s", got.EventType)
	}
}

func TestGetAuditLogNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)

	_, found := k.GetAuditLog(ctx, 999)
	if found {
		t.Fatal("expected not found for nonexistent ID")
	}
}

func TestGetAuditLogsByActor(t *testing.T) {
	k, ctx := setupKeeper(t)

	k.RecordAuditLog(ctx, types.AuditLog{ID: 1, Actor: "alice", EventType: "contract_deploy", Action: "deploy", Timestamp: 100})
	k.RecordAuditLog(ctx, types.AuditLog{ID: 2, Actor: "bob", EventType: "large_transfer", Action: "transfer", Timestamp: 200})
	k.RecordAuditLog(ctx, types.AuditLog{ID: 3, Actor: "alice", EventType: "custom", Action: "custom", Timestamp: 300})

	aliceLogs := k.GetAuditLogsByActor(ctx, "alice")
	if len(aliceLogs) != 2 {
		t.Errorf("expected 2 logs for alice, got %d", len(aliceLogs))
	}

	bobLogs := k.GetAuditLogsByActor(ctx, "bob")
	if len(bobLogs) != 1 {
		t.Errorf("expected 1 log for bob, got %d", len(bobLogs))
	}
}

func TestGetAuditLogsByType(t *testing.T) {
	k, ctx := setupKeeper(t)

	k.RecordAuditLog(ctx, types.AuditLog{ID: 1, Actor: "a", EventType: "contract_deploy", Action: "deploy", Timestamp: 100})
	k.RecordAuditLog(ctx, types.AuditLog{ID: 2, Actor: "a", EventType: "large_transfer", Action: "transfer", Timestamp: 200})
	k.RecordAuditLog(ctx, types.AuditLog{ID: 3, Actor: "b", EventType: "contract_deploy", Action: "deploy2", Timestamp: 300})

	deploys := k.GetAuditLogsByType(ctx, "contract_deploy")
	if len(deploys) != 2 {
		t.Errorf("expected 2 deploy logs, got %d", len(deploys))
	}

	custom := k.GetAuditLogsByType(ctx, "my_custom_event")
	if len(custom) != 0 {
		t.Errorf("expected 0 logs for unknown type, got %d", len(custom))
	}
}

func TestIDCounter(t *testing.T) {
	k, ctx := setupKeeper(t)

	id1 := k.GetNextID(ctx)
	k.IncrementCounter(ctx, id1)

	id2 := k.GetNextID(ctx)
	k.IncrementCounter(ctx, id2)

	if id1 == id2 {
		t.Fatal("IDs should be unique")
	}
	if id1 != 1 {
		t.Errorf("first ID: want 1, got %d", id1)
	}
	if id2 != 2 {
		t.Errorf("second ID: want 2, got %d", id2)
	}
}

func TestGenesisImportExport(t *testing.T) {
	k, ctx := setupKeeper(t)

	gs := types.GenesisState{
		Params: types.DefaultParams(),
		Logs: []types.AuditLog{
			{ID: 1, Actor: "alice", EventType: "custom", Action: "test", Timestamp: 100},
		},
	}

	k.InitGenesis(ctx, gs)

	exported := k.ExportGenesis(ctx)
	if len(exported.Logs) != 1 {
		t.Errorf("exported logs: want 1, got %d", len(exported.Logs))
	}
}
