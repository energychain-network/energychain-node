package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func validateBech32(addr, label string) error {
	if addr == "" {
		return fmt.Errorf("%s must be non-empty", label)
	}
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("%s %q invalid bech32: %w", label, addr, err)
	}
	return nil
}

func validateCategoryList(cs []int32, max uint32) error {
	if uint32(len(cs)) > max {
		return fmt.Errorf("too many categories (got %d, max %d)", len(cs), max)
	}
	seen := map[int32]bool{}
	for _, c := range cs {
		if seen[c] {
			return fmt.Errorf("duplicate category %d", c)
		}
		seen[c] = true
		if !AssetCategoryValid(AssetCategory(c)) {
			return fmt.Errorf("invalid category %d", c)
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
	if err := validateCategoryList(m.Categories, MaxCategoriesPerIssuerUpper); err != nil {
		return err
	}
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	return validateBech32(m.Admin, "admin")
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
	if m.Categories != nil {
		if err := validateCategoryList(m.Categories, MaxCategoriesPerIssuerUpper); err != nil {
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

// ---- Asset issuance -------------------------------------------------------

func (m MsgIssueAllowance) ValidateBasic() error {
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateRegistry(m.Registry); err != nil {
		return err
	}
	if err := ValidateProgram(m.Program); err != nil {
		return err
	}
	if err := ValidateJurisdiction(m.Jurisdiction); err != nil {
		return err
	}
	if err := ValidateVintageYear(m.VintageYear); err != nil {
		return err
	}
	if err := ValidateRegistry(m.SourceRegistry); err != nil {
		// SourceRegistry uses the same alphabet bound as Registry
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
	return ValidatePolicyID(m.PolicyId)
}
func (m MsgIssueAllowance) GetSigners() []sdk.AccAddress { return mustSigner(m.IssuerAuthority) }

func (m MsgIssueOffset) ValidateBasic() error {
	if err := validateBech32(m.IssuerAuthority, "issuer_authority"); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateRegistry(m.Registry); err != nil {
		return err
	}
	if err := ValidateProgram(m.Program); err != nil {
		return err
	}
	if err := ValidateProjectID(m.ProjectId); err != nil {
		return err
	}
	if err := ValidateMethodology(m.Methodology); err != nil {
		return err
	}
	if err := ValidateJurisdiction(m.Jurisdiction); err != nil {
		return err
	}
	if err := ValidateVintageYear(m.VintageYear); err != nil {
		return err
	}
	if err := ValidateRegistry(m.SourceRegistry); err != nil {
		return err
	}
	if err := ValidateSourceSerial(m.SourceSerial); err != nil {
		return err
	}
	if err := ValidateCCPLabels(m.CcpLabels, DefaultMaxCCPLabels*4); err != nil {
		// Loose upper bound here; per-keeper bound enforced inside.
		return err
	}
	if !Article6StatusValid(m.Article6Status) {
		return fmt.Errorf("invalid article6_status")
	}
	if m.Article6Status != Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE {
		if err := ValidateOptionalJurisdiction(m.HostCountry); err != nil {
			return err
		}
		if err := ValidateOptionalJurisdiction(m.RecipientCountry); err != nil {
			return err
		}
		if m.HostCountry == "" {
			return fmt.Errorf("host_country required when article6_status != NOT_APPLICABLE")
		}
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	if err := validateBech32(m.Recipient, "recipient"); err != nil {
		return err
	}
	return ValidatePolicyID(m.PolicyId)
}
func (m MsgIssueOffset) GetSigners() []sdk.AccAddress { return mustSigner(m.IssuerAuthority) }

func (m MsgSealAsset) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m MsgSealAsset) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

func (m MsgUnsealAsset) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m MsgUnsealAsset) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

// ---- Hot path -------------------------------------------------------------

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
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
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
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
	}
	if m.Units == 0 {
		return fmt.Errorf("units must be > 0")
	}
	if err := ValidatePurpose(m.Purpose); err != nil {
		return err
	}
	if err := ValidateClaim(m.Claim); err != nil {
		return err
	}
	if err := ValidateMemo(m.Memo); err != nil {
		return err
	}
	return ValidateOptionalJurisdiction(m.BeneficiaryJurisdiction)
}
func (m MsgRetire) GetSigners() []sdk.AccAddress { return mustSigner(m.Retirer) }

// ---- Article 6 ------------------------------------------------------------

func (m MsgSetArticle6Status) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
	}
	if !Article6StatusValid(m.Status) {
		return fmt.Errorf("invalid status")
	}
	if m.Status != Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE {
		if m.HostCountry == "" {
			return fmt.Errorf("host_country required for non NOT_APPLICABLE status")
		}
	}
	if err := ValidateOptionalJurisdiction(m.HostCountry); err != nil {
		return err
	}
	if err := ValidateOptionalJurisdiction(m.RecipientCountry); err != nil {
		return err
	}
	if err := ValidateDocumentURI(m.DocumentUri); err != nil {
		return err
	}
	return ValidateDocumentHash(m.DocumentHash)
}
func (m MsgSetArticle6Status) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

// ---- Bridge ---------------------------------------------------------------

func (m MsgSetBridgeAttestation) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
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
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
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
	if m.AssetId == 0 {
		return fmt.Errorf("asset_id must be > 0")
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
