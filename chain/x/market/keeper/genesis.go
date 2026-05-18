package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/market/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("market: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextPairId > 0 {
		if err := k.PairIDSeq.Set(ctx, gs.NextPairId-1); err != nil {
			return err
		}
	}
	if gs.NextOrderId > 0 {
		if err := k.OrderIDSeq.Set(ctx, gs.NextOrderId-1); err != nil {
			return err
		}
	}
	for _, p := range gs.Pairs {
		if err := k.Pairs.Set(ctx, p.Id, p); err != nil {
			return err
		}
	}
	for _, o := range gs.Orders {
		if err := k.Orders.Set(ctx, o.Id, o); err != nil {
			return err
		}
		if err := k.OrderByOwner.Set(ctx, collections.Join(o.Owner, o.Id)); err != nil {
			return err
		}
		if !o.Status.IsTerminal() {
			if err := k.addToBook(ctx, o); err != nil {
				return err
			}
		}
	}
	for _, pos := range gs.Positions {
		if err := k.setPosition(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	pNext, err := k.PairIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	oNext, err := k.OrderIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{
		Params:      p,
		NextPairId:  pNext + 1,
		NextOrderId: oNext + 1,
	}
	if err := k.Pairs.Walk(ctx, nil, func(_ uint64, v types.Pair) (bool, error) {
		out.Pairs = append(out.Pairs, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Orders.Walk(ctx, nil, func(_ uint64, v types.Order) (bool, error) {
		out.Orders = append(out.Orders, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Positions.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.Position) (bool, error) {
		out.Positions = append(out.Positions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
