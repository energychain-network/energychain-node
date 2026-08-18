package keeper_test

import (
	"context"
	"testing"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/testutil"
	"energychain/x/mincast/keeper"
	"energychain/x/mincast/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	admin     = testutil.DeriveAddr("admin")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")

	denom = "weusd"
)

// ---- mocks -----------------------------------------------------------------

type mockCompliance struct {
	sanctioned map[string]bool
	kyc        map[string]bool
	denied     map[string]bool
	requireKYC bool
}

func newMockCompliance() *mockCompliance {
	return &mockCompliance{sanctioned: map[string]bool{}, kyc: map[string]bool{}, denied: map[string]bool{}}
}
func (m *mockCompliance) IsSanctioned(_ sdk.Context, a string) bool { return m.sanctioned[a] }
func (m *mockCompliance) RequireKYC(_ sdk.Context, a string) error {
	if m.requireKYC && !m.kyc[a] {
		return types.ErrCompliance.Wrap("kyc required")
	}
	return nil
}
func (m *mockCompliance) EvaluateTransfer(_ sdk.Context, _, _, _, from, to string, _ uint64) error {
	if m.denied[from+"|"+to] {
		return types.ErrCompliance.Wrap("policy denied")
	}
	return nil
}
func (m *mockCompliance) RecordAction(_ sdk.Context, _, _, _, _, _ string) {}

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
func (m *mockSettlement) total(d string) uint64 {
	var sum uint64
	for k, v := range m.bal {
		if len(k) > len(d) && k[:len(d)+1] == d+"|" {
			sum += v
		}
	}
	return sum
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts  *testutil.TestStore
	k   keeper.Keeper
	srv types.MsgServer
	q   types.QueryServer
	cmp *mockCompliance
	set *mockSettlement
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := newMockCompliance()
	set := newMockSettlement()
	set.denoms[denom] = true
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, set)
	if err := k.SetParams(ts.Ctx, types.DefaultParams()); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), q: keeper.NewQueryServerImpl(k), cmp: cmp, set: set}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }

func (f *fixture) market(t *testing.T, mintFee, meltFee uint32) uint64 {
	t.Helper()
	resp, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Admin: admin, Denom: "mcEV", Name: "Mincast EV", SettlementDenom: denom,
		InitialPrice: 100, MintFeeBps: mintFee, MeltFeeBps: meltFee,
	})
	if err != nil {
		t.Fatalf("create market: %v", err)
	}
	return resp.MarketId
}

func (f *fixture) getMarket(t *testing.T, id uint64) types.Market {
	t.Helper()
	m, ok, err := f.k.GetMarket(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("get market %d: ok=%v err=%v", id, ok, err)
	}
	return m
}

func (f *fixture) bal(t *testing.T, id uint64, h string) uint64 {
	t.Helper()
	v, err := f.k.GetBalance(f.ctx(), id, h)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return v
}

func (f *fixture) mint(t *testing.T, id uint64, buyer string, pay uint64) uint64 {
	t.Helper()
	f.set.fund(denom, buyer, pay)
	resp, err := f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: buyer, MarketId: id, PayAmount: pay})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return resp.Units
}

func (f *fixture) settle(a string) uint64 { return f.set.GetBalance(f.ctx(), denom, a) }

// ---- tests -----------------------------------------------------------------

func TestBootstrapMintAndFloor(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)

	units := f.mint(t, id, alice, 100_000) // initial price 100 -> 1000 units
	if units != 1000 {
		t.Fatalf("bootstrap units=%d want 1000", units)
	}
	m := f.getMarket(t, id)
	if m.Treasury != 100_000 || m.Supply != 1000 || m.FloorPrice != 100 {
		t.Fatalf("bootstrap state wrong: %+v", m)
	}
	if f.bal(t, id, alice) != 1000 {
		t.Fatal("alice balance wrong")
	}
	if f.settle(keeper.TreasuryAccount(id)) != 100_000 {
		t.Fatal("treasury not funded")
	}

	// Second mint at the same floor keeps it constant (no fee).
	f.mint(t, id, bob, 50_000) // 500 units
	m = f.getMarket(t, id)
	if m.Supply != 1500 || m.Treasury != 150_000 || m.FloorPrice != 100 {
		t.Fatalf("post-mint state: %+v", m)
	}
}

func TestMintFeeRaisesFloor(t *testing.T) {
	f := setup(t)
	id := f.market(t, 1000, 0) // 10% mint fee
	f.mint(t, id, alice, 100_000)
	m := f.getMarket(t, id)
	// fee=10000, principal=90000 -> 900 units; treasury=100000; floor=111
	if m.Supply != 900 || m.Treasury != 100_000 {
		t.Fatalf("state: %+v", m)
	}
	if m.FloorPrice != 100_000/900 {
		t.Fatalf("floor=%d want %d", m.FloorPrice, 100_000/900)
	}
	if m.FloorPrice <= 100 {
		t.Fatal("mint fee should lift floor above initial price")
	}
}

func TestMeltReturnsFloorAndIsMonotonic(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 1000) // 10% melt fee
	f.mint(t, id, alice, 100_000)
	floorBefore := f.getMarket(t, id).FloorPrice

	resp, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 100})
	if err != nil {
		t.Fatalf("melt: %v", err)
	}
	// gross=100*100=10000, fee=1000, payout=9000
	if resp.Settlement != 9000 {
		t.Fatalf("payout=%d want 9000", resp.Settlement)
	}
	if f.settle(alice) != 9000 {
		t.Fatalf("alice settle=%d", f.settle(alice))
	}
	m := f.getMarket(t, id)
	if m.Supply != 900 || m.Treasury != 91_000 {
		t.Fatalf("post-melt state: %+v", m)
	}
	if m.FloorPrice < floorBefore {
		t.Fatalf("melt fee must not lower floor: %d < %d", m.FloorPrice, floorBefore)
	}
	if f.bal(t, id, alice) != 900 {
		t.Fatal("alice units not burned")
	}
}

func TestMintMeltSlippageGuards(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.set.fund(denom, alice, 100_000)
	// demand more units than 100000 buys
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: alice, MarketId: id, PayAmount: 100_000, MinUnitsOut: 2000}); err == nil {
		t.Fatal("expected mint slippage")
	}
	// real mint then melt with too-high min settlement
	f.mint(t, id, alice, 100_000)
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 100, MinSettlementOut: 99_999}); err == nil {
		t.Fatal("expected melt slippage")
	}
}

func TestInjectTreasuryRaisesFloor(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.mint(t, id, alice, 100_000) // floor 100
	f.set.fund(denom, carol, 50_000)
	resp, err := f.srv.InjectTreasury(f.ctx(), &types.MsgInjectTreasury{Funder: carol, MarketId: id, Amount: 50_000})
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	// treasury 150000 / supply 1000 = 150
	if resp.FloorPrice != 150 {
		t.Fatalf("floor=%d want 150", resp.FloorPrice)
	}
}

func TestStatusGating(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.mint(t, id, alice, 100_000)

	// PAUSED: mint blocked, melt + transfer allowed
	if _, err := f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Admin: admin, MarketId: id, Status: types.MarketStatus_MARKET_STATUS_PAUSED}); err != nil {
		t.Fatal(err)
	}
	f.set.fund(denom, bob, 1000)
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: bob, MarketId: id, PayAmount: 1000}); err == nil {
		t.Fatal("paused mint must fail")
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, MarketId: id, To: bob, Amount: 10}); err != nil {
		t.Fatalf("paused transfer should work: %v", err)
	}
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 10}); err != nil {
		t.Fatalf("paused melt should work: %v", err)
	}

	// CLOSED: transfer blocked, melt allowed
	if _, err := f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Admin: admin, MarketId: id, Status: types.MarketStatus_MARKET_STATUS_CLOSED}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, MarketId: id, To: bob, Amount: 10}); err == nil {
		t.Fatal("closed transfer must fail")
	}
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 10}); err != nil {
		t.Fatalf("closed melt should work: %v", err)
	}
}

func TestComplianceGates(t *testing.T) {
	f := setup(t)
	f.cmp.requireKYC = true
	resp, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Admin: admin, Denom: "mcKYC", Name: "k", SettlementDenom: denom,
		InitialPrice: 100, RequireKyc: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := resp.MarketId

	// buyer without KYC blocked
	f.set.fund(denom, alice, 100_000)
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: alice, MarketId: id, PayAmount: 100_000}); err == nil {
		t.Fatal("expected KYC gate on mint")
	}
	f.cmp.kyc[alice] = true
	f.mint(t, id, alice, 100_000)

	// sanctioned seller cannot melt
	f.cmp.sanctioned[alice] = true
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 10}); err == nil {
		t.Fatal("expected sanctioned seller melt block")
	}
	f.cmp.sanctioned[alice] = false

	// sanctioned funder cannot inject or fund reward
	f.cmp.sanctioned[carol] = true
	f.set.fund(denom, carol, 10_000)
	if _, err := f.srv.InjectTreasury(f.ctx(), &types.MsgInjectTreasury{Funder: carol, MarketId: id, Amount: 1000}); err == nil {
		t.Fatal("expected sanctioned inject block")
	}
	if _, err := f.srv.FundReward(f.ctx(), &types.MsgFundReward{Funder: carol, MarketId: id, Amount: 1000}); err == nil {
		t.Fatal("expected sanctioned fund-reward block")
	}
}

func TestInvestFullCycle(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.mint(t, id, alice, 100_000) // floor 100, supply 1000

	// open invest: 100 units, 1 year, 20% APY
	open, err := f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{
		Investor: alice, MarketId: id, Units: 100, TermSeconds: types.DefaultYearSeconds, ApyBps: 2000,
	})
	if err != nil {
		t.Fatalf("open invest: %v", err)
	}
	// principalValue = 100*100 = 10000; yield = 20% = 2000
	if open.Yield != 2000 {
		t.Fatalf("yield=%d want 2000", open.Yield)
	}
	// units escrowed: supply unchanged, alice balance down, escrow up
	m := f.getMarket(t, id)
	if m.Supply != 1000 {
		t.Fatal("invest must not change supply")
	}
	if f.bal(t, id, alice) != 900 || f.bal(t, id, types.InvestEscrow) != 100 {
		t.Fatal("invest escrow accounting wrong")
	}

	// close before maturity fails
	if _, err := f.srv.CloseInvest(f.ctx(), &types.MsgCloseInvest{Caller: alice, InvestId: open.InvestId}); err == nil {
		t.Fatal("expected not-matured")
	}

	// fund reward, advance to maturity, close
	f.set.fund(denom, admin, 2000)
	if _, err := f.srv.FundReward(f.ctx(), &types.MsgFundReward{Funder: admin, MarketId: id, Amount: 2000}); err != nil {
		t.Fatalf("fund reward: %v", err)
	}
	f.ts.Advance(types.DefaultYearSeconds + 1)
	closeResp, err := f.srv.CloseInvest(f.ctx(), &types.MsgCloseInvest{Caller: bob, InvestId: open.InvestId})
	if err != nil {
		t.Fatalf("close invest: %v", err)
	}
	if closeResp.Yield != 2000 {
		t.Fatalf("close yield=%d", closeResp.Yield)
	}
	if f.settle(alice) != 2000 {
		t.Fatalf("alice yield paid=%d", f.settle(alice))
	}
	// units returned
	if f.bal(t, id, alice) != 1000 || f.bal(t, id, types.InvestEscrow) != 0 {
		t.Fatal("invest principal not returned")
	}
}

func TestInvestRewardUnderfundedFailsClosed(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.mint(t, id, alice, 100_000)
	open, _ := f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{
		Investor: alice, MarketId: id, Units: 100, TermSeconds: types.DefaultYearSeconds, ApyBps: 2000,
	})
	f.ts.Advance(types.DefaultYearSeconds + 1)
	// no reward funding -> close must fail and leave units escrowed
	if _, err := f.srv.CloseInvest(f.ctx(), &types.MsgCloseInvest{Caller: alice, InvestId: open.InvestId}); err == nil {
		t.Fatal("expected reward underfunded failure")
	}
	if f.bal(t, id, types.InvestEscrow) != 100 {
		t.Fatal("escrow must be intact after failed close")
	}
	iv, _, _ := f.k.GetInvest(f.ctx(), open.InvestId)
	if iv.Status != types.InvestStatus_INVEST_STATUS_ACTIVE {
		t.Fatal("invest should still be active")
	}
}

func TestCancelInvestReturnsPrincipalNoYield(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	f.mint(t, id, alice, 100_000)
	open, _ := f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{
		Investor: alice, MarketId: id, Units: 100, TermSeconds: types.DefaultYearSeconds, ApyBps: 2000,
	})
	if _, err := f.srv.CancelInvest(f.ctx(), &types.MsgCancelInvest{Investor: alice, InvestId: open.InvestId}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if f.bal(t, id, alice) != 1000 || f.bal(t, id, types.InvestEscrow) != 0 {
		t.Fatal("cancel must return principal units")
	}
	if f.settle(alice) != 0 {
		t.Fatal("cancel must not pay yield")
	}
	// non-owner cannot cancel
	open2, _ := f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{
		Investor: alice, MarketId: id, Units: 50, TermSeconds: types.DefaultYearSeconds, ApyBps: 2000,
	})
	if _, err := f.srv.CancelInvest(f.ctx(), &types.MsgCancelInvest{Investor: bob, InvestId: open2.InvestId}); err == nil {
		t.Fatal("non-owner cancel must fail")
	}
}

func TestSettlementConserved(t *testing.T) {
	f := setup(t)
	id := f.market(t, 100, 100)
	f.set.fund(denom, alice, 200_000)
	f.set.fund(denom, admin, 10_000)
	before := f.set.total(denom)

	f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: alice, MarketId: id, PayAmount: 200_000})
	f.srv.FundReward(f.ctx(), &types.MsgFundReward{Funder: admin, MarketId: id, Amount: 10_000})
	f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 200})
	open, _ := f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{Investor: alice, MarketId: id, Units: 100, TermSeconds: types.DefaultYearSeconds, ApyBps: 1000})
	f.ts.Advance(types.DefaultYearSeconds + 1)
	f.srv.CloseInvest(f.ctx(), &types.MsgCloseInvest{Caller: alice, InvestId: open.InvestId})

	if f.set.total(denom) != before {
		t.Fatalf("settlement not conserved: %d != %d", f.set.total(denom), before)
	}
}

func TestAuthAndUnknownMarket(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0)
	if _, err := f.srv.UpdateMarket(f.ctx(), &types.MsgUpdateMarket{Admin: bob, MarketId: id}); err == nil {
		t.Fatal("non-admin update must fail")
	}
	if _, err := f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Admin: bob, MarketId: id, Status: types.MarketStatus_MARKET_STATUS_PAUSED}); err == nil {
		t.Fatal("non-admin status must fail")
	}
	f.set.fund(denom, alice, 100)
	if _, err := f.srv.Mint(f.ctx(), &types.MsgMint{Buyer: alice, MarketId: 999, PayAmount: 100}); err == nil {
		t.Fatal("unknown market must fail")
	}
	// duplicate denom rejected
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Admin: admin, Denom: "mcEV", Name: "dup", SettlementDenom: denom, InitialPrice: 100,
	}); err == nil {
		t.Fatal("duplicate denom must fail")
	}
}

func TestFullExitDrainsTreasuryNoOrphan(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 1000) // 10% melt fee accrues fees into treasury
	f.mint(t, id, alice, 100_000)
	f.mint(t, id, bob, 100_000)
	// bob melts part, leaving fees in treasury and supply > 0
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: bob, MarketId: id, Units: 500}); err != nil {
		t.Fatalf("bob melt: %v", err)
	}
	// now both holders fully exit; the final exit must drain the treasury to 0
	balA := f.bal(t, id, alice)
	balB := f.bal(t, id, bob)
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: balA}); err != nil {
		t.Fatalf("alice exit: %v", err)
	}
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: bob, MarketId: id, Units: balB}); err != nil {
		t.Fatalf("bob exit: %v", err)
	}
	m := f.getMarket(t, id)
	if m.Supply != 0 {
		t.Fatalf("supply should be 0, got %d", m.Supply)
	}
	if m.Treasury != 0 {
		t.Fatalf("treasury must be fully drained to 0 (no orphan), got %d", m.Treasury)
	}
	if f.settle(keeper.TreasuryAccount(id)) != 0 {
		t.Fatalf("treasury account must be empty, got %d", f.settle(keeper.TreasuryAccount(id)))
	}

	// A subsequent bootstrap mint therefore cannot skim orphan backing: the
	// minter's immediate melt cannot exceed what they paid.
	paid := uint64(10_000)
	units := f.mint(t, id, carol, paid)
	resp, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: carol, MarketId: id, Units: units})
	if err != nil {
		t.Fatalf("carol melt: %v", err)
	}
	if resp.Settlement > paid {
		t.Fatalf("bootstrap skim: melted %d > paid %d", resp.Settlement, paid)
	}
}

func TestInjectIntoEmptyMarketBlocked(t *testing.T) {
	f := setup(t)
	id := f.market(t, 0, 0) // supply 0
	f.set.fund(denom, carol, 50_000)
	if _, err := f.srv.InjectTreasury(f.ctx(), &types.MsgInjectTreasury{Funder: carol, MarketId: id, Amount: 50_000}); err == nil {
		t.Fatal("inject into empty market must be rejected")
	}
}

func TestMeltRequiresSellerKYC(t *testing.T) {
	f := setup(t)
	f.cmp.requireKYC = true
	resp, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Admin: admin, Denom: "mcK2", Name: "k", SettlementDenom: denom, InitialPrice: 100, RequireKyc: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := resp.MarketId
	f.cmp.kyc[alice] = true
	f.mint(t, id, alice, 100_000)
	// alice loses KYC; melt (seller side) must now be blocked even with no policy
	f.cmp.kyc[alice] = false
	if _, err := f.srv.Melt(f.ctx(), &types.MsgMelt{Seller: alice, MarketId: id, Units: 10}); err == nil {
		t.Fatal("melt must require seller KYC when market RequireKyc is set")
	}
	// transfer sender side likewise gated
	if _, err := f.srv.Transfer(f.ctx(), &types.MsgTransfer{From: alice, MarketId: id, To: bob, Amount: 10}); err == nil {
		t.Fatal("transfer must require sender KYC")
	}
}

func TestGenesisRejectsOrphanTreasury(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Markets = []types.Market{{
		Id: 1, Denom: "mcEV", Admin: admin, SettlementDenom: denom,
		InitialPrice: 100, Treasury: 1000, Supply: 0, FloorPrice: 0,
		Status: types.MarketStatus_MARKET_STATUS_ACTIVE,
	}}
	gs.MarketIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("genesis with orphan treasury must be rejected")
	}
}

func TestGenesisRejectsInflatedInvestYield(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Markets = []types.Market{{
		Id: 1, Denom: "mcEV", Admin: admin, SettlementDenom: denom,
		InitialPrice: 100, Treasury: 100_000, Supply: 1000, FloorPrice: 100,
		Status: types.MarketStatus_MARKET_STATUS_ACTIVE,
	}}
	gs.Balances = []types.Balance{
		{MarketId: 1, Holder: alice, Amount: 900},
		{MarketId: 1, Holder: types.InvestEscrow, Amount: 100},
	}
	// principalValue=100*100=10000, 1yr @ 20% => bound 2000; declare 9999
	gs.Invests = []types.Invest{{
		Id: 1, MarketId: 1, Investor: alice, PrincipalUnits: 100, Yield: 9999, ApyBps: 2000,
		OpenedAt: 0, Maturity: types.DefaultYearSeconds, Status: types.InvestStatus_INVEST_STATUS_ACTIVE,
	}}
	gs.MarketIdSeq = 1
	gs.InvestIdSeq = 1
	if err := gs.Validate(); err == nil {
		t.Fatal("genesis with inflated invest yield must be rejected")
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	id := f.market(t, 100, 100)
	f.mint(t, id, alice, 100_000)
	f.srv.OpenInvest(f.ctx(), &types.MsgOpenInvest{Investor: alice, MarketId: id, Units: 100, TermSeconds: types.DefaultYearSeconds, ApyBps: 2000})

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	g := setup(t)
	// Mirror the settlement balances x/stableusd genesis would carry so the
	// treasury reconciliation in InitGenesis passes.
	for _, mk := range gs.Markets {
		g.set.fund(denom, keeper.TreasuryAccount(mk.Id), mk.Treasury)
	}
	if err := g.k.InitGenesis(g.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	gs2, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(gs2.Markets) != 1 || len(gs2.Invests) != 1 {
		t.Fatalf("roundtrip mismatch: markets=%d invests=%d", len(gs2.Markets), len(gs2.Invests))
	}
	if gs2.MarketIdSeq != gs.MarketIdSeq || gs2.InvestIdSeq != gs.InvestIdSeq {
		t.Fatal("sequence mismatch")
	}
	// escrow balance preserved
	if g.bal(t, id, types.InvestEscrow) != 100 {
		t.Fatal("escrow balance lost on roundtrip")
	}
}
