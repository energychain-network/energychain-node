package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreatePair{}, "market/MsgCreatePair", nil)
	cdc.RegisterConcrete(&MsgPauseUnpausePair{}, "market/MsgPauseUnpausePair", nil)
	cdc.RegisterConcrete(&MsgUpdatePairRisk{}, "market/MsgUpdatePairRisk", nil)
	cdc.RegisterConcrete(&MsgPlaceLimitOrder{}, "market/MsgPlaceLimitOrder", nil)
	cdc.RegisterConcrete(&MsgCancelOrder{}, "market/MsgCancelOrder", nil)
	cdc.RegisterConcrete(&MsgClearBatch{}, "market/MsgClearBatch", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "market/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreatePair{},
		&MsgPauseUnpausePair{},
		&MsgUpdatePairRisk{},
		&MsgPlaceLimitOrder{},
		&MsgCancelOrder{},
		&MsgClearBatch{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
