package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgRegisterList{}
	_ sdk.Msg = &MsgUpdateList{}
	_ sdk.Msg = &MsgDeprecateList{}
	_ sdk.Msg = &MsgArchiveList{}
	_ sdk.Msg = &MsgAddEntry{}
	_ sdk.Msg = &MsgRemoveEntry{}
	_ sdk.Msg = &MsgProposeDelta{}
	_ sdk.Msg = &MsgConfirmProposal{}
	_ sdk.Msg = &MsgRejectProposal{}
	_ sdk.Msg = &MsgUpdateParams{}
)

func validateAuthorityAddr(authority string) error {
	if _, err := sdk.AccAddressFromBech32(authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	return nil
}

func (m *MsgRegisterList) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if err := ValidateListID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if err := ValidateListName(m.Name); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("name: %s", err)
	}
	if err := ValidateJurisdiction(m.Jurisdiction); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("jurisdiction: %s", err)
	}
	if err := ValidateSourceURI(m.SourceUri); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("source_uri: %s", err)
	}
	if len(m.Description) > ListDescriptionMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("description too long")
	}
	if m.ListAuthority != "" {
		if _, err := sdk.AccAddressFromBech32(m.ListAuthority); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("list_authority: %s", err)
		}
	}
	return nil
}

func (m *MsgUpdateList) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if err := ValidateListID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if m.Name != "" {
		if err := ValidateListName(m.Name); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("name: %s", err)
		}
	}
	if m.SourceUri != "" {
		if err := ValidateSourceURI(m.SourceUri); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("source_uri: %s", err)
		}
	}
	if len(m.Description) > ListDescriptionMaxLen {
		return sdkerrors.ErrInvalidRequest.Wrap("description too long")
	}
	if m.ListAuthority != "" {
		if _, err := sdk.AccAddressFromBech32(m.ListAuthority); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("list_authority: %s", err)
		}
	}
	return nil
}

func (m *MsgDeprecateList) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if err := ValidateListID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if err := ValidateReason(m.Reason); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	return nil
}

func (m *MsgArchiveList) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if err := ValidateListID(m.Id); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("id: %s", err)
	}
	if err := ValidateReason(m.Reason); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	return nil
}

func validateEntryShape(e SanctionEntry) error {
	if err := ValidateListID(e.ListId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("list_id: %s", err)
	}
	if err := ValidateSubject(e.Subject); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("subject: %s", err)
	}
	if !SubjectKindValid(e.SubjectKind) {
		return sdkerrors.ErrInvalidRequest.Wrap("invalid subject_kind")
	}
	if err := ValidateProgram(e.Program); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	if err := ValidateReason(e.Reason); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	if err := ValidateSourceRef(e.SourceRef); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	return nil
}

func (m *MsgAddEntry) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	return validateEntryShape(m.Entry)
}

func (m *MsgRemoveEntry) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if err := ValidateListID(m.ListId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("list_id: %s", err)
	}
	if err := ValidateSubject(m.Subject); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("subject: %s", err)
	}
	if err := ValidateReason(m.Reason); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	return nil
}

func (m *MsgProposeDelta) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Proposer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("proposer: %s", err)
	}
	if err := ValidateListID(m.ListId); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("list_id: %s", err)
	}
	if len(m.Actions) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("at least one action required")
	}
	if uint32(len(m.Actions)) > MaxProposalActionsUpper {
		return sdkerrors.ErrInvalidRequest.Wrapf(
			"actions count %d exceeds upper bound %d", len(m.Actions), MaxProposalActionsUpper)
	}
	if err := ValidateSourceURI(m.SourceUri); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	if err := ValidateSourceDigest(m.SourceDigest); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	for i, a := range m.Actions {
		if !ProposalActionKindValid(a.Kind) {
			return sdkerrors.ErrInvalidRequest.Wrapf("action %d: invalid kind", i)
		}
		// Per-action shape check: the entry's list_id must match the
		// proposal's list_id (a single proposal is scoped to one list).
		if a.Entry.ListId != "" && a.Entry.ListId != m.ListId {
			return sdkerrors.ErrInvalidRequest.Wrapf(
				"action %d: entry.list_id %q != proposal.list_id %q", i, a.Entry.ListId, m.ListId)
		}
		if err := ValidateSubject(a.Entry.Subject); err != nil {
			return sdkerrors.ErrInvalidRequest.Wrapf("action %d: %s", i, err)
		}
		if a.Kind == ProposalAction_KIND_ADD {
			if !SubjectKindValid(a.Entry.SubjectKind) {
				return sdkerrors.ErrInvalidRequest.Wrapf("action %d: invalid subject_kind", i)
			}
			if err := ValidateProgram(a.Entry.Program); err != nil {
				return sdkerrors.ErrInvalidRequest.Wrapf("action %d: %s", i, err)
			}
			if err := ValidateReason(a.Entry.Reason); err != nil {
				return sdkerrors.ErrInvalidRequest.Wrapf("action %d: %s", i, err)
			}
		}
	}
	return nil
}

func (m *MsgConfirmProposal) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if m.ProposalId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("proposal_id must be > 0")
	}
	return nil
}

func (m *MsgRejectProposal) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	if m.ProposalId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("proposal_id must be > 0")
	}
	if err := ValidateReason(m.Reason); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	return nil
}

func (m *MsgUpdateParams) ValidateBasic() error {
	if err := validateAuthorityAddr(m.Authority); err != nil {
		return err
	}
	return m.Params.Validate()
}

// ---------------------------------------------------------------------------
// GetSigners — required by the SDK Msg interface for backwards-compat with
// pre-v0.50 routing.
// ---------------------------------------------------------------------------

func (m *MsgRegisterList) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdateList) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgDeprecateList) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgArchiveList) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgAddEntry) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgRemoveEntry) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgProposeDelta) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Proposer)
	return []sdk.AccAddress{a}
}
func (m *MsgConfirmProposal) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgRejectProposal) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
func (m *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}
