package types

import (
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgRegisterTopic{}
	_ sdk.Msg = &MsgUpdateTopic{}
	_ sdk.Msg = &MsgPauseTopic{}
	_ sdk.Msg = &MsgResumeTopic{}
	_ sdk.Msg = &MsgRegisterProvider{}
	_ sdk.Msg = &MsgTopUpBond{}
	_ sdk.Msg = &MsgRequestWithdrawBond{}
	_ sdk.Msg = &MsgWithdrawBond{}
	_ sdk.Msg = &MsgSuspendProvider{}
	_ sdk.Msg = &MsgSubmitValue{}
	_ sdk.Msg = &MsgSubmitReserveAttestation{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---------------------------------------------------------------------------
// Topic lifecycle
// ---------------------------------------------------------------------------

func validateTopicShape(t Topic) error {
	if err := ValidateTopicID(t.Id); err != nil {
		return err
	}
	if !TopicKindValid(t.Kind) {
		return sdkerrors.ErrInvalidRequest.Wrapf("unknown topic kind %d", t.Kind)
	}
	if !AggregationFnValid(t.Aggregation) {
		return sdkerrors.ErrInvalidRequest.Wrapf("unknown aggregation %d", t.Aggregation)
	}
	if t.OutlierBandBps > OutlierBandUpperBps {
		return sdkerrors.ErrInvalidRequest.Wrapf("outlier_band_bps exceeds %d", OutlierBandUpperBps)
	}
	if t.ValueDecimals > ValueDecimalsUpper {
		return sdkerrors.ErrInvalidRequest.Wrapf("value_decimals exceeds %d", ValueDecimalsUpper)
	}
	if len(t.Quote) > QuoteMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("quote longer than %d", QuoteMaxLen)
	}
	if len(t.Description) > 1024 {
		return sdkerrors.ErrInvalidRequest.Wrap("description too long")
	}
	if t.MinSubmissions == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("min_submissions must be > 0")
	}
	if t.MaxDataAgeSeconds < 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("max_data_age_seconds must be non-negative")
	}
	for _, a := range t.AllowList {
		if _, err := sdk.AccAddressFromBech32(a); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("allow_list entry %q: %s", a, err)
		}
	}
	return nil
}

func (m *MsgRegisterTopic) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return validateTopicShape(m.Topic)
}
func (m *MsgRegisterTopic) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateTopic) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return validateTopicShape(m.Topic)
}
func (m *MsgUpdateTopic) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgPauseTopic) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if err := ValidateTopicID(m.TopicId); err != nil {
		return err
	}
	if len(m.Reason) > ReasonMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}
func (m *MsgPauseTopic) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgResumeTopic) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return ValidateTopicID(m.TopicId)
}
func (m *MsgResumeTopic) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// Provider lifecycle
// ---------------------------------------------------------------------------

func (m *MsgRegisterProvider) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("address: %s", err)
	}
	if len(m.Name) > ProviderNameMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("name too long")
	}
	if len(m.ContactUri) > ContactURIMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("contact_uri too long")
	}
	if !m.Bond.IsValid() {
		return sdkerrors.ErrInvalidRequest.Wrap("bond is not a valid coin")
	}
	if !m.Bond.IsPositive() {
		return sdkerrors.ErrInvalidRequest.Wrap("bond amount must be positive")
	}
	if len(m.Ed25519Pubkey) != 0 && len(m.Ed25519Pubkey) != 32 {
		return sdkerrors.ErrInvalidRequest.Wrap("ed25519_pubkey must be empty or 32 bytes")
	}
	return nil
}
func (m *MsgRegisterProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Address)
	return []sdk.AccAddress{a}
}

func (m *MsgTopUpBond) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("address: %s", err)
	}
	if !m.Amount.IsValid() || !m.Amount.IsPositive() {
		return sdkerrors.ErrInvalidRequest.Wrap("amount must be positive")
	}
	return nil
}
func (m *MsgTopUpBond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Address)
	return []sdk.AccAddress{a}
}

func (m *MsgRequestWithdrawBond) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("address: %s", err)
	}
	return nil
}
func (m *MsgRequestWithdrawBond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Address)
	return []sdk.AccAddress{a}
}

func (m *MsgWithdrawBond) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("address: %s", err)
	}
	return nil
}
func (m *MsgWithdrawBond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Address)
	return []sdk.AccAddress{a}
}

func (m *MsgSuspendProvider) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("address: %s", err)
	}
	if m.Until <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("until must be > 0 (unix seconds)")
	}
	if len(m.Reason) > ReasonMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}
func (m *MsgSuspendProvider) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---------------------------------------------------------------------------
// Submissions
// ---------------------------------------------------------------------------

func (m *MsgSubmitValue) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Provider); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("provider: %s", err)
	}
	if err := ValidateTopicID(m.TopicId); err != nil {
		return err
	}
	if m.Timestamp <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("timestamp must be > 0")
	}
	if len(m.Attestation) > 256 {
		return sdkerrors.ErrInvalidRequest.Wrap("attestation longer than 256 bytes")
	}
	return nil
}
func (m *MsgSubmitValue) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Provider)
	return []sdk.AccAddress{a}
}

func (m *MsgSubmitReserveAttestation) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Custodian); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("custodian: %s", err)
	}
	if m.Id == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	if len(m.Asset) == 0 || len(m.Asset) > ReserveAssetMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("asset length out of range")
	}
	if len(m.AttestationUri) == 0 || len(m.AttestationUri) > ReserveURIMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("attestation_uri length out of range")
	}
	if len(m.AttestationHash) != ReserveHashHexLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("attestation_hash must be %d hex chars", ReserveHashHexLen)
	}
	if _, err := hex.DecodeString(m.AttestationHash); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap("attestation_hash must be hex")
	}
	if len(m.Amount) > ReserveAmountStringMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("amount string too long")
	}
	if _, err := ParseAmountString(m.Amount); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("amount: %s", err)
	}
	if len(m.Signers) == 0 || len(m.Signers) > ReserveMaxSigners {
		return sdkerrors.ErrInvalidRequest.Wrapf("signers length outside (0, %d]", ReserveMaxSigners)
	}
	if m.Threshold == 0 || int(m.Threshold) > len(m.Signers) {
		return sdkerrors.ErrInvalidRequest.Wrap("threshold must be in (0, len(signers)]")
	}
	seen := make(map[string]bool, len(m.Signers))
	for _, s := range m.Signers {
		if _, err := sdk.AccAddressFromBech32(s); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("signer %q: %s", s, err)
		}
		if seen[s] {
			return sdkerrors.ErrInvalidRequest.Wrapf("duplicate signer %s", s)
		}
		seen[s] = true
	}
	return nil
}

// MsgSubmitReserveAttestation deliberately omits a hand-written
// GetSigners() []sdk.AccAddress impl: the proto-generated GetSigners()
// returns the on-chain signer list ([]string) for the attestation, and
// the SDK msg-service routes the actual tx signer via the
// cosmos.msg.v1.signer = "custodian" annotation.

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return m.Params.Validate()
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
