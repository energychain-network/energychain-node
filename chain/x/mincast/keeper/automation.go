package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/mincast/types"
)

// HasMarket reports whether a market exists (cross-module guard for
// x/automation schedules).
func (k Keeper) HasMarket(ctx context.Context, marketID uint64) bool {
	has, _ := k.Markets.Has(ctx, marketID)
	return has
}

// InjectTreasuryFrom is the cross-module entrypoint (x/automation cron) for an
// automated APY/fee injection: it routes through the InjectTreasury handler
// so the sanctions gate, empty-market guard, and floor-monotonicity bookkeeping
// all apply, funding from `funder`'s settlement balance.
func (k Keeper) InjectTreasuryFrom(ctx context.Context, funder string, marketID, amount uint64) error {
	_, err := NewMsgServerImpl(k).InjectTreasury(ctx, &types.MsgInjectTreasury{
		Funder:   funder,
		MarketId: marketID,
		Amount:   amount,
	})
	return err
}

// CloseMaturedInvests settles up to `limit` matured, still-active invests of a
// market (the automated maturity-redemption path). Individual failures (e.g. a
// momentarily underfunded reward pool) are skipped so one stuck invest cannot
// block the rest; the count of successfully closed invests is returned.
func (k Keeper) CloseMaturedInvests(ctx context.Context, caller string, marketID uint64, limit uint32) (uint32, error) {
	if limit == 0 {
		return 0, nil
	}
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Collect candidate ids first; collections does not guarantee safe
	// in-place mutation during a Walk.
	var candidates []uint64
	if err := k.Invests.Walk(ctx, nil, func(id uint64, iv types.Invest) (bool, error) {
		if iv.MarketId == marketID &&
			iv.Status == types.InvestStatus_INVEST_STATUS_ACTIVE &&
			iv.Maturity <= now {
			candidates = append(candidates, id)
			if uint32(len(candidates)) >= limit {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		return 0, err
	}

	srv := NewMsgServerImpl(k)
	var closed uint32
	for _, id := range candidates {
		if _, err := srv.CloseInvest(ctx, &types.MsgCloseInvest{Caller: caller, InvestId: id}); err != nil {
			continue
		}
		closed++
	}
	return closed, nil
}
