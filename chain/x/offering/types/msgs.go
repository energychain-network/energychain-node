package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

func (m *MsgCreateOffering) ValidateBasic() error {
	if err := MustBech32(m.Issuer); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return ErrInvalidField.Wrap("token_id must be > 0")
	}
	if m.UnitPrice == 0 {
		return ErrInvalidField.Wrap("unit_price must be > 0")
	}
	if m.HardCap == 0 || m.SoftCap > m.HardCap {
		return ErrInvalidField.Wrap("require 0 < soft_cap <= hard_cap")
	}
	if m.SoftCap%m.UnitPrice != 0 || m.HardCap%m.UnitPrice != 0 {
		return ErrInvalidField.Wrap("soft_cap and hard_cap must be multiples of unit_price")
	}
	if m.EndTime <= m.StartTime {
		return ErrInvalidField.Wrap("end_time must be after start_time")
	}
	if m.TotalTranches == 0 {
		return ErrInvalidField.Wrap("total_tranches must be > 0")
	}
	if m.InjectionInterval < 0 {
		return ErrInvalidField.Wrap("injection_interval must be >= 0")
	}
	return nil
}
func (m *MsgCreateOffering) GetSigners() []sdk.AccAddress { return signer(m.Issuer) }

func (m *MsgSubscribe) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgSubscribe) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgCancelOffering) ValidateBasic() error {
	if err := MustBech32(m.Issuer); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgCancelOffering) GetSigners() []sdk.AccAddress { return signer(m.Issuer) }

func (m *MsgCloseOffering) ValidateBasic() error {
	if err := MustBech32(m.Caller); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgCloseOffering) GetSigners() []sdk.AccAddress { return signer(m.Caller) }

func (m *MsgClaimAllocation) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgClaimAllocation) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgInjectReturn) ValidateBasic() error {
	if err := MustBech32(m.Issuer); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgInjectReturn) GetSigners() []sdk.AccAddress { return signer(m.Issuer) }

func (m *MsgReleaseTranche) ValidateBasic() error {
	if err := MustBech32(m.Issuer); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgReleaseTranche) GetSigners() []sdk.AccAddress { return signer(m.Issuer) }

func (m *MsgClaimReturns) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgClaimReturns) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgFlagDefault) ValidateBasic() error {
	if err := MustBech32(m.Caller); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgFlagDefault) GetSigners() []sdk.AccAddress { return signer(m.Caller) }

func (m *MsgClaimRefund) ValidateBasic() error {
	if err := MustBech32(m.Investor); err != nil {
		return err
	}
	if m.OfferingId == 0 {
		return ErrInvalidField.Wrap("offering_id must be > 0")
	}
	return nil
}
func (m *MsgClaimRefund) GetSigners() []sdk.AccAddress { return signer(m.Investor) }

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
