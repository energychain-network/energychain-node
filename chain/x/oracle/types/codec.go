package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgSubmitData{}, "oracle/MsgSubmitData", nil)
	cdc.RegisterConcrete(&MsgAddOracle{}, "oracle/MsgAddOracle", nil)
	cdc.RegisterConcrete(&MsgRemoveOracle{}, "oracle/MsgRemoveOracle", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "oracle/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgSubmitData{},
		&MsgAddOracle{},
		&MsgRemoveOracle{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
