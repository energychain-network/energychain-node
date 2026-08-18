package keeper

import (
	"context"
	"sort"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/market/types"
)

// EndBlock runs the frequent batch auction: every ACTIVE market whose batch
// interval has elapsed is cleared at a single uniform price. Each market clears
// inside its own CacheContext so a fault in one market can never halt the chain
// or corrupt another market's books.
func (k Keeper) EndBlock(ctx context.Context) error {
	p, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if p.Paused {
		return nil
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()

	var due []types.Market
	if err := k.Markets.Walk(ctx, nil, func(_ uint64, m types.Market) (bool, error) {
		if m.Status == types.MarketStatus_MARKET_STATUS_ACTIVE && now-m.LastBatchTime >= m.BatchInterval {
			due = append(due, m)
		}
		return false, nil
	}); err != nil {
		return err
	}

	for _, m := range due {
		cctx, commit := sdkCtx.CacheContext()
		if err := k.clearMarket(cctx, m, p, now); err != nil {
			// Discard the partial cache and advance the batch clock on the
			// committed parent so a poisoned order (e.g. an admin-frozen
			// counterparty whose settlement leg reverts) only delays this
			// market by one interval instead of busy-retrying every block.
			sdkCtx.Logger().Error("market batch clear failed", "market", m.Id, "err", err)
			m.LastBatchTime = now
			if e := k.Markets.Set(ctx, m.Id, m); e != nil {
				return e
			}
			continue
		}
		commit()
	}
	return nil
}

// clearMarket clears one market's resting book at the uniform batch price.
func (k Keeper) clearMarket(ctx context.Context, m types.Market, p types.Params, now int64) error {
	orders, err := k.openOrders(ctx, m.Id)
	if err != nil {
		return err
	}

	var buys, sells []types.Order
	for _, o := range orders {
		// A sanctioned owner's resting order is excluded from matching: it can
		// neither fill nor be cancelled for a refund, so its escrow stays
		// locked until the sanction clears. This mirrors the CancelOrder gate
		// and prevents a sanctioned party from receiving batch settlement.
		if k.isSanctioned(ctx, o.Owner) {
			continue
		}
		if o.Side == types.OrderSide_ORDER_SIDE_BUY {
			buys = append(buys, o)
		} else {
			sells = append(sells, o)
		}
	}

	// advanceClock persists the batch clock so an empty/uncrossed book does not
	// retry every block. Called on every successful return path.
	advanceClock := func() error {
		m.LastBatchTime = now
		return k.Markets.Set(ctx, m.Id, m)
	}

	// RWA-base markets: void any BUY whose owner can no longer receive the
	// units (KYC revoked, frozen, policy/jurisdiction change, or per-holder cap
	// reached since placement) BEFORE pricing. The buy's quote escrow is
	// refunded and the order closed, so one non-deliverable buyer can never
	// revert the whole uniform-price batch.
	if baseTokenID, isRWA := parseRWADenom(m.BaseDenom); isRWA && k.asset != nil && len(buys) > 0 {
		kept := buys[:0]
		for _, o := range buys {
			unfilled := o.Quantity - o.Filled
			if err := k.asset.MarketReceiverOK(ctx, baseTokenID, o.Owner, unfilled); err == nil {
				kept = append(kept, o)
				continue
			}
			// Refund the buyer's quote escrow and close the order. If even the
			// refund fails (e.g. quote denom paused), leave it resting and skip
			// it this batch so the batch still clears for everyone else.
			if refundErr := k.moveSettlement(ctx, m.QuoteDenom, EscrowAccount(), o.Owner, o.Escrowed); refundErr != nil {
				continue
			}
			o.Escrowed = 0
			o.Status = types.OrderStatus_ORDER_STATUS_CANCELLED
			if e := k.closeOrder(ctx, o); e != nil {
				return e
			}
			emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeOrder, types.AttrAction, "void",
				types.AttrOrderID, u(o.Id), types.AttrMarketID, u(m.Id), types.AttrOwner, o.Owner)
		}
		buys = kept
	}

	if len(buys) == 0 || len(sells) == 0 {
		return advanceClock()
	}

	// price-time priority: buys high->low, sells low->high, ties by seq asc.
	sort.Slice(buys, func(i, j int) bool {
		if buys[i].Price != buys[j].Price {
			return buys[i].Price > buys[j].Price
		}
		return buys[i].Seq < buys[j].Seq
	})
	sort.Slice(sells, func(i, j int) bool {
		if sells[i].Price != sells[j].Price {
			return sells[i].Price < sells[j].Price
		}
		return sells[i].Seq < sells[j].Seq
	})

	buyLevels := make([]types.PriceLevel, len(buys))
	for i, o := range buys {
		buyLevels[i] = types.PriceLevel{Price: o.Price, Qty: o.Quantity - o.Filled}
	}
	sellLevels := make([]types.PriceLevel, len(sells))
	for i, o := range sells {
		sellLevels[i] = types.PriceLevel{Price: o.Price, Qty: o.Quantity - o.Filled}
	}

	price, volume, ok := types.ComputeClearing(buyLevels, sellLevels)
	if !ok || volume == 0 {
		return advanceClock()
	}

	// Bound per-batch settlement writes: cap how many orders each side may
	// touch, and clear only the balanced volume reachable within that cap. The
	// uniform price is unchanged; the residual crossing volume clears next
	// batch. This keeps EndBlocker work bounded without ever producing an
	// unbalanced (non-conserving) partial clear.
	half := p.MaxFillsPerBatch / 2
	if half == 0 {
		half = 1
	}
	matched := capVolume(buys, price, true, half)
	if sv := capVolume(sells, price, false, half); sv < matched {
		matched = sv
	}
	if matched > volume {
		matched = volume
	}
	if matched == 0 {
		return advanceClock()
	}

	if err := k.settleBatch(ctx, &m, buys, sells, price, matched); err != nil {
		return err
	}
	m.LastClearingPrice = price
	emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeClear, types.AttrMarketID, u(m.Id),
		types.AttrPrice, u(price), types.AttrQty, u(matched))
	return advanceClock()
}

// capVolume returns the cumulative crossing quantity of at most `maxOrders`
// orders on one side at the clearing price (buys with price>=p, sells<=p), in
// priority order.
func capVolume(side []types.Order, price uint64, isBuy bool, maxOrders uint32) uint64 {
	var vol uint64
	var n uint32
	for _, o := range side {
		if (isBuy && o.Price < price) || (!isBuy && o.Price > price) {
			continue
		}
		vol = types.AddSat(vol, o.Quantity-o.Filled)
		n++
		if n >= maxOrders {
			break
		}
	}
	return vol
}

// settleBatch moves base and quote for a matched volume `M` at uniform price
// `price`, using cumulative-floor (telescoping) accounting so the sum of buyer
// costs equals floor(price*M/scale) exactly, and the sum of seller proceeds
// equals that minus fees exactly — guaranteeing per-denom conservation.
func (k Keeper) settleBatch(ctx context.Context, m *types.Market, buys, sells []types.Order, price, M uint64) error {
	fTot, err := types.QuoteFloor(price, M) // total clearing quote
	if err != nil {
		return err
	}
	fee, err := types.MulDivFloor(fTot, uint64(m.FeeBps), types.BpsDenominator)
	if err != nil {
		return err
	}
	sellTotal, err := types.SafeSub(fTot, fee)
	if err != nil {
		return err
	}

	// ---- buy side: deliver base, charge clearing cost, refund improvement ----
	var cumB uint64
	left := M
	for i := range buys {
		if left == 0 {
			break
		}
		o := buys[i]
		if o.Price < price {
			continue
		}
		fillQ := o.Quantity - o.Filled
		if fillQ > left {
			fillQ = left
		}
		if fillQ == 0 {
			continue
		}
		fBefore, err := types.QuoteFloor(price, cumB)
		if err != nil {
			return err
		}
		fAfter, err := types.QuoteFloor(price, cumB+fillQ)
		if err != nil {
			return err
		}
		cost := fAfter - fBefore
		// deliver base to buyer (routes through the compliant rwatoken ledger
		// for an RWA-base market, the stableusd reserve otherwise)
		if err := k.baseDeliver(ctx, *m, o.Owner, fillQ); err != nil {
			return err
		}
		newEscrow, err := types.SafeSub(o.Escrowed, cost)
		if err != nil {
			return err
		}
		o.Escrowed = newEscrow
		o.Filled += fillQ
		if o.Filled == o.Quantity {
			// refund leftover escrow (limit-price improvement + ceil rounding)
			if err := k.moveSettlement(ctx, m.QuoteDenom, EscrowAccount(), o.Owner, o.Escrowed); err != nil {
				return err
			}
			o.Escrowed = 0
			o.Status = types.OrderStatus_ORDER_STATUS_FILLED
			if err := k.closeOrder(ctx, o); err != nil {
				return err
			}
		} else {
			if err := k.setOrderOpen(ctx, o); err != nil {
				return err
			}
		}
		// Per-fill event: the settlement legs move funds through the stableusd /
		// rwatoken ledgers without emitting their own balance events, so this is
		// the only signal an off-chain indexer gets to re-sync the buyer's
		// base/quote balances after a batch clear.
		emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeOrder, types.AttrAction, "fill",
			types.AttrOrderID, u(o.Id), types.AttrMarketID, u(m.Id), types.AttrOwner, o.Owner,
			types.AttrSide, o.Side.String(), types.AttrPrice, u(price), types.AttrQty, u(fillQ))
		cumB += fillQ
		left -= fillQ
	}

	// ---- sell side: surrender base escrow, receive proceeds ----
	var cumS uint64
	left = M
	for i := range sells {
		if left == 0 {
			break
		}
		o := sells[i]
		if o.Price > price {
			continue
		}
		fillQ := o.Quantity - o.Filled
		if fillQ > left {
			fillQ = left
		}
		if fillQ == 0 {
			continue
		}
		gBefore, err := types.MulDivFloor(sellTotal, cumS, M)
		if err != nil {
			return err
		}
		gAfter, err := types.MulDivFloor(sellTotal, cumS+fillQ, M)
		if err != nil {
			return err
		}
		proceeds := gAfter - gBefore
		// base leaves the seller's escrow accounting (already delivered to buyers)
		newEscrow, err := types.SafeSub(o.Escrowed, fillQ)
		if err != nil {
			return err
		}
		o.Escrowed = newEscrow
		// pay quote proceeds
		if err := k.moveSettlement(ctx, m.QuoteDenom, EscrowAccount(), o.Owner, proceeds); err != nil {
			return err
		}
		o.Filled += fillQ
		if o.Filled == o.Quantity {
			o.Status = types.OrderStatus_ORDER_STATUS_FILLED
			if err := k.closeOrder(ctx, o); err != nil {
				return err
			}
		} else {
			if err := k.setOrderOpen(ctx, o); err != nil {
				return err
			}
		}
		emitEvent(sdk.UnwrapSDKContext(ctx), types.EventTypeOrder, types.AttrAction, "fill",
			types.AttrOrderID, u(o.Id), types.AttrMarketID, u(m.Id), types.AttrOwner, o.Owner,
			types.AttrSide, o.Side.String(), types.AttrPrice, u(price), types.AttrQty, u(fillQ))
		cumS += fillQ
		left -= fillQ
	}

	// protocol fee
	if fee > 0 {
		if err := k.moveSettlement(ctx, m.QuoteDenom, EscrowAccount(), FeeAccount(), fee); err != nil {
			return err
		}
	}
	return nil
}
