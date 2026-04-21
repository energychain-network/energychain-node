package keeper_test

import (
	"strings"
	"testing"

	"cosmossdk.io/collections"

	"energychain/x/audit/keeper"
	"energychain/x/audit/types"
)

func TestIDSequenceMonotonicInvariant_AuditOK(t *testing.T) {
	k, ctx := setupKeeper(t)
	k.RecordAuditLog(ctx, types.AuditLog{ID: 1, Actor: "a", EventType: "e", Action: "x"})
	k.IncrementCounter(ctx, 1)
	if msg, broken := keeper.IDSequenceMonotonicInvariant(k)(ctx); broken {
		t.Fatalf("expected invariant to hold, got %q", msg)
	}
}

// TestIDSequenceMonotonicInvariant_AuditBroken stores a log with an id
// higher than the sequence — what a buggy migration that "rewinds" the
// counter would produce.
func TestIDSequenceMonotonicInvariant_AuditBroken(t *testing.T) {
	k, ctx := setupKeeper(t)
	// Bypass GetNextID/IncrementCounter: write log directly with a high ID.
	if err := k.Logs.Set(ctx, 999, types.AuditLog{ID: 999, Actor: "a", EventType: "e", Action: "x"}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.IDSequenceMonotonicInvariant(k)(ctx)
	if !broken || !strings.Contains(msg, "highest log id 999") {
		t.Fatalf("expected sequence-regression break, got %q (broken=%v)", msg, broken)
	}
}

func TestIndexConsistencyInvariant_AuditByActor_BrokenOnDangling(t *testing.T) {
	k, ctx := setupKeeper(t)
	// Inject a dangling (actor, id) entry.
	if err := k.ByActor.Set(ctx, collections.Join("ghost", uint64(404))); err != nil {
		t.Fatal(err)
	}
	inv := keeper.IndexConsistencyInvariant(k, "by-actor", k.ByActor,
		func(l types.AuditLog) string { return l.Actor })
	msg, broken := inv(ctx)
	if !broken || !strings.Contains(msg, "missing log") {
		t.Fatalf("expected dangling break, got %q (broken=%v)", msg, broken)
	}
}

func TestIndexConsistencyInvariant_AuditByEventType_BrokenOnMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)
	k.RecordAuditLog(ctx, types.AuditLog{ID: 1, Actor: "a", EventType: "deploy", Action: "x"})
	// Add an index entry under the wrong event type.
	if err := k.ByEventType.Set(ctx, collections.Join("transfer", uint64(1))); err != nil {
		t.Fatal(err)
	}
	inv := keeper.IndexConsistencyInvariant(k, "by-event-type", k.ByEventType,
		func(l types.AuditLog) string { return l.EventType })
	msg, broken := inv(ctx)
	if !broken || !strings.Contains(msg, "field mismatch") {
		t.Fatalf("expected mismatch break, got %q (broken=%v)", msg, broken)
	}
}

func TestDataWithinCapInvariant_AuditBrokenAfterShrink(t *testing.T) {
	k, ctx := setupKeeper(t)
	if err := k.SetParams(ctx, types.Params{MaxDataSize: 1024}); err != nil {
		t.Fatal(err)
	}
	k.RecordAuditLog(ctx, types.AuditLog{
		ID: 1, Actor: "a", EventType: "e", Action: "x",
		Data: strings.Repeat("X", 512),
	})
	if err := k.Params.Set(ctx, types.Params{MaxDataSize: 256}); err != nil {
		t.Fatal(err)
	}
	msg, broken := keeper.DataWithinCapInvariant(k)(ctx)
	if !broken {
		t.Fatalf("expected cap break, got %q", msg)
	}
}

func TestAllInvariants_Audit_Healthy(t *testing.T) {
	k, ctx := setupKeeper(t)
	id := k.GetNextID(ctx)
	k.RecordAuditLog(ctx, types.AuditLog{ID: id, Actor: "a", EventType: "e", Action: "x"})
	k.IncrementCounter(ctx, id)
	if msg, broken := keeper.AllInvariants(k)(ctx); broken {
		t.Fatalf("expected invariants to hold, got %q", msg)
	}
}
