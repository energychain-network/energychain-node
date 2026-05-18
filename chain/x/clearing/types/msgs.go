package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ---- RegisterMember ----------------------------------------------------

func (m *MsgRegisterMember) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if err := ValidateAddr("member_address", m.MemberAddress); err != nil {
		return err
	}
	for d, v := range m.RequiredMargin {
		if err := ValidateDenom(d); err != nil {
			return err
		}
		if v == 0 {
			return fmt.Errorf("required_margin[%s] must be > 0 (omit instead of zero)", d)
		}
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgRegisterMember) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- UpdateMemberStatus ------------------------------------------------

func (m *MsgUpdateMemberStatus) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.MemberId == 0 {
		return fmt.Errorf("member_id must be > 0")
	}
	if !MemberStatusValid(m.NewStatus) {
		return fmt.Errorf("invalid status")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgUpdateMemberStatus) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- PostMargin / WithdrawMargin / FundDefaultFund --------------------

func (m *MsgPostMargin) ValidateBasic() error {
	if err := ValidateAddr("member_address", m.MemberAddress); err != nil {
		return err
	}
	if m.MemberId == 0 {
		return fmt.Errorf("member_id must be > 0")
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgPostMargin) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.MemberAddress)
	return []sdk.AccAddress{a}
}

func (m *MsgWithdrawMargin) ValidateBasic() error {
	if err := ValidateAddr("member_address", m.MemberAddress); err != nil {
		return err
	}
	if m.MemberId == 0 {
		return fmt.Errorf("member_id must be > 0")
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgWithdrawMargin) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.MemberAddress)
	return []sdk.AccAddress{a}
}

func (m *MsgFundDefaultFund) ValidateBasic() error {
	if err := ValidateAddr("member_address", m.MemberAddress); err != nil {
		return err
	}
	if m.MemberId == 0 {
		return fmt.Errorf("member_id must be > 0")
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgFundDefaultFund) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.MemberAddress)
	return []sdk.AccAddress{a}
}

// ---- Cycle lifecycle --------------------------------------------------

func (m *MsgOpenCycle) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	return ValidateMemo(m.Memo, 0)
}
func (m *MsgOpenCycle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgSubmitObligation) ValidateBasic() error {
	if err := ValidateAddr("submitter", m.Submitter); err != nil {
		return err
	}
	if m.CycleId == 0 {
		return fmt.Errorf("cycle_id must be > 0")
	}
	if m.FromMemberId == 0 || m.ToMemberId == 0 {
		return fmt.Errorf("from/to_member_id must be > 0")
	}
	if m.FromMemberId == m.ToMemberId {
		return fmt.Errorf("from == to is forbidden (self-leg)")
	}
	if err := ValidateDenom(m.Denom); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateSourceRef(m.SourceRef, 0)
}
func (m *MsgSubmitObligation) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}

func (m *MsgCloseCycle) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.CycleId == 0 {
		return fmt.Errorf("cycle_id must be > 0")
	}
	return nil
}
func (m *MsgCloseCycle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgSettleCycle) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.CycleId == 0 {
		return fmt.Errorf("cycle_id must be > 0")
	}
	return nil
}
func (m *MsgSettleCycle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgCancelCycle) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.CycleId == 0 {
		return fmt.Errorf("cycle_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelCycle) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- UpdateParams -----------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
