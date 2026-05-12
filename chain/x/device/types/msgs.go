package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var (
	_ sdk.Msg = &MsgRegisterDevice{}
	_ sdk.Msg = &MsgUpdateDevice{}
	_ sdk.Msg = &MsgTransferDevice{}
	_ sdk.Msg = &MsgRevokeDevice{}
	_ sdk.Msg = &MsgSubmitAttestation{}
	_ sdk.Msg = &MsgConfirmAttestation{}
	_ sdk.Msg = &MsgFlagFirmwareMismatch{}
	_ sdk.Msg = &MsgUpdateParams{}
)

func (m *MsgRegisterDevice) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("owner: %s", err)
	}
	if m.Device.DeviceDid == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("device.device_did required")
	}
	if _, err := sdk.AccAddressFromBech32(m.Device.DeviceDid); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("device.device_did: %s", err)
	}
	if m.Device.OwnerDid != "" && m.Device.OwnerDid != m.Owner {
		return sdkerrors.ErrUnauthorized.Wrap("device.owner_did must equal signer or be empty (auto-filled)")
	}
	if m.Device.Class == DeviceClass_DEVICE_CLASS_UNSPECIFIED {
		return sdkerrors.ErrInvalidRequest.Wrap("device.class must be set")
	}
	return nil
}
func (m *MsgRegisterDevice) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgUpdateDevice) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("owner: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.DeviceDid); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("device_did: %s", err)
	}
	return nil
}
func (m *MsgUpdateDevice) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgTransferDevice) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.CurrentOwner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("current_owner: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.NewOwner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("new_owner: %s", err)
	}
	if m.CurrentOwner == m.NewOwner {
		return sdkerrors.ErrInvalidRequest.Wrap("new_owner equals current_owner: nothing to do")
	}
	if _, err := sdk.AccAddressFromBech32(m.DeviceDid); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("device_did: %s", err)
	}
	return nil
}
func (m *MsgTransferDevice) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.CurrentOwner)
	return []sdk.AccAddress{a}
}

func (m *MsgRevokeDevice) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Actor); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("actor: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.DeviceDid); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("device_did: %s", err)
	}
	return nil
}
func (m *MsgRevokeDevice) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
	return []sdk.AccAddress{a}
}

func (m *MsgSubmitAttestation) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Owner); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("owner: %s", err)
	}
	if m.Evidence.DeviceDid == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("evidence.device_did required")
	}
	if m.Evidence.Format == "" {
		return sdkerrors.ErrInvalidRequest.Wrap("evidence.format required")
	}
	if len(m.Evidence.Evidence) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("evidence.evidence empty")
	}
	if len(m.Evidence.Nonce) == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("evidence.nonce empty")
	}
	return nil
}
func (m *MsgSubmitAttestation) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Owner)
	return []sdk.AccAddress{a}
}

func (m *MsgConfirmAttestation) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Verifier); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("verifier: %s", err)
	}
	if m.AttestationId == 0 {
		return sdkerrors.ErrInvalidRequest.Wrap("attestation_id required")
	}
	return nil
}
func (m *MsgConfirmAttestation) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Verifier)
	return []sdk.AccAddress{a}
}

func (m *MsgFlagFirmwareMismatch) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(m.Actor); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("actor: %s", err)
	}
	if _, err := sdk.AccAddressFromBech32(m.DeviceDid); err != nil {
		return sdkerrors.ErrInvalidAddress.Wrapf("device_did: %s", err)
	}
	return nil
}
func (m *MsgFlagFirmwareMismatch) GetSigners() []sdk.AccAddress {
	a, _ := sdk.AccAddressFromBech32(m.Actor)
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
