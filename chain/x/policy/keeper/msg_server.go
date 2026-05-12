package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"energychain/x/policy/types"
)

type msgServer struct {
	Keeper
}

func NewMsgServerImpl(k Keeper) types.MsgServer { return &msgServer{Keeper: k} }

var _ types.MsgServer = msgServer{}

func (m msgServer) onlyAuthority(authority string) error {
	if authority != m.GetAuthority() {
		return sdkerrors.ErrUnauthorized.Wrapf("expected authority %s", m.GetAuthority())
	}
	return nil
}

// ---------------------------------------------------------------------------
// Policy lifecycle
// ---------------------------------------------------------------------------

func (m msgServer) RegisterPolicy(goCtx context.Context, msg *types.MsgRegisterPolicy) (*types.MsgRegisterPolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if m.HasPolicy(ctx, msg.Id) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("policy %s already exists", msg.Id)
	}
	params := m.GetParams(ctx)
	if m.CountPolicies(ctx) >= params.MaxPolicies {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"max_policies cap %d reached; raise via governance before registering more", params.MaxPolicies)
	}
	now := ctx.BlockTime().Unix()
	p := types.Policy{
		Id:          msg.Id,
		Name:        msg.Name,
		Description: msg.Description,
		Version:     msg.Version,
		Status:      types.PolicyStatus_POLICY_STATUS_ACTIVE,
		Rules:       msg.Rules,
		CreatedBy:   msg.Authority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := types.ValidatePolicyShape(p, params.MaxParamValueSize, params.MaxRulesPerPolicy); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("policy shape: %s", err)
	}
	if err := m.SetPolicy(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "policy_registered",
		"policy_id", p.Id, "name", p.Name, "version", p.Version,
		"rules", fmt.Sprintf("%d", len(p.Rules)))
	return &types.MsgRegisterPolicyResponse{}, nil
}

func (m msgServer) UpdatePolicy(goCtx context.Context, msg *types.MsgUpdatePolicy) (*types.MsgUpdatePolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	prior, ok := m.GetPolicy(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("policy %s", msg.Id)
	}
	if prior.Status == types.PolicyStatus_POLICY_STATUS_DISABLED {
		// SECURITY: a DISABLED policy must be explicitly re-enabled via
		// a separate governance step before it can be edited; otherwise
		// a disabled policy could be silently rewritten to permissive
		// rules and then re-bound. Today we simply refuse the update.
		return nil, sdkerrors.ErrInvalidRequest.Wrap(
			"cannot update DISABLED policy; restore via gov MsgRegisterPolicy under a new id")
	}
	params := m.GetParams(ctx)
	next := prior
	if msg.Name != "" {
		next.Name = msg.Name
	}
	if msg.Description != "" {
		next.Description = msg.Description
	}
	if msg.Version != "" {
		next.Version = msg.Version
	}
	if msg.Rules != nil {
		next.Rules = msg.Rules
	}
	next.UpdatedAt = ctx.BlockTime().Unix()
	if err := types.ValidatePolicyShape(next, params.MaxParamValueSize, params.MaxRulesPerPolicy); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("policy shape: %s", err)
	}
	if err := m.SetPolicy(ctx, next); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "policy_updated", "policy_id", next.Id, "version", next.Version)
	return &types.MsgUpdatePolicyResponse{}, nil
}

func (m msgServer) DeprecatePolicy(goCtx context.Context, msg *types.MsgDeprecatePolicy) (*types.MsgDeprecatePolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetPolicy(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("policy %s", msg.Id)
	}
	p.Status = types.PolicyStatus_POLICY_STATUS_DEPRECATED
	p.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetPolicy(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "policy_deprecated", "policy_id", p.Id, "reason", msg.Reason)
	return &types.MsgDeprecatePolicyResponse{}, nil
}

func (m msgServer) DisablePolicy(goCtx context.Context, msg *types.MsgDisablePolicy) (*types.MsgDisablePolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetPolicy(ctx, msg.Id)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("policy %s", msg.Id)
	}
	p.Status = types.PolicyStatus_POLICY_STATUS_DISABLED
	p.UpdatedAt = ctx.BlockTime().Unix()
	if err := m.SetPolicy(ctx, p); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "policy_disabled", "policy_id", p.Id, "reason", msg.Reason)
	return &types.MsgDisablePolicyResponse{}, nil
}

// ---------------------------------------------------------------------------
// Bindings
// ---------------------------------------------------------------------------

func (m msgServer) BindPolicy(goCtx context.Context, msg *types.MsgBindPolicy) (*types.MsgBindPolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	p, ok := m.GetPolicy(ctx, msg.PolicyId)
	if !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("policy %s", msg.PolicyId)
	}
	// SECURITY: only ACTIVE policies accept new bindings. DEPRECATED is
	// the half-life status (existing bindings continue) but no NEW
	// bindings allowed; DISABLED rejects everything.
	if p.Status != types.PolicyStatus_POLICY_STATUS_ACTIVE {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"policy %s status %s does not accept new bindings", p.Id, p.Status.String())
	}
	if _, exists := m.GetBinding(ctx, msg.AssetClass, msg.AssetId); exists {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"asset (%s, %s) already bound; unbind first", msg.AssetClass, msg.AssetId)
	}
	params := m.GetParams(ctx)
	if m.CountBindings(ctx) >= params.MaxBindings {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf(
			"max_bindings cap %d reached; raise via governance", params.MaxBindings)
	}
	b := types.PolicyBinding{
		AssetClass: msg.AssetClass,
		AssetId:    msg.AssetId,
		PolicyId:   msg.PolicyId,
		BoundBy:    msg.Authority,
		BoundAt:    ctx.BlockTime().Unix(),
	}
	if err := m.SetBinding(ctx, b); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	emit(ctx, "policy_bound",
		"asset_class", b.AssetClass, "asset_id", b.AssetId, "policy_id", b.PolicyId)
	return &types.MsgBindPolicyResponse{}, nil
}

func (m msgServer) UnbindPolicy(goCtx context.Context, msg *types.MsgUnbindPolicy) (*types.MsgUnbindPolicyResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if _, ok := m.GetBinding(ctx, msg.AssetClass, msg.AssetId); !ok {
		return nil, sdkerrors.ErrNotFound.Wrapf("binding (%s, %s)", msg.AssetClass, msg.AssetId)
	}
	if err := m.RemoveBinding(ctx, msg.AssetClass, msg.AssetId); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("remove: %s", err)
	}
	emit(ctx, "policy_unbound",
		"asset_class", msg.AssetClass, "asset_id", msg.AssetId, "reason", msg.Reason)
	return &types.MsgUnbindPolicyResponse{}, nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m msgServer) UpdateParams(goCtx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := m.onlyAuthority(msg.Authority); err != nil {
		return nil, err
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := msg.Params.Validate(); err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("params: %s", err)
	}
	if err := m.SetParams(ctx, msg.Params); err != nil {
		return nil, sdkerrors.ErrLogic.Wrapf("set: %s", err)
	}
	return &types.MsgUpdateParamsResponse{}, nil
}
