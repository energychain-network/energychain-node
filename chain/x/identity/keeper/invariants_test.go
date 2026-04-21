package keeper_test

import (
	"strings"
	"testing"

	"cosmossdk.io/collections"

	"energychain/x/identity/keeper"
	"energychain/x/identity/types"
)

func TestByRoleConsistencyInvariant_OK(t *testing.T) {
	k, ctx := setupKeeper(t)
	setIdentity(t, k, ctx, types.Identity{
		Address: "energy1alice", Name: "Alice", Role: "vpp", Status: types.StatusActive,
	})
	if msg, broken := keeper.ByRoleConsistencyInvariant(k)(ctx); broken {
		t.Fatalf("expected invariant to hold, got %q", msg)
	}
}

func TestByRoleConsistencyInvariant_BrokenOnDanglingIndex(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.ByRole.Set(ctx, collections.Join("ghost", "energy1nobody")); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.ByRoleConsistencyInvariant(k)(ctx)
	if !broken || !strings.Contains(msg, "missing identity") {
		t.Fatalf("expected dangling-index break, got %q (broken=%v)", msg, broken)
	}
}

func TestByRoleConsistencyInvariant_BrokenOnRoleMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)
	setIdentity(t, k, ctx, types.Identity{
		Address: "energy1alice", Name: "Alice", Role: "vpp", Status: types.StatusActive,
	})
	// Index entry under wrong role.
	if err := k.ByRole.Set(ctx, collections.Join("retail_company", "energy1alice")); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.ByRoleConsistencyInvariant(k)(ctx)
	if !broken || !strings.Contains(msg, "role mismatch") {
		t.Fatalf("expected role-mismatch break, got %q (broken=%v)", msg, broken)
	}
}

func TestAddressKeyMatchesValueInvariant_BrokenOnTypo(t *testing.T) {
	k, ctx := setupKeeper(t)
	// Use raw collection write to bypass any normalization in SetIdentity.
	if err := k.Identities.Set(ctx, "energy1alice", types.Identity{
		Address: "energy1typo", Name: "Alice", Role: "vpp", Status: types.StatusActive,
	}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.AddressKeyMatchesValueInvariant(k)(ctx)
	if !broken || !strings.Contains(msg, "energy1typo") {
		t.Fatalf("expected key-mismatch break, got %q (broken=%v)", msg, broken)
	}
}

func TestMetadataWithinCapInvariant_IdentityBrokenAfterShrink(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.Params{MaxMetadataSize: 1024}); err != nil {
		t.Fatal(err)
	}
	setIdentity(t, k, ctx, types.Identity{
		Address: "energy1alice", Name: "Alice", Role: "vpp", Status: types.StatusActive,
		Metadata: strings.Repeat("X", 512),
	})
	if err := k.Params.Set(ctx, types.Params{MaxMetadataSize: 256}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.MetadataWithinCapInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected cap break, got %q", msg)
	}
}

func TestAllInvariants_Identity_Healthy(t *testing.T) {
	k, ctx := setupKeeper(t)
	setIdentity(t, k, ctx, types.Identity{
		Address: "energy1alice", Name: "Alice", Role: "vpp", Status: types.StatusActive,
	})
	if msg, broken := keeper.AllInvariants(k)(ctx); broken {
		t.Fatalf("expected invariants to hold, got %q", msg)
	}
}
