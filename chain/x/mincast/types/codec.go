package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateMarket{}, "mincast/MsgCreateMarket", nil)
	cdc.RegisterConcrete(&MsgUpdateMarket{}, "mincast/MsgUpdateMarket", nil)
	cdc.RegisterConcrete(&MsgSetMarketStatus{}, "mincast/MsgSetMarketStatus", nil)
	cdc.RegisterConcrete(&MsgMint{}, "mincast/MsgMint", nil)
	cdc.RegisterConcrete(&MsgMelt{}, "mincast/MsgMelt", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "mincast/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgInjectTreasury{}, "mincast/MsgInjectTreasury", nil)
	cdc.RegisterConcrete(&MsgFundReward{}, "mincast/MsgFundReward", nil)
	cdc.RegisterConcrete(&MsgOpenInvest{}, "mincast/MsgOpenInvest", nil)
	cdc.RegisterConcrete(&MsgCloseInvest{}, "mincast/MsgCloseInvest", nil)
	cdc.RegisterConcrete(&MsgCancelInvest{}, "mincast/MsgCancelInvest", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "mincast/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateMarket{},
		&MsgUpdateMarket{},
		&MsgSetMarketStatus{},
		&MsgMint{},
		&MsgMelt{},
		&MsgTransfer{},
		&MsgInjectTreasury{},
		&MsgFundReward{},
		&MsgOpenInvest{},
		&MsgCloseInvest{},
		&MsgCancelInvest{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
