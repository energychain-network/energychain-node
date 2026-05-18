package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/market/types"
)

// matchContinuous is the per-order continuous matcher invoked
// by PlaceLimitOrder. It walks the opposite side's book in
// price-time priority, fills against orders whose price is
// acceptable to the taker, and stops when the taker is filled
// or the opposite side no longer crosses.
//
// Fill price = MAKER price. The taker gets the maker's posted
// price (price improvement for the taker). This is the standard
// CEX convention and is essential to make the order-book mode
// useful (otherwise large taker orders would arbitrage the book
// against themselves).
//
// Returns the filled quantity. Caller is responsible for
// resting the residual (if any) on the same side's book.
func (k Keeper) matchContinuous(ctx context.Context, pair types.Pair, taker *types.Order) (uint64, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return 0, err
	}
	if taker.RemainingQty == 0 {
		return 0, nil
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	var (
		filled    uint64
		fills     uint32
		remaining = taker.RemainingQty
	)
	// We need to collect the keys to remove from the book after
	// the walk so we don't mutate the keyset we're iterating.
	// `remaining` is a local copy of taker.RemainingQty; the
	// keeper.applyFill call below is what actually decrements
	// the taker's order — we MUST NOT decrement here too or we
	// would underflow taker.RemainingQty in the caller.
	type fillRec struct {
		makerID    uint64
		makerPrice uint64
		fillQty    uint64
	}
	var pending []fillRec

	walk := func(makerID uint64) (stop bool, err error) {
		if fills >= p.MaxFillsPerMatch {
			return true, nil
		}
		maker, err := k.MustGetOrder(ctx, makerID)
		if err != nil {
			return true, err
		}
		if maker.Status.IsTerminal() || maker.RemainingQty == 0 {
			return false, nil
		}
		// Cross check.
		switch taker.Side {
		case types.Side_SIDE_BUY:
			if taker.Price < maker.Price {
				return true, nil
			}
		case types.Side_SIDE_SELL:
			if taker.Price > maker.Price {
				return true, nil
			}
		}
		fillQty := remaining
		if maker.RemainingQty < fillQty {
			fillQty = maker.RemainingQty
		}
		pending = append(pending, fillRec{makerID: maker.Id, makerPrice: maker.Price, fillQty: fillQty})
		remaining -= fillQty
		filled += fillQty
		fills++
		if remaining == 0 {
			return true, nil
		}
		return false, nil
	}

	if taker.Side == types.Side_SIDE_BUY {
		rng := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pair.Id)
		if err := k.SellBook.Walk(ctx, rng, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			return walk(key.K3())
		}); err != nil {
			return 0, err
		}
	} else {
		rng := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pair.Id)
		if err := k.BuyBook.Walk(ctx, rng, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			return walk(key.K3())
		}); err != nil {
			return 0, err
		}
	}

	// Apply pending fills (now safe to mutate state).
	for _, f := range pending {
		if err := k.applyFill(ctx, pair, taker, f.makerID, f.makerPrice, f.fillQty, now); err != nil {
			return 0, err
		}
	}
	return filled, nil
}

// applyFill consummates a single fill leg. It:
//  1. drains the buyer's locked quote (price*qty) from pool to seller
//  2. drains the seller's locked base (qty) from pool to buyer
//  3. updates both orders' remaining qty / cumulative_quote / status
//  4. removes the maker from the book if it is now FILLED
//  5. updates positions on both sides (with per-pair cap check)
func (k Keeper) applyFill(
	ctx context.Context,
	pair types.Pair,
	taker *types.Order,
	makerID uint64,
	fillPrice uint64,
	fillQty uint64,
	now int64,
) error {
	maker, err := k.MustGetOrder(ctx, makerID)
	if err != nil {
		return err
	}
	quote, err := types.QuoteForFill(fillPrice, fillQty)
	if err != nil {
		return err
	}
	// Identify buyer / seller from sides.
	var buyer, seller *types.Order
	switch taker.Side {
	case types.Side_SIDE_BUY:
		buyer, seller = taker, &maker
	case types.Side_SIDE_SELL:
		buyer, seller = &maker, taker
	default:
		return fmt.Errorf("invalid side")
	}
	// Sanctions re-check on both counterparties at every fill.
	// Defends against a counterparty being added to OFAC after
	// they posted a resting order.
	if err := k.requireUnsanctioned(ctx, buyer.Owner, "buyer"); err != nil {
		return err
	}
	if err := k.requireUnsanctioned(ctx, seller.Owner, "seller"); err != nil {
		return err
	}
	// Per-pair cap check on the resulting position. We compute
	// the prospective position WITHOUT mutating, then commit
	// only after both legs pass.
	if pair.MaxPositionPerUser > 0 {
		if err := k.preflightPositionCap(ctx, pair, buyer.Owner, types.Side_SIDE_BUY, fillQty); err != nil {
			return err
		}
		if err := k.preflightPositionCap(ctx, pair, seller.Owner, types.Side_SIDE_SELL, fillQty); err != nil {
			return err
		}
	}
	// Pool -> seller: quote leg.
	if err := k.drainPool(ctx, pair.QuoteDenom, seller.Owner, quote); err != nil {
		return err
	}
	// Pool -> buyer: base leg.
	if err := k.drainPool(ctx, pair.BaseDenom, buyer.Owner, fillQty); err != nil {
		return err
	}
	// Escrow accounting for the BUYER, using a telescoping
	// cumulative formulation:
	//
	//   buyerLockedConsumed =
	//     QuoteForFill(buyer.Price, filled_after)
	//   - QuoteForFill(buyer.Price, filled_before)
	//
	// Summed across all fills this telescopes to exactly the
	// pre-escrow `QuoteForFill(buyer.Price, buyer.Quantity)`,
	// so the order's locked balance reaches zero precisely on
	// the final fill — no floor-divide residue is left in the
	// pool. (Naively recomputing QuoteForFill(buyer.Price,
	// fillQty) per leg would leave up to 1 micro-unit per fill
	// trapped.) Refund to buyer = consumed - drained, which is
	// non-negative by the subadditivity of floor.
	preFilled := buyer.FilledQty
	postFilled, err := types.SafeAdd(preFilled, fillQty)
	if err != nil {
		return err
	}
	preLocked, err := types.QuoteForFill(buyer.Price, preFilled)
	if err != nil {
		return err
	}
	postLocked, err := types.QuoteForFill(buyer.Price, postFilled)
	if err != nil {
		return err
	}
	if postLocked < preLocked {
		return fmt.Errorf("internal: locked telescope went backward (%d -> %d)", preLocked, postLocked)
	}
	buyerLockedConsumed := postLocked - preLocked
	if buyerLockedConsumed < quote {
		return fmt.Errorf("internal: buyer locked %d < drain %d", buyerLockedConsumed, quote)
	}
	if refund := buyerLockedConsumed - quote; refund > 0 {
		if err := k.drainPool(ctx, pair.QuoteDenom, buyer.Owner, refund); err != nil {
			return err
		}
	}
	if buyer.EscrowLocked < buyerLockedConsumed {
		return fmt.Errorf("internal: buyer escrow underflow %d < %d", buyer.EscrowLocked, buyerLockedConsumed)
	}
	buyer.EscrowLocked -= buyerLockedConsumed
	if seller.EscrowLocked < fillQty {
		return fmt.Errorf("internal: seller escrow underflow %d < %d", seller.EscrowLocked, fillQty)
	}
	seller.EscrowLocked -= fillQty
	// Mutate orders.
	buyer.RemainingQty -= fillQty
	seller.RemainingQty -= fillQty
	buyer.FilledQty += fillQty
	seller.FilledQty += fillQty
	buyer.CumulativeQuote, _ = types.SafeAdd(buyer.CumulativeQuote, quote)
	seller.CumulativeQuote, _ = types.SafeAdd(seller.CumulativeQuote, quote)
	buyer.LastFilledAt = now
	seller.LastFilledAt = now
	updateStatusAfterFill(buyer)
	updateStatusAfterFill(seller)

	// Persist (and book-prune) the maker if it just terminated.
	// The telescoping locked computation above guarantees the
	// pool holds exactly zero residue per side after each
	// fill, so no residue refund is required at terminal
	// transition — an invariant exercised by
	// TestNoPoolLeakageOnPartialFillPlusCancel.
	if seller == &maker || buyer == &maker {
		if maker.Status.IsTerminal() {
			if err := k.removeFromBook(ctx, maker); err != nil {
				return err
			}
		}
		if err := k.SetOrder(ctx, maker); err != nil {
			return err
		}
	}
	// Positions.
	if _, err := k.applyFillToPosition(ctx, pair.Id, buyer.Owner, types.Side_SIDE_BUY, fillQty); err != nil {
		return err
	}
	if _, err := k.applyFillToPosition(ctx, pair.Id, seller.Owner, types.Side_SIDE_SELL, fillQty); err != nil {
		return err
	}
	k.emit(ctx, "fill",
		"pair_id", u64s(pair.Id),
		"maker_id", u64s(maker.Id),
		"taker_id", u64s(taker.Id),
		"price", u64s(fillPrice),
		"quantity", u64s(fillQty),
		"quote", u64s(quote),
	)
	return nil
}

func updateStatusAfterFill(o *types.Order) {
	if o.RemainingQty == 0 {
		o.Status = types.OrderStatus_ORDER_STATUS_FILLED
		return
	}
	o.Status = types.OrderStatus_ORDER_STATUS_PARTIALLY_FILLED
}

// preflightPositionCap projects the position after a candidate
// fill and refuses if magnitude would exceed pair.MaxPositionPerUser.
func (k Keeper) preflightPositionCap(ctx context.Context, pair types.Pair, owner string, side types.Side, qty uint64) error {
	if pair.MaxPositionPerUser == 0 {
		return nil
	}
	pos, err := k.GetPosition(ctx, pair.Id, owner)
	if err != nil {
		return err
	}
	var newMag uint64
	switch side {
	case types.Side_SIDE_BUY:
		if pos.IsShort {
			if qty >= pos.Magnitude {
				newMag = qty - pos.Magnitude
			} else {
				newMag = pos.Magnitude - qty
			}
		} else {
			newMag = pos.Magnitude + qty
		}
	case types.Side_SIDE_SELL:
		if !pos.IsShort {
			if qty >= pos.Magnitude {
				newMag = qty - pos.Magnitude
			} else {
				newMag = pos.Magnitude - qty
			}
		} else {
			newMag = pos.Magnitude + qty
		}
	}
	if newMag > pair.MaxPositionPerUser {
		return fmt.Errorf("position cap exceeded for %s on pair %d: %d > %d", owner, pair.Id, newMag, pair.MaxPositionPerUser)
	}
	return nil
}

// matchFBA implements the Frequent Batch Auction clearer.
//
// Algorithm:
//  1. Collect all open BUY orders, sorted highest-price first.
//  2. Collect all open SELL orders, sorted lowest-price first.
//  3. Walk both sides in tandem; while best_bid >= best_ask,
//     match at the MIDPOINT price (rounded down to PriceScale
//     unit) and decrement both sides.
//  4. Stop after max_matches_per_clear or when sides stop
//     crossing.
//
// The midpoint clearing rule is a common simplification —
// commercial FBA implementations search for a single price
// that maximizes traded volume — but produces the same set of
// crossing pairs in the common case where the spread is small,
// and is materially simpler to reason about. The caller can
// re-invoke ClearBatch until batch_done==true to flush the
// queue when there were more matches than the per-call cap.
func (k Keeper) matchFBA(ctx context.Context, pair types.Pair) (uint32, uint64, bool, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return 0, 0, false, err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	var matchedPairs uint32
	var lastClearing uint64
	rngB := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pair.Id)
	rngS := collections.NewPrefixedTripleRange[uint64, uint64, uint64](pair.Id)

	// Snapshot best-of-book id pairs until cap exhausted or
	// non-cross. We re-snapshot inside the loop because each
	// fill mutates the book.
	for matchedPairs < p.MaxMatchesPerClear {
		var (
			bestBuyID, bestSellID     uint64
			bestBuyPx, bestSellPx     uint64
			haveBest                  bool
		)
		// best buy = first iteration (highest price by key invert)
		if err := k.BuyBook.Walk(ctx, rngB, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			bestBuyID = key.K3()
			bestBuyPx = types.MaxPrice - key.K2()
			haveBest = true
			return true, nil
		}); err != nil {
			return matchedPairs, lastClearing, false, err
		}
		if !haveBest {
			return matchedPairs, lastClearing, true, nil
		}
		haveBest = false
		if err := k.SellBook.Walk(ctx, rngS, func(key collections.Triple[uint64, uint64, uint64]) (bool, error) {
			bestSellID = key.K3()
			bestSellPx = key.K2()
			haveBest = true
			return true, nil
		}); err != nil {
			return matchedPairs, lastClearing, false, err
		}
		if !haveBest {
			return matchedPairs, lastClearing, true, nil
		}
		if bestBuyPx < bestSellPx {
			return matchedPairs, lastClearing, true, nil
		}
		// Match at midpoint. applyFill computes the buyer's
		// price-improvement refund as `locked - drained`, so
		// truncation in QuoteForFill never leaves residue in
		// the pool. We do NOT round clearing to PriceScale —
		// doing so would discard up to a full whole-quote unit
		// of precision per fill (PriceScale is typically 1e6
		// micro-units of quote_denom per base_unit).
		clearing := (bestBuyPx + bestSellPx) / 2
		if clearing < bestSellPx {
			clearing = bestSellPx
		}
		if clearing > bestBuyPx {
			clearing = bestBuyPx
		}
		lastClearing = clearing

		taker, err := k.MustGetOrder(ctx, bestBuyID) // BUY is taker by convention
		if err != nil {
			return matchedPairs, lastClearing, false, err
		}
		maker, err := k.MustGetOrder(ctx, bestSellID)
		if err != nil {
			return matchedPairs, lastClearing, false, err
		}
		fillQty := taker.RemainingQty
		if maker.RemainingQty < fillQty {
			fillQty = maker.RemainingQty
		}
		if err := k.applyFill(ctx, pair, &taker, maker.Id, clearing, fillQty, now); err != nil {
			return matchedPairs, lastClearing, false, err
		}
		// Note: BUY-side price improvement (taker.Price > clearing)
		// is refunded INSIDE applyFill via its buyer==taker branch.
		// Do NOT refund again here — doing so would double-pay the
		// buyer and drain the pool below the locked balance.
		//
		// Persist (and possibly book-remove) the BUY taker
		// directly here — matchContinuous would do this in
		// the caller path; in FBA there is no taker/maker
		// distinction at the message layer.
		if taker.Status.IsTerminal() {
			if err := k.removeFromBook(ctx, taker); err != nil {
				return matchedPairs, lastClearing, false, err
			}
		}
		if err := k.SetOrder(ctx, taker); err != nil {
			return matchedPairs, lastClearing, false, err
		}
		matchedPairs++
	}
	return matchedPairs, lastClearing, false, nil
}
