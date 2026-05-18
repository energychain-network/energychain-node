package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgRegisterArbitrator{}
	_ sdk.Msg = &MsgUpdateArbitratorStatus{}
	_ sdk.Msg = &MsgOpenDispute{}
	_ sdk.Msg = &MsgRespond{}
	_ sdk.Msg = &MsgSubmitEvidence{}
	_ sdk.Msg = &MsgAssignTribunal{}
	_ sdk.Msg = &MsgCastVote{}
	_ sdk.Msg = &MsgFinalizeRuling{}
	_ sdk.Msg = &MsgCancelDispute{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---- MsgRegisterArbitrator -------------------------------

func (m *MsgRegisterArbitrator) ValidateBasic() error {
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
	for _, s := range m.AccreditedStandards {
		if err := ValidateNonEmpty("accredited_standards", s, 128); err != nil {
			return err
		}
	}
	return nil
}
func (m *MsgRegisterArbitrator) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgUpdateArbitratorStatus ---------------------------

func (m *MsgUpdateArbitratorStatus) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.ArbitratorId == 0 {
		return fmt.Errorf("arbitrator_id required")
	}
	if !ArbitratorStatusValid(m.NewStatus) {
		return fmt.Errorf("invalid new_status")
	}
	if len(m.Reason) > 2048 { // tighter bound enforced at keeper via params
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgUpdateArbitratorStatus) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgOpenDispute --------------------------------------

func (m *MsgOpenDispute) ValidateBasic() error {
	if err := ValidateAddr("plaintiff", m.Plaintiff); err != nil {
		return err
	}
	if err := ValidateAddr("respondent", m.Respondent); err != nil {
		return err
	}
	if m.Plaintiff == m.Respondent {
		return fmt.Errorf("plaintiff and respondent must differ")
	}
	if !SubjectKindValid(m.SubjectKind) {
		return fmt.Errorf("invalid subject_kind")
	}
	if err := ValidateSubjectRef(m.SubjectRef); err != nil {
		return err
	}
	if err := ValidateDenom(m.BondDenom); err != nil {
		return err
	}
	if m.PlaintiffBond == 0 {
		return fmt.Errorf("plaintiff_bond must be > 0")
	}
	if m.RespondentBondRequired == 0 {
		return fmt.Errorf("respondent_bond_required must be > 0")
	}
	if err := ValidateHash("claim_hash", m.ClaimHash); err != nil {
		return err
	}
	return nil
}
func (m *MsgOpenDispute) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Plaintiff)
	return []sdk.AccAddress{a}
}

// ---- MsgRespond ------------------------------------------

func (m *MsgRespond) ValidateBasic() error {
	if err := ValidateAddr("respondent", m.Respondent); err != nil {
		return err
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if m.Bond == 0 {
		return fmt.Errorf("bond must be > 0")
	}
	return nil
}
func (m *MsgRespond) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Respondent)
	return []sdk.AccAddress{a}
}

// ---- MsgSubmitEvidence -----------------------------------

func (m *MsgSubmitEvidence) ValidateBasic() error {
	if err := ValidateAddr("submitter", m.Submitter); err != nil {
		return err
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if m.Uri == "" {
		return fmt.Errorf("uri required")
	}
	if err := ValidateHash("hash", m.Hash); err != nil {
		return err
	}
	return nil
}
func (m *MsgSubmitEvidence) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Submitter)
	return []sdk.AccAddress{a}
}

// ---- MsgAssignTribunal -----------------------------------

func (m *MsgAssignTribunal) ValidateBasic() error {
	if err := ValidateAddr("authority", m.Authority); err != nil {
		return err
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if len(m.ArbitratorIds) == 0 {
		return fmt.Errorf("arbitrator_ids: empty")
	}
	seen := map[uint64]struct{}{}
	for _, id := range m.ArbitratorIds {
		if id == 0 {
			return fmt.Errorf("arbitrator_ids: zero")
		}
		if _, dup := seen[id]; dup {
			return fmt.Errorf("arbitrator_ids: duplicate %d", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
func (m *MsgAssignTribunal) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

// ---- MsgCastVote -----------------------------------------

func (m *MsgCastVote) ValidateBasic() error {
	if err := ValidateAddr("voter_address", m.VoterAddress); err != nil {
		return err
	}
	if m.ArbitratorId == 0 {
		return fmt.Errorf("arbitrator_id required")
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if !VoteChoiceValid(m.Choice) {
		return fmt.Errorf("invalid vote choice")
	}
	if len(m.Reason) > 2048 {
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgCastVote) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.VoterAddress)
	return []sdk.AccAddress{a}
}

// ---- MsgFinalizeRuling -----------------------------------

func (m *MsgFinalizeRuling) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if m.OverrideSlashBps > 10_000 {
		return fmt.Errorf("override_slash_bps > 10000")
	}
	if err := ValidateHash("ruling_hash", m.RulingHash); err != nil {
		return err
	}
	return nil
}
func (m *MsgFinalizeRuling) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{a}
}

// ---- MsgCancelDispute ------------------------------------

func (m *MsgCancelDispute) ValidateBasic() error {
	if err := ValidateAddr("actor", m.Actor); err != nil {
		return err
	}
	if m.DisputeId == 0 {
		return fmt.Errorf("dispute_id required")
	}
	if len(m.Reason) > 2048 {
		return fmt.Errorf("reason too long")
	}
	return nil
}
func (m *MsgCancelDispute) GetSigners() []sdk.AccAddress {
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
