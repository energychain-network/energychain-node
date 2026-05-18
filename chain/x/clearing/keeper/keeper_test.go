package keeper_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/log/v2"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/clearing/keeper"
	"energychain/x/clearing/types"
)

// ---- Stubs --------------------------------------------------------------

type stubSanctions struct{ bad map[string]bool }

func (s *stubSanctions) IsSanctioned(_ sdk.Context, who string) bool { return s.bad[who] }

type stubStablecoin struct {
	denoms      map[string]bool
	bal         map[string]map[string]uint64
	pausedDenom map[string]bool
	blocked     map[string]map[string]bool
}

func newStubStablecoin() *stubStablecoin {
	return &stubStablecoin{
		denoms:      map[string]bool{},
		bal:         map[string]map[string]uint64{},
		pausedDenom: map[string]bool{},
		blocked:     map[string]map[string]bool{},
	}
}
func (s *stubStablecoin) HasDenom(_ sdk.Context, d string) bool      { return s.denoms[d] }
func (s *stubStablecoin) IsDenomPaused(_ sdk.Context, d string) bool { return s.pausedDenom[d] }
func (s *stubStablecoin) IsAccountBlocked(_ sdk.Context, d, acc string) bool {
	if _, ok := s.blocked[d]; !ok {
		return false
	}
	return s.blocked[d][acc]
}
func (s *stubStablecoin) credit(d, acc string, n uint64) {
	if _, ok := s.bal[d]; !ok {
		s.bal[d] = map[string]uint64{}
	}
	s.bal[d][acc] += n
}
func (s *stubStablecoin) get(d, acc string) uint64 {
	if _, ok := s.bal[d]; !ok {
		return 0
	}
	return s.bal[d][acc]
}
func (s *stubStablecoin) Move(_ sdk.Context, d, from, to string, amount uint64) error {
	if !s.denoms[d] {
		return fmt.Errorf("denom %s missing", d)
	}
	cur := s.get(d, from)
	if cur < amount {
		return fmt.Errorf("insufficient %s: %d < %d", d, cur, amount)
	}
	s.bal[d][from] = cur - amount
	if s.bal[d][from] == 0 {
		delete(s.bal[d], from)
	}
	s.credit(d, to, amount)
	return nil
}

// ---- Setup --------------------------------------------------------------

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
	carol     = mkAddr(4)
	dave      = mkAddr(5)
	stranger  = mkAddr(6)
	usd       = "usd"
	eur       = "eur"
)

type fixture struct {
	k   keeper.Keeper
	ctx sdk.Context
	srv types.MsgServer
	san *stubSanctions
	sc  *stubStablecoin
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
	sc := newStubStablecoin()
	sc.denoms[usd] = true
	sc.denoms[eur] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, sc: sc}
}

// ---- helpers ------------------------------------------------------------

func (f *fixture) registerMember(t *testing.T, addr string, requiredMargin map[string]uint64) uint64 {
	t.Helper()
	r, err := f.srv.RegisterMember(f.ctx, &types.MsgRegisterMember{
		Authority:      authority,
		MemberAddress:  addr,
		RequiredMargin: requiredMargin,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return r.MemberId
}

func (f *fixture) postMargin(t *testing.T, id uint64, addr, denom string, amount uint64) {
	t.Helper()
	f.sc.credit(denom, addr, amount)
	if _, err := f.srv.PostMargin(f.ctx, &types.MsgPostMargin{
		MemberAddress: addr, MemberId: id, Denom: denom, Amount: amount,
	}); err != nil {
		t.Fatalf("post-margin: %v", err)
	}
}

func (f *fixture) openCycle(t *testing.T) uint64 {
	t.Helper()
	r, err := f.srv.OpenCycle(f.ctx, &types.MsgOpenCycle{Authority: authority})
	if err != nil {
		t.Fatalf("open cycle: %v", err)
	}
	return r.CycleId
}

func (f *fixture) submit(t *testing.T, submitter string, cycleID, fromID, toID uint64, denom string, amount uint64) {
	t.Helper()
	if _, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: submitter, CycleId: cycleID,
		FromMemberId: fromID, ToMemberId: toID,
		Denom: denom, Amount: amount,
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
}

func (f *fixture) close(t *testing.T, cycleID uint64) {
	t.Helper()
	if _, err := f.srv.CloseCycle(f.ctx, &types.MsgCloseCycle{Authority: authority, CycleId: cycleID}); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func (f *fixture) settle(t *testing.T, cycleID uint64) *types.MsgSettleCycleResponse {
	t.Helper()
	r, err := f.srv.SettleCycle(f.ctx, &types.MsgSettleCycle{Authority: authority, CycleId: cycleID})
	if err != nil {
		t.Fatalf("settle: %v", err)
	}
	return r
}

// forceMargin overwrites a member's margin balance directly,
// bypassing the reservation gate. Tests use this to simulate
// pathological post-submit margin loss (e.g., an off-chain
// emergency or an unforeseen state machine bug) so the
// default waterfall can be exercised end-to-end. Real msg
// flow keeps margin ≥ reservation by construction.
func (f *fixture) forceMargin(t *testing.T, memberID uint64, denom string, amount uint64) {
	t.Helper()
	if amount == 0 {
		if err := f.k.Margin.Remove(f.ctx, collections.Join(memberID, denom)); err != nil {
			t.Fatalf("force-margin remove: %v", err)
		}
		return
	}
	if err := f.k.Margin.Set(f.ctx, collections.Join(memberID, denom), amount); err != nil {
		t.Fatalf("force-margin set: %v", err)
	}
}

// ---- tests --------------------------------------------------------------

func TestRegisterMemberHappyPath(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, map[string]uint64{usd: 10})
	if id != 1 {
		t.Fatalf("expected id 1, got %d", id)
	}
	m, _ := f.k.MustGetMember(f.ctx, id)
	if m.Address != alice || m.Status != types.MemberStatus_MEMBER_STATUS_ACTIVE {
		t.Fatalf("member shape wrong: %+v", m)
	}
}

func TestRegisterMemberRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.RegisterMember(f.ctx, &types.MsgRegisterMember{
		Authority:     stranger,
		MemberAddress: alice,
	})
	if err == nil {
		t.Fatal("expected authority error")
	}
}

func TestRegisterMemberRefusesDoubleRegistration(t *testing.T) {
	f := setup(t)
	f.registerMember(t, alice, nil)
	_, err := f.srv.RegisterMember(f.ctx, &types.MsgRegisterMember{
		Authority: authority, MemberAddress: alice,
	})
	if err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected duplicate refusal, got %v", err)
	}
}

func TestRegisterMemberRefusesSanctioned(t *testing.T) {
	f := setup(t)
	f.san.bad[alice] = true
	_, err := f.srv.RegisterMember(f.ctx, &types.MsgRegisterMember{
		Authority: authority, MemberAddress: alice,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned refusal, got %v", err)
	}
}

func TestPostMarginAccumulates(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, nil)
	f.postMargin(t, id, alice, usd, 100)
	f.postMargin(t, id, alice, usd, 50)
	bal, _ := f.k.GetMargin(f.ctx, id, usd)
	if bal != 150 {
		t.Fatalf("expected 150, got %d", bal)
	}
	if f.sc.get(usd, keeper.PoolAddress()) != 150 {
		t.Fatal("pool not funded")
	}
}

func TestPostMarginRefusesAddressMismatch(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, nil)
	f.sc.credit(usd, bob, 100)
	_, err := f.srv.PostMargin(f.ctx, &types.MsgPostMargin{
		MemberAddress: bob, MemberId: id, Denom: usd, Amount: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected address mismatch, got %v", err)
	}
}

func TestWithdrawMarginRespectsFloor(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, map[string]uint64{usd: 50})
	f.postMargin(t, id, alice, usd, 100)
	// Withdraw 60 would leave 40 < required 50.
	_, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: id, Denom: usd, Amount: 60,
	})
	if err == nil || !strings.Contains(err.Error(), "required_margin") {
		t.Fatalf("expected floor refusal, got %v", err)
	}
	// Withdraw 50 leaves exactly 50 — allowed.
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: id, Denom: usd, Amount: 50,
	}); err != nil {
		t.Fatal(err)
	}
	if f.sc.get(usd, alice) != 50 {
		t.Fatalf("alice balance: %d", f.sc.get(usd, alice))
	}
}

func TestFundDefaultFundCreditsShareAndTotal(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, nil)
	f.sc.credit(usd, alice, 100)
	if _, err := f.srv.FundDefaultFund(f.ctx, &types.MsgFundDefaultFund{
		MemberAddress: alice, MemberId: id, Denom: usd, Amount: 100,
	}); err != nil {
		t.Fatal(err)
	}
	mem, _ := f.k.MustGetMember(f.ctx, id)
	if mem.DefaultFundShare[usd] != 100 {
		t.Fatalf("share: %d", mem.DefaultFundShare[usd])
	}
	tot, _ := f.k.GetDefaultFund(f.ctx, usd)
	if tot != 100 {
		t.Fatalf("fund total: %d", tot)
	}
}

func TestOpenCycleRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.OpenCycle(f.ctx, &types.MsgOpenCycle{Authority: stranger})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestSubmitObligationGate(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	// Both must have margin: submitting an obligation reserves
	// the gross outbound amount against the from-side margin.
	f.postMargin(t, a, alice, usd, 100)
	f.postMargin(t, b, bob, usd, 100)
	c := f.openCycle(t)
	// Third party may not submit a→b.
	_, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: stranger, CycleId: c,
		FromMemberId: a, ToMemberId: b, Denom: usd, Amount: 10,
	})
	if err == nil {
		t.Fatal("expected third-party refusal")
	}
	// From-member can submit.
	f.submit(t, alice, c, a, b, usd, 10)
	// Authority can submit on behalf of any member.
	f.submit(t, authority, c, b, a, usd, 5)
}

func TestSubmitRefusesSelfLeg(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	c := f.openCycle(t)
	_, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: alice, CycleId: c,
		FromMemberId: a, ToMemberId: a, Denom: usd, Amount: 10,
	})
	if err == nil {
		t.Fatal("expected self-leg refusal")
	}
}

func TestSubmitRefusesOnClosedCycle(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	c := f.openCycle(t)
	f.close(t, c)
	_, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: alice, CycleId: c, FromMemberId: a, ToMemberId: b,
		Denom: usd, Amount: 1,
	})
	if err == nil {
		t.Fatal("expected closed-cycle refusal")
	}
}

// ---- core netting + settlement scenarios -------------------------------

func TestSimpleBilateralNetting(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	f.postMargin(t, b, bob, usd, 100)
	c := f.openCycle(t)
	// Alice owes Bob 60; Bob owes Alice 40. Net: Alice → Bob 20.
	f.submit(t, alice, c, a, b, usd, 60)
	f.submit(t, bob, c, b, a, usd, 40)
	f.close(t, c)
	r := f.settle(t, c)
	if r.HasDefaults {
		t.Fatalf("unexpected defaults: %+v", r)
	}
	cyc, _ := f.k.MustGetCycle(f.ctx, c)
	if cyc.Status != types.CycleStatus_CYCLE_STATUS_SETTLED {
		t.Fatalf("status: %s", cyc.Status)
	}
	// Alice's margin should have dropped by 20 to 80.
	if got, _ := f.k.GetMargin(f.ctx, a, usd); got != 80 {
		t.Fatalf("alice margin: %d", got)
	}
	// Bob's margin untouched (he was net receiver) — at 100.
	if got, _ := f.k.GetMargin(f.ctx, b, usd); got != 100 {
		t.Fatalf("bob margin: %d", got)
	}
	// Bob received 20 USD.
	if f.sc.get(usd, bob) != 20 {
		t.Fatalf("bob payout: %d", f.sc.get(usd, bob))
	}
}

func TestThreeWayMultilateralNetting(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	cc := f.registerMember(t, carol, nil)
	f.postMargin(t, a, alice, usd, 1000)
	f.postMargin(t, b, bob, usd, 1000)
	f.postMargin(t, cc, carol, usd, 1000)
	cyc := f.openCycle(t)
	// Circular: a→b 30, b→c 30, c→a 30. All net to zero.
	f.submit(t, alice, cyc, a, b, usd, 30)
	f.submit(t, bob, cyc, b, cc, usd, 30)
	f.submit(t, carol, cyc, cc, a, usd, 30)
	f.close(t, cyc)
	r := f.settle(t, cyc)
	if r.HasDefaults {
		t.Fatal("unexpected defaults")
	}
	// Nobody's margin should have moved.
	for _, id := range []uint64{a, b, cc} {
		if got, _ := f.k.GetMargin(f.ctx, id, usd); got != 1000 {
			t.Fatalf("member %d margin moved: %d", id, got)
		}
	}
}

func TestDefaultWaterfallDrawsFromFundOnShortfall(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	// Alice posts enough margin to clear the reservation
	// gate at submit time (30).
	f.postMargin(t, a, alice, usd, 30)
	f.postMargin(t, b, bob, usd, 100)
	// Default fund is funded with 50 USD by bob.
	f.sc.credit(usd, bob, 50)
	if _, err := f.srv.FundDefaultFund(f.ctx, &types.MsgFundDefaultFund{
		MemberAddress: bob, MemberId: b, Denom: usd, Amount: 50,
	}); err != nil {
		t.Fatal(err)
	}
	c := f.openCycle(t)
	// Alice owes Bob 30 (covered at submit time).
	f.submit(t, alice, c, a, b, usd, 30)
	// Simulate pathological post-submit margin loss (e.g.,
	// an off-chain emergency) by draining Alice's margin
	// to 10. The waterfall must now cover the 20 shortfall.
	f.forceMargin(t, a, usd, 10)
	f.close(t, c)
	r := f.settle(t, c)
	if r.HasDefaults {
		t.Fatalf("default flag should be false: fund covers shortfall fully (events=%d)", r.DefaultEventsCount)
	}
	if r.DefaultEventsCount == 0 {
		t.Fatalf("expected a default event for the fund draw")
	}
	cyc, _ := f.k.MustGetCycle(f.ctx, c)
	if cyc.Status != types.CycleStatus_CYCLE_STATUS_SETTLED {
		t.Fatalf("status should be SETTLED (fund covered): %s", cyc.Status)
	}
	if got, _ := f.k.GetDefaultFund(f.ctx, usd); got != 30 {
		t.Fatalf("fund residue: %d (expected 30)", got)
	}
	if got, _ := f.k.GetMargin(f.ctx, a, usd); got != 0 {
		t.Fatalf("alice margin: %d", got)
	}
	if f.sc.get(usd, bob) != 30 {
		t.Fatalf("bob payout: %d", f.sc.get(usd, bob))
	}
}

func TestDefaultWaterfallProRataHaircutWhenFundExhausted(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	cc := f.registerMember(t, carol, nil)
	// Alice posts 20 to cover the two outbound legs at submit
	// time; the waterfall test then drains her to 5 to
	// simulate post-submit margin loss.
	f.postMargin(t, a, alice, usd, 20)
	f.postMargin(t, b, bob, usd, 100)
	f.postMargin(t, cc, carol, usd, 100)
	c := f.openCycle(t)
	// Alice owes 10 to bob and 10 to carol (each backed at
	// submit time).
	f.submit(t, alice, c, a, b, usd, 10)
	f.submit(t, alice, c, a, cc, usd, 10)
	// Margin drained to 5 between submit and settle.
	f.forceMargin(t, a, usd, 5)
	f.close(t, c)
	r := f.settle(t, c)
	if !r.HasDefaults {
		t.Fatal("expected has_defaults")
	}
	cyc, _ := f.k.MustGetCycle(f.ctx, c)
	if cyc.Status != types.CycleStatus_CYCLE_STATUS_DEFAULTED {
		t.Fatalf("status: %s", cyc.Status)
	}
	// 5 total available, 20 owed → 25% haircut. Each receiver
	// gets 10 * 5 / 20 = 2 (integer divide).
	if f.sc.get(usd, bob) != 2 {
		t.Fatalf("bob: %d", f.sc.get(usd, bob))
	}
	if f.sc.get(usd, carol) != 2 {
		t.Fatalf("carol: %d", f.sc.get(usd, carol))
	}
	if got, _ := f.k.GetMargin(f.ctx, a, usd); got != 0 {
		t.Fatalf("alice margin: %d", got)
	}
}

func TestCancelCycleClearsObligations(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 10)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 5)
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 5 {
		t.Fatalf("pre-cancel reservation: %d", got)
	}
	if _, err := f.srv.CancelCycle(f.ctx, &types.MsgCancelCycle{
		Authority: authority, CycleId: c, Reason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	cyc, _ := f.k.MustGetCycle(f.ctx, c)
	if cyc.Status != types.CycleStatus_CYCLE_STATUS_CANCELLED {
		t.Fatalf("status: %s", cyc.Status)
	}
	// Reservation must be released on cancel.
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 0 {
		t.Fatalf("post-cancel reservation should be 0, got %d", got)
	}
}

func TestCancelRefusesTerminal(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 5)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 5)
	f.close(t, c)
	f.settle(t, c)
	_, err := f.srv.CancelCycle(f.ctx, &types.MsgCancelCycle{
		Authority: authority, CycleId: c, Reason: "x",
	})
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal refusal, got %v", err)
	}
}

func TestSuspendedMemberCannotInitiateOutbound(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	// Pre-fund alice's margin BEFORE suspension, since
	// suspended members can no longer post margin.
	f.postMargin(t, a, alice, usd, 50)
	if _, err := f.srv.UpdateMemberStatus(f.ctx, &types.MsgUpdateMemberStatus{
		Authority: authority, MemberId: a,
		NewStatus: types.MemberStatus_MEMBER_STATUS_SUSPENDED,
	}); err != nil {
		t.Fatal(err)
	}
	c := f.openCycle(t)
	// Alice (suspended) cannot self-submit.
	_, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: alice, CycleId: c,
		FromMemberId: a, ToMemberId: b, Denom: usd, Amount: 10,
	})
	if err == nil || !strings.Contains(err.Error(), "suspended") {
		t.Fatalf("expected suspended err, got %v", err)
	}
	// Authority CAN still submit on alice's behalf (close-out).
	f.submit(t, authority, c, a, b, usd, 10)
}

func TestExpelledMemberCannotPostMargin(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	if _, err := f.srv.UpdateMemberStatus(f.ctx, &types.MsgUpdateMemberStatus{
		Authority: authority, MemberId: a,
		NewStatus: types.MemberStatus_MEMBER_STATUS_EXPELLED,
	}); err != nil {
		t.Fatal(err)
	}
	f.sc.credit(usd, alice, 100)
	_, err := f.srv.PostMargin(f.ctx, &types.MsgPostMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "expelled") {
		t.Fatalf("expected expel refusal, got %v", err)
	}
}

func TestExpelledMemberCanWithdrawBelowFloor(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, map[string]uint64{usd: 50})
	f.postMargin(t, a, alice, usd, 100)
	if _, err := f.srv.UpdateMemberStatus(f.ctx, &types.MsgUpdateMemberStatus{
		Authority: authority, MemberId: a,
		NewStatus: types.MemberStatus_MEMBER_STATUS_EXPELLED,
	}); err != nil {
		t.Fatal(err)
	}
	// Even though required_margin is 50, expelled members can drain
	// (no outstanding obligations to back).
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 100,
	}); err != nil {
		t.Fatalf("expelled withdraw: %v", err)
	}
	if f.sc.get(usd, alice) != 100 {
		t.Fatalf("alice got: %d", f.sc.get(usd, alice))
	}
}

// REGRESSION: an EXPELLED member must NOT be able to drain
// margin below their outstanding reservation. The expulsion
// override only bypasses the static required_margin floor;
// active obligations to counterparties continue to lock margin.
func TestExpelledMemberCannotBypassReservation(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, map[string]uint64{usd: 50})
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 60)
	// Authority expels alice mid-flight.
	if _, err := f.srv.UpdateMemberStatus(f.ctx, &types.MsgUpdateMemberStatus{
		Authority: authority, MemberId: a,
		NewStatus: types.MemberStatus_MEMBER_STATUS_EXPELLED,
	}); err != nil {
		t.Fatal(err)
	}
	// Alice still has 100 margin, reservation 60 → withdraw of
	// 50 leaves 50 < reservation 60 → must refuse.
	_, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 50,
	})
	if err == nil || !strings.Contains(err.Error(), "reservation") {
		t.Fatalf("expected reservation refusal even when expelled, got %v", err)
	}
	// Withdrawal of 40 (leaves 60 == reservation) is allowed.
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 40,
	}); err != nil {
		t.Fatalf("expelled withdraw to floor: %v", err)
	}
}

// REGRESSION: the original "submit-then-withdraw" underwriting
// leak. Without reservation, alice could submit then yank
// margin and force the default fund to absorb the loss. With
// reservation, the withdraw must be refused.
func TestSubmitReservesMarginAtSubmitTime(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 60)
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 60 {
		t.Fatalf("reservation should be 60, got %d", got)
	}
	// Withdraw of 50 would leave 50 < reservation 60.
	_, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 50,
	})
	if err == nil || !strings.Contains(err.Error(), "reservation") {
		t.Fatalf("expected reservation refusal, got %v", err)
	}
	// Withdraw of 40 leaves exactly the reservation; OK.
	if _, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 40,
	}); err != nil {
		t.Fatalf("withdraw to reservation floor: %v", err)
	}
}

// REGRESSION: reservation must be released exactly on settle.
// If we under-release we'll lock margin permanently; if we
// over-release the next submit can over-commit margin.
func TestSettleReleasesReservationExactly(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	f.postMargin(t, b, bob, usd, 100)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 40)
	f.submit(t, bob, c, b, a, usd, 10)
	f.close(t, c)
	f.settle(t, c)
	for _, id := range []uint64{a, b} {
		if got, _ := f.k.GetReservation(f.ctx, id, usd); got != 0 {
			t.Fatalf("post-settle reservation member %d: %d", id, got)
		}
	}
}

// REGRESSION: cross-cycle reservation accounting. A member
// who has obligations in two cycles must reserve the gross
// total. Settling one cycle must only release that cycle's
// portion.
func TestReservationAggregatesAcrossCycles(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	f.postMargin(t, b, bob, usd, 100)
	c1 := f.openCycle(t)
	c2 := f.openCycle(t)
	f.submit(t, alice, c1, a, b, usd, 30)
	f.submit(t, alice, c2, a, b, usd, 40)
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 70 {
		t.Fatalf("aggregate reservation: %d", got)
	}
	// Settle cycle 1 only; reservation should drop to 40.
	f.close(t, c1)
	f.settle(t, c1)
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 40 {
		t.Fatalf("post-c1 reservation: %d", got)
	}
	// Alice now has 100 - 30 = 70 margin and 40 reservation.
	// Withdraw of 31 (leaves 39 < 40) must refuse.
	_, err := f.srv.WithdrawMargin(f.ctx, &types.MsgWithdrawMargin{
		MemberAddress: alice, MemberId: a, Denom: usd, Amount: 31,
	})
	if err == nil || !strings.Contains(err.Error(), "reservation") {
		t.Fatalf("expected refusal: %v", err)
	}
	// Settle cycle 2; reservation drops to 0.
	f.close(t, c2)
	f.settle(t, c2)
	if got, _ := f.k.GetReservation(f.ctx, a, usd); got != 0 {
		t.Fatalf("post-c2 reservation: %d", got)
	}
}

func TestSettleRefusesUnclosedCycle(t *testing.T) {
	f := setup(t)
	c := f.openCycle(t)
	_, err := f.srv.SettleCycle(f.ctx, &types.MsgSettleCycle{Authority: authority, CycleId: c})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestSanctionedReceiverAccruesUncovered(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 100)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 50)
	f.close(t, c)
	// Bob added to OFAC mid-flight.
	f.san.bad[bob] = true
	r := f.settle(t, c)
	if !r.HasDefaults {
		t.Fatal("expected has_defaults due to sanctioned receiver")
	}
	cyc, _ := f.k.MustGetCycle(f.ctx, c)
	if cyc.Status != types.CycleStatus_CYCLE_STATUS_DEFAULTED {
		t.Fatalf("status: %s", cyc.Status)
	}
	if f.sc.get(usd, bob) != 0 {
		t.Fatalf("bob should not have been paid: %d", f.sc.get(usd, bob))
	}
	// Alice's margin still drained — that's the design.
	if got, _ := f.k.GetMargin(f.ctx, a, usd); got != 50 {
		t.Fatalf("alice margin: %d", got)
	}
}

func TestMaxOpenCyclesCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxOpenCycles = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	f.openCycle(t)
	_, err := f.srv.OpenCycle(f.ctx, &types.MsgOpenCycle{Authority: authority})
	if err == nil || !strings.Contains(err.Error(), "max_open_cycles") {
		t.Fatalf("expected cap refusal, got %v", err)
	}
}

func TestMaxObligationsCap(t *testing.T) {
	f := setup(t)
	p, _ := f.k.GetParams(f.ctx)
	p.MaxObligationsPerCycle = 1
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 10)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 1)
	_, err := f.srv.SubmitObligation(f.ctx, &types.MsgSubmitObligation{
		Submitter: alice, CycleId: c, FromMemberId: a, ToMemberId: b, Denom: usd, Amount: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("expected cap refusal, got %v", err)
	}
}

func TestQueriesReturnCorrectShape(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, nil)
	b := f.registerMember(t, bob, nil)
	f.postMargin(t, a, alice, usd, 50)
	c := f.openCycle(t)
	f.submit(t, alice, c, a, b, usd, 30)
	f.close(t, c)
	f.settle(t, c)

	qs := keeper.NewQueryServerImpl(f.k)
	if r, err := qs.Params(f.ctx, &types.QueryParamsRequest{}); err != nil || r.Params.MaxMembers == 0 {
		t.Fatalf("params: %v %v", r, err)
	}
	if r, err := qs.Member(f.ctx, &types.QueryMemberRequest{Id: a}); err != nil || r.Member.Id != a {
		t.Fatalf("member: %v %v", r, err)
	}
	if r, err := qs.Cycle(f.ctx, &types.QueryCycleRequest{Id: c}); err != nil || r.Cycle.Id != c {
		t.Fatalf("cycle: %v %v", r, err)
	}
	if r, err := qs.Obligations(f.ctx, &types.QueryObligationsRequest{CycleId: c}); err != nil || len(r.Obligations) != 1 {
		t.Fatalf("obligations: %v %v", r, err)
	}
	if r, err := qs.NetPositions(f.ctx, &types.QueryNetPositionsRequest{CycleId: c}); err != nil || len(r.Positions) != 2 {
		t.Fatalf("net: %v %v", r, err)
	}
	if r, err := qs.MarginBalance(f.ctx, &types.QueryMarginBalanceRequest{MemberId: a, Denom: usd}); err != nil || r.Balance != 20 {
		t.Fatalf("margin: %v %v", r, err)
	}
	if r, err := qs.PoolAddress(f.ctx, &types.QueryPoolAddressRequest{}); err != nil || r.Pool != keeper.PoolAddress() {
		t.Fatalf("pool: %v %v", r, err)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	a := f.registerMember(t, alice, map[string]uint64{usd: 10})
	f.postMargin(t, a, alice, usd, 50)
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Members) != 1 {
		t.Fatalf("export: %+v", gs)
	}
	f2 := setup(t)
	if err := f2.k.InitGenesis(f2.ctx, gs); err != nil {
		t.Fatalf("import: %v", err)
	}
	m2, _ := f2.k.MustGetMember(f2.ctx, a)
	if m2.Address != alice {
		t.Fatalf("member missing after import: %+v", m2)
	}
}

func TestUpdateParamsRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: stranger, Params: types.DefaultParams(),
	})
	if err == nil {
		t.Fatal("expected refusal")
	}
}

func TestStablecoinFreezeBlocksMargin(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, nil)
	f.sc.blocked[usd] = map[string]bool{alice: true}
	f.sc.credit(usd, alice, 100)
	_, err := f.srv.PostMargin(f.ctx, &types.MsgPostMargin{
		MemberAddress: alice, MemberId: id, Denom: usd, Amount: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected freeze, got %v", err)
	}
}

func TestPausedDenomBlocksMargin(t *testing.T) {
	f := setup(t)
	id := f.registerMember(t, alice, nil)
	f.sc.pausedDenom[usd] = true
	f.sc.credit(usd, alice, 100)
	_, err := f.srv.PostMargin(f.ctx, &types.MsgPostMargin{
		MemberAddress: alice, MemberId: id, Denom: usd, Amount: 100,
	})
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("expected paused refusal, got %v", err)
	}
}
