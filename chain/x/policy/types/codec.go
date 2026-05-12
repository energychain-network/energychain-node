package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterPolicy{}, "policy/MsgRegisterPolicy", nil)
	cdc.RegisterConcrete(&MsgUpdatePolicy{}, "policy/MsgUpdatePolicy", nil)
	cdc.RegisterConcrete(&MsgDeprecatePolicy{}, "policy/MsgDeprecatePolicy", nil)
	cdc.RegisterConcrete(&MsgDisablePolicy{}, "policy/MsgDisablePolicy", nil)
	cdc.RegisterConcrete(&MsgBindPolicy{}, "policy/MsgBindPolicy", nil)
	cdc.RegisterConcrete(&MsgUnbindPolicy{}, "policy/MsgUnbindPolicy", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "policy/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterPolicy{},
		&MsgUpdatePolicy{},
		&MsgDeprecatePolicy{},
		&MsgDisablePolicy{},
		&MsgBindPolicy{},
		&MsgUnbindPolicy{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
