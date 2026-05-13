package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ---- Helpers ----

func mustAddr(s string) (sdk.AccAddress, error) {
	if s == "" {
		return nil, fmt.Errorf("address must be non-empty")
	}
	addr, err := sdk.AccAddressFromBech32(s)
	if err != nil {
		return nil, fmt.Errorf("address %q invalid: %w", s, err)
	}
	return addr, nil
}

func validateAuthority(a string) error {
	_, err := mustAddr(a)
	return err
}

// ---- Denom ----

func (m *MsgRegisterDenom) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
		return err
	}
	if err := ValidateDenomID(m.Id); err != nil {
		return err
	}
	if err := ValidateDenomSymbol(m.Symbol); err != nil {
		return err
	}
	if err := ValidateDenomName(m.Name); err != nil {
		return err
	}
	if err := ValidateDecimals(m.Decimals); err != nil {
		return err
	}
	if err := ValidateJurisdiction(m.Jurisdiction); err != nil {
		return err
	}
	if err := ValidatePolicyID(m.PolicyId); err != nil {
		return err
	}
	return nil
}
func (m *MsgRegisterDenom) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateDenom) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
		return err
	}
	if err := ValidateDenomID(m.Id); err != nil {
		return err
	}
	if m.Symbol != "" {
		if err := ValidateDenomSymbol(m.Symbol); err != nil {
			return err
		}
	}
	if m.Name != "" {
		if err := ValidateDenomName(m.Name); err != nil {
			return err
		}
	}
	if m.Jurisdiction != "" {
		if err := ValidateJurisdiction(m.Jurisdiction); err != nil {
			return err
		}
	}
	if err := ValidatePolicyID(m.PolicyId); err != nil {
		return err
	}
	return nil
}
func (m *MsgUpdateDenom) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func validateLifecycle(authority, id, reason string) error {
	if err := validateAuthority(authority); err != nil {
		return err
	}
	if err := ValidateDenomID(id); err != nil {
		return err
	}
	return ValidateReason(reason)
}

func (m *MsgPauseDenom) ValidateBasic() error  { return validateLifecycle(m.Authority, m.Id, m.Reason) }
func (m *MsgResumeDenom) ValidateBasic() error { return validateLifecycle(m.Authority, m.Id, m.Reason) }
func (m *MsgRetireDenom) ValidateBasic() error { return validateLifecycle(m.Authority, m.Id, m.Reason) }

func (m *MsgPauseDenom) GetSigners() []sdk.AccAddress  { a, _ := sdk.AccAddressFromBech32(m.Authority); return []sdk.AccAddress{a} }
func (m *MsgResumeDenom) GetSigners() []sdk.AccAddress { a, _ := sdk.AccAddressFromBech32(m.Authority); return []sdk.AccAddress{a} }
func (m *MsgRetireDenom) GetSigners() []sdk.AccAddress { a, _ := sdk.AccAddressFromBech32(m.Authority); return []sdk.AccAddress{a} }

// ---- Issuer ----

func (m *MsgRegisterIssuer) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
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
	if len(m.MintAuthorities) == 0 {
		return fmt.Errorf("at least one mint authority required")
	}
	seen := map[string]bool{}
	for _, a := range m.MintAuthorities {
		if _, err := mustAddr(a); err != nil {
			return fmt.Errorf("mint_authority %q: %w", a, err)
		}
		if seen[a] {
			return fmt.Errorf("duplicate mint_authority %q", a)
		}
		seen[a] = true
	}
	if _, err := mustAddr(m.Admin); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	return nil
}
func (m *MsgRegisterIssuer) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateIssuer) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
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
	seen := map[string]bool{}
	for _, a := range m.MintAuthorities {
		if _, err := mustAddr(a); err != nil {
			return fmt.Errorf("mint_authority %q: %w", a, err)
		}
		if seen[a] {
			return fmt.Errorf("duplicate mint_authority %q", a)
		}
		seen[a] = true
	}
	if m.Admin != "" {
		if _, err := mustAddr(m.Admin); err != nil {
			return fmt.Errorf("admin: %w", err)
		}
	}
	return nil
}
func (m *MsgUpdateIssuer) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func validateIssuerLifecycle(authority, id, reason string) error {
	if err := validateAuthority(authority); err != nil {
		return err
	}
	if err := ValidateIssuerID(id); err != nil {
		return err
	}
	return ValidateReason(reason)
}

func (m *MsgSuspendIssuer) ValidateBasic() error { return validateIssuerLifecycle(m.Authority, m.Id, m.Reason) }
func (m *MsgRevokeIssuer) ValidateBasic() error  { return validateIssuerLifecycle(m.Authority, m.Id, m.Reason) }
func (m *MsgSuspendIssuer) GetSigners() []sdk.AccAddress { a, _ := sdk.AccAddressFromBech32(m.Authority); return []sdk.AccAddress{a} }
func (m *MsgRevokeIssuer) GetSigners() []sdk.AccAddress  { a, _ := sdk.AccAddressFromBech32(m.Authority); return []sdk.AccAddress{a} }

// ---- Quotas / reserves ----

func (m *MsgSetMintQuota) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	return ValidateDenomID(m.DenomId)
}
func (m *MsgSetMintQuota) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgSetMintPaused) ValidateBasic() error {
	if _, err := mustAddr(m.Admin); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m *MsgSetMintPaused) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{a}
}

func (m *MsgSetReserveRequirement) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if err := ValidateOracleTopic(m.OracleTopicId); err != nil {
		return err
	}
	if m.RequiredRatioBps == 0 {
		return fmt.Errorf("required_ratio_bps must be > 0")
	}
	if m.MaxStalenessSeconds == 0 {
		return fmt.Errorf("max_staleness_seconds must be > 0")
	}
	return nil
}
func (m *MsgSetReserveRequirement) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- Hot path ----

func (m *MsgMint) ValidateBasic() error {
	if _, err := mustAddr(m.Minter); err != nil {
		return fmt.Errorf("minter: %w", err)
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if _, err := mustAddr(m.Recipient); err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgMint) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Minter)
	return []sdk.AccAddress{a}
}

func (m *MsgBurn) ValidateBasic() error {
	if _, err := mustAddr(m.Holder); err != nil {
		return fmt.Errorf("holder: %w", err)
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgBurn) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Holder)
	return []sdk.AccAddress{a}
}

func (m *MsgTransfer) ValidateBasic() error {
	from, err := mustAddr(m.From)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	to, err := mustAddr(m.To)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if from.Equals(to) {
		return fmt.Errorf("from and to must differ")
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgTransfer) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.From)
	return []sdk.AccAddress{a}
}

func (m *MsgApprove) ValidateBasic() error {
	owner, err := mustAddr(m.Owner)
	if err != nil {
		return fmt.Errorf("owner: %w", err)
	}
	spender, err := mustAddr(m.Spender)
	if err != nil {
		return fmt.Errorf("spender: %w", err)
	}
	if owner.Equals(spender) {
		return fmt.Errorf("owner and spender must differ")
	}
	return ValidateDenomID(m.DenomId)
}
func (m *MsgApprove) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgTransferFrom) ValidateBasic() error {
	if _, err := mustAddr(m.Spender); err != nil {
		return fmt.Errorf("spender: %w", err)
	}
	from, err := mustAddr(m.From)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	to, err := mustAddr(m.To)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if from.Equals(to) {
		return fmt.Errorf("from and to must differ")
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgTransferFrom) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Spender)
	return []sdk.AccAddress{a}
}

// ---- Compliance / force ----

func validateAdminFlag(admin, issuerID, denomID, account, reason string) error {
	if _, err := mustAddr(admin); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	if err := ValidateIssuerID(issuerID); err != nil {
		return err
	}
	if err := ValidateDenomID(denomID); err != nil {
		return err
	}
	if _, err := mustAddr(account); err != nil {
		return fmt.Errorf("account: %w", err)
	}
	return ValidateReason(reason)
}

func (m *MsgFreeze) ValidateBasic() error {
	return validateAdminFlag(m.Admin, m.IssuerId, m.DenomId, m.Account, m.Reason)
}
func (m *MsgUnfreeze) ValidateBasic() error {
	return validateAdminFlag(m.Admin, m.IssuerId, m.DenomId, m.Account, m.Reason)
}
func (m *MsgBlacklist) ValidateBasic() error {
	return validateAdminFlag(m.Admin, m.IssuerId, m.DenomId, m.Account, m.Reason)
}
func (m *MsgUnblacklist) ValidateBasic() error {
	return validateAdminFlag(m.Admin, m.IssuerId, m.DenomId, m.Account, m.Reason)
}
func (m *MsgFreeze) GetSigners() []sdk.AccAddress      { a, _ := sdk.AccAddressFromBech32(m.Admin); return []sdk.AccAddress{a} }
func (m *MsgUnfreeze) GetSigners() []sdk.AccAddress    { a, _ := sdk.AccAddressFromBech32(m.Admin); return []sdk.AccAddress{a} }
func (m *MsgBlacklist) GetSigners() []sdk.AccAddress   { a, _ := sdk.AccAddressFromBech32(m.Admin); return []sdk.AccAddress{a} }
func (m *MsgUnblacklist) GetSigners() []sdk.AccAddress { a, _ := sdk.AccAddressFromBech32(m.Admin); return []sdk.AccAddress{a} }

func (m *MsgForceTransfer) ValidateBasic() error {
	if _, err := mustAddr(m.Admin); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	from, err := mustAddr(m.From)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	to, err := mustAddr(m.To)
	if err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if from.Equals(to) {
		return fmt.Errorf("from and to must differ")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgForceTransfer) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{a}
}

// ---- Redemption ----

func (m *MsgRequestRedemption) ValidateBasic() error {
	if _, err := mustAddr(m.Holder); err != nil {
		return fmt.Errorf("holder: %w", err)
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateDenomID(m.DenomId); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgRequestRedemption) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Holder)
	return []sdk.AccAddress{a}
}

func (m *MsgFulfillRedemption) ValidateBasic() error {
	if _, err := mustAddr(m.Admin); err != nil {
		return fmt.Errorf("admin: %w", err)
	}
	if m.RedemptionId == 0 {
		return fmt.Errorf("redemption_id must be > 0")
	}
	return ValidatePayoutRef(m.PayoutRef)
}
func (m *MsgFulfillRedemption) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{a}
}

func (m *MsgCancelRedemption) ValidateBasic() error {
	if _, err := mustAddr(m.Signer); err != nil {
		return fmt.Errorf("signer: %w", err)
	}
	if m.RedemptionId == 0 {
		return fmt.Errorf("redemption_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelRedemption) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Signer)
	return []sdk.AccAddress{a}
}

// ---- Params ----

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := validateAuthority(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
