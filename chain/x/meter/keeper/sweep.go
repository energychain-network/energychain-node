package keeper

import (
	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/meter/types"
)

// SweepExpiredStreams walks the (expires_at, id) queue and revokes any
// authorisation whose expiry has elapsed. It processes at most
// types.StreamSweepMaxPerBlock entries per call so a backlog can never
// blow the EndBlocker's gas budget.
//
// Returns the number of expired entries that were processed.
func (k Keeper) SweepExpiredStreams(ctx sdk.Context) (int, error) {
	now := ctx.BlockTime().Unix()
	type entry struct {
		expiresAt int64
		id        string
	}
	var pending []entry

	// Walk all queue entries with expires_at <= now. The PairPrefix range
	// is exclusive on the upper bound (now+1) and inclusive at the
	// implicit zero lower bound, mirroring oracle's bond-release sweep.
	rng := new(collections.Range[collections.Pair[int64, string]]).
		EndExclusive(collections.PairPrefix[int64, string](now + 1))

	if err := k.StreamByExpiry.Walk(ctx, rng, func(key collections.Pair[int64, string]) (bool, error) {
		pending = append(pending, entry{expiresAt: key.K1(), id: key.K2()})
		if len(pending) >= types.StreamSweepMaxPerBlock {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return 0, err
	}

	processed := 0
	for _, e := range pending {
		a, ok := k.GetStreamAuth(ctx, e.id)
		if !ok {
			// Drop the orphaned queue entry; corresponding row was
			// removed out-of-band (e.g. genesis migration).
			_ = k.StreamByExpiry.Remove(ctx, collections.Join(e.expiresAt, e.id))
			continue
		}
		if a.Revoked {
			// MarkStreamRevoked already removed the index entry on a
			// graceful path. This branch handles drift.
			_ = k.StreamByExpiry.Remove(ctx, collections.Join(e.expiresAt, e.id))
			continue
		}
		if err := k.MarkStreamRevoked(ctx, a, "expired"); err != nil {
			ctx.Logger().Error("meter: stream sweep mark", "id", e.id, "err", err)
			continue
		}
		processed++
		ctx.EventManager().EmitEvent(sdk.NewEvent("meter_stream_expired",
			sdk.NewAttribute("id", e.id),
			sdk.NewAttribute("mp", a.MeteringPointId),
		))
	}
	return processed, nil
}
