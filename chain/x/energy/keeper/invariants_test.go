package keeper_test

import (
	"strings"
	"testing"

	"cosmossdk.io/collections"

	"energychain/x/energy/keeper"
	"energychain/x/energy/types"
)

// TestIDSequenceMonotonicInvariant_OK asserts the invariant returns
// (msg, broken=false) when the sequence has been advanced past every stored
// record's numeric suffix (the SDK contract: broken=false means PASS).
func TestIDSequenceMonotonicInvariant_OK(t *testing.T) {
	k, ctx := setupKeeper(t)

	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-2", Category: "meter", Submitter: "a", DataHash: "h2",
	}); err != nil {
		t.Fatal(err)
	}
	k.SetCurrentID(ctx, 2)

	msg, broken := keeper.IDSequenceMonotonicInvariant(k)(ctx)
	if broken {
		t.Fatalf("expected invariant to hold, got broken=%q", msg)
	}
}

// TestIDSequenceMonotonicInvariant_BrokenOnRegression simulates a buggy
// migration that rewinds the sequence below an existing record. The
// invariant must catch this.
func TestIDSequenceMonotonicInvariant_BrokenOnRegression(t *testing.T) {
	k, ctx := setupKeeper(t)

	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-42", Category: "meter", Submitter: "a", DataHash: "h",
	}); err != nil {
		t.Fatal(err)
	}
	k.SetCurrentID(ctx, 5) // < 42

	msg, broken := keeper.IDSequenceMonotonicInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected invariant to be broken, got %q", msg)
	}
	if !strings.Contains(msg, "highest observed id 42") {
		t.Errorf("invariant message should mention id 42, got %q", msg)
	}
}

// TestByCategoryConsistencyInvariant_BrokenOnDanglingIndex injects a stale
// index entry pointing at a non-existent record id. The invariant must
// flag it.
func TestByCategoryConsistencyInvariant_BrokenOnDanglingIndex(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Create one good record.
	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h",
	}); err != nil {
		t.Fatal(err)
	}
	// Inject a dangling index entry for an id that doesn't exist.
	if err := k.ByCategory.Set(ctx, collections.Join("ghost", "energy-9999")); err != nil {
		t.Fatal(err)
	}

	msg, broken := keeper.ByCategoryConsistencyInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected invariant to be broken, got %q", msg)
	}
	if !strings.Contains(msg, "missing record") {
		t.Errorf("invariant message should mention dangling index, got %q", msg)
	}
}

// TestByCategoryConsistencyInvariant_BrokenOnMismatch flips the category of
// a stored record so the (category,id) index no longer matches the record's
// own Category field. This catches the "forgot to delete the old index
// entry on update" bug.
func TestByCategoryConsistencyInvariant_BrokenOnMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)

	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h",
	}); err != nil {
		t.Fatal(err)
	}
	// Add an index entry under a wrong category.
	if err := k.ByCategory.Set(ctx, collections.Join("battery", "energy-1")); err != nil {
		t.Fatal(err)
	}

	msg, broken := keeper.ByCategoryConsistencyInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected invariant to be broken, got %q", msg)
	}
	if !strings.Contains(msg, "mismatch") {
		t.Errorf("invariant message should mention mismatch, got %q", msg)
	}
}

// TestBySubmitterConsistencyInvariant_BrokenOnDanglingIndex mirrors the
// ByCategory test for the (submitter, id) index.
func TestBySubmitterConsistencyInvariant_BrokenOnDanglingIndex(t *testing.T) {
	k, ctx := setupKeeper(t)

	if err := k.BySubmitter.Set(ctx, collections.Join("ghost-submitter", "energy-9999")); err != nil {
		t.Fatal(err)
	}

	msg, broken := keeper.BySubmitterConsistencyInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected invariant to be broken, got %q", msg)
	}
}

// TestMetadataWithinCapInvariant_OK passes when every record's metadata
// fits the configured cap.
func TestMetadataWithinCapInvariant_OK(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    256,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h",
		Metadata: strings.Repeat("x", 64),
	}); err != nil {
		t.Fatal(err)
	}
	if msg, broken := keeper.MetadataWithinCapInvariant(k)(ctx); broken {
		t.Fatalf("invariant should hold, got %q", msg)
	}
}

// TestMetadataWithinCapInvariant_BrokenAfterCapShrink seeds an oversized
// record then lowers the cap (something only a buggy migration could do).
// Invariant must surface the violation.
func TestMetadataWithinCapInvariant_BrokenAfterCapShrink(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Wide cap to admit the record initially.
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "a", DataHash: "h",
		Metadata: strings.Repeat("x", 512),
	}); err != nil {
		t.Fatal(err)
	}
	// Shrink cap below stored size — bypass Validate by writing direct.
	if err := k.Params.Set(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    256,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	msg, broken := keeper.MetadataWithinCapInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected invariant to break after cap shrink, got %q", msg)
	}
	if !strings.Contains(msg, "energy-1") {
		t.Errorf("expected violation message to mention the offending id, got %q", msg)
	}
}

// TestAllInvariants_StopsAtFirstFailure verifies the AllInvariants
// composite returns broken=true as soon as any sub-invariant fails,
// without depending on which one fires first.
func TestAllInvariants_StopsAtFirstFailure(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Inject a clearly-broken state: a dangling category index entry.
	if err := k.ByCategory.Set(ctx, collections.Join("ghost", "energy-404")); err != nil {
		t.Fatal(err)
	}

	msg, broken := keeper.AllInvariants(k)(ctx)
	if !broken {
		t.Fatalf("expected AllInvariants to break, got %q", msg)
	}
}

// TestAllInvariants_PassOnHealthyState confirms the composite returns
// broken=false on a freshly-seeded, consistent keeper.
func TestAllInvariants_PassOnHealthyState(t *testing.T) {
	k, ctx := setupKeeper(t)

	for i, hash := range []string{"h1", "h2", "h3"} {
		id := k.GenerateID(ctx)
		_ = i
		if err := k.SubmitEnergyData(ctx, types.EnergyData{
			ID:        id,
			Category:  "meter",
			Submitter: "alice",
			DataHash:  hash,
			Metadata:  "{}",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if msg, broken := keeper.AllInvariants(k)(ctx); broken {
		t.Fatalf("expected invariants to hold, got %q", msg)
	}
}
