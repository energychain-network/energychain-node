package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgRegisterProvider{}
	_ sdk.Msg = &MsgUpdateProviderInfo{}
	_ sdk.Msg = &MsgPostBond{}
	_ sdk.Msg = &MsgIncreaseBond{}
	_ sdk.Msg = &MsgRequestUnbond{}
	_ sdk.Msg = &MsgWithdrawUnbonded{}
	_ sdk.Msg = &MsgReportInfraction{}
	_ sdk.Msg = &MsgJailProvider{}
	_ sdk.Msg = &MsgUnjailProvider{}
	_ sdk.Msg = &MsgBanProvider{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---- MsgRegisterProvider ---------------------------------

func (m *MsgRegisterProvider) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if err := ValidateAddr("signer_address", m.SignerAddress); err != nil {
		return err
	}
	if err := ValidateAddr("bond_owner", m.BondOwner); err != nil {
		return err
	}
	if err := ValidateDID("did", m.Did); err != nil {
		return err
	}
	if err := ValidateNonEmpty("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if !ProviderRoleValid(m.Role) {
		return fmt.Errorf("invalid role")
	}
	if m.BondDenom != "" {
		if err := ValidateDenom(m.BondDenom); err != nil {
			return err
		}
	}
	for _, j := range m.Jurisdictions {
		if err := ValidateJurisdiction("jurisdictions", j); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgRegisterProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgUpdateProviderInfo -------------------------------

func (m *MsgUpdateProviderInfo) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if m.NewBondOwner != "" {
		if err := ValidateAddr("new_bond_owner", m.NewBondOwner); err != nil {
			return err
		}
	}
	if m.NewName != "" {
		if len(m.NewName) > NameMaxLen {
			return fmt.Errorf("new_name too long")
		}
	}
	for _, j := range m.NewJurisdictions {
		if err := ValidateJurisdiction("new_jurisdictions", j); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgUpdateProviderInfo) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgPostBond / MsgIncreaseBond -----------------------

func (m *MsgPostBond) ValidateBasic() error {
	if err := ValidateAddr("bond_owner", m.BondOwner); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgPostBond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.BondOwner)
	return []sdk.AccAddress{a}
}

func (m *MsgIncreaseBond) ValidateBasic() error {
	if err := ValidateAddr("bond_owner", m.BondOwner); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return nil
}
func (m *MsgIncreaseBond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.BondOwner)
	return []sdk.AccAddress{a}
}

// ---- MsgRequestUnbond / MsgWithdrawUnbonded --------------

func (m *MsgRequestUnbond) ValidateBasic() error {
	if err := ValidateAddr("bond_owner", m.BondOwner); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	return nil
}
func (m *MsgRequestUnbond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.BondOwner)
	return []sdk.AccAddress{a}
}

func (m *MsgWithdrawUnbonded) ValidateBasic() error {
	if err := ValidateAddr("bond_owner", m.BondOwner); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	return nil
}
func (m *MsgWithdrawUnbonded) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.BondOwner)
	return []sdk.AccAddress{a}
}

// ---- MsgReportInfraction ---------------------------------

func (m *MsgReportInfraction) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if !InfractionKindValid(m.Kind) {
		return fmt.Errorf("invalid kind")
	}
	if m.OverrideSlashBps > 10_000 {
		return fmt.Errorf("override_slash_bps > 10000")
	}
	if err := ValidateHash("evidence_hash", m.EvidenceHash); err != nil {
		return err
	}
	return nil
}
func (m *MsgReportInfraction) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{a}
}

// ---- MsgJailProvider / Unjail / Ban ----------------------

func (m *MsgJailProvider) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if m.DurationSeconds <= 0 {
		return fmt.Errorf("duration_seconds must be > 0")
	}
	if len(m.Reason) > 2048 {
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgJailProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgUnjailProvider) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if len(m.Reason) > 2048 {
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgUnjailProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgBanProvider) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ProviderId == 0 {
		return fmt.Errorf("provider_id required")
	}
	if len(m.Reason) > 2048 {
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgBanProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgUpdateParams -------------------------------------

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
