package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/mincast/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, m := range gs.Markets {
		if err := k.setMarket(ctx, m); err != nil {
			return err
		}
		if err := k.MarketByDenom.Set(ctx, m.Denom, m.Id); err != nil {
			return err
		}
		// Reconcile the declared treasury against the actual settlement
		// balance held by the per-market treasury account (x/stableusd loads
		// before mincast in genesisModuleOrder). A mismatch means melt pricing
		// (state-based) would diverge from the redeemable backing (balance-
		// based), so fail closed at import rather than ship a market that
		// over- or under-pays redemptions.
		if k.settlement != nil && m.Treasury > 0 {
			got := k.TreasuryBalance(ctx, m)
			if got != m.Treasury {
				return types.ErrSettlement.Wrapf("market %d treasury %d != settlement balance %d", m.Id, m.Treasury, got)
			}
		}
	}
	for _, b := range gs.Balances {
		if err := k.setBalance(ctx, b.MarketId, b.Holder, b.Amount); err != nil {
			return err
		}
	}
	for _, iv := range gs.Invests {
		if err := k.setInvest(ctx, iv); err != nil {
			return err
		}
	}
	if err := k.MarketIDSeq.Set(ctx, gs.MarketIdSeq); err != nil {
		return err
	}
	return k.InvestIDSeq.Set(ctx, gs.InvestIdSeq)
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
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[uint64, string], amt uint64) (bool, error) {
		gs.Balances = append(gs.Balances, types.Balance{MarketId: key.K1(), Holder: key.K2(), Amount: amt})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Invests.Walk(ctx, nil, func(_ uint64, iv types.Invest) (bool, error) {
		gs.Invests = append(gs.Invests, iv)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.MarketIdSeq, err = k.MarketIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.InvestIdSeq, err = k.InvestIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
