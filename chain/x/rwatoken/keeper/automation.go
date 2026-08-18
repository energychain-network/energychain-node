package keeper

import (
	"context"

	"energychain/x/rwatoken/types"
)

// SnapshotHolders is the cross-module entrypoint (x/automation cron) for an
// automated holder snapshot. It is a thin wrapper over the TakeSnapshot
// handler so the admin authorization and snapshot bookkeeping stay in one
// place; `admin` must be the token admin.
func (k Keeper) SnapshotHolders(ctx context.Context, admin string, tokenID uint64) (uint64, error) {
	resp, err := NewMsgServerImpl(k).TakeSnapshot(ctx, &types.MsgTakeSnapshot{Admin: admin, TokenId: tokenID})
	if err != nil {
		return 0, err
	}
	return resp.SnapshotId, nil
}

// DistributeDividend is the cross-module entrypoint (x/automation cron) for an
// automated pro-rata dividend. It escrows `amount` of the token's settlement
// denom from `admin` against `snapshotID`; `admin` must be the token admin.
func (k Keeper) DistributeDividend(ctx context.Context, admin string, tokenID, snapshotID, amount uint64) (uint64, error) {
	resp, err := NewMsgServerImpl(k).CreateDistribution(ctx, &types.MsgCreateDistribution{
		Admin:       admin,
		TokenId:     tokenID,
		SnapshotId:  snapshotID,
		TotalAmount: amount,
	})
	if err != nil {
		return 0, err
	}
	return resp.DistributionId, nil
}
