package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	TypeMsgRegisterIdentity = "register_identity"
	TypeMsgUpdateIdentity   = "update_identity"
	TypeMsgRevokeIdentity   = "revoke_identity"
)

var (
	_ sdk.Msg = &MsgRegisterIdentity{}
	_ sdk.Msg = &MsgUpdateIdentity{}
	_ sdk.Msg = &MsgRevokeIdentity{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// ---------------------------------------------------------------------------
// MsgRegisterIdentity
// ---------------------------------------------------------------------------

func NewMsgRegisterIdentity(creator, address, name, role, metadata string) *MsgRegisterIdentity {
	return &MsgRegisterIdentity{
		Creator:  creator,
		Address:  address,
		Name:     name,
		Role:     role,
		Metadata: metadata,
	}
}

func (msg *MsgRegisterIdentity) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid identity address: %s", err)
	}
	if msg.Name == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("name cannot be empty")
	}
	if msg.Role == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("role cannot be empty")
	}
	return nil
}

func (msg *MsgRegisterIdentity) GetSigners() []sdk.AccAddress {
	creator, _ := sdk.AccAddressFromBech32(msg.Creator)
	return []sdk.AccAddress{creator}
}

// ---------------------------------------------------------------------------
// MsgUpdateIdentity
// ---------------------------------------------------------------------------

func NewMsgUpdateIdentity(creator, address, name, metadata string) *MsgUpdateIdentity {
	return &MsgUpdateIdentity{
		Creator:  creator,
		Address:  address,
		Name:     name,
		Metadata: metadata,
	}
}

func (msg *MsgUpdateIdentity) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid identity address: %s", err)
	}
	return nil
}

func (msg *MsgUpdateIdentity) GetSigners() []sdk.AccAddress {
	creator, _ := sdk.AccAddressFromBech32(msg.Creator)
	return []sdk.AccAddress{creator}
}

// ---------------------------------------------------------------------------
// MsgRevokeIdentity
// ---------------------------------------------------------------------------

func NewMsgRevokeIdentity(creator, address, reason string) *MsgRevokeIdentity {
	return &MsgRevokeIdentity{
		Creator: creator,
		Address: address,
		Reason:  reason,
	}
}

func (msg *MsgRevokeIdentity) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(msg.Address); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid identity address: %s", err)
	}
	return nil
}

func (msg *MsgRevokeIdentity) GetSigners() []sdk.AccAddress {
	creator, _ := sdk.AccAddressFromBech32(msg.Creator)
	return []sdk.AccAddress{creator}
}

// ---------------------------------------------------------------------------
// MsgUpdateParams (gov-only)
// ---------------------------------------------------------------------------

func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid authority address: %s", err)
	}
	if err := msg.Params.Validate(); err != nil {
		return sdkerrors.ErrInvalidRequest.Wrapf("invalid params: %s", err)
	}
	return nil
}

func (msg *MsgUpdateParams) GetSigners() []sdk.AccAddress {
	signer, _ := sdk.AccAddressFromBech32(msg.Authority)
	return []sdk.AccAddress{signer}
}
