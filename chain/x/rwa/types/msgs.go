package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func mustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("invalid bech32 %q: %w", addr, err)
	}
	return nil
}

// ---- Issuer lifecycle -----------------------------------------------------

func (m *MsgRegisterIssuer) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return fmt.Errorf("authority: %w", err)
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
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return fmt.Errorf("issuer_authority: %w", err)
	}
	if err := mustBech32(m.IssuerAdmin); err != nil {
		return fmt.Errorf("issuer_admin: %w", err)
	}
	return nil
}
func (m *MsgRegisterIssuer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateIssuer) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
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
	if m.IssuerAuthority != "" {
		if err := mustBech32(m.IssuerAuthority); err != nil {
			return err
		}
	}
	if m.IssuerAdmin != "" {
		if err := mustBech32(m.IssuerAdmin); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgUpdateIssuer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m *MsgSuspendIssuer) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m *MsgSuspendIssuer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

func (m *MsgRevokeIssuer) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.Id); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m *MsgRevokeIssuer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}

// ---- Token lifecycle ------------------------------------------------------

func (m *MsgCreateToken) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if err := ValidateIssuerID(m.IssuerId); err != nil {
		return err
	}
	if err := ValidateTokenSymbol(m.Symbol); err != nil {
		return err
	}
	if err := ValidateDisplayName(m.DisplayName); err != nil {
		return err
	}
	if m.Decimals > MaxDecimals {
		return fmt.Errorf("decimals %d > %d", m.Decimals, MaxDecimals)
	}
	if !AssetClassValid(m.AssetClass) {
		return fmt.Errorf("invalid asset_class")
	}
	if err := ValidateOptionalJurisdiction(m.Jurisdiction); err != nil {
		return err
	}
	if err := ValidatePolicyID(m.PolicyId); err != nil {
		return err
	}
	if err := ValidateStablecoinDenom(m.SettlementDenom); err != nil {
		return err
	}
	if m.RedemptionDelaySeconds < 0 || m.RedemptionDelaySeconds > MaxRedemptionDelay {
		return fmt.Errorf("redemption_delay_seconds out of range [0, %d]", MaxRedemptionDelay)
	}
	if m.RedemptionRate == 0 && m.RedemptionDelaySeconds != 0 {
		return fmt.Errorf("redemption_delay_seconds set but redemption_rate is 0 (redemption disabled)")
	}
	return nil
}
func (m *MsgCreateToken) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgUpdateToken) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if m.DisplayName != "" {
		if err := ValidateDisplayName(m.DisplayName); err != nil {
			return err
		}
	}
	if m.PolicyId != "" {
		if err := ValidatePolicyID(m.PolicyId); err != nil {
			return err
		}
	}
	if m.RedemptionDelaySeconds < 0 || m.RedemptionDelaySeconds > MaxRedemptionDelay {
		return fmt.Errorf("redemption_delay_seconds out of range [0, %d]", MaxRedemptionDelay)
	}
	return nil
}
func (m *MsgUpdateToken) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgPauseToken) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgPauseToken) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgUnpauseToken) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgUnpauseToken) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgTerminateToken) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgTerminateToken) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

// ---- Account flags --------------------------------------------------------

func (m *MsgSetAccountFlags) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if err := mustBech32(m.Account); err != nil {
		return fmt.Errorf("account: %w", err)
	}
	return ValidateOptionalJurisdiction(m.Jurisdiction)
}
func (m *MsgSetAccountFlags) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

// ---- Mint / burn ----------------------------------------------------------

func (m *MsgMint) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if err := mustBech32(m.Recipient); err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgMint) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

func (m *MsgBurn) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if err := mustBech32(m.From); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgBurn) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

// ---- Transfer -------------------------------------------------------------

func (m *MsgTransfer) ValidateBasic() error {
	if err := mustBech32(m.From); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if err := mustBech32(m.To); err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if m.From == m.To {
		return fmt.Errorf("from == to")
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgTransfer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.From)
	return []sdk.AccAddress{addr}
}

func (m *MsgForceTransfer) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if err := mustBech32(m.From); err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if err := mustBech32(m.To); err != nil {
		return fmt.Errorf("to: %w", err)
	}
	if m.From == m.To {
		return fmt.Errorf("from == to")
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgForceTransfer) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

// ---- Freeze / lockup ------------------------------------------------------

func (m *MsgSetFrozenBalance) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if err := mustBech32(m.Account); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m *MsgSetFrozenBalance) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

func (m *MsgAddLockup) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if err := mustBech32(m.Account); err != nil {
		return err
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	if m.UnlockTime <= 0 {
		return fmt.Errorf("unlock_time must be > 0 (unix seconds)")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgAddLockup) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

// ---- Snapshot / distribution ---------------------------------------------

func (m *MsgTakeSnapshot) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	return nil
}
func (m *MsgTakeSnapshot) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgCreateDistribution) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if m.SnapshotId == 0 {
		return fmt.Errorf("snapshot_id required")
	}
	if m.TotalAmount == 0 {
		return fmt.Errorf("total_amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgCreateDistribution) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

func (m *MsgFundDistribution) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.DistributionId == 0 {
		return fmt.Errorf("distribution_id required")
	}
	return nil
}
func (m *MsgFundDistribution) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

func (m *MsgClaimDistribution) ValidateBasic() error {
	if err := mustBech32(m.Claimer); err != nil {
		return err
	}
	if m.DistributionId == 0 {
		return fmt.Errorf("distribution_id required")
	}
	return nil
}
func (m *MsgClaimDistribution) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Claimer)
	return []sdk.AccAddress{addr}
}

func (m *MsgFinalizeDistribution) ValidateBasic() error {
	if err := mustBech32(m.Admin); err != nil {
		return err
	}
	if m.DistributionId == 0 {
		return fmt.Errorf("distribution_id required")
	}
	return nil
}
func (m *MsgFinalizeDistribution) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Admin)
	return []sdk.AccAddress{addr}
}

// ---- Redemption -----------------------------------------------------------

func (m *MsgRequestRedemption) ValidateBasic() error {
	if err := mustBech32(m.Holder); err != nil {
		return err
	}
	if m.TokenId == 0 {
		return fmt.Errorf("token_id required")
	}
	if m.Amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgRequestRedemption) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Holder)
	return []sdk.AccAddress{addr}
}

func (m *MsgSettleRedemption) ValidateBasic() error {
	if err := mustBech32(m.IssuerAuthority); err != nil {
		return err
	}
	if m.RedemptionId == 0 {
		return fmt.Errorf("redemption_id required")
	}
	return nil
}
func (m *MsgSettleRedemption) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.IssuerAuthority)
	return []sdk.AccAddress{addr}
}

func (m *MsgCancelRedemption) ValidateBasic() error {
	if err := mustBech32(m.Actor); err != nil {
		return err
	}
	if m.RedemptionId == 0 {
		return fmt.Errorf("redemption_id required")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancelRedemption) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{addr}
}

// ---- Params ---------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}
