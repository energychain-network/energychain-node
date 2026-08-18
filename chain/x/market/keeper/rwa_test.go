package keeper_test

import (
	"strconv"
	"testing"

	"energychain/x/market/keeper"
	"energychain/x/market/types"
)

func (f *fixture) createRWAMarket(t *testing.T, tokenID uint64, feeBps uint32) uint64 {
	t.Helper()
	r, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/" + strconv.FormatUint(tokenID, 10), QuoteDenom: quote,
		FeeBps: feeBps, MinBaseQty: 1, BatchInterval: 10,
	})
	if err != nil {
		t.Fatalf("create rwa market: %v", err)
	}
	return r.MarketId
}

// A full secondary trade over an RWA base: the seller's units move through the
// compliant escrow to the buyer, the buyer's quote pays the seller, and the
// market escrow nets to zero on both legs.
func TestRWAMarketTrade(t *testing.T) {
	f := setup(t)
	f.asset.register(1, quote) // token 1 settles in `quote`
	f.asset.fund(1, alice, 100)
	f.set.fund(quote, bob, 100)

	mid := f.createRWAMarket(t, 1, 0)
	// price 1e6 == 1.0 quote per unit => 100 units cost 100 quote.
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_SELL, types.PriceScale, 100)
	f.place(t, mid, bob, types.OrderSide_ORDER_SIDE_BUY, types.PriceScale, 100)
	f.runBatch(t)

	if got := f.asset.MarketUnitBalance(f.ctx(), 1, bob); got != 100 {
		t.Fatalf("buyer units=%d want 100", got)
	}
	if got := f.asset.MarketUnitBalance(f.ctx(), 1, alice); got != 0 {
		t.Fatalf("seller units=%d want 0", got)
	}
	if got := f.asset.MarketUnitBalance(f.ctx(), 1, keeper.EscrowAccount()); got != 0 {
		t.Fatalf("escrow units=%d want 0", got)
	}
	if got := bal(f, quote, alice); got != 100 {
		t.Fatalf("seller quote=%d want 100", got)
	}
	if got := bal(f, quote, bob); got != 0 {
		t.Fatalf("buyer quote=%d want 0", got)
	}
}

// A buyer that loses delivery eligibility AFTER placement must be voided (quote
// refunded, order cancelled) at batch time rather than reverting the batch.
func TestRWAMarketVoidsNonDeliverableBuyer(t *testing.T) {
	f := setup(t)
	f.asset.register(1, quote)
	f.asset.fund(1, alice, 100)
	f.set.fund(quote, bob, 100)

	mid := f.createRWAMarket(t, 1, 0)
	f.place(t, mid, alice, types.OrderSide_ORDER_SIDE_SELL, types.PriceScale, 100)
	bid := f.place(t, mid, bob, types.OrderSide_ORDER_SIDE_BUY, types.PriceScale, 100)

	// Eligibility revoked between placement and clearing.
	f.asset.denyRecv[bob] = true
	f.runBatch(t)

	o := f.order(t, bid)
	if o.Status != types.OrderStatus_ORDER_STATUS_CANCELLED {
		t.Fatalf("buy status=%v want CANCELLED", o.Status)
	}
	if got := bal(f, quote, bob); got != 100 {
		t.Fatalf("buyer refund quote=%d want 100", got)
	}
	if got := f.asset.MarketUnitBalance(f.ctx(), 1, bob); got != 0 {
		t.Fatalf("buyer units=%d want 0", got)
	}
	// Seller's units remain escrowed (their sell still rests).
	if got := f.asset.MarketUnitBalance(f.ctx(), 1, keeper.EscrowAccount()); got != 100 {
		t.Fatalf("escrow units=%d want 100", got)
	}
}

// A buyer over the per-holder cap is rejected at placement time.
func TestRWAMarketRejectsOverCapBuyerAtPlacement(t *testing.T) {
	f := setup(t)
	f.asset.register(1, quote)
	f.asset.cap[1] = 50 // per-holder cap of 50 units
	f.set.fund(quote, bob, 100)

	mid := f.createRWAMarket(t, 1, 0)
	_, err := f.srv.PlaceOrder(f.ctx(), &types.MsgPlaceOrder{
		Owner: bob, MarketId: mid, Side: types.OrderSide_ORDER_SIDE_BUY, Price: types.PriceScale, Quantity: 100,
	})
	if err == nil {
		t.Fatal("expected over-cap buy to be rejected at placement")
	}
	// No quote should have been escrowed.
	if got := bal(f, quote, bob); got != 100 {
		t.Fatalf("buyer quote=%d want 100 (untouched)", got)
	}
}

// CreateMarket must reject an RWA base whose quote is not the token's
// settlement denom, and an unknown token id.
func TestRWAMarketCreateValidation(t *testing.T) {
	f := setup(t)
	f.asset.register(1, quote)

	// wrong quote denom
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/1", QuoteDenom: base, FeeBps: 0, MinBaseQty: 1, BatchInterval: 10,
	}); err == nil {
		t.Fatal("expected reject: quote != token settlement denom")
	}
	// unknown token
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/99", QuoteDenom: quote, FeeBps: 0, MinBaseQty: 1, BatchInterval: 10,
	}); err == nil {
		t.Fatal("expected reject: unknown rwa token")
	}
	// valid
	if _, err := f.srv.CreateMarket(f.ctx(), &types.MsgCreateMarket{
		Authority: authority, BaseDenom: "rwa/1", QuoteDenom: quote, FeeBps: 0, MinBaseQty: 1, BatchInterval: 10,
	}); err != nil {
		t.Fatalf("valid rwa market rejected: %v", err)
	}
}
