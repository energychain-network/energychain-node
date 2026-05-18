package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/dataslash/types"
)

// EndBlock runs two bounded sweeps:
//
//  1. Auto-unjail providers whose jail_until has passed. The
//     index is ordered by jail_until so we can stop the walk
//     as soon as we see a future timestamp.
//
//  2. Auto-release UNBONDING providers that have crossed
//     withdraw_at. The bond is refunded best-effort: if the
//     receiver is sanctioned/blocked, the residual stays in
//     the pool but the provider still flips to WITHDRAWN so
//     the index doesn't keep growing on every block.
//
// Both sweeps respect params.max_processings_per_block as
// the per-block budget. Crossing that budget is normal in a
// chain-restart catch-up window; the index ordering means
// the *next* block resumes exactly where this one stopped.
func (k Keeper) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	p, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	budget := uint32(0)
	cap := p.MaxProcessingsPerBlock
	if cap == 0 {
		cap = types.DefaultMaxProcessingsPerBlock
	}
	now := sdkCtx.BlockTime().Unix()

	if err := k.autoUnjail(ctx, now, &budget, cap); err != nil {
		return err
	}
	if err := k.autoWithdrawUnbonded(ctx, now, &budget, cap); err != nil {
		return err
	}
	return nil
}

func (k Keeper) autoUnjail(ctx context.Context, now int64, budget *uint32, cap uint32) error {
	if *budget >= cap {
		return nil
	}
	// Snapshot the (jail_until, id) pairs we'll process so we
	// don't mutate the index mid-walk (which Set/Remove on
	// the index would otherwise invalidate). The snapshot is
	// bounded by `cap`, so memory pressure stays O(cap).
	type entry struct {
		jailUntil int64
		id        uint64
	}
	pending := make([]entry, 0, cap-*budget)
	stop := false
	if err := k.ProviderByJailUntil.Walk(ctx, nil, func(p collections.Pair[int64, uint64]) (bool, error) {
		if p.K1() > now {
			stop = true
			return true, nil
		}
		pending = append(pending, entry{jailUntil: p.K1(), id: p.K2()})
		if uint32(len(pending))+*budget >= cap {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return err
	}
	_ = stop
	for _, e := range pending {
		prov, ok, err := k.GetProvider(ctx, e.id)
		if err != nil {
			return err
		}
		if !ok {
			// Provider gone — defensively clear the dangling
			// index row to keep the sweep idempotent.
			_ = k.ProviderByJailUntil.Remove(ctx, collections.Join(e.jailUntil, e.id))
			continue
		}
		if prov.Status != types.ProviderStatus_PROVIDER_STATUS_JAILED || prov.JailUntil > now {
			// State drifted between snapshot and apply (e.g.
			// manual unjail). Clear the stale index entry.
			_ = k.ProviderByJailUntil.Remove(ctx, collections.Join(e.jailUntil, e.id))
			continue
		}
		prov.Status = types.ProviderStatus_PROVIDER_STATUS_ACTIVE
		prov.JailUntil = 0
		prov.UpdatedAt = now
		if err := k.SetProvider(ctx, prov); err != nil {
			return err
		}
		k.emit(ctx, "dataslash.provider.auto_unjailed",
			sdk.NewAttribute("id", u64s(prov.Id)),
		)
		k.recordAudit(ctx, prov.Id, "provider.auto_unjail", k.authority, prov.Did, "")
		*budget++
		if *budget >= cap {
			break
		}
	}
	return nil
}

func (k Keeper) autoWithdrawUnbonded(ctx context.Context, now int64, budget *uint32, cap uint32) error {
	if *budget >= cap {
		return nil
	}
	type entry struct {
		withdrawAt int64
		id         uint64
	}
	pending := make([]entry, 0, cap-*budget)
	if err := k.ProviderByWithdrawAt.Walk(ctx, nil, func(p collections.Pair[int64, uint64]) (bool, error) {
		if p.K1() > now {
			return true, nil
		}
		pending = append(pending, entry{withdrawAt: p.K1(), id: p.K2()})
		if uint32(len(pending))+*budget >= cap {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return err
	}
	for _, e := range pending {
		prov, ok, err := k.GetProvider(ctx, e.id)
		if err != nil {
			return err
		}
		if !ok {
			_ = k.ProviderByWithdrawAt.Remove(ctx, collections.Join(e.withdrawAt, e.id))
			continue
		}
		if prov.Status != types.ProviderStatus_PROVIDER_STATUS_UNBONDING || prov.WithdrawAt > now {
			_ = k.ProviderByWithdrawAt.Remove(ctx, collections.Join(e.withdrawAt, e.id))
			continue
		}
		amount := prov.BondAmount
		k.tryRefundOrForfeit(ctx, prov.Id, prov.BondDenom, prov.BondOwner, amount, "auto_withdraw")
		// Whether the refund landed or was forfeited, the
		// provider transitions to WITHDRAWN: bond is no
		// longer the chain's responsibility and the index
		// row is removed via SetProvider's prev-status diff.
		prov.BondAmount = 0
		prov.Status = types.ProviderStatus_PROVIDER_STATUS_WITHDRAWN
		prov.WithdrawAt = 0
		prov.UpdatedAt = now
		if err := k.SetProvider(ctx, prov); err != nil {
			return err
		}
		k.emit(ctx, "dataslash.provider.auto_withdrawn",
			sdk.NewAttribute("id", u64s(prov.Id)),
			sdk.NewAttribute("amount", u64s(amount)),
		)
		k.recordAudit(ctx, prov.Id, "provider.auto_withdraw", k.authority, prov.Did, "")
		*budget++
		if *budget >= cap {
			break
		}
	}
	return nil
}
