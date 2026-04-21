package keeper_test

import (
	"strings"
	"testing"

	"energychain/x/oracle/keeper"
	"energychain/x/oracle/types"
)

// TestLatestDataConsistencyInvariant_OK seeds a clean dataset and asserts
// the cache cross-check passes.
func TestLatestDataConsistencyInvariant_OK(t *testing.T) {
	k, ctx := setupKeeper(t)
	k.SetOracleData(ctx, types.OracleData{
		Category: "spot_price", Value: "100", Timestamp: 1000, Submitter: "o1",
	})
	if msg, broken := keeper.LatestDataConsistencyInvariant(k)(ctx); broken {
		t.Fatalf("invariant should hold, got %q", msg)
	}
}

// TestLatestDataConsistencyInvariant_BrokenOnDivergentCache writes a
// LatestData entry that does not agree with the time-series Data row.
// The invariant must flag it.
func TestLatestDataConsistencyInvariant_BrokenOnDivergentCache(t *testing.T) {
	k, ctx := setupKeeper(t)
	k.SetOracleData(ctx, types.OracleData{
		Category: "spot_price", Value: "100", Timestamp: 1000, Submitter: "o1",
	})
	// Corrupt the cache: set LatestData to a different value.
	if err := k.LatestData.Set(ctx, "spot_price", types.OracleData{
		Category: "spot_price", Value: "999", Timestamp: 1000, Submitter: "o1",
	}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.LatestDataConsistencyInvariant(k)(ctx)
	if !broken || !strings.Contains(msg, "divergent payload") {
		t.Fatalf("expected divergent-payload break, got %q (broken=%v)", msg, broken)
	}
}

func TestOracleAddressNonEmptyInvariant_BrokenOnMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)
	// Inject a broken oracle entry where map key != value.Address.
	if err := k.Oracles.Set(ctx, "k1", types.OracleInfo{Address: "k2", Name: "x", Active: true}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.OracleAddressNonEmptyInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected break on key/value mismatch, got %q", msg)
	}
}

func TestMetadataWithinCapInvariant_OracleBrokenAfterShrink(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.Params{MaxMetadataSize: 1024}); err != nil {
		t.Fatal(err)
	}
	k.SetOracleData(ctx, types.OracleData{
		Category: "spot_price", Value: "1", Timestamp: 1, Submitter: "o1",
		Metadata: strings.Repeat("X", 512),
	})
	// Shrink cap below stored data via direct Item.Set (bypassing Validate).
	if err := k.Params.Set(ctx, types.Params{MaxMetadataSize: 256}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.MetadataWithinCapInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected metadata-cap break, got %q", msg)
	}
}

func TestAllInvariants_Oracle_Healthy(t *testing.T) {
	k, ctx := setupKeeper(t)
	k.AddOracle(ctx, types.OracleInfo{Address: "o1", Name: "Test", Active: true})
	k.SetOracleData(ctx, types.OracleData{Category: "x", Value: "1", Timestamp: 1, Submitter: "o1"})
	if msg, broken := keeper.AllInvariants(k)(ctx); broken {
		t.Fatalf("expected invariants to hold, got %q", msg)
	}
}
