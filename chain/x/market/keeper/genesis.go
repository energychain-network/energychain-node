package keeper

import (
	"context"

	"energychain/x/market/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, m := range gs.Markets {
		if err := k.Markets.Set(ctx, m.Id, m); err != nil {
			return err
		}
	}
	for _, o := range gs.Orders {
		if o.Status == types.OrderStatus_ORDER_STATUS_OPEN {
			if err := k.setOrderOpen(ctx, o); err != nil {
				return err
			}
		} else {
			if err := k.Orders.Set(ctx, o.Id, o); err != nil {
				return err
			}
		}
	}
	// Reconcile the escrow account against the sum of open-order escrow per
	// denom (x/stableusd loads first). A mismatch means orders could not be
	// settled or refunded, so fail closed.
	if k.settlement != nil {
		want, err := gs.EscrowByDenom()
		if err != nil {
			return err
		}
		for denom, amt := range want {
			got := k.settlement.GetBalance(ctx, denom, EscrowAccount())
			if got != amt {
				return types.ErrSettlement.Wrapf("escrow %s: balance %d != open escrow %d", denom, got, amt)
			}
		}
	}
	// Reconcile the bond pool against the sum of posted listing bonds per
	// denom (x/bank loads first). A mismatch means a bond could not be
	// refunded on delisting, so fail closed.
	if k.bank != nil {
		for denom, want := range gs.BondsByDenom() {
			got := k.bank.GetBalance(ctx, BondPoolAccount(), denom).Amount
			if !got.Equal(want) {
				return types.ErrBond.Wrapf("bond pool %s: balance %s != posted bonds %s", denom, got, want)
			}
		}
	}
	if err := k.MarketSeq.Set(ctx, gs.MarketIdSeq); err != nil {
		return err
	}
	if err := k.OrderSeq.Set(ctx, gs.OrderIdSeq); err != nil {
		return err
	}
	return k.PlaceSeq.Set(ctx, gs.OrderPlacementSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Markets.Walk(ctx, nil, func(_ uint64, m types.Market) (bool, error) {
		gs.Markets = append(gs.Markets, m)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Orders.Walk(ctx, nil, func(_ uint64, o types.Order) (bool, error) {
		gs.Orders = append(gs.Orders, o)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.MarketIdSeq, err = k.MarketSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.OrderIdSeq, err = k.OrderSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.OrderPlacementSeq, err = k.PlaceSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
