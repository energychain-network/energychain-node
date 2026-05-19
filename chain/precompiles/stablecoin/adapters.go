package stablecoin

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	stablecoinkeeper "energychain/x/stablecoin/keeper"
	stablecointypes "energychain/x/stablecoin/types"
)

// stablecoinKeeperAdapter pins the precompile to the keeper's narrow
// read surface. Lives in adapters.go (not stablecoin.go) so that
// app.go can inspect the live binding while tests can substitute a
// hand-rolled StablecoinKeeperAPI without dragging in real state.
type stablecoinKeeperAdapter struct {
	k stablecoinkeeper.Keeper
}

func (a stablecoinKeeperAdapter) HasDenom(ctx sdk.Context, denomID string) bool {
	return a.k.HasDenom(ctx, denomID)
}

func (a stablecoinKeeperAdapter) GetBalance(ctx sdk.Context, denomID, account string) uint64 {
	return a.k.GetBalance(ctx, denomID, account)
}

func (a stablecoinKeeperAdapter) GetSupply(ctx sdk.Context, denomID string) uint64 {
	return a.k.GetSupply(ctx, denomID)
}

func (a stablecoinKeeperAdapter) IsDenomPaused(ctx sdk.Context, denomID string) bool {
	return a.k.IsDenomPaused(ctx, denomID)
}

func (a stablecoinKeeperAdapter) IsAccountBlocked(ctx sdk.Context, denomID, account string) bool {
	return a.k.IsAccountBlocked(ctx, denomID, account)
}

func (a stablecoinKeeperAdapter) GetAllowance(ctx sdk.Context, denomID, owner, spender string) uint64 {
	return a.k.GetAllowance(ctx, denomID, owner, spender)
}

// msgServerAdapter wraps the keeper's generated MsgServer so the
// precompile can call Transfer / TransferFrom / Approve with the
// same compliance pipeline as a native tx — and so the precompile
// inherits the keeper's event emission verbatim.
type msgServerAdapter struct {
	srv stablecointypes.MsgServer
}

func (a msgServerAdapter) Transfer(ctx sdk.Context, msg *stablecointypes.MsgTransfer) error {
	_, err := a.srv.Transfer(sdkContext(ctx), msg)
	return err
}

func (a msgServerAdapter) TransferFrom(ctx sdk.Context, msg *stablecointypes.MsgTransferFrom) error {
	_, err := a.srv.TransferFrom(sdkContext(ctx), msg)
	return err
}

func (a msgServerAdapter) Approve(ctx sdk.Context, msg *stablecointypes.MsgApprove) error {
	_, err := a.srv.Approve(sdkContext(ctx), msg)
	return err
}

// sdkContext lifts an sdk.Context into the context.Context that
// generated MsgServers require, preserving the SDK ctx through
// UnwrapSDKContext on the other side.
func sdkContext(ctx sdk.Context) context.Context {
	return sdk.WrapSDKContext(ctx)
}
