package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/rwatoken/types"
)

// EndBlock auto-expires fixed-tenor tokens: any ACTIVE token whose
// maturity_time has been reached transitions to MATURED so that — without
// any issuer/keeper tx — transfers and minting stop and only redemption
// remains open. This is the on-chain expiry of a sold revenue right (e.g.
// "future 3-10 years of solar revenue"): once the term ends the token can
// no longer change hands, holders simply cash out via redemption.
//
// PAUSED tokens are left untouched (an admin paused them deliberately);
// only the ACTIVE → MATURED edge is automated, and it is one-way. State is
// collected before mutation so we never write while walking the map. The
// walk is bounded by params.max_tokens.
func (k Keeper) EndBlock(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()

	var matured []types.Token
	if err := k.Tokens.Walk(ctx, nil, func(_ uint64, t types.Token) (bool, error) {
		if t.Status == types.TokenStatus_TOKEN_STATUS_ACTIVE &&
			t.MaturityTime > 0 && now >= t.MaturityTime {
			matured = append(matured, t)
		}
		return false, nil
	}); err != nil {
		return err
	}

	for _, t := range matured {
		t.Status = types.TokenStatus_TOKEN_STATUS_MATURED
		t.UpdatedAt = now
		if err := k.Tokens.Set(ctx, t.Id, t); err != nil {
			return err
		}
		k.recordAudit(ctx, "auto_mature", "", u(t.Id), "")
		emitEvent(sdkCtx, types.EventTypeToken, types.AttrAction, "auto_mature", types.AttrTokenID, u(t.Id))
	}
	return nil
}
