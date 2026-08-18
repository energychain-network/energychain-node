package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

// ---- denom lifecycle ------------------------------------------------------

func (m *MsgCreateDenom) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateDenomID(m.Id); err != nil {
		return err
	}
	if err := ValidateSymbol(m.Symbol); err != nil {
		return err
	}
	if m.Decimals > MaxDecimals {
		return ErrInvalidField.Wrapf("decimals %d > %d", m.Decimals, MaxDecimals)
	}
	if err := MustBech32(m.Admin); err != nil {
		return ErrInvalidField.Wrap("admin: " + err.Error())
	}
	if err := ValidateOptionalPegCurrency(m.PegCurrency); err != nil {
		return err
	}
	return ValidateOptionalPolicyID(m.PolicyId)
}
func (m *MsgCreateDenom) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func validateAdminDenom(admin, denomID string) error {
	if err := MustBech32(admin); err != nil {
		return err
	}
	return ValidateDenomID(denomID)
}

func (m *MsgAddMinter) ValidateBasic() error {
	if err := validateAdminDenom(m.Admin, m.DenomId); err != nil {
		return err
	}
	return MustBech32(m.Minter)
}
func (m *MsgAddMinter) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgRemoveMinter) ValidateBasic() error {
	if err := validateAdminDenom(m.Admin, m.DenomId); err != nil {
		return err
	}
	return MustBech32(m.Minter)
}
func (m *MsgRemoveMinter) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgSetDenomStatus) ValidateBasic() error {
	if err := validateAdminDenom(m.Admin, m.DenomId); err != nil {
		return err
	}
	if !DenomStatusValid(m.Status) {
		return ErrInvalidField.Wrap("status")
	}
	return nil
}
func (m *MsgSetDenomStatus) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgBindReserve) ValidateBasic() error {
	if err := validateAdminDenom(m.Admin, m.DenomId); err != nil {
		return err
	}
	if err := ValidateOptionalTopicID(m.ReserveTopic); err != nil {
		return err
	}
	if m.RequiredRatioBps == 0 {
		return ErrInvalidField.Wrap("required_ratio_bps must be > 0")
	}
	if m.MaxStalenessSeconds < 0 {
		return ErrInvalidField.Wrap("max_staleness_seconds must be >= 0")
	}
	return nil
}
func (m *MsgBindReserve) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

// ---- supply ---------------------------------------------------------------

func (m *MsgMint) ValidateBasic() error {
	if err := MustBech32(m.Minter); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if err := MustBech32(m.Recipient); err != nil {
		return ErrInvalidField.Wrap("recipient: " + err.Error())
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgMint) GetSigners() []sdk.AccAddress { return signer(m.Minter) }

func (m *MsgBurn) ValidateBasic() error {
	if err := MustBech32(m.Holder); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgBurn) GetSigners() []sdk.AccAddress { return signer(m.Holder) }

// ---- transfers ------------------------------------------------------------

func (m *MsgTransfer) ValidateBasic() error {
	if err := MustBech32(m.From); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if err := MustBech32(m.To); err != nil {
		return ErrInvalidField.Wrap("to: " + err.Error())
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgTransfer) GetSigners() []sdk.AccAddress { return signer(m.From) }

func (m *MsgApprove) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	return MustBech32(m.Spender)
}
func (m *MsgApprove) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgTransferFrom) ValidateBasic() error {
	if err := MustBech32(m.Spender); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if err := MustBech32(m.From); err != nil {
		return ErrInvalidField.Wrap("from: " + err.Error())
	}
	if err := MustBech32(m.To); err != nil {
		return ErrInvalidField.Wrap("to: " + err.Error())
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgTransferFrom) GetSigners() []sdk.AccAddress { return signer(m.Spender) }

// ---- compliance controls --------------------------------------------------

func validateAdminAccount(admin, denomID, account string) error {
	if err := validateAdminDenom(admin, denomID); err != nil {
		return err
	}
	return MustBech32(account)
}

func (m *MsgFreeze) ValidateBasic() error         { return validateAdminAccount(m.Admin, m.DenomId, m.Account) }
func (m *MsgFreeze) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgUnfreeze) ValidateBasic() error {
	return validateAdminAccount(m.Admin, m.DenomId, m.Account)
}
func (m *MsgUnfreeze) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgBlacklist) ValidateBasic() error {
	return validateAdminAccount(m.Admin, m.DenomId, m.Account)
}
func (m *MsgBlacklist) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgUnblacklist) ValidateBasic() error {
	return validateAdminAccount(m.Admin, m.DenomId, m.Account)
}
func (m *MsgUnblacklist) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgForceTransfer) ValidateBasic() error {
	if err := validateAdminDenom(m.Admin, m.DenomId); err != nil {
		return err
	}
	if err := MustBech32(m.From); err != nil {
		return ErrInvalidField.Wrap("from: " + err.Error())
	}
	if err := MustBech32(m.To); err != nil {
		return ErrInvalidField.Wrap("to: " + err.Error())
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgForceTransfer) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

// ---- redemption -----------------------------------------------------------

func (m *MsgRequestRedemption) ValidateBasic() error {
	if err := MustBech32(m.Holder); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return ValidateMaxLen("memo", m.Memo, MemoMaxLen)
}
func (m *MsgRequestRedemption) GetSigners() []sdk.AccAddress { return signer(m.Holder) }

func (m *MsgSettleRedemption) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if m.RedemptionId == 0 {
		return ErrInvalidField.Wrap("redemption_id must be > 0")
	}
	return ValidateMaxLen("memo", m.Memo, MemoMaxLen)
}
func (m *MsgSettleRedemption) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgCancelRedemption) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if m.RedemptionId == 0 {
		return ErrInvalidField.Wrap("redemption_id must be > 0")
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgCancelRedemption) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

// ---- params ---------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
