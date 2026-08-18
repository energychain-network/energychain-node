package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/offering/types"
)

// EndBlock advances offering lifecycle automatically so the chain — not a
// manual issuer/keeper tx — drives the time- and cap-triggered transitions:
//
//   - an OPEN offering whose window has ended OR whose hard cap is met is
//     closed (SUCCEEDED if it reached the soft cap, otherwise FAILED);
//   - a SUCCEEDED offering whose scheduled yield injections are delinquent
//     is flagged DEFAULTED, snapshotting the remaining treasury for pro-rata
//     refunds.
//
// Manual MsgCloseOffering / MsgFlagDefault remain available (e.g. to close a
// hard-cap-met raise in the same block); this loop is the safety net that
// guarantees no live offering is left hanging past its deadlines. State is
// collected before mutation so we never write while walking the map.
func (k Keeper) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()

	var toClose, toDefault []types.Offering
	if err := k.Offerings.Walk(ctx, nil, func(_ uint64, o types.Offering) (bool, error) {
		switch o.Status {
		case types.OfferingStatus_OFFERING_STATUS_OPEN:
			if now >= o.EndTime || o.Raised >= o.HardCap {
				toClose = append(toClose, o)
			}
		case types.OfferingStatus_OFFERING_STATUS_SUCCEEDED:
			if k.injectionsDelinquent(now, o) {
				toDefault = append(toDefault, o)
			}
		}
		return false, nil
	}); err != nil {
		return err
	}

	for _, o := range toClose {
		if o.Raised >= o.SoftCap {
			o.Status = types.OfferingStatus_OFFERING_STATUS_SUCCEEDED
			o.SucceededAt = now
			o.AllocatedUnits = o.Raised / o.UnitPrice
		} else {
			o.Status = types.OfferingStatus_OFFERING_STATUS_FAILED
		}
		o.UpdatedAt = now
		if err := k.setOffering(ctx, o); err != nil {
			return err
		}
		emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "auto_close",
			types.AttrOfferingID, u(o.Id), types.AttrStatus, o.Status.String())
	}

	for _, o := range toDefault {
		o.Status = types.OfferingStatus_OFFERING_STATUS_DEFAULTED
		o.DefaultTreasury = k.TreasuryBalance(ctx, o)
		o.UpdatedAt = now
		if err := k.setOffering(ctx, o); err != nil {
			return err
		}
		emitEvent(sdkCtx, types.EventTypeOffering, types.AttrAction, "auto_default",
			types.AttrOfferingID, u(o.Id), types.AttrAmount, u(o.DefaultTreasury))
	}
	return nil
}

// injectionsDelinquent reports whether a successful offering has missed a
// scheduled yield injection — the same gate MsgFlagDefault enforces.
func (k Keeper) injectionsDelinquent(now int64, o types.Offering) bool {
	if o.InjectionsDone >= o.TotalTranches {
		return false
	}
	due := types.DueInjections(now, o.SucceededAt, o.InjectionInterval, o.TotalTranches)
	return due > o.InjectionsDone
}
