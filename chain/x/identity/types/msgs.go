package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func signer(addr string) []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(addr)
	return []sdk.AccAddress{a}
}

// ---- Registrar ------------------------------------------------------------

func (m *MsgAddRegistrar) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := MustBech32(m.Registrar); err != nil {
		return err
	}
	return ValidateDisplayName(m.DisplayName)
}
func (m *MsgAddRegistrar) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgRemoveRegistrar) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return MustBech32(m.Registrar)
}
func (m *MsgRemoveRegistrar) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

// ---- Account --------------------------------------------------------------

func (m *MsgSetAccount) ValidateBasic() error {
	if err := MustBech32(m.Registrar); err != nil {
		return err
	}
	if err := MustBech32(m.Address); err != nil {
		return err
	}
	if err := ValidateDID(m.Did); err != nil {
		return err
	}
	if err := ValidateOptionalJurisdiction(m.Jurisdiction); err != nil {
		return err
	}
	if m.KycExpiresAt < 0 {
		return ErrInvalidField.Wrap("kyc_expires_at must be >= 0")
	}
	return nil
}
func (m *MsgSetAccount) GetSigners() []sdk.AccAddress { return signer(m.Registrar) }

func (m *MsgFreezeAccount) ValidateBasic() error {
	if err := MustBech32(m.Registrar); err != nil {
		return err
	}
	if err := MustBech32(m.Address); err != nil {
		return err
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgFreezeAccount) GetSigners() []sdk.AccAddress { return signer(m.Registrar) }

func (m *MsgUnfreezeAccount) ValidateBasic() error {
	if err := MustBech32(m.Registrar); err != nil {
		return err
	}
	if err := MustBech32(m.Address); err != nil {
		return err
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgUnfreezeAccount) GetSigners() []sdk.AccAddress { return signer(m.Registrar) }

// ---- Sanctions ------------------------------------------------------------

func (m *MsgAddSanction) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	if err := MustBech32(m.Address); err != nil {
		return err
	}
	if err := ValidateMaxLen("list_source", m.ListSource, ListSourceMaxLen); err != nil {
		return err
	}
	return ValidateMaxLen("reason", m.Reason, ReasonMaxLen)
}
func (m *MsgAddSanction) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

func (m *MsgRemoveSanction) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return MustBech32(m.Address)
}
func (m *MsgRemoveSanction) GetSigners() []sdk.AccAddress { return signer(m.Authority) }

// ---- Policy ---------------------------------------------------------------

func (m *MsgCreatePolicy) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	return ValidatePolicyShape(m.Id, m.Description, m.AllowedJurisdictions, m.DeniedJurisdictions, int(HardMaxJurisdictionsPerPolicy))
}
func (m *MsgCreatePolicy) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgUpdatePolicy) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	return ValidatePolicyShape(m.Id, m.Description, m.AllowedJurisdictions, m.DeniedJurisdictions, int(HardMaxJurisdictionsPerPolicy))
}
func (m *MsgUpdatePolicy) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgPausePolicy) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	return ValidatePolicyID(m.Id)
}
func (m *MsgPausePolicy) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

func (m *MsgDeletePolicy) ValidateBasic() error {
	if err := MustBech32(m.Owner); err != nil {
		return err
	}
	return ValidatePolicyID(m.Id)
}
func (m *MsgDeletePolicy) GetSigners() []sdk.AccAddress { return signer(m.Owner) }

// ---- Params ---------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := MustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress { return signer(m.Authority) }
