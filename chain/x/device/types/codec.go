package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterDevice{}, "device/MsgRegisterDevice", nil)
	cdc.RegisterConcrete(&MsgUpdateDevice{}, "device/MsgUpdateDevice", nil)
	cdc.RegisterConcrete(&MsgTransferDevice{}, "device/MsgTransferDevice", nil)
	cdc.RegisterConcrete(&MsgRevokeDevice{}, "device/MsgRevokeDevice", nil)
	cdc.RegisterConcrete(&MsgSubmitAttestation{}, "device/MsgSubmitAttestation", nil)
	cdc.RegisterConcrete(&MsgConfirmAttestation{}, "device/MsgConfirmAttestation", nil)
	cdc.RegisterConcrete(&MsgFlagFirmwareMismatch{}, "device/MsgFlagFirmwareMismatch", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "device/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterDevice{}, &MsgUpdateDevice{}, &MsgTransferDevice{}, &MsgRevokeDevice{},
		&MsgSubmitAttestation{}, &MsgConfirmAttestation{}, &MsgFlagFirmwareMismatch{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
