package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgCreateDID{}
	_ sdk.Msg = &MsgUpdateDID{}
	_ sdk.Msg = &MsgDeactivateDID{}
	_ sdk.Msg = &MsgRotateKey{}
	_ sdk.Msg = &MsgIssueCredential{}
	_ sdk.Msg = &MsgRevokeCredential{}
	_ sdk.Msg = &MsgRegisterAnchor{}
	_ sdk.Msg = &MsgDeactivateAnchor{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ValidateBasic only catches structural problems we can verify without
// reading state. Cross-document checks (e.g. controller actually holds a
// DID) live in the msg_server.

func (m *MsgCreateDID) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("creator: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Subject); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("subject: %s", err)
	}
	if m.Creator != m.Subject {
		return sdkerrors.ErrUnauthorized.Wrap("DID creation requires creator == subject")
	}
	if len(m.Verification) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("at least one verification method required")
	}
	return nil
}
func (m *MsgCreateDID) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Creator)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateDID) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Controller); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("controller: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Subject); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("subject: %s", err)
	}
	if m.ExpectedVersion == 0 {
		// 0 is the unset sentinel; force callers to be explicit so they
		// cannot accidentally clobber an updated document.
		return sdkerrors.ErrInvalidRequest.Wrap("expected_version must be > 0")
	}
	return nil
}
func (m *MsgUpdateDID) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Controller)
	return []sdk.AccAddress{a}
}

func (m *MsgDeactivateDID) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Controller); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("controller: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Subject); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("subject: %s", err)
	}
	return nil
}
func (m *MsgDeactivateDID) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Controller)
	return []sdk.AccAddress{a}
}

func (m *MsgRotateKey) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Controller); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("controller: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Subject); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("subject: %s", err)
	}
	if m.OldKeyId == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("old_key_id required")
	}
	if m.NewKey.Id == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("new_key.id required")
	}
	return nil
}
func (m *MsgRotateKey) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Controller)
	return []sdk.AccAddress{a}
}

func (m *MsgIssueCredential) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Issuer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("issuer: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Subject); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("subject: %s", err)
	}
	if m.Id == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("credential id required")
	}
	if m.Type == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("credential type required")
	}
	if m.Hash == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("credential hash required")
	}
	return nil
}
func (m *MsgIssueCredential) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Issuer)
	return []sdk.AccAddress{a}
}

func (m *MsgRevokeCredential) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Issuer); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("issuer: %s", err)
	}
	if m.Id == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("credential id required")
	}
	return nil
}
func (m *MsgRevokeCredential) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Issuer)
	return []sdk.AccAddress{a}
}

func (m *MsgRegisterAnchor) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Did); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("did: %s", err)
	}
	switch m.Tier {
	case AnchorTierRoot:
		// parent_did must be empty for roots
		if m.ParentDid != "" {
			return sdkerrors.ErrInvalidRequest.Wrap("root anchor: parent_did must be empty")
		}
	case AnchorTierIntermediate:
		if _, err := sdk.AccAddressFromBech32(m.ParentDid); err != nil {
			return sdkerrors.ErrInvalidAddress.Wrapf("parent_did: %s", err)
		}
		// SECURITY: prevent self-parented intermediates. Without this
		// check, an intermediate could be registered with parent_did
		// equal to its own DID, creating a one-node "cycle" that the
		// keeper's chain-walk would never escape (we cap the depth, but
		// a self-loop would still mean the anchor passes the active-
		// chain check the moment it is created — even after governance
		// disables every legitimate root above it).
		if m.Did == m.ParentDid {
			return sdkerrors.ErrInvalidRequest.Wrap("intermediate anchor: did must differ from parent_did")
		}
	default:
		return sdkerrors.ErrInvalidRequest.Wrapf("unknown tier %q", m.Tier)
	}
	return nil
}
func (m *MsgRegisterAnchor) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

func (m *MsgDeactivateAnchor) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("authority: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.Did); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("did: %s", err)
	}
	return nil
}
func (m *MsgDeactivateAnchor) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Authority)
	return []sdk.AccAddress{a}
}

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
