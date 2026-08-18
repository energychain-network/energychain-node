package types

import sdk "github.com/cosmos/cosmos-sdk/types"

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

// ---- Provider -------------------------------------------------------------

func (m *MsgRegisterProvider) ValidateBasic() error {
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	if !ProviderRoleValid(m.Role) {
		return ErrInvalidField.Wrap("role")
	}
	if m.Bond == 0 {
		return ErrInsufficientBond.Wrap("bond must be > 0")
	}
	return ValidateMaxLen("display_name", m.DisplayName, DisplayNameMaxLen)
}
func (m *MsgRegisterProvider) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

func (m *MsgIncreaseBond) ValidateBasic() error {
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgIncreaseBond) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

func (m *MsgWithdrawBond) ValidateBasic() error {
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	if m.Amount == 0 {
		return ErrInvalidField.Wrap("amount must be > 0")
	}
	return nil
}
func (m *MsgWithdrawBond) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

func (m *MsgDeregisterProvider) ValidateBasic() error         { return MustBech32(m.Provider) }
func (m *MsgDeregisterProvider) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

func (m *MsgSlashProvider) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgSlashProvider) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgUnjailProvider) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return MustBech32(m.Provider)
}
func (m *MsgUnjailProvider) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

// ---- Device ---------------------------------------------------------------

func (m *MsgRegisterDevice) ValidateBasic() error {
	if err := MustBech32(m.Operator); err != nil {
		return err
	}
	if err := ValidateID(m.Id); err != nil {
		return err
	}
	if err := ValidateDeviceType(m.DeviceType); err != nil {
		return err
	}
	if err := ValidateMaxLen("pubkey", m.Pubkey, PubkeyMaxLen); err != nil {
		return err
	}
	return ValidateOptionalJurisdiction(m.Jurisdiction)
}
func (m *MsgRegisterDevice) GetSigners() []sdk.AccAddress { return signer(m.Operator) }

func (m *MsgUpdateDevice) ValidateBasic() error {
	if err := MustBech32(m.Operator); err != nil {
		return err
	}
	if err := ValidateID(m.Id); err != nil {
		return err
	}
	if m.DeviceType != "" {
		if err := ValidateDeviceType(m.DeviceType); err != nil {
			return err
		}
	}
	return ValidateOptionalJurisdiction(m.Jurisdiction)
}
func (m *MsgUpdateDevice) GetSigners() []sdk.AccAddress { return signer(m.Operator) }

func (m *MsgAttestDevice) ValidateBasic() error {
	if err := MustBech32(m.Operator); err != nil {
		return err
	}
	if err := ValidateID(m.Id); err != nil {
		return err
	}
	if err := ValidateMaxLen("attestation_hash", m.AttestationHash, HashMaxLen); err != nil {
		return err
	}
	return ValidateMaxLen("firmware", m.Firmware, FirmwareMaxLen)
}
func (m *MsgAttestDevice) GetSigners() []sdk.AccAddress { return signer(m.Operator) }

func (m *MsgRevokeDevice) ValidateBasic() error {
	if err := MustBech32(m.Actor); err != nil {
		return err
	}
	if err := ValidateID(m.Id); err != nil {
		return err
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgRevokeDevice) GetSigners() []sdk.AccAddress { return signer(m.Actor) }

// ---- Reading --------------------------------------------------------------

func (m *MsgSubmitReading) ValidateBasic() error {
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	if err := ValidateID(m.DeviceId); err != nil {
		return err
	}
	if err := ValidateMaxLen("unit", m.Unit, UnitMaxLen); err != nil {
		return err
	}
	if m.Unit == "" {
		return ErrInvalidReading.Wrap("unit required")
	}
	if m.PeriodEnd < m.PeriodStart {
		return ErrInvalidReading.Wrap("period_end < period_start")
	}
	return nil
}
func (m *MsgSubmitReading) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

// ---- Oracle ---------------------------------------------------------------

func (m *MsgCreateTopic) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateTopicID(m.Id); err != nil {
		return err
	}
	if m.MinSources == 0 {
		return ErrInvalidField.Wrap("min_sources must be > 0")
	}
	return ValidateMaxLen("description", m.Description, DescriptionMaxLen)
}
func (m *MsgCreateTopic) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgSubmitValue) ValidateBasic() error {
	if err := MustBech32(m.Provider); err != nil {
		return err
	}
	return ValidateTopicID(m.TopicId)
}
func (m *MsgSubmitValue) GetSigners() []sdk.AccAddress { return signer(m.Provider) }

// ---- Params ---------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
