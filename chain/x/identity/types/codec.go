package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterIdentity{}, "identity/MsgRegisterIdentity", nil)
	cdc.RegisterConcrete(&MsgUpdateIdentity{}, "identity/MsgUpdateIdentity", nil)
	cdc.RegisterConcrete(&MsgRevokeIdentity{}, "identity/MsgRevokeIdentity", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "identity/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterIdentity{},
		&MsgUpdateIdentity{},
		&MsgRevokeIdentity{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
