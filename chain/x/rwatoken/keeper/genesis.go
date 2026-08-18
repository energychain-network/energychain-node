package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"energychain/x/rwatoken/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := gs.Validate(); err != nil {
		return err
	}
	if err := k.Params.Set(ctx, gs.Params); err != nil {
		return err
	}
	for _, t := range gs.Tokens {
		if err := k.setToken(ctx, t); err != nil {
			return err
		}
	}
	for _, b := range gs.Balances {
		if err := k.Balances.Set(ctx, collections.Join(b.TokenId, b.Holder), b.Amount); err != nil {
			return err
		}
	}
	for _, f := range gs.Flags {
		if err := k.setFlags(ctx, f); err != nil {
			return err
		}
	}
	for _, s := range gs.Snapshots {
		if err := k.Snapshots.Set(ctx, s.Id, s); err != nil {
			return err
		}
	}
	for _, sb := range gs.SnapshotBalances {
		if err := k.SnapshotBalances.Set(ctx, collections.Join(sb.SnapshotId, sb.Holder), sb.Amount); err != nil {
			return err
		}
	}
	for _, d := range gs.Distributions {
		if err := k.Distributions.Set(ctx, d.Id, d); err != nil {
			return err
		}
	}
	for _, c := range gs.DistributionClaims {
		if err := k.DistributionClaim.Set(ctx, collections.Join(c.DistributionId, c.Holder), c); err != nil {
			return err
		}
	}
	for _, r := range gs.Redemptions {
		if err := k.setRedemption(ctx, r); err != nil {
			return err
		}
	}
	for _, iss := range gs.Issuers {
		if err := k.Issuers.Set(ctx, iss); err != nil {
			return err
		}
	}
	if err := k.TokenIDSeq.Set(ctx, gs.TokenIdSeq); err != nil {
		return err
	}
	if err := k.SnapshotIDSeq.Set(ctx, gs.SnapshotIdSeq); err != nil {
		return err
	}
	if err := k.DistributionIDSeq.Set(ctx, gs.DistributionIdSeq); err != nil {
		return err
	}
	return k.RedemptionIDSeq.Set(ctx, gs.RedemptionIdSeq)
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	gs := types.DefaultGenesis()
	p, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs.Params = p

	if err := k.Tokens.Walk(ctx, nil, func(_ uint64, v types.Token) (bool, error) {
		gs.Tokens = append(gs.Tokens, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		if v != 0 {
			gs.Balances = append(gs.Balances, types.Balance{TokenId: key.K1(), Holder: key.K2(), Amount: v})
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Flags.Walk(ctx, nil, func(_ collections.Pair[uint64, string], f types.AccountFlags) (bool, error) {
		gs.Flags = append(gs.Flags, f)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Snapshots.Walk(ctx, nil, func(_ uint64, s types.Snapshot) (bool, error) {
		gs.Snapshots = append(gs.Snapshots, s)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.SnapshotBalances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		if v != 0 {
			gs.SnapshotBalances = append(gs.SnapshotBalances, types.SnapshotBalanceRow{SnapshotId: key.K1(), Holder: key.K2(), Amount: v})
		}
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Distributions.Walk(ctx, nil, func(_ uint64, d types.Distribution) (bool, error) {
		gs.Distributions = append(gs.Distributions, d)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.DistributionClaim.Walk(ctx, nil, func(_ collections.Pair[uint64, string], c types.DistributionClaimRow) (bool, error) {
		gs.DistributionClaims = append(gs.DistributionClaims, c)
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
	if err := k.Issuers.Walk(ctx, nil, func(addr string) (bool, error) {
		gs.Issuers = append(gs.Issuers, addr)
		return false, nil
	}); err != nil {
		return nil, err
	}

	if gs.TokenIdSeq, err = k.TokenIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.SnapshotIdSeq, err = k.SnapshotIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.DistributionIdSeq, err = k.DistributionIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.RedemptionIdSeq, err = k.RedemptionIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	return gs, nil
}
