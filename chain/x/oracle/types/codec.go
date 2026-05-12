package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterTopic{}, "oracle/MsgRegisterTopic", nil)
	cdc.RegisterConcrete(&MsgUpdateTopic{}, "oracle/MsgUpdateTopic", nil)
	cdc.RegisterConcrete(&MsgPauseTopic{}, "oracle/MsgPauseTopic", nil)
	cdc.RegisterConcrete(&MsgResumeTopic{}, "oracle/MsgResumeTopic", nil)
	cdc.RegisterConcrete(&MsgRegisterProvider{}, "oracle/MsgRegisterProvider", nil)
	cdc.RegisterConcrete(&MsgTopUpBond{}, "oracle/MsgTopUpBond", nil)
	cdc.RegisterConcrete(&MsgRequestWithdrawBond{}, "oracle/MsgRequestWithdrawBond", nil)
	cdc.RegisterConcrete(&MsgWithdrawBond{}, "oracle/MsgWithdrawBond", nil)
	cdc.RegisterConcrete(&MsgSuspendProvider{}, "oracle/MsgSuspendProvider", nil)
	cdc.RegisterConcrete(&MsgSubmitValue{}, "oracle/MsgSubmitValue", nil)
	cdc.RegisterConcrete(&MsgSubmitReserveAttestation{}, "oracle/MsgSubmitReserveAttestation", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "oracle/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterTopic{},
		&MsgUpdateTopic{},
		&MsgPauseTopic{},
		&MsgResumeTopic{},
		&MsgRegisterProvider{},
		&MsgTopUpBond{},
		&MsgRequestWithdrawBond{},
		&MsgWithdrawBond{},
		&MsgSuspendProvider{},
		&MsgSubmitValue{},
		&MsgSubmitReserveAttestation{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
