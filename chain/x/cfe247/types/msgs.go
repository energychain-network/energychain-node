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

func mustSigner(addr string) []sdk.AccAddress {
	a, err := sdk.AccAddressFromBech32(addr)
	if err != nil {
		return nil
	}
	return []sdk.AccAddress{a}
}

// ---- Data provider --------------------------------------------------------

func (m MsgRegisterDataProvider) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "provider"); err != nil {
		return err
	}
	if err := ValidateDID(m.Did); err != nil {
		return err
	}
	if err := ValidateDisplayName(m.DisplayName); err != nil {
		return err
	}
	if err := validateBech32(m.Attestor, "attestor"); err != nil {
		return err
	}
	return validateBech32(m.Admin, "admin")
}
func (m MsgRegisterDataProvider) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgUpdateDataProvider) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "provider"); err != nil {
		return err
	}
	if m.DisplayName != "" {
		if err := ValidateDisplayName(m.DisplayName); err != nil {
			return err
		}
	}
	if m.Attestor != "" {
		if err := validateBech32(m.Attestor, "attestor"); err != nil {
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
func (m MsgUpdateDataProvider) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgSuspendDataProvider) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "provider"); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m MsgSuspendDataProvider) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgRevokeDataProvider) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "provider"); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m MsgRevokeDataProvider) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

// ---- Grid zone ------------------------------------------------------------

func (m MsgRegisterGridZone) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "zone"); err != nil {
		return err
	}
	if err := ValidateDisplayName(m.DisplayName); err != nil {
		return err
	}
	if m.Country != "" {
		if err := ValidateCountry(m.Country); err != nil {
			return err
		}
	}
	if m.Description != "" {
		if err := ValidateDescription(m.Description); err != nil {
			return err
		}
	}
	return nil
}
func (m MsgRegisterGridZone) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgUpdateGridZone) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "zone"); err != nil {
		return err
	}
	if m.DisplayName != "" {
		if err := ValidateDisplayName(m.DisplayName); err != nil {
			return err
		}
	}
	if m.Country != "" {
		if err := ValidateCountry(m.Country); err != nil {
			return err
		}
	}
	if m.Description != "" {
		if err := ValidateDescription(m.Description); err != nil {
			return err
		}
	}
	return nil
}
func (m MsgUpdateGridZone) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

func (m MsgRemoveGridZone) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	return ValidateID(m.Id, "zone")
}
func (m MsgRemoveGridZone) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }

// ---- Subject --------------------------------------------------------------

func (m MsgRegisterSubject) ValidateBasic() error {
	if err := validateBech32(m.Creator, "creator"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "subject"); err != nil {
		return err
	}
	if err := ValidateDisplayName(m.DisplayName); err != nil {
		return err
	}
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if m.DefaultGridZone != "" {
		if err := ValidateID(m.DefaultGridZone, "default_grid_zone"); err != nil {
			return err
		}
	}
	return nil
}
func (m MsgRegisterSubject) GetSigners() []sdk.AccAddress { return mustSigner(m.Creator) }

func (m MsgUpdateSubject) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "subject"); err != nil {
		return err
	}
	if m.DisplayName != "" {
		if err := ValidateDisplayName(m.DisplayName); err != nil {
			return err
		}
	}
	if m.DefaultGridZone != "" {
		if err := ValidateID(m.DefaultGridZone, "default_grid_zone"); err != nil {
			return err
		}
	}
	if m.NewAdmin != "" {
		if err := validateBech32(m.NewAdmin, "new_admin"); err != nil {
			return err
		}
	}
	return nil
}
func (m MsgUpdateSubject) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

func (m MsgDeactivateSubject) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if err := ValidateID(m.Id, "subject"); err != nil {
		return err
	}
	return ValidateReason(m.Reason)
}
func (m MsgDeactivateSubject) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

// ---- Consumption ----------------------------------------------------------

func validateConsumption(subjectID, gridZone string, hourStart int64, wh uint64, batchRef string) error {
	if err := ValidateID(subjectID, "subject"); err != nil {
		return err
	}
	if err := ValidateID(gridZone, "grid_zone"); err != nil {
		return err
	}
	if err := ValidateHourStart(hourStart); err != nil {
		return err
	}
	if wh == 0 {
		return fmt.Errorf("wh_consumed must be > 0")
	}
	return ValidateBatchRef(batchRef)
}

func (m MsgAttestHourlyConsumption) ValidateBasic() error {
	if err := validateBech32(m.Attestor, "attestor"); err != nil {
		return err
	}
	if err := ValidateID(m.DataProviderId, "provider"); err != nil {
		return err
	}
	return validateConsumption(m.SubjectId, m.GridZone, m.HourStart, m.WhConsumed, m.MeterBatchId)
}
func (m MsgAttestHourlyConsumption) GetSigners() []sdk.AccAddress { return mustSigner(m.Attestor) }

func (m MsgAttestHourlyConsumptionBatch) ValidateBasic() error {
	if err := validateBech32(m.Attestor, "attestor"); err != nil {
		return err
	}
	if err := ValidateID(m.DataProviderId, "provider"); err != nil {
		return err
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("entries must be non-empty")
	}
	if uint32(len(m.Entries)) > MaxAttestationsPerBatchUpper {
		return fmt.Errorf("entries exceeds max_attestations_per_batch upper bound %d", MaxAttestationsPerBatchUpper)
	}
	for i, e := range m.Entries {
		if err := validateConsumption(e.SubjectId, e.GridZone, e.HourStart, e.WhConsumed, e.MeterBatchId); err != nil {
			return fmt.Errorf("entry[%d]: %w", i, err)
		}
	}
	return nil
}
func (m MsgAttestHourlyConsumptionBatch) GetSigners() []sdk.AccAddress { return mustSigner(m.Attestor) }

// ---- Match ----------------------------------------------------------------

func (m MsgAllocateMatch) ValidateBasic() error {
	if err := validateBech32(m.Allocator, "allocator"); err != nil {
		return err
	}
	if err := ValidateID(m.SubjectId, "subject"); err != nil {
		return err
	}
	if err := ValidateHourStart(m.HourStart); err != nil {
		return err
	}
	if m.EacRetirementId == 0 {
		return fmt.Errorf("eac_retirement_id must be > 0")
	}
	if m.WhMatched == 0 {
		return fmt.Errorf("wh_matched must be > 0")
	}
	return nil
}
func (m MsgAllocateMatch) GetSigners() []sdk.AccAddress { return mustSigner(m.Allocator) }

// ---- Report ---------------------------------------------------------------

func (m MsgGenerateReport) ValidateBasic() error {
	if err := validateBech32(m.Admin, "admin"); err != nil {
		return err
	}
	if err := ValidateID(m.SubjectId, "subject"); err != nil {
		return err
	}
	if !ReportFormatValid(m.Format) {
		return fmt.Errorf("invalid report format")
	}
	if err := ValidateHourStart(m.PeriodStart); err != nil {
		return err
	}
	if err := ValidateHourStart(m.PeriodEnd); err != nil {
		return err
	}
	if m.PeriodEnd <= m.PeriodStart {
		return fmt.Errorf("period_end must be > period_start")
	}
	if err := ValidateReportURI(m.ReportUri); err != nil {
		return err
	}
	return ValidateReportHash(m.ReportHash)
}
func (m MsgGenerateReport) GetSigners() []sdk.AccAddress { return mustSigner(m.Admin) }

// ---- Params ---------------------------------------------------------------

func (m MsgUpdateParams) ValidateBasic() error {
	if err := validateBech32(m.Authority, "authority"); err != nil {
		return err
	}
	return m.Params.Validate()
}
func (m MsgUpdateParams) GetSigners() []sdk.AccAddress { return mustSigner(m.Authority) }
