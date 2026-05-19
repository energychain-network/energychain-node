package market

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	marketkeeper "energychain/x/market/keeper"
	markettypes "energychain/x/market/types"
)

type keeperAdapter struct {
	k marketkeeper.Keeper
}

func (a keeperAdapter) GetOrder(ctx context.Context, id uint64) (markettypes.Order, bool, error) {
	return a.k.GetOrder(ctx, id)
}

func (a keeperAdapter) GetPosition(ctx context.Context, pairID uint64, owner string) (markettypes.Position, error) {
	return a.k.GetPosition(ctx, pairID, owner)
}

type msgServerAdapter struct {
	srv markettypes.MsgServer
}

func (a msgServerAdapter) PlaceLimitOrder(
	ctx sdk.Context,
	msg *markettypes.MsgPlaceLimitOrder,
) (orderID, filledQty, remainingQty uint64, err error) {
	resp, err := a.srv.PlaceLimitOrder(sdk.WrapSDKContext(ctx), msg)
	if err != nil {
		return 0, 0, 0, err
	}
	return resp.OrderId, resp.FilledQty, resp.RemainingQty, nil
}

func (a msgServerAdapter) CancelOrder(
	ctx sdk.Context,
	msg *markettypes.MsgCancelOrder,
) (uint64, error) {
	resp, err := a.srv.CancelOrder(sdk.WrapSDKContext(ctx), msg)
	if err != nil {
		return 0, err
	}
	return resp.RefundedAmount, nil
}
