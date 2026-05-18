package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateStream{}, "streampay/MsgCreateStream", nil)
	cdc.RegisterConcrete(&MsgDepositToStream{}, "streampay/MsgDepositToStream", nil)
	cdc.RegisterConcrete(&MsgWithdraw{}, "streampay/MsgWithdraw", nil)
	cdc.RegisterConcrete(&MsgPause{}, "streampay/MsgPause", nil)
	cdc.RegisterConcrete(&MsgResume{}, "streampay/MsgResume", nil)
	cdc.RegisterConcrete(&MsgCancel{}, "streampay/MsgCancel", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "streampay/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgChangeRate{}, "streampay/MsgChangeRate", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "streampay/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateStream{},
		&MsgDepositToStream{},
		&MsgWithdraw{},
		&MsgPause{},
		&MsgResume{},
		&MsgCancel{},
		&MsgTransfer{},
		&MsgChangeRate{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
