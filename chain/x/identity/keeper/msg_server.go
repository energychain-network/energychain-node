package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/identity/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (m msgServer) isAdmin(ctx sdk.Context, address string) bool {
	if address == m.GetAuthority() {
		return true
	}
	params := m.GetParams(ctx)
	if params.AdminAddress != "" && params.AdminAddress == address {
		return true
	}
	return false
}

func (m msgServer) RegisterIdentity(goCtx context.Context, msg *types.MsgRegisterIdentity) (*types.MsgRegisterIdentityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.isAdmin(ctx, msg.Creator) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only the admin can register identities")
	}

	params := m.GetParams(ctx)
	if params.MaxMetadataSize > 0 && uint32(len(msg.Metadata)) > params.MaxMetadataSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metadata size %d exceeds max %d", len(msg.Metadata), params.MaxMetadataSize)
	}

	if m.IsRegistered(ctx, msg.Address) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("identity already registered: %s", msg.Address)
	}

	identity := types.Identity{
		Address:      msg.Address,
		Name:         msg.Name,
		Role:         msg.Role,
		Status:       types.StatusActive,
		Metadata:     msg.Metadata,
		RegisteredAt: ctx.BlockTime().Unix(),
		UpdatedAt:    ctx.BlockTime().Unix(),
	}

	if err := m.SetIdentity(ctx, identity); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("storing identity: %s", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"identity_registered",
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("role", msg.Role),
		sdk.NewAttribute("name", msg.Name),
		sdk.NewAttribute("metadata", msg.Metadata),
	))

	return &types.MsgRegisterIdentityResponse{}, nil
}

func (m msgServer) UpdateIdentity(goCtx context.Context, msg *types.MsgUpdateIdentity) (*types.MsgUpdateIdentityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	identity, found := m.GetIdentity(ctx, msg.Address)
	if !found {
		return nil, sdkerrors.ErrNotFound.Wrapf("identity not found: %s", msg.Address)
	}

	isAdmin := m.isAdmin(ctx, msg.Creator)
	isOwner := msg.Creator == msg.Address
	if !isAdmin && !isOwner {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only admin or identity owner can update")
	}

	params := m.GetParams(ctx)
	if params.MaxMetadataSize > 0 && uint32(len(msg.Metadata)) > params.MaxMetadataSize {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("metadata size %d exceeds max %d", len(msg.Metadata), params.MaxMetadataSize)
	}

	if msg.Name != "" {
		identity.Name = msg.Name
	}
	if msg.Metadata != "" {
		identity.Metadata = msg.Metadata
	}
	identity.UpdatedAt = ctx.BlockTime().Unix()

	if err := m.SetIdentity(ctx, identity); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("storing identity: %s", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"identity_updated",
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("name", identity.Name),
		sdk.NewAttribute("metadata", identity.Metadata),
		sdk.NewAttribute("updated_by", msg.Creator),
	))

	return &types.MsgUpdateIdentityResponse{}, nil
}

func (m msgServer) RevokeIdentity(goCtx context.Context, msg *types.MsgRevokeIdentity) (*types.MsgRevokeIdentityResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !m.isAdmin(ctx, msg.Creator) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("only the admin can revoke identities")
	}

	identity, found := m.GetIdentity(ctx, msg.Address)
	if !found {
		return nil, sdkerrors.ErrNotFound.Wrapf("identity not found: %s", msg.Address)
	}

	if identity.Status == types.StatusRevoked {
		return nil, sdkerrors.ErrInvalidRequest.Wrap("identity is already revoked")
	}

	identity.Status = types.StatusRevoked
	identity.UpdatedAt = ctx.BlockTime().Unix()

	if err := m.SetIdentity(ctx, identity); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("storing identity: %s", err)
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"identity_revoked",
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("reason", msg.Reason),
		sdk.NewAttribute("revoked_by", msg.Creator),
	))

	return &types.MsgRevokeIdentityResponse{}, nil
}

// UpdateParams replaces the entire identity module Params. Authority is the
// gov module address; any other signer is rejected.
func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if msg.Authority != m.GetAuthority() {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("invalid authority: expected %s, got %s", m.GetAuthority(), msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid params: %s", err)
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, fmt.Errorf("setting params: %w", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
