package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateAuction{}, "auction/MsgCreateAuction", nil)
	cdc.RegisterConcrete(&MsgCancelAuction{}, "auction/MsgCancelAuction", nil)
	cdc.RegisterConcrete(&MsgPlaceBid{}, "auction/MsgPlaceBid", nil)
	cdc.RegisterConcrete(&MsgCommitBid{}, "auction/MsgCommitBid", nil)
	cdc.RegisterConcrete(&MsgRevealBid{}, "auction/MsgRevealBid", nil)
	cdc.RegisterConcrete(&MsgClose{}, "auction/MsgClose", nil)
	cdc.RegisterConcrete(&MsgSettle{}, "auction/MsgSettle", nil)
	cdc.RegisterConcrete(&MsgWithdrawRefund{}, "auction/MsgWithdrawRefund", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "auction/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateAuction{},
		&MsgCancelAuction{},
		&MsgPlaceBid{},
		&MsgCommitBid{},
		&MsgRevealBid{},
		&MsgClose{},
		&MsgSettle{},
		&MsgWithdrawRefund{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
