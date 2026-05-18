package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/contract/types"
)

// settlementDelta is the keeper-internal "per-period" computation.
// It returns a SIGNED amount denoting net flow direction:
//
//	delta > 0  → buyer pays seller |delta|
//	delta < 0  → seller pays buyer |delta|
//	delta == 0 → no movement this period
//
// The function is intentionally pure of state mutation: it only
// READS the oracle. Callers wrap it in their own state-machine
// step so the read / compute / apply story is auditable.
//
// PPA:
//
//	qty := oracle(quantity_topic)  // falls back to notional_quantity
//	delta := strike_price * qty / PriceScale          (buyer -> seller)
//
// VPPA (floor):
//
//	idx := oracle(price_topic)
//	if strike > idx: delta := (strike - idx) * notional / PriceScale   (seller -> buyer)
//	else:            delta := 0
//
// CFD (bidirectional):
//
//	idx := oracle(price_topic)
//	if idx > strike: delta := (idx - strike) * notional / PriceScale   (seller -> buyer)
//	else:            delta := (strike - idx) * notional / PriceScale   (buyer -> seller)
//
// max_oracle_staleness_seconds (per-contract) bounds how old an
// oracle reading may be at "now" — older reads fail the call so
// settlements never trust a stale index.
func (k Keeper) settlementDelta(ctx context.Context, c types.Contract, now int64) (int64, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.oracle == nil && (c.PriceOracleTopic != "" || c.QuantityOracleTopic != "") {
		return 0, fmt.Errorf("oracle keeper not wired")
	}

	getOracle := func(topic string) (uint64, error) {
		v, ts, ok := k.oracle.GetAggregatedReserve(sdkCtx, topic)
		if !ok {
			return 0, fmt.Errorf("oracle topic %q has no aggregated reading", topic)
		}
		if v < 0 {
			return 0, fmt.Errorf("oracle topic %q returned negative %d", topic, v)
		}
		if c.MaxOracleStalenessSeconds > 0 && now-ts > c.MaxOracleStalenessSeconds {
			return 0, fmt.Errorf("oracle topic %q stale: %ds > cap %ds", topic, now-ts, c.MaxOracleStalenessSeconds)
		}
		return uint64(v), nil
	}

	switch c.Kind {
	case types.Kind_KIND_PPA:
		qty := c.NotionalQuantity
		if c.QuantityOracleTopic != "" {
			v, err := getOracle(c.QuantityOracleTopic)
			if err != nil {
				return 0, err
			}
			qty = v
		}
		raw, overflow := types.SafeMul(c.StrikePrice, qty)
		if overflow {
			return 0, fmt.Errorf("PPA settlement amount overflow: strike=%d qty=%d", c.StrikePrice, qty)
		}
		return int64(raw / types.PriceScale), nil

	case types.Kind_KIND_VPPA:
		idx, err := getOracle(c.PriceOracleTopic)
		if err != nil {
			return 0, err
		}
		if idx >= c.StrikePrice {
			return 0, nil
		}
		gap := c.StrikePrice - idx
		raw, overflow := types.SafeMul(gap, c.NotionalQuantity)
		if overflow {
			return 0, fmt.Errorf("VPPA settlement amount overflow: gap=%d notional=%d", gap, c.NotionalQuantity)
		}
		return -int64(raw / types.PriceScale), nil // seller -> buyer

	case types.Kind_KIND_CFD:
		idx, err := getOracle(c.PriceOracleTopic)
		if err != nil {
			return 0, err
		}
		if idx >= c.StrikePrice {
			gap := idx - c.StrikePrice
			raw, overflow := types.SafeMul(gap, c.NotionalQuantity)
			if overflow {
				return 0, fmt.Errorf("CFD long-leg overflow: gap=%d", gap)
			}
			return -int64(raw / types.PriceScale), nil // seller -> buyer
		}
		gap := c.StrikePrice - idx
		raw, overflow := types.SafeMul(gap, c.NotionalQuantity)
		if overflow {
			return 0, fmt.Errorf("CFD short-leg overflow: gap=%d", gap)
		}
		return int64(raw / types.PriceScale), nil // buyer -> seller
	}
	return 0, fmt.Errorf("unsupported kind %v", c.Kind)
}

// applyDelta mutates a contract's margin counters by `delta`.
// Positive delta debits buyer / credits seller; negative is the
// reverse. Returns an error iff the debited side does not have
// enough margin to cover — the caller (Settle / Default) decides
// whether to propagate that as a settlement failure or a
// default-trigger.
func applyDelta(c *types.Contract, delta int64) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		amt := uint64(delta)
		if c.MarginBuyer < amt {
			return fmt.Errorf("buyer margin %d < settlement %d", c.MarginBuyer, amt)
		}
		c.MarginBuyer -= amt
		c.MarginSeller += amt
		c.CumulativeBuyerToSeller += amt
		return nil
	}
	amt := uint64(-delta)
	if c.MarginSeller < amt {
		return fmt.Errorf("seller margin %d < settlement %d", c.MarginSeller, amt)
	}
	c.MarginSeller -= amt
	c.MarginBuyer += amt
	c.CumulativeSellerToBuyer += amt
	return nil
}
