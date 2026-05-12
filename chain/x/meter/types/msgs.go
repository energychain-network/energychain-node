package types

import (
	"encoding/hex"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgRegisterMeteringPoint{}
	_ sdk.Msg = &MsgUpdateMeteringPoint{}
	_ sdk.Msg = &MsgDeactivateMeteringPoint{}
	_ sdk.Msg = &MsgSubmitReading{}
	_ sdk.Msg = &MsgSubmitBatch{}
	_ sdk.Msg = &MsgAuthorizeStream{}
	_ sdk.Msg = &MsgRevokeStream{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---------------------------------------------------------------------------
// Metering point lifecycle
// ---------------------------------------------------------------------------

func (m *MsgRegisterMeteringPoint) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("submitter: %s", err)
	}
	if err := ValidateMeteringPointID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.OwnerAddress); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("owner_address: %s", err)
	}
	if m.DeviceDid == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("device_did required")
	}
	if !MeasurementTypeValid(m.MeasurementType) {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid measurement_type")
	}
	if len(m.GridZone) > GridZoneMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("grid_zone too long")
	}
	if len(m.GridOperator) > GridOperatorMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("grid_operator too long")
	}
	if len(m.Tariff) > TariffMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("tariff too long")
	}
	if m.Unit == "" || len(m.Unit) > UnitMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("unit length out of range")
	}
	if len(m.Timezone) > TimezoneMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("timezone too long")
	}
	// Strict size cap is enforced again in the keeper against Params,
	// but we cap raw bytes here so an oversize tx never makes it past
	// the mempool's cheap checks.
	if len(m.MetadataUri) > int(MaxMetadataURISizeUpper) {
		return sdkerrors.ErrInvalidRequest.Wrap("metadata_uri exceeds upper bound")
	}
	return nil
}

func (m *MsgUpdateMeteringPoint) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("submitter: %s", err)
	}
	if err := ValidateMeteringPointID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if len(m.GridZone) > GridZoneMaxLen ||
		len(m.GridOperator) > GridOperatorMaxLen ||
		len(m.Tariff) > TariffMaxLen ||
		len(m.Timezone) > TimezoneMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("string field too long")
	}
	if m.Unit != "" && len(m.Unit) > UnitMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("unit too long")
	}
	if len(m.MetadataUri) > int(MaxMetadataURISizeUpper) {
		return sdkerrors.ErrInvalidRequest.Wrap("metadata_uri exceeds upper bound")
	}
	return nil
}

func (m *MsgDeactivateMeteringPoint) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("submitter: %s", err)
	}
	if err := ValidateMeteringPointID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if len(m.Reason) > 256 {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Readings + batches
// ---------------------------------------------------------------------------

func (m *MsgSubmitReading) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("submitter: %s", err)
	}
	if err := ValidateMeteringPointID(m.MeteringPointId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("metering_point_id: %s", err)
	}
	period, ok := ReadingPeriodSeconds(m.Period)
	if !ok {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid period")
	}
	if m.StartTime <= 0 || m.EndTime <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("non-positive timestamps")
	}
	if m.EndTime-m.StartTime != period {
		return sdkerrors.ErrInvalidRequest.Wrapf(
			"end_time-start_time (%d) must equal period seconds (%d)",
			m.EndTime-m.StartTime, period)
	}
	if !CommitmentSchemeValid(m.CommitmentScheme) {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid commitment_scheme")
	}
	if len(m.CommitmentBytes) == 0 || len(m.CommitmentBytes) > int(MaxCommitmentSizeUpper) {
		return sdkerrors.ErrInvalidRequest.Wrap("commitment_bytes length out of range")
	}
	if !DataQualityValid(m.DataQuality) {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid data_quality")
	}
	if len(m.DeviceSignature) > int(MaxSignatureSizeUpper) {
		return sdkerrors.ErrInvalidRequest.Wrap("device_signature too long")
	}
	// Plaintext exposure is strictly opt-in and only meaningful for the
	// SHA256 scheme; reject negatives outright (energy units are
	// non-negative cumulative counters).
	if m.PlaintextValue < 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("plaintext_value must be non-negative")
	}
	if m.PlaintextValue != 0 && m.CommitmentScheme != CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256 {
		return sdkerrors.ErrInvalidRequest.Wrap(
			"plaintext_value may only be set when commitment_scheme=PLAINTEXT_SHA256")
	}
	return nil
}

func (m *MsgSubmitBatch) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Submitter); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("submitter: %s", err)
	}
	if err := ValidateMeteringPointID(m.MeteringPointId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("metering_point_id: %s", err)
	}
	if len(m.MerkleRoot) != MerkleHashHexLen {
		return sdkerrors.ErrInvalidRequest.Wrapf("merkle_root must be %d hex chars", MerkleHashHexLen)
	}
	if _, err := hex.DecodeString(m.MerkleRoot); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap("merkle_root must be hex")
	}
	if m.MerkleAlgorithm != MerkleAlgoSHA256 {
		return sdkerrors.ErrInvalidRequest.Wrapf("merkle_algorithm %q unsupported (only %q today)",
			m.MerkleAlgorithm, MerkleAlgoSHA256)
	}
	if m.Count == 0 || m.Count > MaxBatchCountUpper {
		return sdkerrors.ErrInvalidRequest.Wrap("count out of range")
	}
	if _, ok := ReadingPeriodSeconds(m.Period); !ok {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid period")
	}
	if m.StartTime <= 0 || m.EndTime <= 0 || m.StartTime > m.EndTime {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid time range")
	}
	if m.Uri == "" || len(m.Uri) > BatchURIMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("uri length out of range")
	}
	if len(m.DeviceSignature) > int(MaxSignatureSizeUpper) {
		return sdkerrors.ErrInvalidRequest.Wrap("device_signature too long")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stream authorisation
// ---------------------------------------------------------------------------

func (m *MsgAuthorizeStream) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authorizer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authorizer: %s", err)
	}
	if err := ValidateMeteringPointID(m.MeteringPointId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("metering_point_id: %s", err)
	}
	if m.DeviceDid == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("device_did required")
	}
	if m.ExpiresAt <= 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("expires_at must be > 0 (unix seconds)")
	}
	if len(m.Metadata) > StreamMetadataMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("metadata too long")
	}
	return nil
}

func (m *MsgRevokeStream) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authorizer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authorizer: %s", err)
	}
	if m.Id == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("id required")
	}
	if len(m.Reason) > 256 {
		return sdkerrors.ErrInvalidRequest.Wrap("reason too long")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Params
// ---------------------------------------------------------------------------

func (m *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return m.Params.Validate()
}

// ---------------------------------------------------------------------------
// GetSigners — required by the SDK Msg interface for backwards-compat with
// pre-v0.50 routing. The cosmos.msg.v1.signer annotation drives msg-service
// routing in modern SDKs but we keep these explicit for clarity.
// ---------------------------------------------------------------------------

func (m *MsgRegisterMeteringPoint) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdateMeteringPoint) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}
func (m *MsgDeactivateMeteringPoint) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}
func (m *MsgSubmitReading) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}
func (m *MsgSubmitBatch) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}
func (m *MsgAuthorizeStream) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authorizer)
	return []sdk.AccAddress{a}
}
func (m *MsgRevokeStream) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authorizer)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
