package keeper_test

import (
	"context"
	"strconv"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"energychain/testutil"
	"energychain/x/market/keeper"
	"energychain/x/market/types"
)

var (
	authority = testutil.DeriveAddr("authority")
	alice     = testutil.DeriveAddr("alice")
	bob       = testutil.DeriveAddr("bob")
	carol     = testutil.DeriveAddr("carol")
	dave      = testutil.DeriveAddr("dave")

	base  = "wecny"
	quote = "weusd"
)

// ---- mocks -----------------------------------------------------------------

type mockCompliance struct {
	sanctioned map[string]bool
	noKyc      map[string]bool   // addresses that FAIL the KYC gate
	policyDeny map[string]bool   // policyID -> deny EvaluateTransfer
}

func (m *mockCompliance) IsSanctioned(_ sdk.Context, a string) bool { return m.sanctioned[a] }
func (m *mockCompliance) RequireKYC(_ sdk.Context, a string) error {
	if m.noKyc[a] {
		return types.ErrCompliance.Wrapf("%s not kyc", a)
	}
	return nil
}
func (m *mockCompliance) EvaluateTransfer(_ sdk.Context, _, _, policyID, _, _ string, _ uint64) error {
	if m.policyDeny[policyID] {
		return types.ErrCompliance.Wrapf("policy %s denied", policyID)
	}
	return nil
}

type mockSettlement struct {
	denoms  map[string]bool
	paused  map[string]bool
	blocked map[string]bool // accounts whose inbound MoveBalance reverts (freeze)
	bal     map[string]uint64
}

func newSettlement() *mockSettlement {
	return &mockSettlement{
		denoms:  map[string]bool{base: true, quote: true},
		paused:  map[string]bool{},
		blocked: map[string]bool{},
		bal:     map[string]uint64{},
	}
}
func (m *mockSettlement) key(d, a string) string                    { return d + "|" + a }
func (m *mockSettlement) HasDenom(_ context.Context, d string) bool { return m.denoms[d] }
func (m *mockSettlement) DenomActive(_ context.Context, d string) bool {
	return m.denoms[d] && !m.paused[d]
}
func (m *mockSettlement) GetBalance(_ context.Context, d, a string) uint64 { return m.bal[m.key(d, a)] }
func (m *mockSettlement) MoveBalance(_ context.Context, d, from, to string, amt uint64) error {
	if !m.denoms[d] {
		return types.ErrSettlement.Wrap("unknown denom")
	}
	if m.blocked[to] {
		return types.ErrSettlement.Wrap("recipient frozen")
	}
	if m.bal[m.key(d, from)] < amt {
		return types.ErrSettlement.Wrap("insufficient")
	}
	m.bal[m.key(d, from)] -= amt
	m.bal[m.key(d, to)] += amt
	return nil
}
func (m *mockSettlement) fund(d, a string, amt uint64) { m.bal[m.key(d, a)] += amt }

// mockAsset is a minimal x/rwatoken stand-in implementing types.RWAAssetKeeper:
// a per-(token,holder) unit ledger plus compliance/cap toggles, enough to
// exercise the market's RWA-base escrow/settlement and the void-on-batch path.
type mockAsset struct {
	settle   map[uint64]string // tokenID -> settlement denom
	tradable map[uint64]bool
	admin    map[uint64]string // tokenID -> admin (issuer)
	units    map[string]uint64 // tokenID|holder -> units
	cap      map[uint64]uint64 // tokenID -> per-holder cap (0 = none)
	denyRecv map[string]bool   // holders that fail the receiver compliance gate
}

func newAsset() *mockAsset {
	return &mockAsset{
		settle:   map[uint64]string{},
		tradable: map[uint64]bool{},
		admin:    map[uint64]string{},
		units:    map[string]uint64{},
		cap:      map[uint64]uint64{},
		denyRecv: map[string]bool{},
	}
}
func (a *mockAsset) key(id uint64, h string) string { return strconv.FormatUint(id, 10) + "|" + h }
func (a *mockAsset) register(id uint64, denom string) {
	a.settle[id] = denom
	a.tradable[id] = true
}
func (a *mockAsset) fund(id uint64, h string, amt uint64) { a.units[a.key(id, h)] += amt }

func (a *mockAsset) MarketHasToken(_ context.Context, id uint64) bool { _, ok := a.settle[id]; return ok }
func (a *mockAsset) MarketTokenTradable(_ context.Context, id uint64) bool { return a.tradable[id] }
func (a *mockAsset) MarketTokenAdmin(_ context.Context, id uint64) (string, bool) {
	adm, ok := a.admin[id]
	return adm, ok
}
func (a *mockAsset) MarketSettlementDenom(_ context.Context, id uint64) (string, bool) {
	d, ok := a.settle[id]
	return d, ok
}
func (a *mockAsset) MarketUnitBalance(_ context.Context, id uint64, h string) uint64 {
	return a.units[a.key(id, h)]
}
func (a *mockAsset) MarketReceiverOK(_ context.Context, id uint64, to string, amt uint64) error {
	if a.denyRecv[to] {
		return types.ErrCompliance.Wrapf("%s receiver denied", to)
	}
	if c := a.cap[id]; c > 0 && a.units[a.key(id, to)]+amt > c {
		return types.ErrLimitExceeded.Wrap("per-holder cap")
	}
	return nil
}
func (a *mockAsset) MarketEscrowUnits(_ context.Context, id uint64, from, escrow string, amt uint64) error {
	if a.units[a.key(id, from)] < amt {
		return types.ErrSettlement.Wrap("insufficient units")
	}
	a.units[a.key(id, from)] -= amt
	a.units[a.key(id, escrow)] += amt
	return nil
}
func (a *mockAsset) MarketReleaseUnits(_ context.Context, id uint64, escrow, to string, amt uint64) error {
	if err := a.MarketReceiverOK(context.Background(), id, to, amt); err != nil {
		return err
	}
	if a.units[a.key(id, escrow)] < amt {
		return types.ErrSettlement.Wrap("insufficient escrow units")
	}
	a.units[a.key(id, escrow)] -= amt
	a.units[a.key(id, to)] += amt
	return nil
}
func (a *mockAsset) MarketRefundUnits(_ context.Context, id uint64, escrow, to string, amt uint64) error {
	if a.units[a.key(id, escrow)] < amt {
		return types.ErrSettlement.Wrap("insufficient escrow units")
	}
	a.units[a.key(id, escrow)] -= amt
	a.units[a.key(id, to)] += amt
	return nil
}
// mockBank is a minimal x/bank stand-in for the listing-bond escrow.
type mockBank struct {
	bal map[string]sdkmath.Int // addr|denom -> amount
}

func newBank() *mockBank { return &mockBank{bal: map[string]sdkmath.Int{}} }

func (b *mockBank) key(addr sdk.AccAddress, denom string) string { return addr.String() + "|" + denom }
func (b *mockBank) get(addr sdk.AccAddress, denom string) sdkmath.Int {
	if v, ok := b.bal[b.key(addr, denom)]; ok {
		return v
	}
	return sdkmath.ZeroInt()
}
func (b *mockBank) fund(addr sdk.AccAddress, denom string, amt sdkmath.Int) {
	b.bal[b.key(addr, denom)] = b.get(addr, denom).Add(amt)
}
func (b *mockBank) move(from, to sdk.AccAddress, coins sdk.Coins) error {
	for _, c := range coins {
		have := b.get(from, c.Denom)
		if have.LT(c.Amount) {
			return types.ErrBond.Wrapf("insufficient %s: have %s need %s", c.Denom, have, c.Amount)
		}
		b.bal[b.key(from, c.Denom)] = have.Sub(c.Amount)
		b.bal[b.key(to, c.Denom)] = b.get(to, c.Denom).Add(c.Amount)
	}
	return nil
}
func (b *mockBank) SendCoinsFromAccountToModule(_ context.Context, sender sdk.AccAddress, mod string, amt sdk.Coins) error {
	return b.move(sender, authtypes.NewModuleAddress(mod), amt)
}
func (b *mockBank) SendCoinsFromModuleToAccount(_ context.Context, mod string, recipient sdk.AccAddress, amt sdk.Coins) error {
	return b.move(authtypes.NewModuleAddress(mod), recipient, amt)
}
func (b *mockBank) GetBalance(_ context.Context, addr sdk.AccAddress, denom string) sdk.Coin {
	return sdk.NewCoin(denom, b.get(addr, denom))
}

func (m *mockSettlement) total(d string) uint64 {
	var s uint64
	for k, v := range m.bal {
		if len(k) > len(d) && k[:len(d)] == d && k[len(d)] == '|' {
			s += v
		}
	}
	return s
}

// ---- fixture ---------------------------------------------------------------

type fixture struct {
	ts    *testutil.TestStore
	k     keeper.Keeper
	srv   types.MsgServer
	cmp   *mockCompliance
	set   *mockSettlement
	asset *mockAsset
	bank  *mockBank
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ts := testutil.NewTestStore(t, types.StoreKey)
	cmp := &mockCompliance{sanctioned: map[string]bool{}, noKyc: map[string]bool{}, policyDeny: map[string]bool{}}
	set := newSettlement()
	asset := newAsset()
	bank := newBank()
	k := keeper.NewKeeper(ts.Cdc, runtime.NewKVStoreService(ts.Key(t, types.StoreKey)), authority, cmp, set, asset, bank)
	// Most tests exercise the order book, not the listing-bond gate, so the
	// fixture disables the bond (markets activate immediately). Bond tests
	// re-enable it explicitly via setListingBond.
	p := types.DefaultParams()
	p.ListingBond = sdkmath.ZeroInt()
	if err := k.SetParams(ts.Ctx, p); err != nil {
		t.Fatalf("set params: %v", err)
	}
	return &fixture{ts: ts, k: k, srv: keeper.NewMsgServerImpl(k), cmp: cmp, set: set, asset: asset, bank: bank}
}

func (f *fixture) setListingBond(t *testing.T, amount sdkmath.Int, denom string) {
	t.Helper()
	p, err := f.k.GetParams(f.ctx())
	if err != nil {
		t.Fatal(err)
	}
	p.ListingBond = amount
	p.ListingBondDenom = denom
	if err := f.k.SetParams(f.ctx(), p); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) ctx() context.Context { return f.ts.Ctx }

func (f *fixture) createMarket(t *testing.T, feeBps uint32) uint64 {
	t.Helper()
	r, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: base, QuoteDenom: quote,
		FeeBps: feeBps, MinBaseQty: 1, BatchInterval: 10,
	})
	if err != nil {
		t.Fatalf("create market: %v", err)
	}
	return r.MarketId
}

func (f *fixture) place(t *testing.T, mid uint64, owner string, side types.OrderSide, price, qty uint64) uint64 {
	t.Helper()
	r, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: owner, MarketId: mid, Side: side, Price: price, Quantity: qty})
	if err != nil {
		t.Fatalf("place order: %v", err)
	}
	return r.OrderId
}

func (f *fixture) order(t *testing.T, id uint64) types.Order {
	t.Helper()
	o, ok, err := f.k.GetOrder(f.ctx(), id)
	if err != nil || !ok {
		t.Fatalf("order %d ok=%v err=%v", id, ok, err)
	}
	return o
}

// runBatch advances past the batch interval and runs EndBlock.
func (f *fixture) runBatch(t *testing.T) {
	t.Helper()
	f.ts.Advance(11)
	if err := f.k.EndBlock(f.ts.Ctx); err != nil {
		t.Fatalf("endblock: %v", err)
	}
}

func bal(f *fixture, d, a string) uint64 { return f.set.GetBalance(f.ctx(), d, a) }

// ---- admin -----------------------------------------------------------------

func TestCreateMarketAuthAndDenoms(t *testing.T) {
	f := setup(t)
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{Authority: alice, BaseDenom: base, QuoteDenom: quote, FeeBps: 0, MinBaseQty: 1, BatchInterval: 10}); err == nil {
		t.Fatal("non-authority must fail")
	}
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{Authority: authority, BaseDenom: "nope", QuoteDenom: quote, FeeBps: 0, MinBaseQty: 1, BatchInterval: 10}); err == nil {
		t.Fatal("unregistered base denom must fail")
	}
}

// ---- escrow & cancel -------------------------------------------------------

func TestPlaceEscrowsAndCancelRefunds(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(quote, alice, 1_000_000) // alice has 1 weusd (1e6 micro)
	// BUY 2 base @ price 500000 (0.5 quote per base) -> escrow ceil(0.5*2)=1 quote unit
	oid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 500_000, 2)
	if bal(f, quote, alice) != 999_999 || bal(f, quote, keeper.EscrowAccount()) != 1 {
		t.Fatalf("escrow wrong: alice=%d escrow=%d", bal(f, quote, alice), bal(f, quote, keeper.EscrowAccount()))
	}
	// cancel refunds the escrow
	if _, err := f.srv.CancelOrder(f.ctx(), &types.MsgCancelOrder{Owner: alice, OrderId: oid}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if bal(f, quote, alice) != 1_000_000 || bal(f, quote, keeper.EscrowAccount()) != 0 {
		t.Fatalf("refund wrong: alice=%d escrow=%d", bal(f, quote, alice), bal(f, quote, keeper.EscrowAccount()))
	}
}

func TestPlaceGuards(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(quote, alice, 100)
	// sanctioned
	f.cmp.sanctioned[alice] = true
	if _, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: alice, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: 1_000_000, Quantity: 1}); err == nil {
		t.Fatal("sanctioned owner must fail")
	}
	f.cmp.sanctioned[alice] = false
	// insufficient balance for escrow
	if _, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: alice, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: 1_000_000, Quantity: 1000}); err == nil {
		t.Fatal("insufficient escrow must fail")
	}
	// paused market
	f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Authority: authority, MarketId: mid, Status: types.MarketStatus_MARKET_STATUS_PAUSED})
	if _, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: alice, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: 1_000_000, Quantity: 1}); err == nil {
		t.Fatal("paused market place must fail")
	}
}

// ---- batch clearing --------------------------------------------------------

func TestBatchClearUniformPrice(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	// price 1_000_000 == 1 quote per base. Seller carol sells 100 base @ 1.0.
	// Buyer alice buys 100 base @ 1.2 (bids higher, gets price improvement).
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 1_000_000) // enough for ceil(1.2*100)=120 quote
	sid := f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	bid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_200_000, 100)

	beforeBase := f.set.total(base)
	beforeQuote := f.set.total(quote)

	f.runBatch(t)

	// both fully filled
	if f.order(t, sid).Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatal("seller not filled")
	}
	if f.order(t, bid).Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatal("buyer not filled")
	}
	// alice received 100 base
	if bal(f, base, alice) != 100 {
		t.Fatalf("alice base=%d want 100", bal(f, base, alice))
	}
	// clearing price is between 1.0 and 1.2; uniform price chosen from book.
	// carol receives quote = price*100/1e6; alice paid same; improvement refunded.
	cp := f.order(t, bid).Price // not the clearing price; check market
	_ = cp
	mk, _, _ := f.k.GetMarket(f.ctx(), mid)
	wantQuote, _ := types.QuoteFloor(mk.LastClearingPrice, 100)
	if bal(f, quote, carol) != wantQuote {
		t.Fatalf("carol quote=%d want %d (price=%d)", bal(f, quote, carol), wantQuote, mk.LastClearingPrice)
	}
	// alice quote spent == wantQuote (rest refunded)
	if bal(f, quote, alice) != 1_000_000-wantQuote {
		t.Fatalf("alice quote=%d want %d", bal(f, quote, alice), 1_000_000-wantQuote)
	}
	// conservation
	if f.set.total(base) != beforeBase || f.set.total(quote) != beforeQuote {
		t.Fatalf("not conserved base %d/%d quote %d/%d", f.set.total(base), beforeBase, f.set.total(quote), beforeQuote)
	}
	// escrow drained
	if bal(f, base, keeper.EscrowAccount()) != 0 || bal(f, quote, keeper.EscrowAccount()) != 0 {
		t.Fatalf("escrow not drained: base=%d quote=%d", bal(f, base, keeper.EscrowAccount()), bal(f, quote, keeper.EscrowAccount()))
	}
}

func TestBatchPartialFill(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	// seller offers 100, buyer wants 40 -> 40 clears, seller rests with 60.
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 1_000_000)
	sid := f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 40)

	f.runBatch(t)

	so := f.order(t, sid)
	if so.Status != types.OrderStatus_ORDER_STATUS_OPEN || so.Filled != 40 {
		t.Fatalf("seller should be partially filled: status=%v filled=%d", so.Status, so.Filled)
	}
	// seller still escrows the remaining 60 base
	if so.Escrowed != 60 {
		t.Fatalf("seller escrow=%d want 60", so.Escrowed)
	}
	if bal(f, base, alice) != 40 {
		t.Fatalf("alice base=%d want 40", bal(f, base, alice))
	}
	if bal(f, base, keeper.EscrowAccount()) != 60 {
		t.Fatalf("escrow base=%d want 60", bal(f, base, keeper.EscrowAccount()))
	}
}

func TestBatchFeeToCollector(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 100) // 1% fee
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 10_000_000)
	f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 100)

	beforeQuote := f.set.total(quote)
	f.runBatch(t)

	mk, _, _ := f.k.GetMarket(f.ctx(), mid)
	fTot, _ := types.QuoteFloor(mk.LastClearingPrice, 100) // 100 quote
	fee, _ := types.MulDivFloor(fTot, 100, 10000)          // 1% = 1
	if bal(f, quote, keeper.FeeAccount()) != fee || fee == 0 {
		t.Fatalf("fee=%d want %d", bal(f, quote, keeper.FeeAccount()), fee)
	}
	if bal(f, quote, carol) != fTot-fee {
		t.Fatalf("carol=%d want %d", bal(f, quote, carol), fTot-fee)
	}
	if f.set.total(quote) != beforeQuote {
		t.Fatal("quote not conserved with fee")
	}
}

func TestBatchManyToMany(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	// two sellers, two buyers, asymmetric quantities -> uniform price clear.
	f.set.fund(base, carol, 30)
	f.set.fund(base, dave, 50)
	f.set.fund(quote, alice, 1_000_000)
	f.set.fund(quote, bob, 1_000_000)
	f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 900_000, 30)
	f.place(t, mid, dave, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 50)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_100_000, 40)
	f.place(t, mid, bob, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 35)

	beforeBase := f.set.total(base)
	beforeQuote := f.set.total(quote)
	f.runBatch(t)

	// conservation must hold regardless of partition
	if f.set.total(base) != beforeBase || f.set.total(quote) != beforeQuote {
		t.Fatalf("not conserved base %d/%d quote %d/%d", f.set.total(base), beforeBase, f.set.total(quote), beforeQuote)
	}
	// base delivered to buyers == base removed from escrow
	delivered := bal(f, base, alice) + bal(f, base, bob)
	if delivered == 0 {
		t.Fatal("no base delivered")
	}
	// escrow base remaining == sum of unfilled sell quantities
	escBase := bal(f, base, keeper.EscrowAccount())
	if delivered+escBase != 80 {
		t.Fatalf("base accounting off: delivered=%d escrow=%d", delivered, escBase)
	}
}

func TestNoCrossNoClear(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(base, carol, 10)
	f.set.fund(quote, alice, 1_000_000)
	f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_200_000, 10)       // ask 1.2
	bid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 10) // bid 1.0
	f.runBatch(t)
	if f.order(t, bid).Status != types.OrderStatus_ORDER_STATUS_OPEN {
		t.Fatal("non-crossing order should remain open")
	}
	if bal(f, base, alice) != 0 {
		t.Fatal("no base should move")
	}
}

func TestGlobalPauseSkipsClearing(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(base, carol, 10)
	f.set.fund(quote, alice, 1_000_000)
	f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 10)
	bid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 10)
	p := types.DefaultParams()
	p.Paused = true
	f.srv.UpdateParams(f.ctx(), &types.MsgUpdateParams{Authority: authority, Params: p})
	f.runBatch(t)
	if f.order(t, bid).Status != types.OrderStatus_ORDER_STATUS_OPEN {
		t.Fatal("paused: no clearing expected")
	}
}

func TestCancelSanctionedBlocked(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(quote, alice, 1_000_000)
	oid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 1)
	f.cmp.sanctioned[alice] = true
	if _, err := f.srv.CancelOrder(f.ctx(), &types.MsgCancelOrder{Owner: alice, OrderId: oid}); err == nil {
		t.Fatal("sanctioned owner cancel must be blocked")
	}
}

// Regression: a resting order whose owner is sanctioned after placement must
// NOT settle in the batch (consistent with the blocked cancel refund); its
// escrow stays locked and the counterparty is left resting.
func TestSanctionedRestingOrderExcludedFromBatch(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 1_000_000)
	sid := f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	bid := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 100)

	// sanction the seller after they placed a crossing order
	f.cmp.sanctioned[carol] = true
	f.runBatch(t)

	// nothing settles
	if f.order(t, sid).Status != types.OrderStatus_ORDER_STATUS_OPEN || f.order(t, sid).Filled != 0 {
		t.Fatal("sanctioned seller order must not fill")
	}
	if f.order(t, bid).Status != types.OrderStatus_ORDER_STATUS_OPEN || f.order(t, bid).Filled != 0 {
		t.Fatal("buyer must remain open (no counterparty)")
	}
	if bal(f, base, alice) != 0 {
		t.Fatal("no base delivered while seller sanctioned")
	}
	// seller escrow stays locked
	if bal(f, base, keeper.EscrowAccount()) != 100 {
		t.Fatalf("seller escrow should stay locked, escrow base=%d", bal(f, base, keeper.EscrowAccount()))
	}

	// clearing the sanction lets the next batch settle normally
	f.cmp.sanctioned[carol] = false
	f.runBatch(t)
	if f.order(t, sid).Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatal("seller should fill after sanction cleared")
	}
}

// Regression: a settlement leg that reverts (e.g. an admin-frozen counterparty)
// must not busy-retry every block — the batch clock advances on failure — and
// must leave no partial state. Once the block clears, the market settles.
func TestSettlementFailureAdvancesClockAndRecovers(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 0)
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 1_000_000)
	sid := f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_000_000, 100)

	// freeze the buyer: delivering base to alice will revert, aborting the batch
	f.set.blocked[alice] = true
	before := f.set.total(base) + f.set.total(quote)
	f.runBatch(t)

	// no partial state committed
	if f.order(t, sid).Filled != 0 || f.order(t, sid).Status != types.OrderStatus_ORDER_STATUS_OPEN {
		t.Fatal("failed batch must not leave partial fill")
	}
	if f.set.total(base)+f.set.total(quote) != before {
		t.Fatal("failed batch must not move funds")
	}
	// clock advanced (no per-block busy loop)
	mk, _, _ := f.k.GetMarket(f.ctx(), mid)
	if mk.LastBatchTime != f.ts.Ctx.BlockTime().Unix() {
		t.Fatalf("batch clock not advanced on failure: %d", mk.LastBatchTime)
	}

	// unfreeze and clear next interval
	f.set.blocked[alice] = false
	f.runBatch(t)
	if f.order(t, sid).Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatal("should settle once unfrozen")
	}
	if bal(f, base, alice) != 100 {
		t.Fatalf("alice base=%d want 100", bal(f, base, alice))
	}
}

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	mid := f.createMarket(t, 50)
	f.set.fund(base, carol, 100)
	f.set.fund(quote, alice, 1_000_000)
	f.place(t, mid, carol, types.OrderSide_ORDER_SIDE_SELL, 1_000_000, 100)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 1_100_000, 40)

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}
	// new fixture; mirror escrow balances stableusd genesis would carry
	g := setup(t)
	want, err := gs.EscrowByDenom()
	if err != nil {
		t.Fatal(err)
	}
	for d, amt := range want {
		g.set.fund(d, keeper.EscrowAccount(), amt)
	}
	if err := g.k.InitGenesis(g.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	gs2, err := g.k.ExportGenesis(g.ctx())
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if len(gs2.Markets) != 1 || len(gs2.Orders) != 2 {
		t.Fatalf("roundtrip mismatch: %+v", gs2)
	}
	if gs2.OrderIdSeq != gs.OrderIdSeq || gs2.OrderPlacementSeq != gs.OrderPlacementSeq {
		t.Fatal("sequence mismatch")
	}
}

// ---- listing bond ------------------------------------------------------------

const bondDenom = "uecy"

func bondAmt(n int64) sdkmath.Int { return sdkmath.NewInt(n) }

// Full lifecycle: gov creates a market with an operator -> PENDING_BOND (no
// orders accepted), only the operator can post the bond -> ACTIVE, and a
// governance delist cancels orders, refunds escrow and returns the bond.
func TestListingBondLifecycle(t *testing.T) {
	f := setup(t)
	f.setListingBond(t, bondAmt(10_000), bondDenom)

	r, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: base, QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10, Operator: carol,
	})
	if err != nil {
		t.Fatalf("create market: %v", err)
	}
	mid := r.MarketId
	mk, _, _ := f.k.GetMarket(f.ctx(), mid)
	if mk.Status != types.MarketStatus_MARKET_STATUS_PENDING_BOND || mk.Operator != carol {
		t.Fatalf("want PENDING_BOND/carol got %s/%s", mk.Status, mk.Operator)
	}

	// no orders while awaiting the bond
	f.set.fund(quote, alice, 1_000_000)
	if _, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: alice, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: 500_000, Quantity: 2}); err == nil {
		t.Fatal("pending-bond market must reject orders")
	}
	// gov cannot bypass the bond via SetMarketStatus
	if _, err := f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Authority: authority, MarketId: mid, Status: types.MarketStatus_MARKET_STATUS_ACTIVE}); err == nil {
		t.Fatal("SetMarketStatus must not bypass the bond gate")
	}
	// only the designated operator may post
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: alice, MarketId: mid}); err == nil {
		t.Fatal("non-operator must not post the bond")
	}
	// operator without funds is rejected
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: carol, MarketId: mid}); err == nil {
		t.Fatal("underfunded operator must not post the bond")
	}

	carolAcc, _ := sdk.AccAddressFromBech32(carol)
	f.bank.fund(carolAcc, bondDenom, bondAmt(15_000))
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: carol, MarketId: mid}); err != nil {
		t.Fatalf("post bond: %v", err)
	}
	mk, _, _ = f.k.GetMarket(f.ctx(), mid)
	if mk.Status != types.MarketStatus_MARKET_STATUS_ACTIVE || !mk.BondAmount.Equal(bondAmt(10_000)) || mk.BondDenom != bondDenom {
		t.Fatalf("post-bond market wrong: %+v", mk)
	}
	if got := f.bank.GetBalance(f.ctx(), keeper.BondPoolAccount(), bondDenom).Amount; !got.Equal(bondAmt(10_000)) {
		t.Fatalf("bond pool holds %s want 10000", got)
	}
	// double post rejected (no longer pending)
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: carol, MarketId: mid}); err == nil {
		t.Fatal("double post must fail")
	}

	// trading works now; leave two resting orders for the delist to cancel
	f.set.fund(base, bob, 100)
	buyID := f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_BUY, 500_000, 2)
	sellID := f.place(t, mid, bob, types.OrderSide_ORDER_SIDE_SELL, 900_000, 50)

	// raising the param later must NOT change this market's refund
	f.setListingBond(t, bondAmt(99_999), bondDenom)

	// delist is authority-gated
	if _, err := f.srv.DelistMarket(f.ctx(), &types.MsgDelistMarket{Authority: carol, MarketId: mid}); err == nil {
		t.Fatal("non-authority must not delist")
	}
	resp, err := f.srv.DelistMarket(f.ctx(), &types.MsgDelistMarket{Authority: authority, MarketId: mid})
	if err != nil {
		t.Fatalf("delist: %v", err)
	}
	if resp.CancelledOrders != 2 || resp.Refunded != "10000" {
		t.Fatalf("delist resp: %+v", resp)
	}
	mk, _, _ = f.k.GetMarket(f.ctx(), mid)
	if mk.Status != types.MarketStatus_MARKET_STATUS_DELISTED || !mk.BondAmount.IsZero() {
		t.Fatalf("delisted market wrong: %+v", mk)
	}
	// orders cancelled with full refunds
	if f.order(t, buyID).Status != types.OrderStatus_ORDER_STATUS_CANCELLED || f.order(t, sellID).Status != types.OrderStatus_ORDER_STATUS_CANCELLED {
		t.Fatal("open orders must be cancelled on delist")
	}
	if bal(f, quote, alice) != 1_000_000 || bal(f, base, bob) != 100 {
		t.Fatalf("escrow not refunded: alice=%d bob=%d", bal(f, quote, alice), bal(f, base, bob))
	}
	// bond returned in full to the operator; pool empty
	if got := f.bank.get(carolAcc, bondDenom); !got.Equal(bondAmt(15_000)) {
		t.Fatalf("carol bond refund wrong: %s", got)
	}
	if got := f.bank.GetBalance(f.ctx(), keeper.BondPoolAccount(), bondDenom).Amount; !got.IsZero() {
		t.Fatalf("bond pool not emptied: %s", got)
	}
	// terminal: no re-delist, no status change, no orders
	if _, err := f.srv.DelistMarket(f.ctx(), &types.MsgDelistMarket{Authority: authority, MarketId: mid}); err == nil {
		t.Fatal("re-delist must fail")
	}
	if _, err := f.srv.SetMarketStatus(f.ctx(), &types.MsgSetMarketStatus{Authority: authority, MarketId: mid, Status: types.MarketStatus_MARKET_STATUS_ACTIVE}); err == nil {
		t.Fatal("DELISTED is terminal")
	}
	if _, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{Owner: alice, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: 500_000, Quantity: 2}); err == nil {
		t.Fatal("delisted market must reject orders")
	}
}

// CreateMarket requires an operator while the bond gate is on, and an RWA
// market's operator must be the token's admin (issuer).
func TestCreateMarketOperatorRules(t *testing.T) {
	f := setup(t)
	f.setListingBond(t, bondAmt(10_000), bondDenom)

	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: base, QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10,
	}); err == nil {
		t.Fatal("missing operator must fail while bond > 0")
	}

	f.asset.register(7, quote)
	f.asset.admin[7] = carol
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/7", QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10, Operator: alice,
	}); err == nil {
		t.Fatal("rwa operator must be the token admin")
	}
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/7", QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10, Operator: carol,
	}); err != nil {
		t.Fatalf("issuer operator rejected: %v", err)
	}
}

// If governance zeroes the bond while a market is pending, PostBond
// activates it without moving funds.
func TestPostBondAfterParamZeroed(t *testing.T) {
	f := setup(t)
	f.setListingBond(t, bondAmt(10_000), bondDenom)
	r, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: base, QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10, Operator: carol,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.setListingBond(t, sdkmath.ZeroInt(), bondDenom)
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: carol, MarketId: r.MarketId}); err != nil {
		t.Fatalf("post bond with zero param: %v", err)
	}
	mk, _, _ := f.k.GetMarket(f.ctx(), r.MarketId)
	if mk.Status != types.MarketStatus_MARKET_STATUS_ACTIVE || !mk.BondAmount.IsZero() {
		t.Fatalf("zero-bond activation wrong: %+v", mk)
	}
	if got := f.bank.GetBalance(f.ctx(), keeper.BondPoolAccount(), bondDenom).Amount; !got.IsZero() {
		t.Fatalf("no funds should move: %s", got)
	}
}

// Genesis: a bonded market roundtrips, and InitGenesis fails closed when the
// bond pool's bank balance does not match the recorded bonds.
func TestGenesisBondReconciliation(t *testing.T) {
	f := setup(t)
	f.setListingBond(t, bondAmt(10_000), bondDenom)
	r, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: base, QuoteDenom: quote,
		FeeBps: 0, MinBaseQty: 1, BatchInterval: 10, Operator: carol,
	})
	if err != nil {
		t.Fatal(err)
	}
	carolAcc, _ := sdk.AccAddressFromBech32(carol)
	f.bank.fund(carolAcc, bondDenom, bondAmt(10_000))
	if _, err := f.srv.PostBond(f.ctx(), &types.MsgPostBond{Operator: carol, MarketId: r.MarketId}); err != nil {
		t.Fatal(err)
	}

	gs, err := f.k.ExportGenesis(f.ctx())
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.Validate(); err != nil {
		t.Fatalf("exported genesis invalid: %v", err)
	}

	// fresh keeper WITHOUT the bank balance -> fail closed
	g := setup(t)
	if err := g.k.InitGenesis(g.ctx(), gs); err == nil {
		t.Fatal("init must fail when bond pool balance is missing")
	}

	// fund the pool -> import succeeds and the market keeps its bond
	h := setup(t)
	h.bank.fund(keeper.BondPoolAccount(), bondDenom, bondAmt(10_000))
	if err := h.k.InitGenesis(h.ctx(), gs); err != nil {
		t.Fatalf("init: %v", err)
	}
	mk, _, _ := h.k.GetMarket(h.ctx(), r.MarketId)
	if !mk.BondAmount.Equal(bondAmt(10_000)) || mk.BondDenom != bondDenom || mk.Operator != carol {
		t.Fatalf("bond lost in roundtrip: %+v", mk)
	}
}
