// Package testutil provides shared in-memory fixtures for module
// keeper_test packages. Historically every module re-implemented an
// identical memDB + codec + context bootstrap inside its own
// keeper_test.go; this package centralises that boilerplate so the
// rewritten RWA-core modules share one audited setup path.
package testutil

import (
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultBlockTime is a fixed, deterministic wall-clock used by every
// fixture so time-dependent logic (lockups, T+N redemption windows,
// scheduler intervals) is reproducible across test runs.
const DefaultBlockTime int64 = 1_700_000_000

// DefaultBlockHeight is the starting block height for fixtures.
const DefaultBlockHeight int64 = 10

// TestStore bundles a freshly mounted in-memory multistore together
// with a proto codec and a base SDK context. Each requested store key
// is mounted as an IAVL store backed by the same MemDB.
type TestStore struct {
	Ctx      sdk.Context
	Cdc      codec.Codec
	Registry codectypes.InterfaceRegistry
	Keys     map[string]*storetypes.KVStoreKey
}

// NewTestStore mounts one IAVL store per name and returns a ready
// context. It fails the test (rather than panicking) on any setup
// error so callers get a clean stack at the offending line.
func NewTestStore(t *testing.T, storeKeys ...string) *TestStore {
	t.Helper()
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())

	keys := make(map[string]*storetypes.KVStoreKey, len(storeKeys))
	for _, name := range storeKeys {
		if _, dup := keys[name]; dup {
			t.Fatalf("testutil: duplicate store key %q", name)
		}
		k := storetypes.NewKVStoreKey(name)
		keys[name] = k
		cms.MountStoreWithDB(k, storetypes.StoreTypeIAVL, db)
	}
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatalf("testutil: load multistore: %v", err)
	}

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(DefaultBlockHeight).
		WithBlockTime(time.Unix(DefaultBlockTime, 0))

	return &TestStore{Ctx: ctx, Cdc: cdc, Registry: registry, Keys: keys}
}

// Key returns the mounted store key for name or fails the test.
func (ts *TestStore) Key(t *testing.T, name string) *storetypes.KVStoreKey {
	t.Helper()
	k, ok := ts.Keys[name]
	if !ok {
		t.Fatalf("testutil: store key %q was not mounted", name)
	}
	return k
}

// Advance returns a copy of the context moved forward by d seconds and
// one block height, for testing time-gated flows.
func (ts *TestStore) Advance(seconds int64) sdk.Context {
	ts.Ctx = ts.Ctx.
		WithBlockHeight(ts.Ctx.BlockHeight() + 1).
		WithBlockTime(ts.Ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
	return ts.Ctx
}
