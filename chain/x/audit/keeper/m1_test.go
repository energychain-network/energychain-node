package keeper_test

import (
	"strings"
	"testing"
	"time"

	"energychain/x/audit/keeper"
	"energychain/x/audit/types"
)

// ---------------------------------------------------------------------------
// Schema registry
// ---------------------------------------------------------------------------

func TestSetGetSchema(t *testing.T) {
	k, ctx := setupKeeper(t)
	d := types.SchemaDescriptor{
		EventType:    "param_update.x/eac",
		Uri:          "ipfs://Qm123",
		Hash:         strings.Repeat("a", 64),
		Version:      1,
		RegisteredBy: "energy1gov",
		RegisteredAt: 100,
	}
	if err := k.SetSchema(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, ok := k.GetSchema(ctx, "param_update.x/eac")
	if !ok {
		t.Fatalf("schema not found")
	}
	if got.Uri != d.Uri || got.Hash != d.Hash || got.Version != 1 {
		t.Errorf("descriptor mismatch: %+v", got)
	}
}

// ---------------------------------------------------------------------------
// Archive eviction
// ---------------------------------------------------------------------------

func TestEvictLogsForArchive(t *testing.T) {
	k, ctx := setupKeeper(t)
	for i := uint64(1); i <= 5; i++ {
		if err := k.RecordAuditLog(ctx, types.AuditLog{
			ID: i, Actor: "a", EventType: "e", Action: "x",
			Severity: types.Severity_SEVERITY_INFO,
		}); err != nil {
			t.Fatal(err)
		}
	}

	evicted, err := k.EvictLogsForArchive(ctx, 2, 4, 100)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != 3 {
		t.Errorf("evicted = %d, want 3", evicted)
	}
	for _, id := range []uint64{2, 3, 4} {
		if _, ok := k.GetAuditLog(ctx, id); ok {
			t.Errorf("log %d should be evicted", id)
		}
	}
	for _, id := range []uint64{1, 5} {
		if _, ok := k.GetAuditLog(ctx, id); !ok {
			t.Errorf("log %d should remain", id)
		}
	}
}

// TestEvictLogsForArchive_BatchCap verifies that the cap really stops
// further work — what bounds block gas in the archive flow.
func TestEvictLogsForArchive_BatchCap(t *testing.T) {
	k, ctx := setupKeeper(t)
	for i := uint64(1); i <= 5; i++ {
		_ = k.RecordAuditLog(ctx, types.AuditLog{
			ID: i, Actor: "a", EventType: "e", Action: "x",
		})
	}
	evicted, err := k.EvictLogsForArchive(ctx, 1, 5, 2)
	if err != nil {
		t.Fatal(err)
	}
	if evicted != 2 {
		t.Errorf("expected cap to stop at 2, got %d", evicted)
	}
	if _, ok := k.GetAuditLog(ctx, 3); !ok {
		t.Errorf("log 3 should still be present (cap should have stopped earlier)")
	}
}

// ---------------------------------------------------------------------------
// View key grants
// ---------------------------------------------------------------------------

func TestGrantLifecycle(t *testing.T) {
	k, sctx := setupKeeper(t)
	sctx = sctx.WithBlockTime(time.Unix(1_700_000_000, 0))

	g := types.ViewKeyGrant{
		Id:           "vk-1",
		Grantor:      "energy1gov",
		Grantee:      "energy1regulator",
		Scope:        types.ViewScope{EventTypePrefix: "freeze."},
		EncryptedKey: []byte("ciphertext"),
		GrantedAt:    sctx.BlockTime().Unix(),
		ExpiresAt:    sctx.BlockTime().Unix() + 3600,
	}
	if err := k.SetGrant(sctx, g); err != nil {
		t.Fatal(err)
	}
	if got, ok := k.GetGrant(sctx, "vk-1"); !ok || got.Grantee != "energy1regulator" {
		t.Fatalf("grant lookup failed: %+v ok=%v", got, ok)
	}

	if err := k.MarkGrantRevoked(sctx, g, "test"); err != nil {
		t.Fatal(err)
	}
	got, _ := k.GetGrant(sctx, "vk-1")
	if !got.Revoked || got.RevocationReason != "test" {
		t.Errorf("revoke state wrong: %+v", got)
	}
}

// TestSweepExpiredGrants verifies the EndBlock sweep actually flips a
// grant to revoked once its expires_at slips into the past.
func TestSweepExpiredGrants(t *testing.T) {
	k, ctx := setupKeeper(t)
	now := int64(1_700_000_000)
	ctx = ctx.WithBlockTime(time.Unix(now, 0))

	g := types.ViewKeyGrant{
		Id:           "vk-7",
		Grantor:      "energy1gov",
		Grantee:      "energy1regulator",
		EncryptedKey: []byte("ct"),
		GrantedAt:    now - 7200,
		ExpiresAt:    now - 1, // already expired
	}
	if err := k.SetGrant(ctx, g); err != nil {
		t.Fatal(err)
	}

	n, err := k.SweepExpiredGrants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("swept = %d, want 1", n)
	}

	got, _ := k.GetGrant(ctx, "vk-7")
	if !got.Revoked || got.RevocationReason != "expired" {
		t.Errorf("expected expired-revoked, got %+v", got)
	}
}

// TestRecordAuditWithSchemaRequiresRegistration locks in the gating
// behaviour: a writer that names a schema_id must reference a registered
// descriptor; otherwise the message is rejected.
func TestRecordAuditWithSchemaRequiresRegistration(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RecordAudit(ctx, &types.MsgRecordAudit{
		Creator:   "energy1alice",
		EventType: "any",
		Action:    "do",
		SchemaId:  "ghost",
		Severity:  types.Severity_SEVERITY_INFO,
	})
	if err == nil {
		t.Fatalf("expected schema-not-registered rejection")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should reference missing schema id, got %v", err)
	}

	// Register the schema, retry — should succeed.
	if err := k.SetSchema(ctx, types.SchemaDescriptor{
		EventType: "ghost", Uri: "ipfs://x", Hash: strings.Repeat("a", 64),
		Version: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RecordAudit(ctx, &types.MsgRecordAudit{
		Creator:   "energy1alice",
		EventType: "any",
		Action:    "do",
		SchemaId:  "ghost",
		Severity:  types.Severity_SEVERITY_INFO,
	}); err != nil {
		t.Errorf("retry after registration should succeed, got %v", err)
	}
}

// TestArchiveTooYoungRejected guards the archive-min-age safety net so a
// rogue authority cannot evict freshly written logs.
func TestArchiveTooYoungRejected(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	ctx = ctx.WithBlockHeight(100)

	// Record a log at height 100.
	if err := k.RecordAuditLog(ctx, types.AuditLog{
		ID: 1, Actor: "a", EventType: "e", Action: "x", BlockHeight: 100,
	}); err != nil {
		t.Fatal(err)
	}
	k.IncrementCounter(ctx, 1)

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.Archive(ctx, &types.MsgArchive{
		Authority:  "authority",
		FromLogId:  1,
		ToLogId:    1,
		MerkleRoot: "deadbeef",
		Uri:        "ipfs://Qm",
	})
	if err == nil {
		t.Fatalf("expected too-young rejection")
	}
	if !strings.Contains(err.Error(), "too young") {
		t.Errorf("error should mention too young, got %v", err)
	}
}

// TestArchiveSeals end-to-end: aged logs are evicted, a segment row is
// written, and the segment fields cover the eviction span.
func TestArchiveSeals(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := types.DefaultParams()
	params.ArchiveMinAgeBlocks = 10
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}

	// Five logs at height 1; we'll archive once head = 100.
	ctx = ctx.WithBlockHeight(1).WithBlockTime(time.Unix(1_700_000_000, 0))
	for i := uint64(1); i <= 5; i++ {
		if err := k.RecordAuditLog(ctx, types.AuditLog{
			ID: i, Actor: "a", EventType: "e", Action: "x",
			BlockHeight: 1, Timestamp: 1_700_000_000 + int64(i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	k.IncrementCounter(ctx, 5)

	ctx = ctx.WithBlockHeight(100).WithBlockTime(time.Unix(1_700_010_000, 0))
	srv := keeper.NewMsgServerImpl(k)
	resp, err := srv.Archive(ctx, &types.MsgArchive{
		Authority:  "authority",
		FromLogId:  2,
		ToLogId:    4,
		MerkleRoot: "0xroot",
		Uri:        "ipfs://Qm",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Evicted != 3 {
		t.Errorf("evicted = %d, want 3", resp.Evicted)
	}
	for _, id := range []uint64{2, 3, 4} {
		if _, ok := k.GetAuditLog(ctx, id); ok {
			t.Errorf("log %d should be evicted", id)
		}
	}
	seg, ok := k.GetArchiveSegment(ctx, resp.SegmentId)
	if !ok {
		t.Fatalf("segment %d not stored", resp.SegmentId)
	}
	if seg.LogCount != 3 || seg.FromLogId != 2 || seg.ToLogId != 4 {
		t.Errorf("segment span wrong: %+v", seg)
	}
	if seg.MerkleAlgorithm != types.MerkleAlgorithmSHA256 {
		t.Errorf("default algo wrong: %s", seg.MerkleAlgorithm)
	}
}

// TestArchiveSpanCap locks in the DoS guard: the from..to span must fit
// inside params.archive_max_batch, otherwise the pre-flight validation
// loop would iterate up to (to - from) times regardless of how many logs
// actually exist in that range.
func TestArchiveSpanCap(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := types.DefaultParams()
	params.ArchiveMaxBatch = 10 // tight cap for the test
	params.ArchiveMinAgeBlocks = 0
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}

	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.Archive(ctx, &types.MsgArchive{
		Authority:  "authority",
		FromLogId:  1,
		ToLogId:    1_000_000_000, // wildly larger than the cap
		MerkleRoot: "deadbeef",
		Uri:        "ipfs://Qm",
	})
	if err == nil {
		t.Fatalf("expected span-cap rejection")
	}
	if !strings.Contains(err.Error(), "exceeds archive_max_batch") {
		t.Errorf("error should mention the cap, got %v", err)
	}
}

// TestGrantViewKeyExpiryCap locks in that view_key_max_validity caps how
// far in the future a grant may expire.
func TestGrantViewKeyExpiryCap(t *testing.T) {
	k, ctx := setupKeeper(t)
	params := types.DefaultParams()
	params.ViewKeyMaxValidity = 60 // 1 minute
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	now := int64(1_700_000_000)
	ctx = ctx.WithBlockTime(time.Unix(now, 0))
	srv := keeper.NewMsgServerImpl(k)

	_, err := srv.GrantViewKey(ctx, &types.MsgGrantViewKey{
		Grantor:      "energy1gov",
		Grantee:      "energy1regulator",
		Scope:        types.ViewScope{},
		EncryptedKey: []byte("ct"),
		ExpiresAt:    now + 3600, // way past the 60s cap
	})
	if err == nil {
		t.Fatalf("expected expires_at-cap rejection")
	}
}

// TestRevokeViewKeyAuthorisation: only grantor or governance may revoke.
func TestRevokeViewKeyAuthorisation(t *testing.T) {
	k, ctx := setupKeeper(t)
	now := int64(1_700_000_000)
	ctx = ctx.WithBlockTime(time.Unix(now, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}

	g := types.ViewKeyGrant{
		Id:           "vk-3",
		Grantor:      "energy1grantor",
		Grantee:      "energy1regulator",
		EncryptedKey: []byte("ct"),
		GrantedAt:    now,
	}
	if err := k.SetGrant(ctx, g); err != nil {
		t.Fatal(err)
	}

	srv := keeper.NewMsgServerImpl(k)

	// Stranger cannot revoke.
	if _, err := srv.RevokeViewKey(ctx, &types.MsgRevokeViewKey{
		Actor: "energy1stranger", GrantId: "vk-3", Reason: "nope",
	}); err == nil {
		t.Errorf("stranger should be rejected")
	}

	// Grantor can.
	if _, err := srv.RevokeViewKey(ctx, &types.MsgRevokeViewKey{
		Actor: "energy1grantor", GrantId: "vk-3", Reason: "ok",
	}); err != nil {
		t.Errorf("grantor revoke should succeed, got %v", err)
	}
	if got, _ := k.GetGrant(ctx, "vk-3"); !got.Revoked {
		t.Errorf("expected revoked")
	}
}
