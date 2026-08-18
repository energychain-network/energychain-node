package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	type mkt struct {
		base, quote string
		status      MarketStatus
	}
	markets := map[uint64]mkt{}
	var maxMarket uint64
	for _, m := range gs.Markets {
		if m.Id == 0 {
			return fmt.Errorf("genesis: market id must be > 0")
		}
		if _, dup := markets[m.Id]; dup {
			return fmt.Errorf("genesis: duplicate market id %d", m.Id)
		}
		if err := ValidateDenom(m.BaseDenom); err != nil {
			return fmt.Errorf("genesis: market %d base: %w", m.Id, err)
		}
		if err := ValidateDenom(m.QuoteDenom); err != nil {
			return fmt.Errorf("genesis: market %d quote: %w", m.Id, err)
		}
		if m.BaseDenom == m.QuoteDenom {
			return fmt.Errorf("genesis: market %d base==quote", m.Id)
		}
		if !MarketStatusValidGenesis(m.Status) {
			return fmt.Errorf("genesis: market %d invalid status", m.Id)
		}
		if m.Operator != "" {
			if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
				return fmt.Errorf("genesis: market %d operator: %w", m.Id, err)
			}
		}
		if m.BondAmount.IsNil() || m.BondAmount.IsNegative() {
			return fmt.Errorf("genesis: market %d bond_amount must be set and >= 0", m.Id)
		}
		if m.BondAmount.IsPositive() {
			if m.Operator == "" {
				return fmt.Errorf("genesis: market %d has a bond but no operator", m.Id)
			}
			if err := sdk.ValidateDenom(m.BondDenom); err != nil {
				return fmt.Errorf("genesis: market %d bond_denom: %w", m.Id, err)
			}
		}
		switch m.Status {
		case MarketStatus_MARKET_STATUS_PENDING_BOND:
			// The bond has not been escrowed yet; the market must not carry one.
			if m.BondAmount.IsPositive() {
				return fmt.Errorf("genesis: pending-bond market %d must have zero bond", m.Id)
			}
			if m.Operator == "" {
				return fmt.Errorf("genesis: pending-bond market %d needs an operator", m.Id)
			}
		case MarketStatus_MARKET_STATUS_DELISTED:
			// Terminal: the bond was refunded on delisting.
			if m.BondAmount.IsPositive() {
				return fmt.Errorf("genesis: delisted market %d must have zero bond", m.Id)
			}
		}
		if uint32(m.FeeBps) > gs.Params.MaxFeeBps {
			return fmt.Errorf("genesis: market %d fee_bps exceeds max", m.Id)
		}
		if m.MinBaseQty == 0 {
			return fmt.Errorf("genesis: market %d min_base_qty must be > 0", m.Id)
		}
		if m.BatchInterval < gs.Params.MinBatchInterval {
			return fmt.Errorf("genesis: market %d batch_interval below min", m.Id)
		}
		markets[m.Id] = mkt{m.BaseDenom, m.QuoteDenom, m.Status}
		if m.Id > maxMarket {
			maxMarket = m.Id
		}
	}
	if uint32(len(gs.Markets)) > gs.Params.MaxMarkets {
		return fmt.Errorf("genesis: markets exceed max")
	}

	orderIDs := map[uint64]bool{}
	openPerMarket := map[uint64]uint32{}
	var maxOrder, maxSeq uint64
	for _, o := range gs.Orders {
		if o.Id == 0 {
			return fmt.Errorf("genesis: order id must be > 0")
		}
		if orderIDs[o.Id] {
			return fmt.Errorf("genesis: duplicate order id %d", o.Id)
		}
		orderIDs[o.Id] = true
		if _, err := sdk.AccAddressFromBech32(o.Owner); err != nil {
			return fmt.Errorf("genesis: order %d owner: %w", o.Id, err)
		}
		if _, ok := markets[o.MarketId]; !ok {
			return fmt.Errorf("genesis: order %d unknown market %d", o.Id, o.MarketId)
		}
		if !OrderSideValid(o.Side) {
			return fmt.Errorf("genesis: order %d invalid side", o.Id)
		}
		if o.Price == 0 || o.Quantity == 0 {
			return fmt.Errorf("genesis: order %d price/quantity must be > 0", o.Id)
		}
		if o.Filled > o.Quantity {
			return fmt.Errorf("genesis: order %d filled exceeds quantity", o.Id)
		}
		if !OrderStatusValid(o.Status) {
			return fmt.Errorf("genesis: order %d invalid status", o.Id)
		}
		switch o.Status {
		case OrderStatus_ORDER_STATUS_OPEN:
			// Orders only rest on tradable books: PENDING_BOND never accepted
			// any and DELISTED cancelled all of them.
			if st := markets[o.MarketId].status; st != MarketStatus_MARKET_STATUS_ACTIVE && st != MarketStatus_MARKET_STATUS_PAUSED {
				return fmt.Errorf("genesis: open order %d on non-tradable market %d (%s)", o.Id, o.MarketId, st)
			}
			if o.Filled >= o.Quantity {
				return fmt.Errorf("genesis: open order %d must have filled < quantity", o.Id)
			}
			remaining := o.Quantity - o.Filled
			if o.Side == OrderSide_ORDER_SIDE_SELL {
				// SELL escrow is base and tracks remaining quantity exactly.
				if o.Escrowed != remaining {
					return fmt.Errorf("genesis: open sell %d escrow %d != remaining %d", o.Id, o.Escrowed, remaining)
				}
			} else {
				// BUY escrow is quote: at least enough to cover the remaining
				// quantity at the limit price, never more than the original
				// full-quantity escrow.
				lo, err := QuoteCeil(o.Price, remaining)
				if err != nil {
					return err
				}
				hi, err := QuoteCeil(o.Price, o.Quantity)
				if err != nil {
					return err
				}
				if o.Escrowed < lo || o.Escrowed > hi {
					return fmt.Errorf("genesis: open buy %d escrow %d outside [%d,%d]", o.Id, o.Escrowed, lo, hi)
				}
			}
			openPerMarket[o.MarketId]++
		case OrderStatus_ORDER_STATUS_FILLED:
			if o.Filled != o.Quantity {
				return fmt.Errorf("genesis: filled order %d must be fully filled", o.Id)
			}
			if o.Escrowed != 0 {
				return fmt.Errorf("genesis: filled order %d must have zero escrow", o.Id)
			}
		case OrderStatus_ORDER_STATUS_CANCELLED:
			if o.Escrowed != 0 {
				return fmt.Errorf("genesis: cancelled order %d must have zero escrow", o.Id)
			}
		}
		if o.Id > maxOrder {
			maxOrder = o.Id
		}
		if o.Seq > maxSeq {
			maxSeq = o.Seq
		}
	}
	for mID, n := range openPerMarket {
		if n > gs.Params.MaxOpenOrdersPerMarket {
			return fmt.Errorf("genesis: market %d open orders exceed max", mID)
		}
	}

	if gs.MarketIdSeq < maxMarket {
		return fmt.Errorf("genesis: market_id_seq %d < max %d", gs.MarketIdSeq, maxMarket)
	}
	if gs.OrderIdSeq < maxOrder {
		return fmt.Errorf("genesis: order_id_seq %d < max %d", gs.OrderIdSeq, maxOrder)
	}
	if gs.OrderPlacementSeq < maxSeq {
		return fmt.Errorf("genesis: order_placement_seq %d < max %d", gs.OrderPlacementSeq, maxSeq)
	}
	return nil
}

// BondsByDenom returns the total listing bond the module bond-pool account
// must hold per native denom (sum of every market's posted bond). Used by
// InitGenesis to reconcile against x/bank.
func (gs GenesisState) BondsByDenom() map[string]sdkmath.Int {
	out := map[string]sdkmath.Int{}
	for _, m := range gs.Markets {
		if m.BondAmount.IsNil() || !m.BondAmount.IsPositive() {
			continue
		}
		cur, ok := out[m.BondDenom]
		if !ok {
			cur = sdkmath.ZeroInt()
		}
		out[m.BondDenom] = cur.Add(m.BondAmount)
	}
	return out
}

// EscrowByDenom returns the settlement the module escrow account must hold per
// denom: each OPEN order's remaining escrow, keyed by the leg's denom (quote
// for BUY, base for SELL). Used by InitGenesis to reconcile against x/stableusd.
func (gs GenesisState) EscrowByDenom() (map[string]uint64, error) {
	denomBase := map[uint64]string{}
	denomQuote := map[uint64]string{}
	for _, m := range gs.Markets {
		denomBase[m.Id] = m.BaseDenom
		denomQuote[m.Id] = m.QuoteDenom
	}
	out := map[string]uint64{}
	for _, o := range gs.Orders {
		if o.Status != OrderStatus_ORDER_STATUS_OPEN || o.Escrowed == 0 {
			continue
		}
		denom := denomQuote[o.MarketId]
		if o.Side == OrderSide_ORDER_SIDE_SELL {
			denom = denomBase[o.MarketId]
		}
		s, err := SafeAdd(out[denom], o.Escrowed)
		if err != nil {
			return nil, err
		}
		out[denom] = s
	}
	return out, nil
}
