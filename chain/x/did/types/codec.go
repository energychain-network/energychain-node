package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateDID{}, "did/MsgCreateDID", nil)
	cdc.RegisterConcrete(&MsgUpdateDID{}, "did/MsgUpdateDID", nil)
	cdc.RegisterConcrete(&MsgDeactivateDID{}, "did/MsgDeactivateDID", nil)
	cdc.RegisterConcrete(&MsgRotateKey{}, "did/MsgRotateKey", nil)
	cdc.RegisterConcrete(&MsgIssueCredential{}, "did/MsgIssueCredential", nil)
	cdc.RegisterConcrete(&MsgRevokeCredential{}, "did/MsgRevokeCredential", nil)
	cdc.RegisterConcrete(&MsgRegisterAnchor{}, "did/MsgRegisterAnchor", nil)
	cdc.RegisterConcrete(&MsgDeactivateAnchor{}, "did/MsgDeactivateAnchor", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "did/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgCreateDID{}, &MsgUpdateDID{}, &MsgDeactivateDID{}, &MsgRotateKey{},
		&MsgIssueCredential{}, &MsgRevokeCredential{},
		&MsgRegisterAnchor{}, &MsgDeactivateAnchor{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
