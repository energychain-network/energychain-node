package keeper_test

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
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/sanctions/keeper"
	"energychain/x/sanctions/types"
)

// stubAudit captures audit hook calls so tests can assert side-effects.
type stubAudit struct {
	calls []string
}

func (s *stubAudit) RecordSanctionsAction(_ sdk.Context, listID, subject, action, detail string) {
	s.calls = append(s.calls, listID+":"+subject+":"+action+":"+detail)
}

const (
	testAuthority = "cosmos1ye4j7hsfmwgvch53kpys4yzm88zwhjkprc7nzx"
	listOwner     = "cosmos15tk4lhmtrwc4qx5gd5sjxdjdy36rg9hk0gx4tv"
	otherAddr     = "cosmos1xrnner78enszhq32u4l5z29slzq2zfvka2lt7g"
	proposer      = "cosmos1uu89qg49evj5pkj2u3xwapdjwt7ymsljt7epry"
	subjectAlice  = "cosmos1alice000000000000000000000000000000000"
	subjectBob    = "cosmos1bob00000000000000000000000000000000000"
	subjectCarol  = "cosmos1carol00000000000000000000000000000000"
)

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context, *stubAudit) {
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
	au := &stubAudit{}

	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), testAuthority, au)
	ctx := sdk.NewContext(stateStore, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return k, ctx, au
}

// registerActiveList is a small helper for the common case in tests.
func registerActiveList(t *testing.T, k keeper.Keeper, ctx sdk.Context, srv types.MsgServer, id, listAuthority string) {
	t.Helper()
	if _, err := srv.RegisterList(ctx, &types.MsgRegisterList{
		Authority:     testAuthority,
		Id:            id,
		Name:          id + " list",
		Jurisdiction:  "US",
		SourceUri:     "https://example.com/" + id,
		ListAuthority: listAuthority,
	}); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// List lifecycle
// ---------------------------------------------------------------------------

func TestRegisterList(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac.sdn", listOwner)
	got, ok := k.GetList(ctx, "ofac.sdn")
	if !ok || got.Status != types.ListStatus_LIST_STATUS_ACTIVE {
		t.Fatalf("expected active list; got %+v", got)
	}
	if got.Authority != listOwner {
		t.Fatalf("expected list_authority=%s; got %s", listOwner, got.Authority)
	}
}

func TestRegisterListRejectsDuplicate(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "dup", "")
	if _, err := srv.RegisterList(ctx, &types.MsgRegisterList{
		Authority: testAuthority, Id: "dup", Name: "x", Jurisdiction: "US",
	}); err == nil {
		t.Fatal("expected duplicate rejection")
	}
}

func TestUpdateListRejectsArchived(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "arch", "")
	if _, err := srv.ArchiveList(ctx, &types.MsgArchiveList{
		Authority: testAuthority, Id: "arch", Reason: "superseded",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.UpdateList(ctx, &types.MsgUpdateList{
		Authority: testAuthority, Id: "arch", Name: "still arch",
	}); err == nil {
		t.Fatal("expected ARCHIVED-list update rejection")
	}
}

// ---------------------------------------------------------------------------
// Entries — direct add path
// ---------------------------------------------------------------------------

func sampleEntry(listID, subject string) types.SanctionEntry {
	return types.SanctionEntry{
		ListId:      listID,
		Subject:     subject,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_ADDRESS,
		Program:     "SDGT",
		Reason:      "OFAC SDN match",
		SourceRef:   "ref-1",
	}
}

func TestAddEntryByGovernance(t *testing.T) {
	k, ctx, au := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority,
		Entry:     sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	if !k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("expected subject to be sanctioned")
	}
	if len(au.calls) == 0 {
		t.Fatal("expected audit hook to fire")
	}
}

func TestAddEntryByListAuthority(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "eu.cons", listOwner)
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: listOwner,
		Entry:     sampleEntry("eu.cons", subjectBob),
	}); err != nil {
		t.Fatalf("list authority should be allowed: %v", err)
	}
	if !k.IsSanctioned(ctx, subjectBob) {
		t.Fatal("expected subject sanctioned")
	}
}

func TestAddEntryRejectsRandomSigner(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "uk", listOwner)
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: otherAddr,
		Entry:     sampleEntry("uk", subjectCarol),
	}); err == nil {
		t.Fatal("expected unauthorized rejection")
	}
}

func TestAddEntryRejectsArchivedList(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "old", "")
	if _, err := srv.ArchiveList(ctx, &types.MsgArchiveList{
		Authority: testAuthority, Id: "old", Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("old", subjectAlice),
	}); err == nil {
		t.Fatal("expected ARCHIVED-list add rejection")
	}
}

func TestAddEntryRespectsRequireProposalParam(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "strict", "")
	params := k.GetParams(ctx)
	params.RequireProposalForAdds = true
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("strict", subjectAlice),
	}); err == nil {
		t.Fatal("expected refusal when require_proposal_for_adds=true")
	}
}

// ---------------------------------------------------------------------------
// Entry removal — append-only invariant
// ---------------------------------------------------------------------------

func TestRemoveEntryFlipsToRemoved(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RemoveEntry(ctx, &types.MsgRemoveEntry{
		Authority: testAuthority, ListId: "ofac", Subject: subjectAlice, Reason: "delisted",
	}); err != nil {
		t.Fatal(err)
	}
	e, ok := k.GetEntry(ctx, "ofac", subjectAlice)
	if !ok || e.Status != types.EntryStatus_ENTRY_STATUS_REMOVED {
		t.Fatalf("expected REMOVED entry; got %+v", e)
	}
	if k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("expected IsSanctioned=false after removal")
	}
}

func TestRemoveEntryByListAuthorityIsRejected(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", listOwner)
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: listOwner, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	// SECURITY: only governance may relax sanctions; list authority
	// can add but not remove.
	if _, err := srv.RemoveEntry(ctx, &types.MsgRemoveEntry{
		Authority: listOwner, ListId: "ofac", Subject: subjectAlice, Reason: "x",
	}); err == nil {
		t.Fatal("expected unauthorized rejection")
	}
}

// ---------------------------------------------------------------------------
// IsSanctioned + ARCHIVED list semantics
// ---------------------------------------------------------------------------

func TestArchivedListEntriesNoLongerMatch(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	if !k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("expected sanctioned")
	}
	if _, err := srv.ArchiveList(ctx, &types.MsgArchiveList{
		Authority: testAuthority, Id: "ofac", Reason: "rotated",
	}); err != nil {
		t.Fatal(err)
	}
	if k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("ARCHIVED list entries must not match IsSanctioned")
	}
}

func TestDeprecatedListEntriesStillMatch(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.DeprecateList(ctx, &types.MsgDeprecateList{
		Authority: testAuthority, Id: "ofac", Reason: "v2",
	}); err != nil {
		t.Fatal(err)
	}
	if !k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("DEPRECATED list entries must still match")
	}
}

// ---------------------------------------------------------------------------
// Propose + confirm workflow
// ---------------------------------------------------------------------------

func TestProposeAndConfirmBatch(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "eu", "")
	resp, err := srv.ProposeDelta(ctx, &types.MsgProposeDelta{
		Proposer: proposer,
		ListId:   "eu",
		Actions: []types.ProposalAction{
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("eu", subjectAlice)},
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("eu", subjectBob)},
		},
		SourceUri:    "https://example.com/eu-2026q2",
		SourceDigest: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	confirm, err := srv.ConfirmProposal(ctx, &types.MsgConfirmProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId,
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirm.AddedCount != 2 || confirm.RemovedCount != 0 {
		t.Fatalf("expected added=2 removed=0; got %+v", confirm)
	}
	if !k.IsSanctioned(ctx, subjectAlice) || !k.IsSanctioned(ctx, subjectBob) {
		t.Fatal("expected both subjects sanctioned post-confirm")
	}
}

func TestConfirmRejectedIfListArchivedMidFlight(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "race", "")
	resp, err := srv.ProposeDelta(ctx, &types.MsgProposeDelta{
		Proposer: proposer, ListId: "race",
		Actions: []types.ProposalAction{
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("race", subjectAlice)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ArchiveList(ctx, &types.MsgArchiveList{
		Authority: testAuthority, Id: "race", Reason: "rotated",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ConfirmProposal(ctx, &types.MsgConfirmProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId,
	}); err == nil {
		t.Fatal("expected confirm rejection on ARCHIVED list")
	}
}

func TestConfirmIsAtomic(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	// Pre-add Bob via direct route so the proposal's REMOVE-Bob arm
	// works while ADD-Alice + REMOVE-Carol exercises the all-or-none
	// rule (Carol does not exist => the whole proposal must fail).
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectBob),
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := srv.ProposeDelta(ctx, &types.MsgProposeDelta{
		Proposer: proposer, ListId: "ofac",
		Actions: []types.ProposalAction{
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("ofac", subjectAlice)},
			{Kind: types.ProposalAction_KIND_REMOVE, Entry: sampleEntry("ofac", subjectCarol)},
			{Kind: types.ProposalAction_KIND_REMOVE, Entry: sampleEntry("ofac", subjectBob)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ConfirmProposal(ctx, &types.MsgConfirmProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId,
	}); err == nil {
		t.Fatal("expected atomic failure (Carol not present)")
	}
	// SECURITY: Atomic invariant — no partial mutation. Alice must NOT
	// be sanctioned and Bob must STILL be sanctioned.
	if k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("partial apply: Alice was added despite atomic failure")
	}
	if !k.IsSanctioned(ctx, subjectBob) {
		t.Fatal("partial apply: Bob was removed despite atomic failure")
	}
}

func TestRejectProposalLeavesStateIntact(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	resp, _ := srv.ProposeDelta(ctx, &types.MsgProposeDelta{
		Proposer: proposer, ListId: "ofac",
		Actions: []types.ProposalAction{
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("ofac", subjectAlice)},
		},
	})
	if _, err := srv.RejectProposal(ctx, &types.MsgRejectProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId, Reason: "bogus source",
	}); err != nil {
		t.Fatal(err)
	}
	if k.IsSanctioned(ctx, subjectAlice) {
		t.Fatal("rejected proposal must not apply")
	}
	p, _ := k.GetProposal(ctx, resp.ProposalId)
	if p.Status != types.ProposalStatus_PROPOSAL_STATUS_REJECTED {
		t.Fatalf("expected REJECTED; got %s", p.Status)
	}
}

func TestConfirmTwiceRejected(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	resp, _ := srv.ProposeDelta(ctx, &types.MsgProposeDelta{
		Proposer: proposer, ListId: "ofac",
		Actions: []types.ProposalAction{
			{Kind: types.ProposalAction_KIND_ADD, Entry: sampleEntry("ofac", subjectAlice)},
		},
	})
	if _, err := srv.ConfirmProposal(ctx, &types.MsgConfirmProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ConfirmProposal(ctx, &types.MsgConfirmProposal{
		Authority: testAuthority, ProposalId: resp.ProposalId,
	}); err == nil {
		t.Fatal("expected refusal to re-confirm")
	}
}

// ---------------------------------------------------------------------------
// RecordHit + ring buffer
// ---------------------------------------------------------------------------

func TestRecordHitAppendsAndIndexes(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	k.RecordHit(ctx, subjectAlice, "ofac", "rwa", "GREEN-001", "transfer_block")
	k.RecordHit(ctx, subjectAlice, "ofac", "rwa", "GREEN-001", "transfer_block")
	var n int
	if err := k.Hits.Walk(ctx, nil, func(_ uint64, _ types.SanctionHit) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 hits stored; got %d", n)
	}
}

func TestHitsRingBufferIsBounded(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	params := k.GetParams(ctx)
	params.HitsLogMax = 4
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		k.RecordHit(ctx, subjectAlice, "ofac", "", "", "transfer_block")
	}
	var n uint32
	if err := k.Hits.Walk(ctx, nil, func(_ uint64, _ types.SanctionHit) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n > params.HitsLogMax {
		t.Fatalf("ring buffer not bounded: %d > %d", n, params.HitsLogMax)
	}
}

// ---------------------------------------------------------------------------
// Subject listings (history) & MatchedLists
// ---------------------------------------------------------------------------

func TestMatchedListsCoversMultipleLists(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	registerActiveList(t, k, ctx, srv, "eu", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("eu", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	matched := k.MatchedLists(ctx, subjectAlice)
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched lists; got %v", matched)
	}
}

// ---------------------------------------------------------------------------
// Regression: subject canonicalization (HIGH severity fix)
// ---------------------------------------------------------------------------
//
// SECURITY: bech32 addresses are case-insensitive on the wire but
// MUST be stored canonical-lowercase. An earlier prototype stored
// the subject verbatim, which let an attacker register
// "cosmos1ABC..." and bypass IsSanctioned("cosmos1abc...").

func TestSubjectCanonicalizationLookup(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	mixed := "cosmos1ABC000000000000000000000000000000000"
	lower := "cosmos1abc000000000000000000000000000000000"
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority,
		Entry: types.SanctionEntry{
			ListId:      "ofac",
			Subject:     mixed,
			SubjectKind: types.SubjectKind_SUBJECT_KIND_ADDRESS,
			Program:     "SDGT",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if !k.IsSanctioned(ctx, mixed) {
		t.Fatal("mixed-case form must hit")
	}
	if !k.IsSanctioned(ctx, lower) {
		t.Fatal("lowercase form must hit (canonical)")
	}
}

func TestSubjectCanonicalizationDedupe(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", "")
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority,
		Entry: types.SanctionEntry{
			ListId:      "ofac",
			Subject:     "cosmos1abc000000000000000000000000000000000",
			SubjectKind: types.SubjectKind_SUBJECT_KIND_ADDRESS,
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Mixed-case duplicate must be rejected.
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority,
		Entry: types.SanctionEntry{
			ListId:      "ofac",
			Subject:     "cosmos1ABC000000000000000000000000000000000",
			SubjectKind: types.SubjectKind_SUBJECT_KIND_ADDRESS,
		},
	}); err == nil {
		t.Fatal("expected mixed-case duplicate rejection (canonicalisation bypass)")
	}
}

// ---------------------------------------------------------------------------
// Regression: HitsLogMax=0 skips writes (prevents unbounded growth)
// ---------------------------------------------------------------------------

func TestRecordHitNoOpWhenHitsLogMaxZero(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	params := k.GetParams(ctx)
	params.HitsLogMax = 0
	if err := k.SetParams(ctx, params); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		k.RecordHit(ctx, subjectAlice, "ofac", "rwa", "X", "transfer_block")
	}
	var n int
	if err := k.Hits.Walk(ctx, nil, func(_ uint64, _ types.SanctionHit) (bool, error) {
		n++
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("HitsLogMax=0 should skip writes; got %d entries", n)
	}
}

// ---------------------------------------------------------------------------
// Genesis round-trip
// ---------------------------------------------------------------------------

func TestGenesisRoundTrip(t *testing.T) {
	k, ctx, _ := setupKeeper(t)
	srv := keeper.NewMsgServerImpl(k)
	registerActiveList(t, k, ctx, srv, "ofac", listOwner)
	if _, err := srv.AddEntry(ctx, &types.MsgAddEntry{
		Authority: testAuthority, Entry: sampleEntry("ofac", subjectAlice),
	}); err != nil {
		t.Fatal(err)
	}
	gs := k.ExportGenesis(ctx)
	if len(gs.Lists) != 1 || len(gs.Entries) != 1 {
		t.Fatalf("export missing rows: %+v", gs)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported gs invalid: %v", err)
	}
}
