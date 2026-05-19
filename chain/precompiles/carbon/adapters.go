package carbon

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	carbonkeeper "energychain/x/carbon/keeper"
	carbontypes "energychain/x/carbon/types"
)

type keeperAdapter struct {
	k carbonkeeper.Keeper
}

func (a keeperAdapter) GetBalance(ctx context.Context, assetID uint64, holder string) (uint64, error) {
	return a.k.GetBalance(ctx, assetID, holder)
}

func (a keeperAdapter) GetEACClaimed(ctx context.Context, eacCertID uint64) (uint64, error) {
	return a.k.GetEACClaimed(ctx, eacCertID)
}

type msgServerAdapter struct {
	srv carbontypes.MsgServer
}

func (a msgServerAdapter) Transfer(ctx sdk.Context, msg *carbontypes.MsgTransfer) error {
	_, err := a.srv.Transfer(sdk.WrapSDKContext(ctx), msg)
	return err
}

func (a msgServerAdapter) Retire(ctx sdk.Context, msg *carbontypes.MsgRetire) (uint64, error) {
	resp, err := a.srv.Retire(sdk.WrapSDKContext(ctx), msg)
	if err != nil {
		return 0, err
	}
	return resp.RetirementId, nil
}
