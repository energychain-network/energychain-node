package keeper_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"golang.org/x/crypto/sha3"

	"energychain/x/energy/keeper"
	"energychain/x/energy/types"
)

// validBech32A and validBech32B are valid bech32 addresses with the
// "energy" prefix. Generated once and pinned so tests don't need a live
// keyring. Anything that calls ValidateBasic on a Msg will accept these.
const (
	validBech32A = "energy1qqqsyqcyq5rqwzqfpg9scrgwpugpzysn3qpz7s"
	validBech32B = "energy1qypqxpq9qcrsszgse4wwrm6lly3qqqq7zalux"
)

func newMsgServer(t *testing.T) (types.MsgServer, keeper.Keeper, sdk.Context) {
	t.Helper()
	k, ctx := setupKeeper(t)
	return keeper.NewMsgServerImpl(k), k, ctx
}

// asCtx unwraps Context for the goCtx parameter expected by msg_server methods.
func asCtx(sdkCtx sdk.Context) context.Context {
	return sdk.WrapSDKContext(sdkCtx)
}

// ---------------------------------------------------------------------------
// SubmitEnergyData — positive cases
// ---------------------------------------------------------------------------

func TestMsgServer_SubmitEnergyData_Permissionless_OK(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	// Default params have Permissionless=true and a non-zero rate limit, so
	// any address can submit at least one record per block.
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	resp, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32A,
		Category:  "meter",
		DataHash:  "0x" + strings.Repeat("a", 64),
		Metadata:  `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if resp.ID == "" || !strings.HasPrefix(resp.ID, "energy-") {
		t.Fatalf("unexpected response id %q", resp.ID)
	}

	got, found := k.GetEnergyData(ctx, resp.ID)
	if !found {
		t.Fatalf("record %s not persisted", resp.ID)
	}
	if got.Submitter != validBech32A || got.Category != "meter" {
		t.Errorf("record fields drifted: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// SubmitEnergyData — negative cases (allow-list, metadata cap, rate limit)
// ---------------------------------------------------------------------------

func TestMsgServer_SubmitEnergyData_NotAllowlisted(t *testing.T) {
	srv, k, ctx := newMsgServer(t)

	// Permissioned mode + empty allow-list = nobody can submit.
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     false,
		AllowedSubmitters:                  []string{validBech32B},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32A,
		Category:  "meter",
		DataHash:  "0xdeadbeef",
	})
	if err == nil {
		t.Fatal("expected allow-list rejection, got nil")
	}
	if !strings.Contains(err.Error(), "not an allowed submitter") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestMsgServer_SubmitEnergyData_MetadataOverCap(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    256,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	tooBig := strings.Repeat("X", 257)
	_, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32A,
		Category:  "meter",
		DataHash:  "h",
		Metadata:  tooBig,
	})
	if err == nil {
		t.Fatal("expected metadata size rejection")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMsgServer_SubmitEnergyData_AtCapAccepted asserts equality with the cap
// is allowed (off-by-one floor).
func TestMsgServer_SubmitEnergyData_AtCapAccepted(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    256,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	exact := strings.Repeat("X", 256)
	_, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32A,
		Category:  "meter",
		DataHash:  "h",
		Metadata:  exact,
	})
	if err != nil {
		t.Fatalf("at-cap submission must succeed: %v", err)
	}
}

func TestMsgServer_SubmitEnergyData_RateLimitPerBlock(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 2, // tight ceiling for the test
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if _, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
			Submitter: validBech32A,
			Category:  "meter",
			DataHash:  "h",
		}); err != nil {
			t.Fatalf("submission %d should succeed: %v", i+1, err)
		}
	}
	// 3rd submission in same block must be rejected.
	_, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32A,
		Category:  "meter",
		DataHash:  "h",
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded per-block submission limit") {
		t.Fatalf("expected rate-limit rejection, got: %v", err)
	}

	// A different address still has its own bucket.
	if _, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
		Submitter: validBech32B,
		Category:  "meter",
		DataHash:  "h",
	}); err != nil {
		t.Fatalf("independent submitter must not be rate-limited: %v", err)
	}
}

func TestMsgServer_SubmitEnergyData_RateLimitDisabledWhenZero(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 0, // disabled
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := srv.SubmitEnergyData(asCtx(ctx), &types.MsgSubmitEnergyData{
			Submitter: validBech32A,
			Category:  "meter",
			DataHash:  "h",
		}); err != nil {
			t.Fatalf("rate limit must be disabled: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// BatchSubmit
// ---------------------------------------------------------------------------

// computeRoot mimics msg_server.verifyMerkleRoot. Used to craft valid roots
// for the positive-path batch tests.
func computeRoot(items []types.BatchItem) string {
	leaves := make([][]byte, len(items))
	for i, item := range items {
		h := sha3.NewLegacyKeccak256()
		var idx [8]byte
		binary.BigEndian.PutUint64(idx[:], uint64(i))
		h.Write(idx[:])
		h.Write([]byte(item.DataHash))
		leaves[i] = h.Sum(nil)
	}
	return hex.EncodeToString(merkle(leaves))
}

func merkle(leaves [][]byte) []byte {
	if len(leaves) == 1 {
		return leaves[0]
	}
	var next [][]byte
	for i := 0; i < len(leaves); i += 2 {
		if i+1 < len(leaves) {
			next = append(next, hashPair(leaves[i], leaves[i+1]))
		} else {
			next = append(next, hashPair(leaves[i], leaves[i]))
		}
	}
	return merkle(next)
}

func hashPair(a, b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	if hex.EncodeToString(a) <= hex.EncodeToString(b) {
		h.Write(a)
		h.Write(b)
	} else {
		h.Write(b)
		h.Write(a)
	}
	return h.Sum(nil)
}

func TestMsgServer_BatchSubmit_OK(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	items := []types.BatchItem{
		{DataHash: "h-1"},
		{DataHash: "h-2"},
		{DataHash: "h-3"},
	}
	root := computeRoot(items)

	resp, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: root,
	})
	if err != nil {
		t.Fatalf("batch submit: %v", err)
	}
	if resp.DataCount != 3 {
		t.Errorf("data_count: want 3, got %d", resp.DataCount)
	}
	batch, ok := k.GetBatch(ctx, resp.BatchID)
	if !ok {
		t.Fatalf("batch %s not stored", resp.BatchID)
	}
	if batch.DataCount != 3 || batch.MerkleRoot != root {
		t.Errorf("batch metadata wrong: %+v", batch)
	}
}

func TestMsgServer_BatchSubmit_SizeOverCap(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       2,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 100,
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	items := []types.BatchItem{{DataHash: "a"}, {DataHash: "b"}, {DataHash: "c"}}
	_, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: computeRoot(items),
	})
	if err == nil || !strings.Contains(err.Error(), "batch size") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

// TestMsgServer_BatchSubmit_BadMerkleRoot proves the chain cannot be tricked
// into accepting a batch whose declared root doesn't match its items. This
// is critical: the root is the only thing off-chain consumers verify.
func TestMsgServer_BatchSubmit_BadMerkleRoot(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	items := []types.BatchItem{{DataHash: "h-1"}, {DataHash: "h-2"}}
	_, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: "0x" + strings.Repeat("0", 64), // garbage
	})
	if err == nil || !strings.Contains(err.Error(), "merkle root") {
		t.Fatalf("expected merkle root rejection, got %v", err)
	}
}

// TestMsgServer_BatchSubmit_DuplicateItemsDifferentLeaves regression test
// for an attack where a malicious submitter declares N copies of the same
// item but only computes a 1-item root. The leaf scheme prefixes each leaf
// with its index, so duplicate hashes produce different leaves and the
// declared "1-item" root will mismatch.
func TestMsgServer_BatchSubmit_DuplicateItemsDifferentLeaves(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	items := []types.BatchItem{{DataHash: "same"}, {DataHash: "same"}}
	// Compute a *single-item* root and try to pass it as a 2-item batch.
	singleItemRoot := computeRoot(items[:1])

	_, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: singleItemRoot,
	})
	if err == nil {
		t.Fatal("must reject batch where root corresponds to fewer items than declared")
	}
}

// TestMsgServer_BatchSubmit_RateLimitedAsOne verifies the design intent
// from msg_server.go: a batch counts as ONE submission for rate-limit
// purposes regardless of length. Bulk operators are the use case to admit;
// spammers can't loop SubmitEnergyData past the cap.
func TestMsgServer_BatchSubmit_RateLimitedAsOne(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	if err := k.SetParams(ctx, types.Params{
		MaxBatchSize:                       100,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 1, // very tight
		Permissionless:                     true,
	}); err != nil {
		t.Fatal(err)
	}

	items := make([]types.BatchItem, 50)
	for i := range items {
		items[i] = types.BatchItem{DataHash: hexHash([]byte{byte(i)})}
	}
	if _, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: computeRoot(items),
	}); err != nil {
		t.Fatalf("first batch must succeed: %v", err)
	}
	// Second batch in same block consumes the per-block budget.
	_, err := srv.BatchSubmit(asCtx(ctx), &types.MsgBatchSubmit{
		Submitter:  validBech32A,
		Category:   "meter",
		Items:      items,
		MerkleRoot: computeRoot(items),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded per-block submission limit") {
		t.Fatalf("second batch in same block must be rate-limited: %v", err)
	}
}

func hexHash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// UpdateParams (governance)
// ---------------------------------------------------------------------------

func TestMsgServer_UpdateParams_OnlyAuthority(t *testing.T) {
	srv, _, ctx := newMsgServer(t)
	_, err := srv.UpdateParams(asCtx(ctx), &types.MsgUpdateParams{
		Authority: validBech32A, // not the configured authority
		Params:    types.DefaultParams(),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid authority") {
		t.Fatalf("non-authority must be rejected: %v", err)
	}
}

func TestMsgServer_UpdateParams_RejectsInvalidParams(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	bad := types.Params{
		MaxBatchSize:                       0, // forbidden
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 100,
	}
	_, err := srv.UpdateParams(asCtx(ctx), &types.MsgUpdateParams{
		Authority: k.GetAuthority(),
		Params:    bad,
	})
	if err == nil || !strings.Contains(err.Error(), "max_batch_size") {
		t.Fatalf("must reject invalid params, got %v", err)
	}
}

func TestMsgServer_UpdateParams_AuthorityHappyPath(t *testing.T) {
	srv, k, ctx := newMsgServer(t)
	newP := types.Params{
		MaxBatchSize:                       42,
		MaxMetadataSize:                    1024,
		MaxSubmissionsPerBlockPerSubmitter: 7,
		Permissionless:                     false,
		AllowedSubmitters:                  []string{validBech32A},
	}
	if _, err := srv.UpdateParams(asCtx(ctx), &types.MsgUpdateParams{
		Authority: k.GetAuthority(),
		Params:    newP,
	}); err != nil {
		t.Fatalf("authority update must succeed: %v", err)
	}
	got := k.GetParams(ctx)
	if got.MaxBatchSize != 42 || got.MaxSubmissionsPerBlockPerSubmitter != 7 {
		t.Errorf("params not persisted: %+v", got)
	}
	if !k.IsAllowedSubmitter(ctx, validBech32A) {
		t.Errorf("allow-list not persisted")
	}
}
