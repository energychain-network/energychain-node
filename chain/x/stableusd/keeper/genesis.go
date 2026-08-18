package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/stableusd/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, d := range gs.Denoms {
		if err := k.Denoms.Set(ctx, d.Id, d); err != nil {
			return err
		}
	}
	for _, b := range gs.Balances {
		if err := k.Balances.Set(ctx, collections.Join(b.DenomId, b.Account), b.Amount); err != nil {
			return err
		}
	}
	for _, s := range gs.Supplies {
		if err := k.Supply.Set(ctx, s.DenomId, s.Amount); err != nil {
			return err
		}
	}
	for _, a := range gs.Allowances {
		if err := k.Allowances.Set(ctx, collections.Join3(a.DenomId, a.Owner, a.Spender), a.Amount); err != nil {
			return err
		}
	}
	for _, f := range gs.Flags {
		if err := k.setFlags(ctx, f); err != nil {
			return err
		}
	}
	for _, r := range gs.Redemptions {
		if err := k.setRedemption(ctx, r); err != nil {
			return err
		}
	}
	return k.RedemptionIDSeq.Set(ctx, gs.RedemptionIdSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	gs.Params = k.GetParams(ctx)

	if err := k.Denoms.Walk(ctx, nil, func(_ string, v types.StableDenom) (bool, error) {
		gs.Denoms = append(gs.Denoms, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[string, string], v uint64) (bool, error) {
		if v != 0 {
			gs.Balances = append(gs.Balances, types.Balance{DenomId: key.K1(), Account: key.K2(), Amount: v})
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Supply.Walk(ctx, nil, func(denom string, v uint64) (bool, error) {
		if v != 0 {
			gs.Supplies = append(gs.Supplies, types.Supply{DenomId: denom, Amount: v})
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Allowances.Walk(ctx, nil, func(key collections.Triple[string, string, string], v uint64) (bool, error) {
		if v != 0 {
			gs.Allowances = append(gs.Allowances, types.Allowance{DenomId: key.K1(), Owner: key.K2(), Spender: key.K3(), Amount: v})
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Flags.Walk(ctx, nil, func(_ collections.Pair[string, string], f types.AccountFlags) (bool, error) {
		gs.Flags = append(gs.Flags, f)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Redemptions.Walk(ctx, nil, func(_ uint64, r types.Redemption) (bool, error) {
		gs.Redemptions = append(gs.Redemptions, r)
		return false, nil
	}); err != nil {
		return nil, err
	}
	seq, err := k.RedemptionIDSeq.Peek(ctx)
	if err != nil {
		return nil, err
	}
	gs.RedemptionIdSeq = seq
	return gs, nil
}
