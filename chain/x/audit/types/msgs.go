package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const TypeMsgRecordAudit = "record_audit"

var (
	_ sdk.Msg = &MsgRecordAudit{}
	_ sdk.Msg = &MsgUpdateParams{}
)

func NewMsgRecordAudit(creator, eventType, target, action, data string) *MsgRecordAudit {
	return &MsgRecordAudit{
		Creator:   creator,
		EventType: eventType,
		Target:    target,
		Action:    action,
		Data:      data,
	}
}

func (msg *MsgRecordAudit) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Creator); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("invalid creator address: %s", err)
	}
	if msg.EventType == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("event_type cannot be empty")
	}
	if msg.Action == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("action cannot be empty")
	}
	return nil
}

func (msg *MsgRecordAudit) GetSigners() []sdk.AccAddress {
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
