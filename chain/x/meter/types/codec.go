package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterMeteringPoint{}, "meter/MsgRegisterMeteringPoint", nil)
	cdc.RegisterConcrete(&MsgUpdateMeteringPoint{}, "meter/MsgUpdateMeteringPoint", nil)
	cdc.RegisterConcrete(&MsgDeactivateMeteringPoint{}, "meter/MsgDeactivateMeteringPoint", nil)
	cdc.RegisterConcrete(&MsgSubmitReading{}, "meter/MsgSubmitReading", nil)
	cdc.RegisterConcrete(&MsgSubmitBatch{}, "meter/MsgSubmitBatch", nil)
	cdc.RegisterConcrete(&MsgAuthorizeStream{}, "meter/MsgAuthorizeStream", nil)
	cdc.RegisterConcrete(&MsgRevokeStream{}, "meter/MsgRevokeStream", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "meter/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterMeteringPoint{},
		&MsgUpdateMeteringPoint{},
		&MsgDeactivateMeteringPoint{},
		&MsgSubmitReading{},
		&MsgSubmitBatch{},
		&MsgAuthorizeStream{},
		&MsgRevokeStream{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
