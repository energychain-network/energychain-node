package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateSchedule{}, "automation/MsgCreateSchedule", nil)
	cdc.RegisterConcrete(&MsgPauseSchedule{}, "automation/MsgPauseSchedule", nil)
	cdc.RegisterConcrete(&MsgResumeSchedule{}, "automation/MsgResumeSchedule", nil)
	cdc.RegisterConcrete(&MsgCancelSchedule{}, "automation/MsgCancelSchedule", nil)
	cdc.RegisterConcrete(&MsgCreateStream{}, "automation/MsgCreateStream", nil)
	cdc.RegisterConcrete(&MsgWithdrawStream{}, "automation/MsgWithdrawStream", nil)
	cdc.RegisterConcrete(&MsgCancelStream{}, "automation/MsgCancelStream", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "automation/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateSchedule{},
		&MsgPauseSchedule{},
		&MsgResumeSchedule{},
		&MsgCancelSchedule{},
		&MsgCreateStream{},
		&MsgWithdrawStream{},
		&MsgCancelStream{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
