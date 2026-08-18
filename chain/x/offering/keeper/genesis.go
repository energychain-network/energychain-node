package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/offering/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, o := range gs.Offerings {
		if err := k.setOffering(ctx, o); err != nil {
			return err
		}
	}
	for _, s := range gs.Subscriptions {
		if err := k.setSubscription(ctx, s); err != nil {
			return err
		}
	}
	return k.OfferingIDSeq.Set(ctx, gs.OfferingIdSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Offerings.Walk(ctx, nil, func(_ uint64, o types.Offering) (bool, error) {
		gs.Offerings = append(gs.Offerings, o)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Subscriptions.Walk(ctx, nil, func(_ collections.Pair[uint64, string], s types.Subscription) (bool, error) {
		gs.Subscriptions = append(gs.Subscriptions, s)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if gs.OfferingIdSeq, err = k.OfferingIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
