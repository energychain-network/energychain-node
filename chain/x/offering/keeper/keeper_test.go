package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/offering/keeper"
	"energychain/x/offering/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	issuer    = testutil.DeriveAddr("issuer")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")
	stranger  = testutil.DeriveAddr("stranger")

	denom = "weusd"
)

const (
	tokenID   uint64 = 1
	unitPrice uint64 = 100
)

// ---- mocks -----------------------------------------------------------------

type mockSettlement struct {
	denoms map[string]bool
	bal    map[string]uint64
}

func newMockSettlement() *mockSettlement {
	return &mockSettlement{denoms: map[string]bool{}, bal: map[string]uint64{}}
}
func (m *mockSettlement) key(d, a string) string                    { return d + "|" + a }
func (m *mockSettlement) HasDenom(_ context.Context, d string) bool { return m.denoms[d] }
func (m *mockSettlement) GetBalance(_ context.Context, d, a string) uint64 {
	return m.bal[m.key(d, a)]
}
func (m *mockSettlement) MoveBalance(_ context.Context, d, from, to string, amt uint64) error {
	if !m.denoms[d] {
		return types.ErrSettlement.Wrapf("unknown denom %q", d)
	}
	if m.bal[m.key(d, from)] < amt {
		return types.ErrSettlement.Wrapf("insufficient %s for %s", d, from)
	}
	m.bal[m.key(d, from)] -= amt
	m.bal[m.key(d, to)] += amt
	return nil
}
func (m *mockSettlement) fund(d, a string, amt uint64) { m.bal[m.key(d, a)] += amt }

// total returns the sum of all balances for a denom (settlement is a closed
// system in the mock, so this is the conservation invariant).
func (m *mockSettlement) total(d string) uint64 {
	var sum uint64
	for k, v := range m.bal {
		if len(k) > len(d) && k[:len(d)+1] == d+"|" {
			sum += v
		}
	}
	return sum
}

type mockRWA struct {
	admins   map[uint64]string
	denoms   map[uint64]string
	kyc      map[uint64]bool // tokenID -> require_kyc
	minted   map[string]uint64 // tokenID|recipient -> units
	allocErr error
}

func newMockRWA() *mockRWA {
	return &mockRWA{
		admins: map[uint64]string{tokenID: issuer},
		denoms: map[uint64]string{tokenID: denom},
		kyc:    map[uint64]bool{},
		minted: map[string]uint64{},
	}
}
func (m *mockRWA) TokenAdmin(_ context.Context, id uint64) (string, bool) {
	a, ok := m.admins[id]
	return a, ok
}
func (m *mockRWA) TokenSettlementDenom(_ context.Context, id uint64) (string, bool) {
	d, ok := m.denoms[id]
	return d, ok
}
func (m *mockRWA) TokenRequiresKYC(_ context.Context, id uint64) (bool, bool) {
	if _, ok := m.admins[id]; !ok {
		return false, false
	}
	return m.kyc[id], true
}
func (m *mockRWA) AllocateUnits(_ context.Context, id uint64, caller, recipient string, amount uint64) error {
	if m.allocErr != nil {
		return m.allocErr
	}
	if m.admins[id] != caller {
		return types.ErrUnauthorized.Wrap("not admin")
	}
	m.minted[key(id, recipient)] += amount
	return nil
}

func key(id uint64, a string) string { return string(rune(id)) + "|" + a }

// mockCompliance lets tests flip sanctions / KYC state for the investor
// gates added to Subscribe / ClaimReturns / ClaimRefund.
type mockCompliance struct {
	sanctioned map[string]bool
	noKyc      map[string]bool // addresses that FAIL the KYC gate
}

func newMockCompliance() *mockCompliance {
	return &mockCompliance{sanctioned: map[string]bool{}, noKyc: map[string]bool{}}
}
func (m *mockCompliance) IsSanctioned(_ sdk.Context, a string) bool { return m.sanctioned[a] }
func (m *mockCompliance) RequireKYC(_ sdk.Context, a string) error {
	if m.noKyc[a] {
		return types.ErrCompliance.Wrapf("%s not kyc", a)
	}
	return nil
}
func (m *mockCompliance) RecordAction(_ sdk.Context, _, _, _, _, _ string) {}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
	set *mockSettlement
	rwa *mockRWA
	cmp *mockCompliance
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	set := newMockSettlement()
	set.denoms[denom] = true
	rwa := newMockRWA()
	cmp := newMockCompliance()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, set, rwa, cmp)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), set: set, rwa: rwa, cmp: cmp}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }

func now(f *fixture) int64 { return f.ts.Ctx.BlockTime().Unix() }

// stdOffering creates a standard offering open now with the given caps and
// tranche config, returning its id.
func (f *fixture) stdOffering(t *testing.T, soft, hard uint64, tranches uint32, requiredInj, interval int64) uint64 {
	t.Helper()
	resp, err := f.srv.CreateOffering(f.ctx(), &types.MsgCreateOffering{
		Issuer:            issuer,
		TokenId:           tokenID,
		UnitPrice:         unitPrice,
		SoftCap:           soft,
		HardCap:           hard,
		StartTime:         now(f) - 10,
		EndTime:           now(f) + 1000,
		TotalTranches:     tranches,
		RequiredInjection: uint64(requiredInj),
		InjectionInterval: interval,
	})
	if err != nil {
		t.Fatalf("create offering: %v", err)
	}
	return resp.OfferingId
}

func (f *fixture) subscribe(t *testing.T, id uint64, investor string, amount uint64) {
	t.Helper()
	f.set.fund(denom, investor, amount)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: investor, OfferingId: id, Amount: amount}); err != nil {
		t.Fatalf("subscribe %s: %v", investor, err)
	}
}

func (f *fixture) bal(a string) uint64 { return f.set.GetBalance(f.ctx(), denom, a) }

// ---- tests -----------------------------------------------------------------

func TestHappyPathFullLifecycle(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)

	f.subscribe(t, id, alice, 600) // 6 units
	f.subscribe(t, id, bob, 300)   // 3 units

	treasury := keeper.TreasuryAccount(id)
	if f.bal(treasury) != 900 {
		t.Fatalf("treasury=%d want 900", f.bal(treasury))
	}

	// Close: end_time not reached but hard_cap not met either -> must advance.
	f.ts.Advance(2000)
	closeResp, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closeResp.Status != types.OfferingStatus_OFFERING_STATUS_SUCCEEDED {
		t.Fatalf("status=%v", closeResp.Status)
	}

	// Allocation mints RWA units to investors.
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err != nil {
		t.Fatalf("alice alloc: %v", err)
	}
	if f.rwa.minted[key(tokenID, alice)] != 6 {
		t.Fatalf("alice units=%d", f.rwa.minted[key(tokenID, alice)])
	}
	// Double allocation rejected.
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err == nil {
		t.Fatal("expected double-allocation rejection")
	}

	// Release gated until injection happens.
	if _, err := f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id}); err == nil {
		t.Fatal("expected tranche gate")
	}

	// Inject yield, then release tranche 1.
	f.set.fund(denom, issuer, 1000)
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 30}); err != nil {
		t.Fatalf("inject: %v", err)
	}
	rel, err := f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id})
	if err != nil {
		t.Fatalf("release1: %v", err)
	}
	if rel.Amount != 450 {
		t.Fatalf("tranche1=%d want 450", rel.Amount)
	}

	// Investors claim pro-rata yield from the 30 injected (6:3 -> 20:10).
	a1, err := f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: alice, OfferingId: id})
	if err != nil {
		t.Fatalf("alice returns: %v", err)
	}
	b1, err := f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: bob, OfferingId: id})
	if err != nil {
		t.Fatalf("bob returns: %v", err)
	}
	if a1.Amount != 20 || b1.Amount != 10 {
		t.Fatalf("returns alice=%d bob=%d want 20/10", a1.Amount, b1.Amount)
	}
	// Nothing left to claim immediately.
	if _, err := f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: alice, OfferingId: id}); err == nil {
		t.Fatal("expected nothing-to-claim")
	}

	// Second injection + final tranche drains the treasury.
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 12}); err != nil {
		t.Fatalf("inject2: %v", err)
	}
	rel2, err := f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id})
	if err != nil {
		t.Fatalf("release2: %v", err)
	}
	if rel2.Amount != 450 {
		t.Fatalf("tranche2=%d want 450", rel2.Amount)
	}
	if f.bal(treasury) != 0 {
		t.Fatalf("treasury not drained: %d", f.bal(treasury))
	}
	// All tranches released -> further release fails.
	if _, err := f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id}); err == nil {
		t.Fatal("expected all-tranches-released")
	}
}

func TestSubscribeValidation(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)

	// Not a multiple of unit_price.
	f.set.fund(denom, alice, 1000)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 150}); err == nil {
		t.Fatal("expected non-multiple rejection")
	}
	// Exceeds hard cap.
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 1100}); err == nil {
		t.Fatal("expected hard-cap rejection")
	}
	// Window closed.
	f.ts.Advance(5000)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: alice, OfferingId: id, Amount: 100}); err == nil {
		t.Fatal("expected window rejection")
	}
}

func TestFailedRaiseRefunds(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 500, 1000, 2, 10, 100)
	f.subscribe(t, id, alice, 200)
	f.subscribe(t, id, bob, 100)

	f.ts.Advance(2000)
	resp, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if resp.Status != types.OfferingStatus_OFFERING_STATUS_FAILED {
		t.Fatalf("status=%v want FAILED", resp.Status)
	}
	// Allocation must be impossible on a failed raise.
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err == nil {
		t.Fatal("expected allocation rejection on failed raise")
	}
	// Full refunds.
	r, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: alice, OfferingId: id})
	if err != nil || r.Amount != 200 {
		t.Fatalf("alice refund=%d err=%v", r.Amount, err)
	}
	if f.bal(alice) != 200 {
		t.Fatalf("alice balance=%d", f.bal(alice))
	}
	// Double refund rejected.
	if _, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: alice, OfferingId: id}); err == nil {
		t.Fatal("expected double-refund rejection")
	}
	if _, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: bob, OfferingId: id}); err != nil {
		t.Fatalf("bob refund: %v", err)
	}
	if f.bal(keeper.TreasuryAccount(id)) != 0 {
		t.Fatalf("treasury not emptied: %d", f.bal(keeper.TreasuryAccount(id)))
	}
}

func TestCancelRefunds(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)
	f.subscribe(t, id, alice, 400)

	if _, err := f.srv.CancelOffering(f.ctx(), &types.MsgCancelOffering{Issuer: issuer, OfferingId: id, Reason: "regulatory hold"}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// Subscriptions blocked after cancel.
	f.set.fund(denom, bob, 100)
	if _, err := f.srv.Subscribe(f.ctx(), &types.MsgSubscribe{Investor: bob, OfferingId: id, Amount: 100}); err == nil {
		t.Fatal("expected subscribe rejection after cancel")
	}
	r, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: alice, OfferingId: id})
	if err != nil || r.Amount != 400 {
		t.Fatalf("alice refund=%d err=%v", r.Amount, err)
	}
}

func TestDefaultProRataRefund(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 4, 10, 100)
	f.subscribe(t, id, alice, 600) // 600/900 share
	f.subscribe(t, id, bob, 300)   // 300/900 share

	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Issuer services one tranche then goes silent.
	f.set.fund(denom, issuer, 1000)
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 10}); err != nil {
		t.Fatalf("inject: %v", err)
	}
	if _, err := f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id}); err != nil {
		t.Fatalf("release: %v", err)
	}
	// raised=900, total=4 -> tranche1 = 225 released; treasury now 675.
	treasury := keeper.TreasuryAccount(id)
	if f.bal(treasury) != 675 {
		t.Fatalf("treasury=%d want 675", f.bal(treasury))
	}

	// Cannot flag default before the next injection deadline.
	if _, err := f.srv.FlagDefault(f.ctx(), &types.MsgFlagDefault{Caller: stranger, OfferingId: id}); err == nil {
		t.Fatal("expected not-delinquent")
	}
	// Advance past the 2nd injection deadline (interval=100, 2 due).
	f.ts.Advance(250)
	if _, err := f.srv.FlagDefault(f.ctx(), &types.MsgFlagDefault{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("flag default: %v", err)
	}

	// Pro-rata refund of the 675 remaining: alice 6/9 -> 450, bob 3/9 -> 225.
	ra, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: alice, OfferingId: id})
	if err != nil || ra.Amount != 450 {
		t.Fatalf("alice default refund=%d err=%v", ra.Amount, err)
	}
	rb, err := f.srv.ClaimRefund(f.ctx(), &types.MsgClaimRefund{Investor: bob, OfferingId: id})
	if err != nil || rb.Amount != 225 {
		t.Fatalf("bob default refund=%d err=%v", rb.Amount, err)
	}
	if f.bal(treasury) != 0 {
		t.Fatalf("treasury not drained after default refunds: %d", f.bal(treasury))
	}
	// Allocation still claimable post-default (investors keep their asset).
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err != nil {
		t.Fatalf("post-default alloc: %v", err)
	}
}

func TestNoDefaultWhenFullyServiced(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 100, 200, 1, 10, 100)
	f.subscribe(t, id, alice, 200)
	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}
	f.set.fund(denom, issuer, 100)
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 10}); err != nil {
		t.Fatalf("inject: %v", err)
	}
	// All injections complete -> never delinquent, no default possible.
	f.ts.Advance(100000)
	if _, err := f.srv.FlagDefault(f.ctx(), &types.MsgFlagDefault{Caller: stranger, OfferingId: id}); err == nil {
		t.Fatal("expected not-delinquent for fully-serviced offering")
	}
}

func TestReturnsProRataDust(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 1, 100)
	f.subscribe(t, id, alice, 600) // 6 units
	f.subscribe(t, id, bob, 300)   // 3 units
	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Inject 10 over 9 units: alice floor(10*6/9)=6, bob floor(10*3/9)=3, dust 1.
	f.set.fund(denom, issuer, 100)
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 10}); err != nil {
		t.Fatalf("inject: %v", err)
	}
	a, _ := f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: alice, OfferingId: id})
	b, _ := f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: bob, OfferingId: id})
	if a.Amount != 6 || b.Amount != 3 {
		t.Fatalf("returns alice=%d bob=%d want 6/3", a.Amount, b.Amount)
	}
	// 1 unit of dust remains in the returns pool, claimable by no one yet.
	if f.bal(keeper.ReturnsAccount(id)) != 1 {
		t.Fatalf("returns dust=%d want 1", f.bal(keeper.ReturnsAccount(id)))
	}
}

func TestAuthAndStateGuards(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)
	f.subscribe(t, id, alice, 600)

	// Non-issuer cannot cancel/inject/release.
	if _, err := f.srv.CancelOffering(f.ctx(), &types.MsgCancelOffering{Issuer: stranger, OfferingId: id}); err == nil {
		t.Fatal("expected unauthorized cancel")
	}

	// Cannot inject/release while still OPEN.
	f.set.fund(denom, issuer, 100)
	if _, err := f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 10}); err == nil {
		t.Fatal("expected state guard on inject")
	}

	// Issuer-not-token-admin offering creation rejected.
	f.rwa.admins[2] = stranger
	f.rwa.denoms[2] = denom
	if _, err := f.srv.CreateOffering(f.ctx(), &types.MsgCreateOffering{
		Issuer: issuer, TokenId: 2, UnitPrice: 100, SoftCap: 100, HardCap: 200,
		StartTime: now(f), EndTime: now(f) + 100, TotalTranches: 1,
	}); err == nil {
		t.Fatal("expected issuer-not-admin rejection")
	}

	// Unknown token rejected.
	if _, err := f.srv.CreateOffering(f.ctx(), &types.MsgCreateOffering{
		Issuer: issuer, TokenId: 999, UnitPrice: 100, SoftCap: 100, HardCap: 200,
		StartTime: now(f), EndTime: now(f) + 100, TotalTranches: 1,
	}); err == nil {
		t.Fatal("expected unknown-token rejection")
	}
}

func TestSettlementConserved(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)
	before := f.set.total(denom) // 0 funded so far
	f.subscribe(t, id, alice, 600)
	f.subscribe(t, id, bob, 300)
	f.set.fund(denom, issuer, 1000)
	funded := uint64(900 + 1000)
	if f.set.total(denom) != before+funded {
		t.Fatalf("conservation broken after funding")
	}
	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}
	f.srv.InjectReturn(f.ctx(), &types.MsgInjectReturn{Issuer: issuer, OfferingId: id, Amount: 30})
	f.srv.ReleaseTranche(f.ctx(), &types.MsgReleaseTranche{Issuer: issuer, OfferingId: id})
	f.srv.ClaimReturns(f.ctx(), &types.MsgClaimReturns{Investor: alice, OfferingId: id})
	// Total settlement is invariant under all internal transfers.
	if f.set.total(denom) != before+funded {
		t.Fatalf("conservation broken: %d != %d", f.set.total(denom), before+funded)
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 300, 1000, 2, 10, 100)
	f.subscribe(t, id, alice, 600)
	f.subscribe(t, id, bob, 300)
	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}

	exported, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := exported.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// Re-import into a fresh keeper.
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx(), exported); err != nil {
		t.Fatalf("init: %v", err)
	}
	re, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(re.Offerings) != 1 || len(re.Subscriptions) != 2 {
		t.Fatalf("roundtrip lost state: offerings=%d subs=%d", len(re.Offerings), len(re.Subscriptions))
	}
	if re.OfferingIdSeq != exported.OfferingIdSeq {
		t.Fatalf("seq mismatch: %d vs %d", re.OfferingIdSeq, exported.OfferingIdSeq)
	}
}

func TestAllocationFailurePropagates(t *testing.T) {
	f := setup(t)
	id := f.stdOffering(t, 100, 1000, 1, 10, 100)
	f.subscribe(t, id, alice, 200)
	f.ts.Advance(2000)
	if _, err := f.srv.CloseOffering(f.ctx(), &types.MsgCloseOffering{Caller: stranger, OfferingId: id}); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Simulate rwatoken compliance refusing the recipient.
	f.rwa.allocErr = types.ErrRWA.Wrap("recipient not kyc")
	if _, err := f.srv.ClaimAllocation(f.ctx(), &types.MsgClaimAllocation{Investor: alice, OfferingId: id}); err == nil {
		t.Fatal("expected allocation failure to surface")
	}
	// Subscription must remain unallocated so it can be retried.
	sub, _, _ := f.k.GetSubscription(f.ctx(), id, alice)
	if sub.Allocated {
		t.Fatal("subscription wrongly marked allocated after failed mint")
	}
}
