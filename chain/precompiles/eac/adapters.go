package eac

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	eackeeper "energychain/x/eac/keeper"
	eactypes "energychain/x/eac/types"
)

type keeperAdapter struct {
	k eackeeper.Keeper
}

func (a keeperAdapter) GetBalance(ctx context.Context, certID uint64, holder string) (uint64, error) {
	return a.k.GetBalance(ctx, certID, holder)
}

func (a keeperAdapter) GetCertificateIssuedUnits(ctx sdk.Context, certID uint64) (uint64, bool) {
	return a.k.GetCertificateIssuedUnits(ctx, certID)
}

type msgServerAdapter struct {
	srv eactypes.MsgServer
}

func (a msgServerAdapter) Transfer(ctx sdk.Context, msg *eactypes.MsgTransfer) error {
	_, err := a.srv.Transfer(sdk.WrapSDKContext(ctx), msg)
	return err
}

func (a msgServerAdapter) Retire(ctx sdk.Context, msg *eactypes.MsgRetire) (uint64, error) {
	resp, err := a.srv.Retire(sdk.WrapSDKContext(ctx), msg)
	if err != nil {
		return 0, err
	}
	return resp.RetirementId, nil
}
