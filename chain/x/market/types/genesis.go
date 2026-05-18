package types

import "fmt"

func DefaultParams() Params {
	return Params{
		MaxPairs:                      DefaultMaxPairs,
		MaxOpenOrdersPerPair:          DefaultMaxOpenOrdersPerPair,
		MaxOpenOrdersPerUserPerPair:   DefaultMaxOpenOrdersPerUserPerPair,
		MaxFillsPerMatch:              DefaultMaxFillsPerMatch,
		MaxMatchesPerClear:            DefaultMaxMatchesPerClear,
		MemoMaxLen:                    DefaultMemoMaxLen,
	}
}

func (p Params) Validate() error {
	if p.MaxPairs == 0 || p.MaxPairs > HardMaxPairs {
		return fmt.Errorf("max_pairs out of range")
	}
	if p.MaxOpenOrdersPerPair == 0 || p.MaxOpenOrdersPerPair > HardMaxOpenOrdersPerPair {
		return fmt.Errorf("max_open_orders_per_pair out of range")
	}
	if p.MaxOpenOrdersPerUserPerPair == 0 || p.MaxOpenOrdersPerUserPerPair > HardMaxOpenOrdersPerUserPerPair {
		return fmt.Errorf("max_open_orders_per_user_per_pair out of range")
	}
	if p.MaxFillsPerMatch == 0 || p.MaxFillsPerMatch > HardMaxFillsPerMatch {
		return fmt.Errorf("max_fills_per_match out of range")
	}
	if p.MaxMatchesPerClear == 0 || p.MaxMatchesPerClear > HardMaxMatchesPerClear {
		return fmt.Errorf("max_matches_per_clear out of range")
	}
	if p.MemoMaxLen > HardMemoMaxLen {
		return fmt.Errorf("memo_max_len must be <= %d", HardMemoMaxLen)
	}
	return nil
}

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		NextPairId:  1,
		NextOrderId: 1,
	}
}

func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if uint32(len(gs.Pairs)) > gs.Params.MaxPairs {
		return fmt.Errorf("genesis: %d pairs > max %d", len(gs.Pairs), gs.Params.MaxPairs)
	}
	pairIDs := map[uint64]bool{}
	for _, p := range gs.Pairs {
		if p.Id == 0 {
			return fmt.Errorf("genesis: pair id must be > 0")
		}
		if pairIDs[p.Id] {
			return fmt.Errorf("genesis: duplicate pair id %d", p.Id)
		}
		pairIDs[p.Id] = true
		if p.Id >= gs.NextPairId {
			return fmt.Errorf("genesis: pair id %d >= next_pair_id %d", p.Id, gs.NextPairId)
		}
		if !ModeValid(p.Mode) {
			return fmt.Errorf("genesis: pair %d invalid mode", p.Id)
		}
		if !PairStatusValid(p.Status) {
			return fmt.Errorf("genesis: pair %d invalid status", p.Id)
		}
		if err := ValidateDenom(p.BaseDenom); err != nil {
			return err
		}
		if err := ValidateDenom(p.QuoteDenom); err != nil {
			return err
		}
		if p.BaseDenom == p.QuoteDenom {
			return fmt.Errorf("genesis: pair %d base == quote", p.Id)
		}
	}
	orderIDs := map[uint64]bool{}
	for _, o := range gs.Orders {
		if o.Id == 0 {
			return fmt.Errorf("genesis: order id must be > 0")
		}
		if orderIDs[o.Id] {
			return fmt.Errorf("genesis: duplicate order id %d", o.Id)
		}
		orderIDs[o.Id] = true
		if o.Id >= gs.NextOrderId {
			return fmt.Errorf("genesis: order id %d >= next_order_id %d", o.Id, gs.NextOrderId)
		}
		if !pairIDs[o.PairId] {
			return fmt.Errorf("genesis: order %d references unknown pair %d", o.Id, o.PairId)
		}
		if !SideValid(o.Side) {
			return fmt.Errorf("genesis: order %d invalid side", o.Id)
		}
		if !OrderStatusValid(o.Status) {
			return fmt.Errorf("genesis: order %d invalid status", o.Id)
		}
		if err := ValidateAddr("owner", o.Owner); err != nil {
			return err
		}
		if o.RemainingQty > o.Quantity {
			return fmt.Errorf("genesis: order %d remaining > quantity", o.Id)
		}
		// Terminal orders must have zero escrow (drained or
		// refunded at close); open orders must have non-zero
		// escrow if any qty remains (otherwise the pool can't
		// settle the fill).
		if o.Status.IsTerminal() && o.EscrowLocked != 0 {
			return fmt.Errorf("genesis: terminal order %d has non-zero escrow %d", o.Id, o.EscrowLocked)
		}
		if !o.Status.IsTerminal() && o.RemainingQty > 0 && o.EscrowLocked == 0 {
			return fmt.Errorf("genesis: open order %d has zero escrow but remaining_qty %d", o.Id, o.RemainingQty)
		}
	}
	for _, pos := range gs.Positions {
		if !pairIDs[pos.PairId] {
			return fmt.Errorf("genesis: position references unknown pair %d", pos.PairId)
		}
		if err := ValidateAddr("position.owner", pos.Owner); err != nil {
			return err
		}
	}
	return nil
}
