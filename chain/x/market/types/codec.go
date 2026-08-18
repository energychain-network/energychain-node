package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateMarket{}, "market/MsgCreateMarket", nil)
	cdc.RegisterConcrete(&MsgSetMarketStatus{}, "market/MsgSetMarketStatus", nil)
	cdc.RegisterConcrete(&MsgPlaceOrder{}, "market/MsgPlaceOrder", nil)
	cdc.RegisterConcrete(&MsgCancelOrder{}, "market/MsgCancelOrder", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "market/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgPostBond{}, "market/MsgPostBond", nil)
	cdc.RegisterConcrete(&MsgDelistMarket{}, "market/MsgDelistMarket", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateMarket{},
		&MsgSetMarketStatus{},
		&MsgPlaceOrder{},
		&MsgCancelOrder{},
		&MsgUpdateParams{},
		&MsgPostBond{},
		&MsgDelistMarket{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
