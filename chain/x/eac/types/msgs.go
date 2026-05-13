package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ---- helpers --------------------------------------------------------------

func validateBech32(addr, label string) error {
	if addr == "" {
		return fmt.Errorf("%s must be non-empty", label)
	}
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("%s %q invalid bech32: %w", label, addr, err)
	}
	return nil
}

func validateKindList(ks []int32, max uint32) error {
	if uint32(len(ks)) > max {
		return fmt.Errorf("too many kinds (got %d, max %d)", len(ks), max)
	}
	seen := map[int32]bool{}
	for _, k := range ks {
		if seen[k] {
			return fmt.Errorf("duplicate kind %d", k)
		}
		seen[k] = true
		if !CertificateKindValid(CertificateKind(k)) {
			return fmt.Errorf("invalid kind %d", k)
		}
	}
	return nil
}

// ---- Issuer ---------------------------------------------------------------

func (m MsgRegisterIssuer) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	if err := ValidateDID(m.Did); err != nil {
		return err
	}
	if err := ValidateIssuerName(m.DisplayName); err != nil {
		return err
	}
	if err := validateKindList(m.Kinds, MaxKindsPerIssuerUpper); err != nil {
		return err
	}
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	return nil
}
func (m MsgRegisterIssuer) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgUpdateIssuer) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	if m.DisplayName != "" {
		if err := ValidateIssuerName(m.DisplayName); err != nil {
			return err
		}
	}
	if m.Kinds != nil {
		if err := validateKindList(m.Kinds, MaxKindsPerIssuerUpper); err != nil {
			return err
		}
	}
	if m.IssuerAuthority != "" {
		if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
			return err
		}
	}
	if m.Admin != "" {
		if err := validateBech32(m.Admin, "admin"); err != nil {
			return err
		}
	}
	return nil
}
func (m MsgUpdateIssuer) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgSuspendIssuer) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m MsgSuspendIssuer) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgRevokeIssuer) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m MsgRevokeIssuer) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

// ---- Certificate ----------------------------------------------------------

func (m MsgIssueBatch) ValidateBasic() error {
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if !CertificateKindValid(m.Kind) {
		return fmt.Errorf("invalid kind")
	}
	if !TechnologyValid(m.Technology) {
		return fmt.Errorf("invalid technology")
	}
	if err := ValidateProjectID(m.ProjectId); err != nil {
		return err
	}
	if err := ValidateDeviceID(m.DeviceId); err != nil {
		return err
	}
	if err := ValidateGridZone(m.GridZone); err != nil {
		return err
	}
	if m.HourStart <= 0 || m.HourEnd <= 0 || m.HourEnd <= m.HourStart {
		return fmt.Errorf("hour window invalid")
	}
	if err := ValidateVintageYear(m.VintageYear); err != nil {
		return err
	}
	if err := ValidateSourceRegistry(m.SourceRegistry); err != nil {
		return err
	}
	if err := ValidateSourceSerial(m.SourceSerial); err != nil {
		return err
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	if err := validateBech32(m.Recipient, "recipient"); err != nil {
		return err
	}
	if err := ValidatePolicyID(m.PolicyId); err != nil {
		return err
	}
	return nil
}
func (m MsgIssueBatch) GetSigners() []sdk.AccAddress { return mustSigner(m.IssuerAuthority) }

func (m MsgSealCertificate) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m MsgSealCertificate) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

func (m MsgUnsealCertificate) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m MsgUnsealCertificate) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

func (m MsgTransfer) ValidateBasic() error {
	if err := validateBech32(m.From, "from"); err != nil {
		return err
	}
	if err := validateBech32(m.To, "to"); err != nil {
		return err
	}
	if m.From == m.To {
		return fmt.Errorf("from and to must differ")
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m MsgTransfer) GetSigners() []sdk.AccAddress { return mustSigner(m.From) }

func (m MsgRetire) ValidateBasic() error {
	if err := validateBech32(m.Retirer, "retirer"); err != nil {
		return err
	}
	if m.Beneficiary != "" {
		if err := validateBech32(m.Beneficiary, "beneficiary"); err != nil {
			return err
		}
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	if err := ValidatePurpose(m.Purpose); err != nil {
		return err
	}
	return ValidateMemo(m.Memo)
}
func (m MsgRetire) GetSigners() []sdk.AccAddress { return mustSigner(m.Retirer) }

// ---- Bridge ---------------------------------------------------------------

func (m MsgSetBridgeAttestation) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	if err := ValidateOracleTopic(m.OracleTopicId); err != nil {
		return err
	}
	if m.MaxStalenessSeconds == 0 {
		return fmt.Errorf("max_staleness_seconds must be > 0")
	}
	return nil
}
func (m MsgSetBridgeAttestation) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgBridgeMint) ValidateBasic() error {
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	return validateBech32(m.Recipient, "recipient")
}
func (m MsgBridgeMint) GetSigners() []sdk.AccAddress { return mustSigner(m.IssuerAuthority) }

func (m MsgBridgeBurn) ValidateBasic() error {
	if err := validateBech32(m.Holder, "holder"); err != nil {
		return err
	}
	if m.CertificateId == 0 {
		return fmt.Errorf("certificate_id must be > 0")
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	if m.ExternalRecipient == "" {
		return fmt.Errorf("external_recipient must be non-empty")
	}
	if len(m.ExternalRecipient) > SourceSerialMaxLen {
		return fmt.Errorf("external_recipient too long (max %d)", SourceSerialMaxLen)
	}
	return ValidateMemo(m.Memo)
}
func (m MsgBridgeBurn) GetSigners() []sdk.AccAddress { return mustSigner(m.Holder) }

// ---- Params ---------------------------------------------------------------

func (m MsgUpdateParams) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m MsgUpdateParams) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func mustSigner(addr string) []sdk.AccAddress {
	a, err := sdk.AccAddressFromBech32(addr)
	if err != nil {
		return nil
	}
	return []sdk.AccAddress{a}
}
