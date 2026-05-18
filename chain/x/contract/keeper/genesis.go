package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/contract/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if gs == nil {
		gs = types.DefaultGenesis()
	}
	if err := gs.Validate(); err != nil {
		return fmt.Errorf("contract: invalid genesis: %w", err)
	}
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	if gs.NextContractId > 0 {
		if err := k.IDSeq.Set(ctx, gs.NextContractId-1); err != nil {
			return err
		}
	}
	for _, c := range gs.Contracts {
		if err := k.Contracts.Set(ctx, c.Id, c); err != nil {
			return err
		}
		if err := k.ContractByParty.Set(ctx, collections.Join(c.Buyer, c.Id)); err != nil {
			return err
		}
		if err := k.ContractByParty.Set(ctx, collections.Join(c.Seller, c.Id)); err != nil {
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
	next, err := k.IDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	out := types.GenesisState{Params: p, NextContractId: next + 1}
	if err := k.Contracts.Walk(ctx, nil, func(_ uint64, v types.Contract) (bool, error) {
		out.Contracts = append(out.Contracts, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &out, nil
}
