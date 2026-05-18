package keeper_test

import (
	"strings"
	"testing"
	"time"

	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dispute/keeper"
	"energychain/x/dispute/types"
)

// ---- Stubs -------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

type stubDID struct {
	controllers map[string]map[string]bool
}

func newStubDID() *stubDID { return &stubDID{controllers: map[string]map[string]bool{}} }
func (d *stubDID) IsControllerOf(_ sdk.Context, did, addr string) bool {
	if _, ok := d.controllers[did]; !ok {
		return false
	}
	return d.controllers[did][addr]
}
func (d *stubDID) bind(did, addr string) {
	if _, ok := d.controllers[did]; !ok {
		d.controllers[did] = map[string]bool{}
	}
	d.controllers[did][addr] = true
}

// stubStablecoin is the in-memory bond ledger the dispute
// keeper uses for bond movements. It mirrors the surface of
// x/escrow's escrowStablecoinAdapter so the dispute keeper's
// bond logic is exercised end-to-end without depending on
// the real x/stablecoin module.
type stubStablecoin struct {
	denoms      map[string]bool
	paused      map[string]bool
	blocked     map[string]map[string]bool // denom → account → blocked
	balances    map[string]map[string]uint64
	failNextMov string // error message to return on the next Move; empty → no override
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms:   map[string]bool{},
		paused:   map[string]bool{},
		blocked:  map[string]map[string]bool{},
		balances: map[string]map[string]uint64{},
	}
}

func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool {
	return s.paused[d]
}
func (s *stubStablecoin) IsAccountBlocked(_ sdk.Context, d, who string) bool {
	if _, ok := s.blocked[d]; !ok {
		return false
	}
	return s.blocked[d][who]
}
func (s *stubStablecoin) Move(_ sdk.Context, denom, from, to string, amount uint64) error {
	if s.failNextMov != "" {
		msg := s.failNextMov
		s.failNextMov = ""
		return testErr(msg)
	}
	if !s.denoms[denom] {
		return testErr("denom not registered")
	}
	if _, ok := s.balances[denom]; !ok {
		s.balances[denom] = map[string]uint64{}
	}
	bal := s.balances[denom]
	if bal[from] < amount {
		return testErr("insufficient balance")
	}
	bal[from] -= amount
	bal[to] += amount
	return nil
}
func (s *stubStablecoin) credit(denom, who string, amount uint64) {
	if _, ok := s.balances[denom]; !ok {
		s.balances[denom] = map[string]uint64{}
	}
	s.balances[denom][who] += amount
}
func (s *stubStablecoin) balance(denom, who string) uint64 {
	if _, ok := s.balances[denom]; !ok {
		return 0
	}
	return s.balances[denom][who]
}
func (s *stubStablecoin) block(denom, who string) {
	if _, ok := s.blocked[denom]; !ok {
		s.blocked[denom] = map[string]bool{}
	}
	s.blocked[denom][who] = true
}

type testErr string

func (e testErr) Error() string { return string(e) }

// ---- Setup -------------------------------------------------------------

func mkAddr(seed byte) string {
	bz := make([]byte, 20)
	for i := range bz {
		bz[i] = seed + byte(i)
	}
	return sdk.AccAddress(bz).String()
}

var (
	authority = mkAddr(1)
	alice     = mkAddr(2)
	bob       = mkAddr(3)
	charlie   = mkAddr(4)
	arbKey1   = mkAddr(5)
	arbKey2   = mkAddr(6)
	arbKey3   = mkAddr(7)
	stranger  = mkAddr(8)
	arbKey4   = mkAddr(9)

	arbDID1 = "did:example:arb-1"
	arbDID2 = "did:example:arb-2"
	arbDID3 = "did:example:arb-3"
	arbDID4 = "did:example:arb-4"

	denomA = "usdc"
)

type fixture struct {
	k    keeper.Keeper
	ctx  sdk.Context
	srv  types.MsgServer
	san  *stubSanctions
	did  *stubDID
	coin *stubStablecoin
}

func setup(t *testing.T) *fixture {
	t.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	san := &stubSanctions{bad: map[string]bool{}}
	did := newStubDID()
	coin := newStubStablecoin()
	coin.denoms[denomA] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, coin, did, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, did: did, coin: coin}
}

// ---- helpers -----------------------------------------------------------

func (f *fixture) registerArb(t *testing.T, signer, did, name string) uint64 {
	t.Helper()
	r, err := f.srv.RegisterArbitrator(f.ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: did, SignerAddress: signer, Name: name,
		AccreditedStandards: []string{"ISO-1234"},
		Jurisdictions:       []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatalf("register-arbitrator: %v", err)
	}
	return r.ArbitratorId
}

func (f *fixture) openDispute(t *testing.T, plaintiff, respondent string, pBond, rBondReq uint64) uint64 {
	t.Helper()
	f.coin.credit(denomA, plaintiff, pBond)
	r, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: plaintiff, Respondent: respondent,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_CONTRACT_SETTLEMENT,
		SubjectRef:  "subj-1",
		ClaimUri:    "ipfs://claim",
		ClaimHash:   strings.Repeat("a", 64),
		Memo:        "test",
		BondDenom:   denomA, PlaintiffBond: pBond, RespondentBondRequired: rBondReq,
	})
	if err != nil {
		t.Fatalf("open-dispute: %v", err)
	}
	return r.DisputeId
}

func (f *fixture) respond(t *testing.T, respondent string, did uint64, bond uint64) {
	t.Helper()
	f.coin.credit(denomA, respondent, bond)
	_, err := f.srv.Respond(f.ctx, &types.MsgRespond{
		Respondent: respondent, DisputeId: did, Bond: bond,
	})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
}

func (f *fixture) assignTribunal(t *testing.T, did uint64, ids ...uint64) {
	t.Helper()
	_, err := f.srv.AssignTribunal(f.ctx, &types.MsgAssignTribunal{
		Authority: authority, DisputeId: did, ArbitratorIds: ids,
	})
	if err != nil {
		t.Fatalf("assign-tribunal: %v", err)
	}
}

func (f *fixture) castVote(t *testing.T, signer string, arbID, did uint64, choice types.VoteChoice) {
	t.Helper()
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: signer, ArbitratorId: arbID, DisputeId: did,
		Choice: choice, Reason: "x",
	})
	if err != nil {
		t.Fatalf("cast-vote arb=%d: %v", arbID, err)
	}
}

func (f *fixture) finalize(t *testing.T, actor string, did uint64, slashBpsOverride uint32) types.RulingOutcome {
	t.Helper()
	resp, err := f.srv.FinalizeRuling(f.ctx, &types.MsgFinalizeRuling{
		Actor: actor, DisputeId: did, OverrideSlashBps: slashBpsOverride,
		RulingUri: "ipfs://ruling", RulingHash: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return resp.Outcome
}

func (f *fixture) advanceTime(t time.Duration) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(t))
}

// ---- tests -------------------------------------------------------------

func TestRegisterArbitratorAuthorityGate(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterArbitrator(f.ctx, &types.MsgRegisterArbitrator{
		Authority: stranger, Did: arbDID1, SignerAddress: arbKey1, Name: "X",
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestRegisterArbitratorDIDUniqueness(t *testing.T) {
	f := setup(t)
	f.registerArb(t, arbKey1, arbDID1, "A")
	_, err := f.srv.RegisterArbitrator(f.ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: arbDID1, SignerAddress: arbKey2, Name: "B",
	})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate DID, got %v", err)
	}
}

func TestRegisterArbitratorSignerUniqueness(t *testing.T) {
	f := setup(t)
	f.registerArb(t, arbKey1, arbDID1, "A")
	_, err := f.srv.RegisterArbitrator(f.ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: arbDID2, SignerAddress: arbKey1, Name: "B",
	})
	if err == nil || !strings.Contains(err.Error(), "signer_address") {
		t.Fatalf("expected duplicate signer, got %v", err)
	}
}

func TestOpenDisputeMovesPlaintiffBond(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 100)
	if got := f.coin.balance(denomA, alice); got != 0 {
		t.Fatalf("plaintiff balance = %d, want 0", got)
	}
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 100 {
		t.Fatalf("pool balance = %d, want 100", got)
	}
	d, err := f.k.MustGetDispute(f.ctx, did)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_OPEN {
		t.Fatalf("status = %s, want OPEN", d.Status)
	}
}

func TestOpenDisputeRejectsSanctionedPlaintiff(t *testing.T) {
	f := setup(t)
	f.san.bad[alice] = true
	f.coin.credit(denomA, alice, 100)
	_, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 100, RespondentBondRequired: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned, got %v", err)
	}
}

func TestOpenDisputeRejectsUnknownDenom(t *testing.T) {
	f := setup(t)
	f.coin.credit(denomA, alice, 100)
	_, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: "nope", PlaintiffBond: 100, RespondentBondRequired: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unknown denom, got %v", err)
	}
}

func TestOpenDisputeRejectsPausedDenom(t *testing.T) {
	f := setup(t)
	f.coin.paused[denomA] = true
	f.coin.credit(denomA, alice, 100)
	_, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 100, RespondentBondRequired: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("expected paused, got %v", err)
	}
}

func TestOpenDisputeRejectsBlockedPlaintiff(t *testing.T) {
	f := setup(t)
	f.coin.block(denomA, alice)
	f.coin.credit(denomA, alice, 100)
	_, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 100, RespondentBondRequired: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected blocked, got %v", err)
	}
}

func TestRespondMovesBondAndAdvancesStatus(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 300 {
		t.Fatalf("pool = %d, want 300", got)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_RESPONDED {
		t.Fatalf("status = %s, want RESPONDED", d.Status)
	}
}

func TestRespondRefusesUnderfundedBond(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.coin.credit(denomA, bob, 200)
	_, err := f.srv.Respond(f.ctx, &types.MsgRespond{Respondent: bob, DisputeId: did, Bond: 199})
	if err == nil || !strings.Contains(err.Error(), "< required") {
		t.Fatalf("expected underfunded, got %v", err)
	}
}

func TestRespondRefusesWrongRespondent(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 100)
	f.coin.credit(denomA, charlie, 100)
	_, err := f.srv.Respond(f.ctx, &types.MsgRespond{Respondent: charlie, DisputeId: did, Bond: 100})
	if err == nil || !strings.Contains(err.Error(), "only the named respondent") {
		t.Fatalf("expected wrong-respondent, got %v", err)
	}
}

func TestAssignTribunalRequiresAccredited(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 100)
	f.respond(t, bob, did, 100)
	// suspend a2
	_, err := f.srv.UpdateArbitratorStatus(f.ctx, &types.MsgUpdateArbitratorStatus{
		Authority: authority, ArbitratorId: a2,
		NewStatus: types.ArbitratorStatus_ARBITRATOR_STATUS_SUSPENDED,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.srv.AssignTribunal(f.ctx, &types.MsgAssignTribunal{
		Authority: authority, DisputeId: did, ArbitratorIds: []uint64{a1, a2, a3},
	})
	if err == nil || !strings.Contains(err.Error(), "not ACCREDITED") {
		t.Fatalf("expected non-accredited reject, got %v", err)
	}
}

func TestAssignTribunalRejectsPanelOverflow(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	a4 := f.registerArb(t, arbKey4, arbDID4, "A4")
	did := f.openDispute(t, alice, bob, 100, 100)
	f.respond(t, bob, did, 100)
	_, err := f.srv.AssignTribunal(f.ctx, &types.MsgAssignTribunal{
		Authority: authority, DisputeId: did, ArbitratorIds: []uint64{a1, a2, a3, a4},
	})
	if err == nil || !strings.Contains(err.Error(), "panel_size") {
		t.Fatalf("expected panel overflow, got %v", err)
	}
}

func TestAssignTribunalRejectsConflictOfInterest(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, alice, arbDID1, "A1") // signer == plaintiff
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 100)
	f.respond(t, bob, did, 100)
	_, err := f.srv.AssignTribunal(f.ctx, &types.MsgAssignTribunal{
		Authority: authority, DisputeId: did, ArbitratorIds: []uint64{a1, a2, a3},
	})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected COI reject, got %v", err)
	}
}

func TestVoteAndFinalizePlaintiffWinsSlashesRespondent(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
	f.castVote(t, arbKey2, a2, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
	f.castVote(t, arbKey3, a3, did, types.VoteChoice_VOTE_CHOICE_RESPONDENT)
	outcome := f.finalize(t, authority, did, 0) // default slash = 100%
	if outcome != types.RulingOutcome_RULING_OUTCOME_PLAINTIFF {
		t.Fatalf("outcome = %s, want PLAINTIFF", outcome)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("alice refund = %d, want 100", got)
	}
	if got := f.coin.balance(denomA, bob); got != 0 {
		t.Fatalf("bob refund = %d, want 0 (slashed)", got)
	}
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 200 {
		t.Fatalf("pool retains slashed bond = %d, want 200", got)
	}
}

func TestFinalizeNoFaultRefundsBothInFull(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	f.castVote(t, arbKey2, a2, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	f.castVote(t, arbKey3, a3, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	out := f.finalize(t, authority, did, 0)
	if out != types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		t.Fatalf("outcome = %s, want NO_FAULT", out)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("alice = %d", got)
	}
	if got := f.coin.balance(denomA, bob); got != 200 {
		t.Fatalf("bob = %d", got)
	}
}

func TestFinalizeSplitSlashesBothByOverride(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_SPLIT)
	f.castVote(t, arbKey2, a2, did, types.VoteChoice_VOTE_CHOICE_SPLIT)
	f.castVote(t, arbKey3, a3, did, types.VoteChoice_VOTE_CHOICE_SPLIT)
	// authority override: 50% slash
	out := f.finalize(t, authority, did, 5000)
	if out != types.RulingOutcome_RULING_OUTCOME_SPLIT {
		t.Fatalf("outcome = %s", out)
	}
	if got := f.coin.balance(denomA, alice); got != 50 {
		t.Fatalf("alice = %d, want 50", got)
	}
	if got := f.coin.balance(denomA, bob); got != 100 {
		t.Fatalf("bob = %d, want 100", got)
	}
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 150 {
		t.Fatalf("pool = %d, want 150 (50 + 100 slashed)", got)
	}
}

func TestNonAuthorityCannotFinalizeBeforeQuorumOrDeadline(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
	// quorum 67% of 3 = 2; only one vote → reject non-authority
	_, err := f.srv.FinalizeRuling(f.ctx, &types.MsgFinalizeRuling{
		Actor: alice, DisputeId: did, RulingUri: "x", RulingHash: strings.Repeat("0", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "not finalizable") {
		t.Fatalf("expected non-finalizable, got %v", err)
	}
}

func TestNonAuthorityCanFinalizeAfterDeadline(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// no votes; advance past deadline
	f.advanceTime(time.Duration(types.DefaultDeliberationPeriodSeconds+1) * time.Second)
	out := f.finalize(t, stranger, did, 0)
	if out != types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		t.Fatalf("expected NO_FAULT after empty tally, got %s", out)
	}
}

func TestVoteRefusesNonTribunalArbitrator(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	a4 := f.registerArb(t, arbKey4, arbDID4, "A4")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: arbKey4, ArbitratorId: a4, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_PLAINTIFF,
	})
	if err == nil || !strings.Contains(err.Error(), "not on tribunal") {
		t.Fatalf("expected not-on-tribunal, got %v", err)
	}
}

func TestVoteRefusesImpersonator(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// stranger tries to vote as a1
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: stranger, ArbitratorId: a1, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_PLAINTIFF,
	})
	if err == nil || !strings.Contains(err.Error(), "does not represent") {
		t.Fatalf("expected impersonation reject, got %v", err)
	}
}

func TestVoteAllowsDIDRotation(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// register stranger as an alt-controller of arb1's DID
	f.did.bind(arbDID1, stranger)
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: stranger, ArbitratorId: a1, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_PLAINTIFF,
	})
	if err != nil {
		t.Fatalf("DID-rotation vote: %v", err)
	}
}

func TestVoteDoubleVoteRejected(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: arbKey1, ArbitratorId: a1, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_RESPONDENT,
	})
	if err == nil || !strings.Contains(err.Error(), "already voted") {
		t.Fatalf("expected double-vote reject, got %v", err)
	}
}

func TestPlaintiffCanCancelWhileOpenRefundsBond(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	_, err := f.srv.CancelDispute(f.ctx, &types.MsgCancelDispute{
		Actor: alice, DisputeId: did, Reason: "wd",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("alice refund = %d", got)
	}
}

func TestPlaintiffCannotCancelAfterRespond(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	_, err := f.srv.CancelDispute(f.ctx, &types.MsgCancelDispute{
		Actor: alice, DisputeId: did, Reason: "wd",
	})
	if err == nil || !strings.Contains(err.Error(), "only cancel while OPEN") {
		t.Fatalf("expected post-respond reject, got %v", err)
	}
}

func TestAuthorityCancelRefundsBothBonds(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	_, err := f.srv.CancelDispute(f.ctx, &types.MsgCancelDispute{
		Actor: authority, DisputeId: did, Reason: "intervene",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("alice = %d", got)
	}
	if got := f.coin.balance(denomA, bob); got != 200 {
		t.Fatalf("bob = %d", got)
	}
}

func TestEndBlockAutoCancelsLapsedOpen(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.advanceTime(time.Duration(types.DefaultRespondPeriodSeconds+1) * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_CANCELLED {
		t.Fatalf("status = %s, want CANCELLED", d.Status)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("auto-refund = %d", got)
	}
}

func TestEndBlockAutoFinalizesLapsedDeliberating(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// no votes; jump past deliberation deadline
	f.advanceTime(time.Duration(types.DefaultDeliberationPeriodSeconds+1) * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_RESOLVED {
		t.Fatalf("status = %s, want RESOLVED", d.Status)
	}
	if d.Outcome != types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		t.Fatalf("outcome = %s", d.Outcome)
	}
	if got := f.coin.balance(denomA, alice); got != 100 {
		t.Fatalf("alice = %d", got)
	}
	if got := f.coin.balance(denomA, bob); got != 200 {
		t.Fatalf("bob = %d", got)
	}
}

func TestEvidenceCapEnforced(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MaxEvidencePerDispute = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	did := f.openDispute(t, alice, bob, 100, 100)
	for i := 0; i < 2; i++ {
		_, err := f.srv.SubmitEvidence(f.ctx, &types.MsgSubmitEvidence{
			Submitter: alice, DisputeId: did,
			Uri: "ipfs://e", Hash: strings.Repeat("c", 64),
		})
		if err != nil {
			t.Fatalf("evidence #%d: %v", i, err)
		}
	}
	_, err := f.srv.SubmitEvidence(f.ctx, &types.MsgSubmitEvidence{
		Submitter: alice, DisputeId: did,
		Uri: "ipfs://e3", Hash: strings.Repeat("d", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "max_evidence_per_dispute") {
		t.Fatalf("expected cap, got %v", err)
	}
}

func TestEvidenceRejectsStranger(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 100)
	_, err := f.srv.SubmitEvidence(f.ctx, &types.MsgSubmitEvidence{
		Submitter: stranger, DisputeId: did,
		Uri: "ipfs://e", Hash: strings.Repeat("c", 64),
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestEvidenceAllowsTribunalArbitrator(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	_, err := f.srv.SubmitEvidence(f.ctx, &types.MsgSubmitEvidence{
		Submitter: arbKey1, DisputeId: did,
		Uri: "ipfs://e", Hash: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatalf("tribunal evidence: %v", err)
	}
}

func TestAnyOpenDisputeForFlagsActive(t *testing.T) {
	f := setup(t)
	q := keeper.NewQueryServerImpl(f.k)
	did := f.openDispute(t, alice, bob, 100, 200)
	resp, err := q.AnyOpenDisputeFor(f.ctx, &types.QueryAnyOpenDisputeForRequest{
		SubjectKind: types.SubjectKind_SUBJECT_KIND_CONTRACT_SETTLEMENT,
		SubjectRef:  "subj-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Any || resp.DisputeId != did {
		t.Fatalf("got %+v", resp)
	}
	// cancel and re-check
	_, _ = f.srv.CancelDispute(f.ctx, &types.MsgCancelDispute{Actor: alice, DisputeId: did, Reason: "wd"})
	resp, err = q.AnyOpenDisputeFor(f.ctx, &types.QueryAnyOpenDisputeForRequest{
		SubjectKind: types.SubjectKind_SUBJECT_KIND_CONTRACT_SETTLEMENT,
		SubjectRef:  "subj-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Any {
		t.Fatalf("expected no active dispute after cancel")
	}
}

func TestRespondAfterDeadlineRejected(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	f.advanceTime(time.Duration(types.DefaultRespondPeriodSeconds+1) * time.Second)
	f.coin.credit(denomA, bob, 200)
	_, err := f.srv.Respond(f.ctx, &types.MsgRespond{Respondent: bob, DisputeId: did, Bond: 200})
	if err == nil || !strings.Contains(err.Error(), "respond_deadline passed") {
		t.Fatalf("expected deadline reject, got %v", err)
	}
}

func TestOpenDisputeBondBoundsEnforced(t *testing.T) {
	f := setup(t)
	p := types.DefaultParams()
	p.MinPlaintiffBond = 50
	p.MaxPlaintiffBond = 500
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.coin.credit(denomA, alice, 10)
	_, err := f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 10, RespondentBondRequired: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "< min") {
		t.Fatalf("expected min reject, got %v", err)
	}
	f.coin.credit(denomA, alice, 1000)
	_, err = f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 1010, RespondentBondRequired: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "> max") {
		t.Fatalf("expected max reject, got %v", err)
	}
	// respondent_bond_required also capped
	_, err = f.srv.OpenDispute(f.ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 100, RespondentBondRequired: 9999,
	})
	if err == nil || !strings.Contains(err.Error(), "respondent_bond_required") {
		t.Fatalf("expected respondent cap reject, got %v", err)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
	_, _ = f.srv.SubmitEvidence(f.ctx, &types.MsgSubmitEvidence{
		Submitter: alice, DisputeId: did,
		Uri: "ipfs://e", Hash: strings.Repeat("c", 64),
	})
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("validate exported genesis: %v", err)
	}
	// fresh keeper / state, import the export, and compare
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	k2 := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, nil, nil, nil, nil)
	ctx2 := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger())
	if err := k2.InitGenesis(ctx2, gs); err != nil {
		t.Fatal(err)
	}
	gs2, err := k2.ExportGenesis(ctx2)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Arbitrators) != len(gs2.Arbitrators) ||
		len(gs.Disputes) != len(gs2.Disputes) ||
		len(gs.Votes) != len(gs2.Votes) ||
		len(gs.TribunalMembers) != len(gs2.TribunalMembers) ||
		len(gs.Evidence) != len(gs2.Evidence) ||
		len(gs.EvidenceSeqs) != len(gs2.EvidenceSeqs) {
		t.Fatalf("genesis round-trip differs:\n  before: %+v\n  after:  %+v", gs, gs2)
	}
}

// TestPartyCannotVoteViaDIDRotation guards against the late-
// binding conflict-of-interest gap: at AssignTribunal time
// arbitrators are checked for COI on their on-file
// signer_address, but the DIDKeeper may later be rotated to
// list a party as a controller. The CastVote path explicitly
// re-checks the voter address against both parties so a
// rotation can never turn a party into an effective voter.
func TestPartyCannotVoteViaDIDRotation(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// rotate: bind plaintiff alice as controller of arbDID1
	f.did.bind(arbDID1, alice)
	_, err := f.srv.CastVote(f.ctx, &types.MsgCastVote{
		VoterAddress: alice, ArbitratorId: a1, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_PLAINTIFF,
	})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected COI rejection on party-as-voter, got %v", err)
	}
	// negative control: arbKey1 (the original arbitrator) can still vote
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_PLAINTIFF)
}

// TestEndBlockBestEffortRefundForfeitsBlockedRecipient guards
// the end-block DoS fix: if a plaintiff's account becomes
// blocked at the stablecoin layer between open and
// respond_deadline, auto-cancel MUST still resolve the
// dispute (forfeiting the bond to the pool) so a single
// blocked counterparty cannot exhaust the per-block
// finalization budget and lock the active-dispute table.
func TestEndBlockBestEffortRefundForfeitsBlockedRecipient(t *testing.T) {
	f := setup(t)
	did := f.openDispute(t, alice, bob, 100, 200)
	// Stablecoin admin blocks plaintiff after open.
	f.coin.block(denomA, alice)
	f.advanceTime(time.Duration(types.DefaultRespondPeriodSeconds+1) * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_CANCELLED {
		t.Fatalf("status = %s, want CANCELLED (forfeit must not stall)", d.Status)
	}
	// Bond stays in the pool, not refunded to the blocked party.
	if got := f.coin.balance(denomA, alice); got != 0 {
		t.Fatalf("blocked alice should NOT receive refund, got %d", got)
	}
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 100 {
		t.Fatalf("pool retains forfeited bond = %d, want 100", got)
	}
}

// TestEndBlockBestEffortFinalizeForfeitsBlockedRecipient is
// the same guarantee for the DELIBERATING → RESOLVED path.
func TestEndBlockBestEffortFinalizeForfeitsBlockedRecipient(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// Both parties get blocked between deliberation and sweep.
	f.coin.block(denomA, alice)
	f.coin.block(denomA, bob)
	f.advanceTime(time.Duration(types.DefaultDeliberationPeriodSeconds+1) * time.Second)
	if err := f.k.EndBlock(f.ctx); err != nil {
		t.Fatal(err)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.Status != types.DisputeStatus_DISPUTE_STATUS_RESOLVED {
		t.Fatalf("status = %s, want RESOLVED", d.Status)
	}
	if got := f.coin.balance(denomA, alice); got != 0 {
		t.Fatalf("blocked alice should NOT receive refund, got %d", got)
	}
	if got := f.coin.balance(denomA, bob); got != 0 {
		t.Fatalf("blocked bob should NOT receive refund, got %d", got)
	}
	if got := f.coin.balance(denomA, keeper.DisputePoolAccount); got != 300 {
		t.Fatalf("pool retains forfeited bonds = %d, want 300", got)
	}
}

// TestFinalizeNoFaultNormalizesSlashBpsToZero protects the
// invariant "outcome == NO_FAULT ⇒ slash_bps == 0" even when
// the authority's override is non-zero. Off-chain consumers
// rely on this to summarize disputes.
func TestFinalizeNoFaultNormalizesSlashBpsToZero(t *testing.T) {
	f := setup(t)
	a1 := f.registerArb(t, arbKey1, arbDID1, "A1")
	a2 := f.registerArb(t, arbKey2, arbDID2, "A2")
	a3 := f.registerArb(t, arbKey3, arbDID3, "A3")
	did := f.openDispute(t, alice, bob, 100, 200)
	f.respond(t, bob, did, 200)
	f.assignTribunal(t, did, a1, a2, a3)
	// tribunal returns NO_FAULT unanimously; authority sets a
	// non-zero override which must be ignored on NO_FAULT
	f.castVote(t, arbKey1, a1, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	f.castVote(t, arbKey2, a2, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	f.castVote(t, arbKey3, a3, did, types.VoteChoice_VOTE_CHOICE_NO_FAULT)
	out := f.finalize(t, authority, did, 5000)
	if out != types.RulingOutcome_RULING_OUTCOME_NO_FAULT {
		t.Fatalf("outcome = %s", out)
	}
	d, _ := f.k.MustGetDispute(f.ctx, did)
	if d.SlashBps != 0 {
		t.Fatalf("NO_FAULT must zero slash_bps, got %d", d.SlashBps)
	}
}

func TestUnknownDIDKeeperFailsClosed(t *testing.T) {
	// Build a fixture with no DIDKeeper to assert the
	// fail-closed signer rule applies even when no DID
	// rotation surface is wired.
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger())
	cms.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	if err := cms.LoadLatestVersion(); err != nil {
		t.Fatal(err)
	}
	registry := cdctypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)
	coin := newStubStablecoin()
	coin.denoms[denomA] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, nil, coin, nil /* DID */, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	srv := keeper.NewMsgServerImpl(k)
	_, err := srv.RegisterArbitrator(ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: arbDID1, SignerAddress: arbKey1, Name: "A1",
		Jurisdictions: []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.RegisterArbitrator(ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: arbDID2, SignerAddress: arbKey2, Name: "A2",
		Jurisdictions: []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.RegisterArbitrator(ctx, &types.MsgRegisterArbitrator{
		Authority: authority, Did: arbDID3, SignerAddress: arbKey3, Name: "A3",
		Jurisdictions: []string{"GLOBAL"},
	})
	if err != nil {
		t.Fatal(err)
	}
	coin.credit(denomA, alice, 100)
	openResp, err := srv.OpenDispute(ctx, &types.MsgOpenDispute{
		Plaintiff: alice, Respondent: bob,
		SubjectKind: types.SubjectKind_SUBJECT_KIND_OTHER, SubjectRef: "x",
		BondDenom: denomA, PlaintiffBond: 100, RespondentBondRequired: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	did := openResp.DisputeId
	coin.credit(denomA, bob, 100)
	_, err = srv.Respond(ctx, &types.MsgRespond{Respondent: bob, DisputeId: did, Bond: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.AssignTribunal(ctx, &types.MsgAssignTribunal{
		Authority: authority, DisputeId: did, ArbitratorIds: []uint64{1, 2, 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.CastVote(ctx, &types.MsgCastVote{
		VoterAddress: stranger, ArbitratorId: 1, DisputeId: did,
		Choice: types.VoteChoice_VOTE_CHOICE_PLAINTIFF,
	})
	if err == nil || !strings.Contains(err.Error(), "does not represent") {
		t.Fatalf("expected impersonation reject, got %v", err)
	}
}
