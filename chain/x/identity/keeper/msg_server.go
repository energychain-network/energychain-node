package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"energychain/x/identity/types"
)

type msgServer struct{ k Keeper }

func NewMsgServerImpl(k Keeper) types.MsgServer { return msgServer{k: k} }

var _ types.MsgServer = msgServer{}

func (s msgServer) requireAuthority(addr string) error {
	if addr != s.k.authority {
		return types.ErrUnauthorized.Wrapf("expected authority %s, got %s", s.k.authority, addr)
	}
	return nil
}

// ---- Registrar ------------------------------------------------------------

func (s msgServer) AddRegistrar(ctx context.Context, m *types.MsgAddRegistrar) (*types.MsgAddRegistrarResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.Registrars.Has(ctx, m.Registrar); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("registrar %s", m.Registrar)
	}
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	n, err := s.k.CountRegistrars(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxRegistrars {
		return nil, types.ErrLimitExceeded.Wrapf("max_registrars %d", params.MaxRegistrars)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.k.Registrars.Set(ctx, m.Registrar, types.Registrar{
		Address:     m.Registrar,
		DisplayName: m.DisplayName,
		AddedBy:     m.Authority,
		AddedAt:     sdkCtx.BlockTime().Unix(),
	}); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, "add_registrar", m.Authority, m.Registrar, m.DisplayName)
	emitEvent(sdkCtx, types.EventTypeRegistrar, "add", m.Registrar)
	return &types.MsgAddRegistrarResponse{}, nil
}

func (s msgServer) RemoveRegistrar(ctx context.Context, m *types.MsgRemoveRegistrar) (*types.MsgRemoveRegistrarResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.Registrars.Has(ctx, m.Registrar); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNotFound.Wrapf("registrar %s", m.Registrar)
	}
	if err := s.k.Registrars.Remove(ctx, m.Registrar); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	s.k.RecordAction(sdkCtx, types.ModuleName, "remove_registrar", m.Authority, m.Registrar, "")
	emitEvent(sdkCtx, types.EventTypeRegistrar, "remove", m.Registrar)
	return &types.MsgRemoveRegistrarResponse{}, nil
}

// ---- Account --------------------------------------------------------------

func (s msgServer) SetAccount(ctx context.Context, m *types.MsgSetAccount) (*types.MsgSetAccountResponse, error) {
	if err := s.requireRegistrar(ctx, m.Registrar); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()
	acc, found, err := s.k.GetAccountRaw(ctx, m.Address)
	if err != nil {
		return nil, err
	}
	if !found {
		acc = types.Account{
			Address:   m.Address,
			Status:    types.AccountStatus_ACCOUNT_STATUS_ACTIVE,
			CreatedAt: now,
		}
	}
	acc.Did = m.Did
	acc.KycCleared = m.KycCleared
	acc.Accredited = m.Accredited
	acc.Jurisdiction = m.Jurisdiction
	acc.KycExpiresAt = m.KycExpiresAt
	acc.Registrar = m.Registrar
	acc.UpdatedAt = now
	if !types.AccountStatusValid(acc.Status) {
		acc.Status = types.AccountStatus_ACCOUNT_STATUS_ACTIVE
	}
	if err := s.k.Accounts.Set(ctx, m.Address, acc); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, "set_account", m.Registrar, m.Address, m.Jurisdiction)
	emitEvent(sdkCtx, types.EventTypeAccount, "set", m.Address)
	return &types.MsgSetAccountResponse{}, nil
}

func (s msgServer) FreezeAccount(ctx context.Context, m *types.MsgFreezeAccount) (*types.MsgFreezeAccountResponse, error) {
	return s.setAccountStatus(ctx, m.Registrar, m.Address, m.Reason, types.AccountStatus_ACCOUNT_STATUS_FROZEN, "freeze")
}

func (s msgServer) UnfreezeAccount(ctx context.Context, m *types.MsgUnfreezeAccount) (*types.MsgUnfreezeAccountResponse, error) {
	if _, err := s.setAccountStatus(ctx, m.Registrar, m.Address, m.Reason, types.AccountStatus_ACCOUNT_STATUS_ACTIVE, "unfreeze"); err != nil {
		return nil, err
	}
	return &types.MsgUnfreezeAccountResponse{}, nil
}

func (s msgServer) setAccountStatus(ctx context.Context, registrar, address, reason string, status types.AccountStatus, action string) (*types.MsgFreezeAccountResponse, error) {
	if err := s.requireRegistrar(ctx, registrar); err != nil {
		return nil, err
	}
	acc, found, err := s.k.GetAccountRaw(ctx, address)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, types.ErrNotFound.Wrapf("account %s", address)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	acc.Status = status
	acc.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Accounts.Set(ctx, address, acc); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, action, registrar, address, reason)
	emitEvent(sdkCtx, types.EventTypeAccount, action, address)
	return &types.MsgFreezeAccountResponse{}, nil
}

// ---- Sanctions ------------------------------------------------------------

func (s msgServer) AddSanction(ctx context.Context, m *types.MsgAddSanction) (*types.MsgAddSanctionResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.Sanctions.Has(ctx, m.Address); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("sanction %s", m.Address)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := s.k.Sanctions.Set(ctx, m.Address, types.Sanction{
		Address:    m.Address,
		ListSource: m.ListSource,
		Reason:     m.Reason,
		AddedBy:    m.Authority,
		AddedAt:    sdkCtx.BlockTime().Unix(),
	}); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, "add_sanction", m.Authority, m.Address, m.ListSource)
	emitEvent(sdkCtx, types.EventTypeSanction, "add", m.Address)
	return &types.MsgAddSanctionResponse{}, nil
}

func (s msgServer) RemoveSanction(ctx context.Context, m *types.MsgRemoveSanction) (*types.MsgRemoveSanctionResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if has, err := s.k.Sanctions.Has(ctx, m.Address); err != nil {
		return nil, err
	} else if !has {
		return nil, types.ErrNotFound.Wrapf("sanction %s", m.Address)
	}
	if err := s.k.Sanctions.Remove(ctx, m.Address); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	s.k.RecordAction(sdkCtx, types.ModuleName, "remove_sanction", m.Authority, m.Address, "")
	emitEvent(sdkCtx, types.EventTypeSanction, "remove", m.Address)
	return &types.MsgRemoveSanctionResponse{}, nil
}

// ---- Policy ---------------------------------------------------------------

func (s msgServer) CreatePolicy(ctx context.Context, m *types.MsgCreatePolicy) (*types.MsgCreatePolicyResponse, error) {
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidatePolicyShape(m.Id, m.Description, m.AllowedJurisdictions, m.DeniedJurisdictions, int(params.MaxJurisdictionsPerPolicy)); err != nil {
		return nil, err
	}
	if has, err := s.k.Policies.Has(ctx, m.Id); err != nil {
		return nil, err
	} else if has {
		return nil, types.ErrAlreadyExists.Wrapf("policy %s", m.Id)
	}
	n, err := s.k.CountPolicies(ctx)
	if err != nil {
		return nil, err
	}
	if n >= params.MaxPolicies {
		return nil, types.ErrLimitExceeded.Wrapf("max_policies %d", params.MaxPolicies)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()
	if err := s.k.Policies.Set(ctx, m.Id, types.Policy{
		Id:                   m.Id,
		Description:          m.Description,
		RequireKyc:           m.RequireKyc,
		RequireAccredited:    m.RequireAccredited,
		DenyFrozen:           m.DenyFrozen,
		AllowedJurisdictions: m.AllowedJurisdictions,
		DeniedJurisdictions:  m.DeniedJurisdictions,
		Paused:               false,
		Owner:                m.Owner,
		CreatedAt:            now,
		UpdatedAt:            now,
	}); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, "create_policy", m.Owner, m.Id, m.Description)
	emitEvent(sdkCtx, types.EventTypePolicy, "create", m.Id)
	return &types.MsgCreatePolicyResponse{}, nil
}

func (s msgServer) UpdatePolicy(ctx context.Context, m *types.MsgUpdatePolicy) (*types.MsgUpdatePolicyResponse, error) {
	params, err := s.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if err := types.ValidatePolicyShape(m.Id, m.Description, m.AllowedJurisdictions, m.DeniedJurisdictions, int(params.MaxJurisdictionsPerPolicy)); err != nil {
		return nil, err
	}
	pol, err := s.requirePolicyEditor(ctx, m.Id, m.Owner)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	pol.Description = m.Description
	pol.RequireKyc = m.RequireKyc
	pol.RequireAccredited = m.RequireAccredited
	pol.DenyFrozen = m.DenyFrozen
	pol.AllowedJurisdictions = m.AllowedJurisdictions
	pol.DeniedJurisdictions = m.DeniedJurisdictions
	pol.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Policies.Set(ctx, m.Id, pol); err != nil {
		return nil, err
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, "update_policy", m.Owner, m.Id, "")
	emitEvent(sdkCtx, types.EventTypePolicy, "update", m.Id)
	return &types.MsgUpdatePolicyResponse{}, nil
}

func (s msgServer) PausePolicy(ctx context.Context, m *types.MsgPausePolicy) (*types.MsgPausePolicyResponse, error) {
	pol, err := s.requirePolicyEditor(ctx, m.Id, m.Owner)
	if err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	pol.Paused = m.Paused
	pol.UpdatedAt = sdkCtx.BlockTime().Unix()
	if err := s.k.Policies.Set(ctx, m.Id, pol); err != nil {
		return nil, err
	}
	action := "unpause_policy"
	if m.Paused {
		action = "pause_policy"
	}
	s.k.RecordAction(sdkCtx, types.ModuleName, action, m.Owner, m.Id, "")
	emitEvent(sdkCtx, types.EventTypePolicy, action, m.Id)
	return &types.MsgPausePolicyResponse{}, nil
}

func (s msgServer) DeletePolicy(ctx context.Context, m *types.MsgDeletePolicy) (*types.MsgDeletePolicyResponse, error) {
	if _, err := s.requirePolicyEditor(ctx, m.Id, m.Owner); err != nil {
		return nil, err
	}
	if err := s.k.Policies.Remove(ctx, m.Id); err != nil {
		return nil, err
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	s.k.RecordAction(sdkCtx, types.ModuleName, "delete_policy", m.Owner, m.Id, "")
	emitEvent(sdkCtx, types.EventTypePolicy, "delete", m.Id)
	return &types.MsgDeletePolicyResponse{}, nil
}

func (s msgServer) UpdateParams(ctx context.Context, m *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if err := s.requireAuthority(m.Authority); err != nil {
		return nil, err
	}
	if err := s.k.SetParams(ctx, m.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---- helpers --------------------------------------------------------------

func (s msgServer) requireRegistrar(ctx context.Context, addr string) error {
	// Governance authority is implicitly a registrar.
	if addr == s.k.authority {
		return nil
	}
	ok, err := s.k.IsRegistrar(ctx, addr)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrUnauthorized.Wrapf("%s is not a registrar", addr)
	}
	return nil
}

// requirePolicyEditor loads the policy and verifies signer is its owner
// or the governance authority.
func (s msgServer) requirePolicyEditor(ctx context.Context, id, signer string) (types.Policy, error) {
	pol, found, err := s.k.GetPolicy(ctx, id)
	if err != nil {
		return types.Policy{}, err
	}
	if !found {
		return types.Policy{}, types.ErrNotFound.Wrapf("policy %s", id)
	}
	if signer != pol.Owner && signer != s.k.authority {
		return types.Policy{}, types.ErrUnauthorized.Wrapf("signer %s is neither policy owner nor authority", signer)
	}
	return pol, nil
}
