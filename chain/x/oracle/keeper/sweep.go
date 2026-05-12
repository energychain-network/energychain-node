package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/oracle/types"
)

// SweepBondReleases scans the queue up to the current block height and
// sets WITHDRAWING providers' BondReleaseHeight to a sentinel that
// MsgWithdrawBond uses to permit a release. We deliberately do NOT
// auto-release bonds in EndBlock — releasing requires a transaction
// signature so the provider proves possession of their key, and so the
// outflow is auditable as a tx in the block stream.
//
// The sweep here only moves the queue entry's marker (no-op today, kept
// as a hook for future ABCI-side cleanup); MsgWithdrawBond does the
// actual coin movement when the provider asks.
func (k Keeper) SweepBondReleases(ctx sdk.Context) (int, error) {
	height := ctx.BlockHeight()
	rng := new(collections.Range[collections.Pair[int64, string]]).
		StartInclusive(collections.PairPrefix[int64, string](0)).
		EndExclusive(collections.PairPrefix[int64, string](height + 1))

	type entry struct {
		height int64
		addr   string
	}
	var hits []entry
	if err := k.BondReleaseQueue.Walk(ctx, rng, func(key collections.Pair[int64, string]) (bool, error) {
		hits = append(hits, entry{height: key.K1(), addr: key.K2()})
		return len(hits) >= 256, nil
	}); err != nil {
		return 0, fmt.Errorf("walk bond release queue: %w", err)
	}
	// Drop the queue entries; the provider's status remains WITHDRAWING
	// so MsgWithdrawBond knows the cooldown has elapsed.
	for _, e := range hits {
		_ = k.DequeueBondRelease(ctx, e.height, e.addr)
	}
	return len(hits), nil
}

// SweepExpiredSuspensions flips providers whose suspended_until has
// elapsed back to ACTIVE. Bounded per-block to keep gas predictable.
func (k Keeper) SweepExpiredSuspensions(ctx sdk.Context) (int, error) {
	now := ctx.BlockTime().Unix()
	if now == 0 {
		return 0, nil
	}
	processed := 0
	const maxPerBlock = 256

	type pending struct {
		addr string
	}
	var hits []pending

	err := k.Providers.Walk(ctx, nil, func(_ string, p types.Provider) (bool, error) {
		if p.Status != types.ProviderStatus_PROVIDER_STATUS_SUSPENDED {
			return false, nil
		}
		if p.SuspendedUntil > 0 && p.SuspendedUntil <= now {
			hits = append(hits, pending{addr: p.Address})
			processed++
			if processed >= maxPerBlock {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk providers: %w", err)
	}
	for _, h := range hits {
		p, ok := k.GetProvider(ctx, h.addr)
		if !ok {
			continue
		}
		p.Status = types.ProviderStatus_PROVIDER_STATUS_ACTIVE
		p.SuspendedUntil = 0
		p.SuspendedReason = ""
		if err := k.SetProvider(ctx, p); err != nil {
			return 0, fmt.Errorf("flip provider %s: %w", h.addr, err)
		}
	}
	return len(hits), nil
}
