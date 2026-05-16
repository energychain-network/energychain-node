package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateJob{}, "scheduler/MsgCreateJob", nil)
	cdc.RegisterConcrete(&MsgTopUpJob{}, "scheduler/MsgTopUpJob", nil)
	cdc.RegisterConcrete(&MsgWithdrawJob{}, "scheduler/MsgWithdrawJob", nil)
	cdc.RegisterConcrete(&MsgPauseJob{}, "scheduler/MsgPauseJob", nil)
	cdc.RegisterConcrete(&MsgResumeJob{}, "scheduler/MsgResumeJob", nil)
	cdc.RegisterConcrete(&MsgCancelJob{}, "scheduler/MsgCancelJob", nil)
	cdc.RegisterConcrete(&MsgUpdateJob{}, "scheduler/MsgUpdateJob", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "scheduler/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateJob{},
		&MsgTopUpJob{},
		&MsgWithdrawJob{},
		&MsgPauseJob{},
		&MsgResumeJob{},
		&MsgCancelJob{},
		&MsgUpdateJob{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
