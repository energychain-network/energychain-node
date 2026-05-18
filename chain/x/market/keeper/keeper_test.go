package keeper_test

import (
	"fmt"
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

	"energychain/x/market/keeper"
	"energychain/x/market/types"
)

// ---- Stubs ---------------------------------------------------------------

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

// ---- Setup ---------------------------------------------------------------

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

	baseDenom  = "kwh"
	quoteDenom = "usd"
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
	sc.denoms[baseDenom] = true
	sc.denoms[quoteDenom] = true
	k := keeper.NewKeeper(cdc, runtime.NewKVStoreService(storeKey), authority, san, sc, nil)
	ctx := sdk.NewContext(cms, cmtproto.Header{}, false, log.NewNopLogger()).
		WithBlockHeight(10).
		WithBlockTime(time.Unix(1_700_000_000, 0))
	if err := k.SetParams(ctx, types.DefaultParams()); err != nil {
		t.Fatal(err)
	}
	return &fixture{k: k, ctx: ctx, srv: keeper.NewMsgServerImpl(k), san: san, sc: sc}
}

func (f *fixture) advance(seconds int64) {
	f.ctx = f.ctx.WithBlockTime(f.ctx.BlockTime().Add(time.Duration(seconds) * time.Second))
}

// ---- helpers -------------------------------------------------------------

func createContinuousPair(t *testing.T, f *fixture) uint64 {
	t.Helper()
	r, err := f.srv.CreatePair(f.ctx, &types.MsgCreatePair{
		Authority: authority, BaseDenom: baseDenom, QuoteDenom: quoteDenom,
		Mode: types.MatchMode_MATCH_MODE_CONTINUOUS,
	})
	if err != nil {
		t.Fatalf("create pair: %v", err)
	}
	return r.PairId
}

func createFBAPair(t *testing.T, f *fixture, interval int64) uint64 {
	t.Helper()
	r, err := f.srv.CreatePair(f.ctx, &types.MsgCreatePair{
		Authority: authority, BaseDenom: baseDenom, QuoteDenom: quoteDenom,
		Mode:                 types.MatchMode_MATCH_MODE_FBA,
		BatchIntervalSeconds: interval,
	})
	if err != nil {
		t.Fatalf("create fba pair: %v", err)
	}
	return r.PairId
}

func placeBuy(t *testing.T, f *fixture, pairID uint64, owner string, price, qty uint64) (uint64, uint64, uint64) {
	t.Helper()
	// Credit owner with the quote escrow they will need.
	quote, err := types.QuoteForFill(price, qty)
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	f.sc.credit(quoteDenom, owner, quote)
	r, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: owner, PairId: pairID, Side: types.Side_SIDE_BUY,
		Price: price, Quantity: qty,
	})
	if err != nil {
		t.Fatalf("place buy: %v", err)
	}
	return r.OrderId, r.FilledQty, r.RemainingQty
}

func placeSell(t *testing.T, f *fixture, pairID uint64, owner string, price, qty uint64) (uint64, uint64, uint64) {
	t.Helper()
	f.sc.credit(baseDenom, owner, qty)
	r, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: owner, PairId: pairID, Side: types.Side_SIDE_SELL,
		Price: price, Quantity: qty,
	})
	if err != nil {
		t.Fatalf("place sell: %v", err)
	}
	return r.OrderId, r.FilledQty, r.RemainingQty
}

// ---- tests --------------------------------------------------------------

func TestCreatePairRejectsBadInputs(t *testing.T) {
	f := setup(t)
	tests := []struct {
		name string
		mut  func(m *types.MsgCreatePair)
	}{
		{"missing authority", func(m *types.MsgCreatePair) { m.Authority = "" }},
		{"bad authority", func(m *types.MsgCreatePair) { m.Authority = mkAddr(99) }},
		{"same denoms", func(m *types.MsgCreatePair) { m.QuoteDenom = m.BaseDenom }},
		{"bad mode", func(m *types.MsgCreatePair) { m.Mode = types.MatchMode_MATCH_MODE_UNSPECIFIED }},
		{"missing fba interval", func(m *types.MsgCreatePair) {
			m.Mode = types.MatchMode_MATCH_MODE_FBA
			m.BatchIntervalSeconds = 0
		}},
		{"bad price band", func(m *types.MsgCreatePair) {
			m.PriceBandLo = 200
			m.PriceBandHi = 100
		}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := &types.MsgCreatePair{
				Authority: authority, BaseDenom: baseDenom, QuoteDenom: quoteDenom,
				Mode: types.MatchMode_MATCH_MODE_CONTINUOUS,
			}
			tc.mut(m)
			if _, err := f.srv.CreatePair(f.ctx, m); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCreatePairRefusesNonAuthority(t *testing.T) {
	f := setup(t)
	_, err := f.srv.CreatePair(f.ctx, &types.MsgCreatePair{
		Authority: stranger, BaseDenom: baseDenom, QuoteDenom: quoteDenom,
		Mode: types.MatchMode_MATCH_MODE_CONTINUOUS,
	})
	if err == nil {
		t.Fatal("expected authority error")
	}
}

func TestPauseUnpause(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	_, err := f.srv.PauseUnpausePair(f.ctx, &types.MsgPauseUnpausePair{
		Authority: authority, PairId: pid, Pause: true, Reason: "drill",
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := f.k.MustGetPair(f.ctx, pid)
	if p.Status != types.PairStatus_PAIR_STATUS_PAUSED {
		t.Fatalf("expected PAUSED, got %s", p.Status)
	}
	// Placing on a paused pair is refused.
	f.sc.credit(quoteDenom, alice, 1_000_000)
	_, err = f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 1,
	})
	if err == nil {
		t.Fatal("expected refusal on paused pair")
	}
	// Unpause and try again.
	if _, err := f.srv.PauseUnpausePair(f.ctx, &types.MsgPauseUnpausePair{
		Authority: authority, PairId: pid, Pause: false,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 1,
	}); err != nil {
		t.Fatalf("place after unpause: %v", err)
	}
}

func TestUpdatePairRiskGovernsPriceBands(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	if _, err := f.srv.UpdatePairRisk(f.ctx, &types.MsgUpdatePairRisk{
		Authority: authority, PairId: pid,
		PriceBandLo: types.PriceScale, PriceBandHi: types.PriceScale * 10,
	}); err != nil {
		t.Fatal(err)
	}
	f.sc.credit(quoteDenom, alice, 1_000_000_000)
	// Below band — refused.
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale / 2, Quantity: 1,
	})
	if err == nil {
		t.Fatal("expected band-lo refusal")
	}
	// Within band — accepted.
	if _, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale * 2, Quantity: 1,
	}); err != nil {
		t.Fatalf("within band: %v", err)
	}
}

func TestPlaceOrderEscrowsAndRefundsOnCancel(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	id, filled, rem := placeBuy(t, f, pid, alice, types.PriceScale*2, 10)
	if filled != 0 || rem != 10 {
		t.Fatalf("expected unfilled, got filled=%d rem=%d", filled, rem)
	}
	// Pool holds the quote escrow.
	expectedEscrow, _ := types.QuoteForFill(types.PriceScale*2, 10)
	if f.sc.get(quoteDenom, keeper.PoolAddress()) != expectedEscrow {
		t.Fatalf("pool escrow mismatch: %d != %d", f.sc.get(quoteDenom, keeper.PoolAddress()), expectedEscrow)
	}
	// Cancel returns the escrow.
	if _, err := f.srv.CancelOrder(f.ctx, &types.MsgCancelOrder{
		Owner: alice, OrderId: id, Reason: "free",
	}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if f.sc.get(quoteDenom, alice) != expectedEscrow {
		t.Fatalf("alice not refunded: have=%d want=%d", f.sc.get(quoteDenom, alice), expectedEscrow)
	}
	if f.sc.get(quoteDenom, keeper.PoolAddress()) != 0 {
		t.Fatalf("pool not drained: %d", f.sc.get(quoteDenom, keeper.PoolAddress()))
	}
}

func TestPlaceOrderRefusesSanctionedOwner(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	f.san.bad[alice] = true
	f.sc.credit(quoteDenom, alice, 1_000_000)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected sanctioned err, got %v", err)
	}
}

func TestContinuousCrossingFillFullyAtMakerPrice(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// Maker SELL 10 @ 1.0
	sellID, _, _ := placeSell(t, f, pid, bob, types.PriceScale, 10)
	// Taker BUY 10 @ 2.0 -- should fully fill at MAKER price (1.0).
	_, filled, rem := placeBuy(t, f, pid, alice, types.PriceScale*2, 10)
	if filled != 10 || rem != 0 {
		t.Fatalf("expected full fill, got filled=%d rem=%d", filled, rem)
	}
	if f.sc.get(baseDenom, alice) != 10 {
		t.Fatalf("alice base mismatch: %d", f.sc.get(baseDenom, alice))
	}
	// Seller receives 10 USD (price 1.0).
	if f.sc.get(quoteDenom, bob) != 10 {
		t.Fatalf("bob quote mismatch: %d", f.sc.get(quoteDenom, bob))
	}
	// Buyer should have refunded the price improvement (2.0 - 1.0)*10 = 10 USD.
	if f.sc.get(quoteDenom, alice) != 10 {
		t.Fatalf("alice price-improvement refund mismatch: %d", f.sc.get(quoteDenom, alice))
	}
	// Maker is FILLED.
	mk, _ := f.k.MustGetOrder(f.ctx, sellID)
	if mk.Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatalf("maker not FILLED: %s", mk.Status)
	}
}

func TestContinuousPartialFillRestsResidual(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	placeSell(t, f, pid, bob, types.PriceScale, 4)
	id, filled, rem := placeBuy(t, f, pid, alice, types.PriceScale, 10)
	if filled != 4 || rem != 6 {
		t.Fatalf("expected partial 4/6, got %d/%d", filled, rem)
	}
	o, _ := f.k.MustGetOrder(f.ctx, id)
	if o.Status != types.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED {
		t.Fatalf("expected PARTIALLY_FILLED, got %s", o.Status)
	}
}

func TestContinuousNoCrossDoesNotMatch(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	placeSell(t, f, pid, bob, types.PriceScale*2, 5) // ask 2.0
	_, filled, rem := placeBuy(t, f, pid, alice, types.PriceScale, 5) // bid 1.0
	if filled != 0 || rem != 5 {
		t.Fatalf("expected no fill, got %d/%d", filled, rem)
	}
}

func TestPriceTimePriority(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// Two sellers post the same price; bob first, then carol.
	sid1, _, _ := placeSell(t, f, pid, bob, types.PriceScale, 3)
	f.advance(1)
	sid2, _, _ := placeSell(t, f, pid, carol, types.PriceScale, 3)
	// Buyer takes 4 units. Bob (3) fully, carol (1).
	_, filled, _ := placeBuy(t, f, pid, alice, types.PriceScale, 4)
	if filled != 4 {
		t.Fatalf("filled=%d", filled)
	}
	o1, _ := f.k.MustGetOrder(f.ctx, sid1)
	o2, _ := f.k.MustGetOrder(f.ctx, sid2)
	if o1.Status != types.OrderStatus_ORDER_STATUS_FILLED {
		t.Fatalf("first maker not FILLED: %s", o1.Status)
	}
	if o2.Status != types.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED || o2.RemainingQty != 2 {
		t.Fatalf("second maker bad state: %s rem=%d", o2.Status, o2.RemainingQty)
	}
}

func TestBestPriceFirst(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// Two sellers — higher first, lower second. Buyer should hit the lower.
	placeSell(t, f, pid, bob, types.PriceScale*3, 5)   // ask 3.0
	f.advance(1)
	placeSell(t, f, pid, carol, types.PriceScale*2, 5) // ask 2.0
	// Buy 1 unit @ 4.0 — must hit carol at 2.0.
	placeBuy(t, f, pid, alice, types.PriceScale*4, 1)
	if f.sc.get(quoteDenom, carol) != 2 {
		t.Fatalf("carol not paid first: have=%d", f.sc.get(quoteDenom, carol))
	}
	if f.sc.get(quoteDenom, bob) != 0 {
		t.Fatalf("bob should not be paid yet: have=%d", f.sc.get(quoteDenom, bob))
	}
}

func TestCancelOnlyByOwner(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	id, _, _ := placeBuy(t, f, pid, alice, types.PriceScale, 1)
	_, err := f.srv.CancelOrder(f.ctx, &types.MsgCancelOrder{
		Owner: bob, OrderId: id, Reason: "x",
	})
	if err == nil {
		t.Fatal("expected ownership check")
	}
}

func TestCancelOnFilledIsRejected(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	placeSell(t, f, pid, bob, types.PriceScale, 1)
	id, _, _ := placeBuy(t, f, pid, alice, types.PriceScale, 1)
	if _, err := f.srv.CancelOrder(f.ctx, &types.MsgCancelOrder{
		Owner: alice, OrderId: id, Reason: "x",
	}); err == nil {
		t.Fatal("expected refusal on FILLED order")
	}
}

func TestPositionMagnitudeAndSign(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	placeSell(t, f, pid, bob, types.PriceScale, 5)
	placeBuy(t, f, pid, alice, types.PriceScale, 5)
	pos, err := f.k.GetPosition(f.ctx, pid, alice)
	if err != nil {
		t.Fatal(err)
	}
	if pos.Magnitude != 5 || pos.IsShort {
		t.Fatalf("alice pos: mag=%d short=%v", pos.Magnitude, pos.IsShort)
	}
	pos2, _ := f.k.GetPosition(f.ctx, pid, bob)
	if pos2.Magnitude != 5 || !pos2.IsShort {
		t.Fatalf("bob pos: mag=%d short=%v", pos2.Magnitude, pos2.IsShort)
	}
}

func TestPositionCapEnforced(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	if _, err := f.srv.UpdatePairRisk(f.ctx, &types.MsgUpdatePairRisk{
		Authority: authority, PairId: pid, MaxPositionPerUser: 3,
	}); err != nil {
		t.Fatal(err)
	}
	placeSell(t, f, pid, bob, types.PriceScale, 10)
	// Alice cannot buy 5 — exceeds cap of 3.
	f.sc.credit(quoteDenom, alice, 100)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "position cap") {
		t.Fatalf("expected position cap err, got %v", err)
	}
}

func TestFBAClearAtMidpoint(t *testing.T) {
	f := setup(t)
	pid := createFBAPair(t, f, 60)
	// BUY 5 @ 2.0
	placeBuy(t, f, pid, alice, types.PriceScale*2, 5)
	// SELL 5 @ 1.0
	placeSell(t, f, pid, bob, types.PriceScale, 5)
	// Advance past batch close.
	f.advance(60)
	r, err := f.srv.ClearBatch(f.ctx, &types.MsgClearBatch{Caller: carol, PairId: pid})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if r.MatchedPairs != 1 {
		t.Fatalf("matched=%d", r.MatchedPairs)
	}
	// Midpoint = 1.5 USD/kWh × 5 kWh = 7.5 USD. Rounded by PriceScale grid.
	if f.sc.get(quoteDenom, bob) != 7 && f.sc.get(quoteDenom, bob) != 8 {
		t.Fatalf("bob clearing payout unexpected: %d", f.sc.get(quoteDenom, bob))
	}
	if f.sc.get(baseDenom, alice) != 5 {
		t.Fatalf("alice base recv unexpected: %d", f.sc.get(baseDenom, alice))
	}
	if !r.BatchDone {
		t.Fatal("batch not done")
	}
}

func TestFBARefusesContinuousPair(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	_, err := f.srv.ClearBatch(f.ctx, &types.MsgClearBatch{Caller: alice, PairId: pid})
	if err == nil {
		t.Fatal("expected refusal on continuous pair")
	}
}

func TestFBAEarlyClearRefused(t *testing.T) {
	f := setup(t)
	pid := createFBAPair(t, f, 60)
	// Before batch close, ClearBatch should refuse.
	_, err := f.srv.ClearBatch(f.ctx, &types.MsgClearBatch{Caller: alice, PairId: pid})
	if err == nil {
		t.Fatal("expected refusal: batch not yet closed")
	}
}

func TestFBANonCrossEmptyClear(t *testing.T) {
	f := setup(t)
	pid := createFBAPair(t, f, 60)
	placeBuy(t, f, pid, alice, types.PriceScale, 5)
	placeSell(t, f, pid, bob, types.PriceScale*2, 5)
	f.advance(60)
	r, err := f.srv.ClearBatch(f.ctx, &types.MsgClearBatch{Caller: carol, PairId: pid})
	if err != nil {
		t.Fatal(err)
	}
	if r.MatchedPairs != 0 || !r.BatchDone {
		t.Fatalf("unexpected: matched=%d done=%v", r.MatchedPairs, r.BatchDone)
	}
}

func TestPlaceRefusesPriceOverflowingMaxPrice(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	f.sc.credit(quoteDenom, alice, 1)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.MaxPrice + 1, Quantity: 1,
	})
	if err == nil {
		t.Fatal("expected MaxPrice refusal")
	}
}

func TestPlaceRefusesNotionalOverflow(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// price * qty overflow inside QuoteForFill.
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.MaxPrice / 2, Quantity: 1 << 8,
	})
	if err == nil {
		t.Fatal("expected overflow refusal")
	}
}

func TestQueryHelpers(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	id, _, _ := placeBuy(t, f, pid, alice, types.PriceScale, 1)
	qs := keeper.NewQueryServerImpl(f.k)
	if r, err := qs.Params(f.ctx, &types.QueryParamsRequest{}); err != nil || r.Params.MaxPairs == 0 {
		t.Fatalf("params: %v %v", r, err)
	}
	if r, err := qs.Pair(f.ctx, &types.QueryPairRequest{Id: pid}); err != nil || r.Pair.Id != pid {
		t.Fatalf("pair: %v %v", r, err)
	}
	if r, err := qs.Order(f.ctx, &types.QueryOrderRequest{Id: id}); err != nil || r.Order.Id != id {
		t.Fatalf("order: %v %v", r, err)
	}
	if r, err := qs.OpenOrders(f.ctx, &types.QueryOpenOrdersRequest{PairId: pid}); err != nil || len(r.Orders) != 1 {
		t.Fatalf("open: %v %v", r, err)
	}
	if r, err := qs.OrdersByOwner(f.ctx, &types.QueryOrdersByOwnerRequest{Owner: alice}); err != nil || len(r.Orders) != 1 {
		t.Fatalf("by-owner: %v %v", r, err)
	}
	if r, err := qs.PoolAddress(f.ctx, &types.QueryPoolAddressRequest{}); err != nil || r.Pool != keeper.PoolAddress() {
		t.Fatalf("pool: %v %v", r, err)
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

func TestGenesisRoundTrip(t *testing.T) {
	f := setup(t)
	createContinuousPair(t, f)
	gs, err := f.k.ExportGenesis(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs.Pairs) != 1 || gs.NextPairId != 2 {
		t.Fatalf("export shape: %+v", gs)
	}
	// Fresh keeper imports without error.
	f2 := setup(t)
	if err := f2.k.InitGenesis(f2.ctx, gs); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := f2.k.MustGetPair(f2.ctx, 1); err != nil {
		t.Fatalf("missing pair after import: %v", err)
	}
}

func TestMaxOpenOrdersPerPairCap(t *testing.T) {
	f := setup(t)
	// Tighten the cap.
	p := types.DefaultParams()
	p.MaxOpenOrdersPerPair = 2
	if err := f.k.SetParams(f.ctx, p); err != nil {
		t.Fatal(err)
	}
	pid := createContinuousPair(t, f)
	placeSell(t, f, pid, bob, types.PriceScale, 1)
	placeSell(t, f, pid, carol, types.PriceScale*2, 1)
	f.sc.credit(quoteDenom, dave, 1_000_000)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: dave, PairId: pid, Side: types.Side_SIDE_SELL,
		Price: types.PriceScale * 3, Quantity: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Fatalf("expected pair cap err, got %v", err)
	}
}

func TestStablecoinFreezeBlocksPlace(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	f.sc.blocked[quoteDenom] = map[string]bool{alice: true}
	f.sc.credit(quoteDenom, alice, 1_000_000)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected stablecoin freeze, got %v", err)
	}
}

func TestPausedDenomBlocksPlace(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	f.sc.pausedDenom[quoteDenom] = true
	f.sc.credit(quoteDenom, alice, 1_000_000)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "paused") {
		t.Fatalf("expected denom paused, got %v", err)
	}
}

// TestNoPoolLeakageOnPartialFillPlusCancel exercises the
// floor-divide subadditivity gap that the EscrowLocked field
// is designed to close: with a non-PriceScale-aligned price,
// `QuoteForFill(price, q1) + QuoteForFill(price, q2)` can be
// less than `QuoteForFill(price, q1+q2)`, which would leak
// micro-units into the pool on cancel if we re-derived the
// refund from remaining_qty. EscrowLocked tracks the exact
// residue, so the pool must end at zero after a partial fill
// + cancel sequence.
func TestNoPoolLeakageOnPartialFillPlusCancel(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// price 1.999999 (one micro-unit shy of 2.0), so that
	// floor(price*1) + floor(price*2) = 1 + 3 = 4
	// while floor(price*3)            = 5 — i.e. residue 1.
	price := types.PriceScale*2 - 1
	// Maker sells 3 at 1.0 — taker BUY at price will fill 1
	// then 2 by way of two separate sells to exercise the
	// truncation gap on the BUY side's escrow consumption.
	placeSell(t, f, pid, bob, types.PriceScale, 1)
	placeSell(t, f, pid, carol, types.PriceScale, 2)
	id, _, _ := placeBuy(t, f, pid, alice, price, 3)
	// Order is now fully filled. Cancel should be a no-op
	// (already terminal) — but verify the pool drained to
	// exactly zero (i.e. no residue from buyer-leg truncation).
	if _, err := f.srv.CancelOrder(f.ctx, &types.MsgCancelOrder{
		Owner: alice, OrderId: id, Reason: "x",
	}); err == nil {
		t.Fatal("expected refusal on FILLED order")
	}
	if got := f.sc.get(quoteDenom, keeper.PoolAddress()); got != 0 {
		t.Fatalf("pool leakage on full fill: %d", got)
	}
}

// TestPartialFillThenCancelRefundsExactEscrow ensures that
// when an order is partially filled and then cancelled, the
// owner is refunded the EXACT amount still locked — not a
// re-derived QuoteForFill(price, remaining_qty) which would
// over- or under-pay by truncation residue.
func TestPartialFillThenCancelRefundsExactEscrow(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	price := types.PriceScale*2 - 1 // 1.999999
	// Maker sells just 1 unit at 1.0; buyer wants 3.
	placeSell(t, f, pid, bob, types.PriceScale, 1)
	id, filled, rem := placeBuy(t, f, pid, alice, price, 3)
	if filled != 1 || rem != 2 {
		t.Fatalf("expected partial 1/2, got %d/%d", filled, rem)
	}
	// Cancel.
	pre := f.sc.get(quoteDenom, alice)
	if _, err := f.srv.CancelOrder(f.ctx, &types.MsgCancelOrder{
		Owner: alice, OrderId: id, Reason: "x",
	}); err != nil {
		t.Fatal(err)
	}
	post := f.sc.get(quoteDenom, alice)
	if got := f.sc.get(quoteDenom, keeper.PoolAddress()); got != 0 {
		t.Fatalf("pool leakage on partial fill + cancel: %d", got)
	}
	if post <= pre {
		t.Fatal("expected refund credit to alice")
	}
}

func TestSellerOnBookBecomesSanctionedRefusesFill(t *testing.T) {
	f := setup(t)
	pid := createContinuousPair(t, f)
	// Bob posts a resting SELL.
	placeSell(t, f, pid, bob, types.PriceScale, 5)
	// Bob is added to OFAC mid-flight.
	f.san.bad[bob] = true
	// Alice tries to take. The applyFill chain re-checks sanctions
	// on the maker (seller) and must refuse to consummate.
	f.sc.credit(quoteDenom, alice, 1_000_000)
	_, err := f.srv.PlaceLimitOrder(f.ctx, &types.MsgPlaceLimitOrder{
		Owner: alice, PairId: pid, Side: types.Side_SIDE_BUY,
		Price: types.PriceScale, Quantity: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "sanctioned") {
		t.Fatalf("expected fill-time sanctions refusal, got %v", err)
	}
}
