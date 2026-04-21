package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"energychain/x/energy/keeper"
	"energychain/x/energy/types"
)

// seed inserts a fixed corpus into the keeper for query tests:
//   - 4 meter records (alice 3, bob 1)
//   - 2 ev_charging records (alice 1, bob 1)
func seedQueryFixture(t *testing.T, k keeper.Keeper, ctx sdk.Context) []string {
	t.Helper()
	wave := []struct {
		Cat, Sub, Hash string
	}{
		{"meter", "alice", "h1"},
		{"meter", "alice", "h2"},
		{"ev_charging", "alice", "h3"},
		{"meter", "alice", "h4"},
		{"meter", "bob", "h5"},
		{"ev_charging", "bob", "h6"},
	}
	ids := make([]string, 0, len(wave))
	for _, r := range wave {
		id := k.GenerateID(ctx)
		if err := k.SubmitEnergyData(ctx, types.EnergyData{
			ID: id, Category: r.Cat, Submitter: r.Sub, DataHash: r.Hash,
		}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestQueryServer_EnergyData_FoundAndNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	if err := k.SubmitEnergyData(ctx, types.EnergyData{
		ID: "energy-1", Category: "meter", Submitter: "alice", DataHash: "h",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := q.EnergyData(asCtx(ctx), &types.QueryEnergyDataRequest{ID: "energy-1"})
	if err != nil {
		t.Fatalf("found query: %v", err)
	}
	if resp.Data.DataHash != "h" {
		t.Errorf("wrong record returned: %+v", resp.Data)
	}

	if _, err := q.EnergyData(asCtx(ctx), &types.QueryEnergyDataRequest{ID: "missing"}); err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestQueryServer_Params_ReturnsCurrentValue(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	target := types.Params{
		MaxBatchSize:                       77,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 9,
		Permissionless:                     true,
	}
	if err := k.SetParams(ctx, target); err != nil {
		t.Fatal(err)
	}

	resp, err := q.Params(asCtx(ctx), &types.QueryParamsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Params.MaxBatchSize != 77 {
		t.Errorf("max_batch_size: want 77, got %d", resp.Params.MaxBatchSize)
	}
}

func TestQueryServer_Batch_FoundAndNotFound(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	if err := k.SubmitBatch(ctx, types.BatchSubmission{
		ID: "batch-1", Submitter: "alice", Category: "meter",
		DataCount: 3, MerkleRoot: "0xroot", Timestamp: 1,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := q.Batch(asCtx(ctx), &types.QueryBatchRequest{ID: "batch-1"})
	if err != nil || resp.Batch.MerkleRoot != "0xroot" {
		t.Fatalf("batch query: %v / %+v", err, resp)
	}

	if _, err := q.Batch(asCtx(ctx), &types.QueryBatchRequest{ID: "no"}); err == nil {
		t.Fatal("expected not-found")
	}
}

// TestQueryServer_EnergyDataByCategory_PaginationIsolatesPrefix proves that
// paginating by category never leaks records from other categories, and
// that the page response next-key chain reaches every member exactly once.
func TestQueryServer_EnergyDataByCategory_PaginationIsolatesPrefix(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	_ = seedQueryFixture(t, k, ctx)

	var collected []string
	var nextKey []byte
	for i := 0; i < 5; i++ {
		resp, err := q.EnergyDataByCategory(asCtx(ctx), &types.QueryEnergyDataByCategoryRequest{
			Category: "meter",
			Pagination: &query.PageRequest{
				Limit: 2,
				Key:   nextKey,
			},
		})
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		for _, d := range resp.Data {
			if d.Category != "meter" {
				t.Errorf("leaked record from category %q", d.Category)
			}
			collected = append(collected, d.ID)
		}
		if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
			break
		}
		nextKey = resp.Pagination.NextKey
	}
	if len(collected) != 4 {
		t.Fatalf("expected 4 meter records via pagination, got %d (%v)", len(collected), collected)
	}
	if dup := firstDup(collected); dup != "" {
		t.Fatalf("duplicate id seen across pages: %s", dup)
	}
}

// TestQueryServer_EnergyDataByCategory_EmptyCategory returns a clean empty
// response (no error) when the category has no records.
func TestQueryServer_EnergyDataByCategory_EmptyCategory(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	resp, err := q.EnergyDataByCategory(asCtx(ctx), &types.QueryEnergyDataByCategoryRequest{
		Category: "nope",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected empty result, got %d records", len(resp.Data))
	}
}

func TestQueryServer_EnergyDataBySubmitter_Pagination(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	_ = seedQueryFixture(t, k, ctx)

	resp, err := q.EnergyDataBySubmitter(asCtx(ctx), &types.QueryEnergyDataBySubmitterRequest{
		Submitter:  "alice",
		Pagination: &query.PageRequest{Limit: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 4 {
		t.Fatalf("alice: want 4, got %d", len(resp.Data))
	}
	for _, d := range resp.Data {
		if d.Submitter != "alice" {
			t.Errorf("leaked record from submitter %q", d.Submitter)
		}
	}
}

// TestQueryServer_EnergyDataByCategory_LargeBatchHonoursLimit ensures the
// pagination layer enforces the requested page size — important for not
// blowing the gRPC response budget.
func TestQueryServer_EnergyDataByCategory_LargeBatchHonoursLimit(t *testing.T) {
	k, ctx := setupKeeper(t)
	q := keeper.NewQueryServerImpl(k)

	for i := 0; i < 50; i++ {
		id := k.GenerateID(ctx)
		if err := k.SubmitEnergyData(ctx, types.EnergyData{
			ID: id, Category: "load", Submitter: "alice", DataHash: "h",
		}); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := q.EnergyDataByCategory(asCtx(ctx), &types.QueryEnergyDataByCategoryRequest{
		Category:   "load",
		Pagination: &query.PageRequest{Limit: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 7 {
		t.Fatalf("limit not honoured: want 7, got %d", len(resp.Data))
	}
	if resp.Pagination == nil || len(resp.Pagination.NextKey) == 0 {
		t.Fatal("next-key missing for partial result")
	}
}

func firstDup(s []string) string {
	seen := make(map[string]struct{}, len(s))
	for _, x := range s {
		if _, ok := seen[x]; ok {
			return x
		}
		seen[x] = struct{}{}
	}
	return ""
}
