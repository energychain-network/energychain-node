package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgRegisterPolicy{}
	_ sdk.Msg = &MsgUpdatePolicy{}
	_ sdk.Msg = &MsgDeprecatePolicy{}
	_ sdk.Msg = &MsgDisablePolicy{}
	_ sdk.Msg = &MsgBindPolicy{}
	_ sdk.Msg = &MsgUnbindPolicy{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// validateBaseMsgPolicy validates the shape that's common to register/
// update messages: id + name + version + description bounds. The full
// rule + param-size enforcement happens in the keeper because it
// requires Params, which ValidateBasic does not have access to.
func validateBaseMsgPolicy(authority, id, name, version, description string) error {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidatePolicyID(id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if err := ValidatePolicyName(name); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("name: %s", err)
	}
	if len(version) > PolicyVersionMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("version too long")
	}
	if len(description) > PolicyDescriptionMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("description too long")
	}
	return nil
}

// validateRulesShallow caps rule count + raw param size at the upper
// bound. The keeper re-runs the strict ValidatePolicyShape against
// current Params.
func validateRulesShallow(rules []PolicyRule) error {
	if uint32(len(rules)) > MaxRulesPerPolicyUpper {
		return sdkerrors.ErrInvalidRequest.Wrapf(
			"rules count %d exceeds upper bound %d", len(rules), MaxRulesPerPolicyUpper)
	}
	for i, r := range rules {
		for k, v := range r.Params {
			if uint32(len(k))+uint32(len(v)) > MaxParamValueSizeUpper {
				return sdkerrors.ErrInvalidRequest.Wrapf(
					"rule %d param %q exceeds upper bound %d", i, k, MaxParamValueSizeUpper)
			}
		}
	}
	return nil
}

func (m *MsgRegisterPolicy) ValidateBasic() error {
	if err := validateBaseMsgPolicy(m.Authority, m.Id, m.Name, m.Version, m.Description); err != nil {
		return err
	}
	return validateRulesShallow(m.Rules)
}

func (m *MsgUpdatePolicy) ValidateBasic() error {
	if err := validateBaseMsgPolicy(m.Authority, m.Id, m.Name, m.Version, m.Description); err != nil {
		return err
	}
	return validateRulesShallow(m.Rules)
}

func (m *MsgDeprecatePolicy) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidatePolicyID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if len(m.Reason) > ReasonMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}

func (m *MsgDisablePolicy) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidatePolicyID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if len(m.Reason) > ReasonMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}

func (m *MsgBindPolicy) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidateAssetClass(m.AssetClass); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("asset_class: %s", err)
	}
	if err := ValidateAssetID(m.AssetId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("asset_id: %s", err)
	}
	if err := ValidatePolicyID(m.PolicyId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("policy_id: %s", err)
	}
	return nil
}

func (m *MsgUnbindPolicy) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidateAssetClass(m.AssetClass); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("asset_class: %s", err)
	}
	if err := ValidateAssetID(m.AssetId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("asset_id: %s", err)
	}
	if len(m.Reason) > ReasonMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return m.Params.Validate()
}

// ---------------------------------------------------------------------------
// GetSigners — required by the SDK Msg interface for backwards-compat with
// pre-v0.50 routing.
// ---------------------------------------------------------------------------

func (m *MsgRegisterPolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdatePolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgDeprecatePolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgDisablePolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgBindPolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgUnbindPolicy) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
