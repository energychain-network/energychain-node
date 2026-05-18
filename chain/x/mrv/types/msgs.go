package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgRegisterSchema{}
	_ sdk.Msg = &MsgUpdateSchemaStatus{}
	_ sdk.Msg = &MsgRegisterVerifier{}
	_ sdk.Msg = &MsgUpdateVerifierStatus{}
	_ sdk.Msg = &MsgSubmitReport{}
	_ sdk.Msg = &MsgAttestReport{}
	_ sdk.Msg = &MsgRejectReport{}
	_ sdk.Msg = &MsgRetractReport{}
	_ sdk.Msg = &MsgGrantViewKey{}
	_ sdk.Msg = &MsgRevokeViewKey{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---- MsgRegisterSchema -----------------------------------

func (m *MsgRegisterSchema) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if err := ValidateNonEmpty("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	if err := ValidateNonEmpty("version", m.Version, VersionMaxLen); err != nil {
		return err
	}
	if err := ValidateJurisdiction("jurisdiction", m.Jurisdiction); err != nil {
		return err
	}
	if !AssetClassValid(m.AssetClass) {
		return fmt.Errorf("invalid asset_class")
	}
	if !TimeWindowValid(m.TimeWindow) {
		return fmt.Errorf("invalid time_window")
	}
	if !ReportFormatValid(m.OutputFormat) {
		return fmt.Errorf("invalid output_format")
	}
	if err := ValidateHash("schema_hash", m.SchemaHash); err != nil {
		return err
	}
	return nil
}
func (m *MsgRegisterSchema) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgUpdateSchemaStatus -------------------------------

func (m *MsgUpdateSchemaStatus) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.SchemaId == 0 {
		return fmt.Errorf("schema_id required")
	}
	if !SchemaStatusValid(m.NewStatus) {
		return fmt.Errorf("invalid new_status")
	}
	if len(m.Reason) > 1024 { // bounded liberally; precise bound enforced at keeper using params
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgUpdateSchemaStatus) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgRegisterVerifier ---------------------------------

func (m *MsgRegisterVerifier) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if err := ValidateAddr("signer_address", m.SignerAddress); err != nil {
		return err
	}
	if err := ValidateDID("did", m.Did); err != nil {
		return err
	}
	if err := ValidateNonEmpty("name", m.Name, NameMaxLen); err != nil {
		return err
	}
	for _, j := range m.Jurisdictions {
		if err := ValidateJurisdiction("jurisdictions", j); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgRegisterVerifier) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgUpdateVerifierStatus -----------------------------

func (m *MsgUpdateVerifierStatus) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.VerifierId == 0 {
		return fmt.Errorf("verifier_id required")
	}
	if !VerifierStatusValid(m.NewStatus) {
		return fmt.Errorf("invalid new_status")
	}
	return nil
}
func (m *MsgUpdateVerifierStatus) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgSubmitReport -------------------------------------

func (m *MsgSubmitReport) ValidateBasic() error {
	if err := ValidateAddr("submitter", m.Submitter); err != nil {
		return err
	}
	if err := ValidateAddr("subject", m.Subject); err != nil {
		return err
	}
	if m.SchemaId == 0 {
		return fmt.Errorf("schema_id required")
	}
	if m.PeriodEnd <= m.PeriodStart {
		return fmt.Errorf("period_end must be > period_start")
	}
	if err := ValidateHash("payload_hash", m.PayloadHash); err != nil {
		return err
	}
	if m.Aggregate.Cfe247HourlyScoreBps > 10_000 {
		return fmt.Errorf("cfe247_hourly_score_bps > 10000")
	}
	if m.Aggregate.Cfe247AnnualScoreBps > 10_000 {
		return fmt.Errorf("cfe247_annual_score_bps > 10000")
	}
	return nil
}
func (m *MsgSubmitReport) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}

// ---- MsgAttestReport -------------------------------------

func (m *MsgAttestReport) ValidateBasic() error {
	if err := ValidateAddr("verifier_address", m.VerifierAddress); err != nil {
		return err
	}
	if m.VerifierId == 0 {
		return fmt.Errorf("verifier_id required")
	}
	if m.ReportId == 0 {
		return fmt.Errorf("report_id required")
	}
	if err := ValidateHash("verifier_payload_hash", m.VerifierPayloadHash); err != nil {
		return err
	}
	return nil
}
func (m *MsgAttestReport) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.VerifierAddress)
	return []sdk.AccAddress{a}
}

// ---- MsgRejectReport -------------------------------------

func (m *MsgRejectReport) ValidateBasic() error {
	if err := ValidateAddr("verifier_address", m.VerifierAddress); err != nil {
		return err
	}
	if m.VerifierId == 0 || m.ReportId == 0 {
		return fmt.Errorf("verifier_id and report_id required")
	}
	return nil
}
func (m *MsgRejectReport) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.VerifierAddress)
	return []sdk.AccAddress{a}
}

// ---- MsgRetractReport ------------------------------------

func (m *MsgRetractReport) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.ReportId == 0 {
		return fmt.Errorf("report_id required")
	}
	return nil
}
func (m *MsgRetractReport) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{a}
}

// ---- MsgGrantViewKey -------------------------------------

func (m *MsgGrantViewKey) ValidateBasic() error {
	if err := ValidateAddr("granter", m.Granter); err != nil {
		return err
	}
	if err := ValidateDID("grantee_did", m.GranteeDid); err != nil {
		return err
	}
	if m.ExpiresAt <= 0 {
		return fmt.Errorf("expires_at must be > 0")
	}
	return nil
}
func (m *MsgGrantViewKey) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Granter)
	return []sdk.AccAddress{a}
}

// ---- MsgRevokeViewKey ------------------------------------

func (m *MsgRevokeViewKey) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.GrantId == 0 {
		return fmt.Errorf("grant_id required")
	}
	return nil
}
func (m *MsgRevokeViewKey) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
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
