package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	"energychain/x/rwa/types"
)

func (k Keeper) InitGenesis(ctx context.Context, gs *types.GenesisState) error {
	if err := k.SetParams(ctx, gs.Params); err != nil {
		return err
	}
	for _, is := range gs.Issuers {
		if err := k.Issuers.Set(ctx, is.Id, is); err != nil {
			return err
		}
	}
	for _, t := range gs.Tokens {
		if err := k.setToken(ctx, t); err != nil {
			return err
		}
	}
	for _, b := range gs.Balances {
		if err := k.setBalance(ctx, b.TokenId, b.Account, b.Amount); err != nil {
			return err
		}
	}
	for _, f := range gs.AccountFlags {
		if err := k.AccountFlags.Set(ctx, collections.Join(f.TokenId, f.Account), f); err != nil {
			return err
		}
	}
	for _, fb := range gs.Frozen {
		if fb.Amount > 0 {
			if err := k.Frozen.Set(ctx, collections.Join(fb.TokenId, fb.Account), fb); err != nil {
				return err
			}
		}
	}
	for _, l := range gs.Lockups {
		if err := k.Lockups.Set(ctx, collections.Join3(l.TokenId, l.Account, l.Id), l); err != nil {
			return err
		}
	}
	for _, s := range gs.Snapshots {
		if err := k.Snapshots.Set(ctx, s.Id, s); err != nil {
			return err
		}
		if err := k.SnapshotByToken.Set(ctx, collections.Join(s.TokenId, s.Id)); err != nil {
			return err
		}
	}
	for _, sb := range gs.SnapshotBalances {
		if err := k.SnapshotBalances.Set(ctx, collections.Join(sb.SnapshotId, sb.Account), sb.Amount); err != nil {
			return err
		}
	}
	for _, d := range gs.Distributions {
		if err := k.Distributions.Set(ctx, d.Id, d); err != nil {
			return err
		}
		if err := k.DistributionByToken.Set(ctx, collections.Join(d.TokenId, d.Id)); err != nil {
			return err
		}
	}
	for _, c := range gs.DistributionClaims {
		if err := k.DistributionClaims.Set(ctx, collections.Join(c.DistributionId, c.Account), c); err != nil {
			return err
		}
	}
	for _, r := range gs.Redemptions {
		if err := k.Redemptions.Set(ctx, r.Id, r); err != nil {
			return err
		}
		if err := k.RedemptionByHolder.Set(ctx, collections.Join(r.Holder, r.Id)); err != nil {
			return err
		}
		if r.Status == types.RedemptionStatus_REDEMPTION_STATUS_PENDING {
			if err := k.RedemptionPendingByTok.Set(ctx, collections.Join(r.TokenId, r.Id)); err != nil {
				return err
			}
			if err := k.RedemptionPendingByHolder.Set(ctx, collections.Join(r.Holder, r.Id)); err != nil {
				return err
			}
		}
	}
	for _, p := range gs.PendingRedemptionUnits {
		if p.Amount > 0 {
			if err := k.PendingRedemptionUnits.Set(ctx, p.TokenId, p.Amount); err != nil {
				return err
			}
		}
	}

	if gs.TokenIdSeq > 0 {
		if err := k.TokenIDSeq.Set(ctx, gs.TokenIdSeq); err != nil {
			return err
		}
	}
	if gs.LockupIdSeq > 0 {
		if err := k.LockupSeq.Set(ctx, gs.LockupIdSeq); err != nil {
			return err
		}
	}
	if gs.SnapshotIdSeq > 0 {
		if err := k.SnapshotIDSeq.Set(ctx, gs.SnapshotIdSeq); err != nil {
			return err
		}
	}
	if gs.DistributionIdSeq > 0 {
		if err := k.DistributionIDSeq.Set(ctx, gs.DistributionIdSeq); err != nil {
			return err
		}
	}
	if gs.RedemptionIdSeq > 0 {
		if err := k.RedemptionIDSeq.Set(ctx, gs.RedemptionIdSeq); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	gs := &types.GenesisState{Params: params}

	if err := k.Issuers.Walk(ctx, nil, func(_ string, v types.Issuer) (bool, error) {
		gs.Issuers = append(gs.Issuers, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Tokens.Walk(ctx, nil, func(_ uint64, v types.Token) (bool, error) {
		gs.Tokens = append(gs.Tokens, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Balances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		gs.Balances = append(gs.Balances, types.Balance{TokenId: key.K1(), Account: key.K2(), Amount: v})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.AccountFlags.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.AccountFlags) (bool, error) {
		gs.AccountFlags = append(gs.AccountFlags, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Frozen.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.FrozenBalance) (bool, error) {
		gs.Frozen = append(gs.Frozen, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Lockups.Walk(ctx, nil, func(_ collections.Triple[uint64, string, uint64], v types.Lockup) (bool, error) {
		gs.Lockups = append(gs.Lockups, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Snapshots.Walk(ctx, nil, func(_ uint64, v types.SnapshotMeta) (bool, error) {
		gs.Snapshots = append(gs.Snapshots, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.SnapshotBalances.Walk(ctx, nil, func(key collections.Pair[uint64, string], v uint64) (bool, error) {
		gs.SnapshotBalances = append(gs.SnapshotBalances, types.SnapshotBalance{
			SnapshotId: key.K1(), Account: key.K2(), Amount: v,
		})
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Distributions.Walk(ctx, nil, func(_ uint64, v types.Distribution) (bool, error) {
		gs.Distributions = append(gs.Distributions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.DistributionClaims.Walk(ctx, nil, func(_ collections.Pair[uint64, string], v types.DistributionClaim) (bool, error) {
		gs.DistributionClaims = append(gs.DistributionClaims, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.Redemptions.Walk(ctx, nil, func(_ uint64, v types.RedemptionRequest) (bool, error) {
		gs.Redemptions = append(gs.Redemptions, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	if err := k.PendingRedemptionUnits.Walk(ctx, nil, func(k uint64, v uint64) (bool, error) {
		gs.PendingRedemptionUnits = append(gs.PendingRedemptionUnits, types.PendingRedemptionUnits{TokenId: k, Amount: v})
		return false, nil
	}); err != nil {
		return nil, err
	}

	if gs.TokenIdSeq, err = k.TokenIDSeq.Peek(ctx); err != nil {
		return nil, err
	}
	if gs.LockupIdSeq, err = k.LockupSeq.Peek(ctx); err != nil {
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

// init-time invariant: keep package import surface stable.
var _ = fmt.Sprintf
