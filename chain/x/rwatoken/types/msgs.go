package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

// ---- token lifecycle ------------------------------------------------------

func (m *MsgCreateToken) ValidateBasic() error {
	if err := MustBech32(m.Admin); err != nil {
		return err
	}
	if err := ValidateSymbol(m.Symbol); err != nil {
		return err
	}
	if err := ValidateMaxLen("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if err := ValidateAssetClass(m.AssetClass); err != nil {
		return err
	}
	if m.Decimals > MaxDecimals {
		return ErrInvalidField.Wrapf("decimals %d > %d", m.Decimals, MaxDecimals)
	}
	if err := ValidateSettlementDenom(m.SettlementDenom); err != nil {
		return err
	}
	if err := ValidateOptionalPolicyID(m.PolicyId); err != nil {
		return err
	}
	if err := ValidateRedemptionDelay(m.RedemptionDelaySeconds); err != nil {
		return err
	}
	if err := ValidateMaturityTime(m.MaturityTime); err != nil {
		return err
	}
	if err := ValidateDeviceIDs(m.DeviceIds); err != nil {
		return err
	}
	return ValidateMaxLen("metadata_uri", m.MetadataUri, MetadataURIMaxLen)
}
func (m *MsgCreateToken) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func adminToken(admin string, tokenID uint64) error {
	if err := MustBech32(admin); err != nil {
		return err
	}
	if tokenID == 0 {
		return ErrInvalidField.Wrap("token_id must be > 0")
	}
	return nil
}

func (m *MsgUpdateToken) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	if err := ValidateOptionalPolicyID(m.PolicyId); err != nil {
		return err
	}
	if err := ValidateRedemptionDelay(m.RedemptionDelaySeconds); err != nil {
		return err
	}
	if err := ValidateMaturityTime(m.MaturityTime); err != nil {
		return err
	}
	if err := ValidateDeviceIDs(m.DeviceIds); err != nil {
		return err
	}
	return ValidateMaxLen("metadata_uri", m.MetadataUri, MetadataURIMaxLen)
}
func (m *MsgUpdateToken) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgSetTokenStatus) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	if !TokenStatusValid(m.Status) {
		return ErrInvalidField.Wrap("status")
	}
	return nil
}
func (m *MsgSetTokenStatus) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

// ---- supply / transfer ----------------------------------------------------

func (m *MsgMint) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
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
func (m *MsgMint) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgBurn) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	if err := MustBech32(m.Holder); err != nil {
		return ErrInvalidField.Wrap("holder: " + err.Error())
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgBurn) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgTransfer) ValidateBasic() error {
	if err := MustBech32(m.From); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return ErrInvalidField.Wrap("token_id must be > 0")
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

func (m *MsgForceTransfer) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
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

func (m *MsgFreeze) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	return MustBech32(m.Holder)
}
func (m *MsgFreeze) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgUnfreeze) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	return MustBech32(m.Holder)
}
func (m *MsgUnfreeze) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

// ---- snapshot / dividend --------------------------------------------------

func (m *MsgTakeSnapshot) ValidateBasic() error {
	return adminToken(m.Admin, m.TokenId)
}
func (m *MsgTakeSnapshot) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgCreateDistribution) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	if m.SnapshotId == 0 {
		return ErrInvalidField.Wrap("snapshot_id must be > 0")
	}
	if m.TotalAmount == 0 {
		return ErrInvalidField.Wrap("total_amount must be > 0")
	}
	return nil
}
func (m *MsgCreateDistribution) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgClaimDistribution) ValidateBasic() error {
	if err := MustBech32(m.Holder); err != nil {
		return err
	}
	if m.DistributionId == 0 {
		return ErrInvalidField.Wrap("distribution_id must be > 0")
	}
	return nil
}
func (m *MsgClaimDistribution) GetSigners() []sdk.AccAddress { return signer(m.Holder) }

// ---- redemption -----------------------------------------------------------

func (m *MsgFundPool) ValidateBasic() error {
	if err := adminToken(m.Admin, m.TokenId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgFundPool) GetSigners() []sdk.AccAddress { return signer(m.Admin) }

func (m *MsgRequestRedemption) ValidateBasic() error {
	if err := MustBech32(m.Holder); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return ErrInvalidField.Wrap("token_id must be > 0")
	}
	if m.Units == 0 {
		return ErrInvalidField.Wrap("units must be > 0")
	}
	return nil
}
func (m *MsgRequestRedemption) GetSigners() []sdk.AccAddress { return signer(m.Holder) }

func (m *MsgExecuteRedemption) ValidateBasic() error {
	if err := MustBech32(m.Executor); err != nil {
		return err
	}
	if m.RedemptionId == 0 {
		return ErrInvalidField.Wrap("redemption_id must be > 0")
	}
	return nil
}
func (m *MsgExecuteRedemption) GetSigners() []sdk.AccAddress { return signer(m.Executor) }

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
