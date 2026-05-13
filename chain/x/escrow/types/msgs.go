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

func validateAssetSpec(kind AssetKind, denom string, tokenID, amount uint64) error {
	if !AssetKindValid(kind) {
		return fmt.Errorf("kind invalid")
	}
	if amount == 0 {
		return fmt.Errorf("amount must be > 0")
	}
	switch kind {
	case AssetKind_ASSET_KIND_STABLECOIN:
		if err := ValidateDenom(denom); err != nil {
			return err
		}
		if tokenID != 0 {
			return fmt.Errorf("rwa_token_id MUST be unset for stablecoin escrow")
		}
	case AssetKind_ASSET_KIND_RWA:
		if tokenID == 0 {
			return fmt.Errorf("rwa_token_id must be > 0 for rwa escrow")
		}
		if denom != "" {
			return fmt.Errorf("stablecoin_denom MUST be unset for rwa escrow")
		}
	}
	return nil
}

func validateTriggers(t Triggers) error {
	if t.ReleaseAfter < 0 {
		return fmt.Errorf("release_after cannot be negative")
	}
	if t.RefundAfter < 0 {
		return fmt.Errorf("refund_after cannot be negative")
	}
	// Allow release_after == refund_after; some flows expire identically.
	if t.OracleTopicId != "" {
		if t.OracleMinValue < 0 {
			return fmt.Errorf("oracle_min_value cannot be negative")
		}
		if len(t.OracleTopicId) > 128 {
			return fmt.Errorf("oracle_topic_id too long (max 128)")
		}
		if t.OracleMaxStalenessSeconds < 0 {
			return fmt.Errorf("oracle_max_staleness_seconds cannot be negative")
		}
	} else {
		if t.OracleMinValue != 0 || t.OracleMaxStalenessSeconds != 0 {
			return fmt.Errorf("oracle settings without oracle_topic_id are meaningless")
		}
	}
	return nil
}

// ---- CreateEscrow / Fund / Cancel ----------------------------------------

func (m *MsgCreateEscrow) ValidateBasic() error {
	if err := ValidateAddr("depositor", m.Depositor); err != nil {
		return err
	}
	if err := ValidateAddr("beneficiary", m.Beneficiary); err != nil {
		return err
	}
	if m.Beneficiary == m.Depositor {
		return fmt.Errorf("beneficiary must differ from depositor")
	}
	if err := ValidateAddr("fallback_addr", m.FallbackAddr); err != nil {
		return err
	}
	if err := ValidateOptionalAddr("arbiter", m.Arbiter); err != nil {
		return err
	}
	if len(m.Committee) == 0 {
		return fmt.Errorf("committee must be non-empty")
	}
	seen := map[string]bool{}
	for _, s := range m.Committee {
		if err := ValidateAddr("signer", s); err != nil {
			return err
		}
		if seen[s] {
			return fmt.Errorf("duplicate signer %s", s)
		}
		seen[s] = true
	}
	if m.ApprovalThreshold == 0 || int(m.ApprovalThreshold) > len(m.Committee) {
		return fmt.Errorf("approval_threshold must be in (0, %d]", len(m.Committee))
	}
	if err := validateAssetSpec(m.Kind, m.StablecoinDenom, m.RwaTokenId, m.Amount); err != nil {
		return err
	}
	if err := validateTriggers(m.Triggers); err != nil {
		return err
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgCreateEscrow) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Depositor)
	return []sdk.AccAddress{addr}
}

func (m *MsgFund) ValidateBasic() error {
	if err := ValidateAddr("depositor", m.Depositor); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return nil
}
func (m *MsgFund) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Depositor)
	return []sdk.AccAddress{addr}
}

func (m *MsgCancel) ValidateBasic() error {
	if err := ValidateAddr("depositor", m.Depositor); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgCancel) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Depositor)
	return []sdk.AccAddress{addr}
}

// ---- Approve / Revoke ----------------------------------------------------

func (m *MsgApprove) ValidateBasic() error {
	if err := ValidateAddr("signer", m.Signer); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	if !IntentValid(m.Intent) {
		return fmt.Errorf("intent invalid")
	}
	return ValidateMemo(m.Memo)
}
func (m *MsgApprove) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Signer)
	return []sdk.AccAddress{addr}
}

func (m *MsgRevokeApproval) ValidateBasic() error {
	if err := ValidateAddr("signer", m.Signer); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return nil
}
func (m *MsgRevokeApproval) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Signer)
	return []sdk.AccAddress{addr}
}

// ---- Release / Refund ----------------------------------------------------

func (m *MsgRelease) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return nil
}
func (m *MsgRelease) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{addr}
}

func (m *MsgRefund) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return nil
}
func (m *MsgRefund) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{addr}
}

// ---- Arbiter overrides ---------------------------------------------------

func (m *MsgArbiterRelease) ValidateBasic() error {
	if err := ValidateAddr("arbiter", m.Arbiter); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgArbiterRelease) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Arbiter)
	return []sdk.AccAddress{addr}
}

func (m *MsgArbiterRefund) ValidateBasic() error {
	if err := ValidateAddr("arbiter", m.Arbiter); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgArbiterRefund) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Arbiter)
	return []sdk.AccAddress{addr}
}

// ---- Dispute hook --------------------------------------------------------

func (m *MsgMarkDisputed) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgMarkDisputed) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{addr}
}

func (m *MsgResolveDispute) ValidateBasic() error {
	if err := ValidateAddr("arbiter", m.Arbiter); err != nil {
		return err
	}
	if m.EscrowId == 0 {
		return fmt.Errorf("escrow_id must be > 0")
	}
	return ValidateReason(m.Reason)
}
func (m *MsgResolveDispute) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Arbiter)
	return []sdk.AccAddress{addr}
}

// ---- UpdateParams --------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := mustBech32(m.Authority); err != nil {
		return fmt.Errorf("authority: %w", err)
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{addr}
}
