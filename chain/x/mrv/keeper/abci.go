package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/mrv/types"
)

// EndBlock walks the GrantByExpiry index and flips grants
// whose expires_at <= now to Revoked. The walk is bounded by
// params.MaxGrantsPerBlockSweep — at chains with a large
// backlog of expired grants the rest are swept on subsequent
// blocks. Each flipped grant emits an event so off-chain
// indexers can drop access promptly.
//
// We do NOT delete the rows: revoked grants remain visible
// so regulators and auditors can prove "this DID had this
// access between block X and block Y". A separate housekeeping
// proposal can prune them later.
func (k Keeper) EndBlock(ctx context.Context) error {
	p, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	budget := p.MaxGrantsPerBlockSweep
	swept := uint32(0)

	// Walk by expiry asc. Stop as soon as we cross now or
	// exhaust the budget. The expiry index includes both
	// already-revoked and still-live grants; we skip the
	// already-revoked ones.
	rng := new(collections.Range[collections.Pair[int64, uint64]]).
		EndInclusive(collections.Join(now, uint64(1<<63-1)))
	if err := k.GrantByExpiry.Walk(ctx, rng, func(p collections.Pair[int64, uint64]) (bool, error) {
		if swept >= budget {
			return true, nil
		}
		id := p.K2()
		g, ok, err := k.GetGrant(ctx, id)
		if err != nil {
			return true, err
		}
		if !ok || g.Revoked {
			return false, nil
		}
		// Defense-in-depth: re-check against the canonical
		// timestamp on the grant in case the expiry index has
		// drifted (e.g., from a buggy migration).
		if g.ExpiresAt > now {
			return false, nil
		}
		g.Revoked = true
		if err := k.SetGrant(ctx, g); err != nil {
			return true, err
		}
		k.emit(ctx, "mrv.view_key.expired",
			sdk.NewAttribute("id", u64s(id)),
			sdk.NewAttribute("granter", g.Granter),
			sdk.NewAttribute("grantee_did", g.GranteeDid),
		)
		swept++
		return false, nil
	}); err != nil {
		return err
	}

	// Surface a single aggregate event per block so indexers
	// can observe sweep pressure without subscribing to each
	// individual expire event.
	if swept > 0 {
		k.emit(ctx, "mrv.view_key.sweep",
			sdk.NewAttribute("swept", u64s(uint64(swept))),
			sdk.NewAttribute("budget", u64s(uint64(budget))),
		)
		_ = types.ModuleName
	}
	return nil
}
