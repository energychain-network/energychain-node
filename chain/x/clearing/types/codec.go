package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterMember{}, "clearing/RegisterMember", nil)
	cdc.RegisterConcrete(&MsgUpdateMemberStatus{}, "clearing/UpdateMemberStatus", nil)
	cdc.RegisterConcrete(&MsgPostMargin{}, "clearing/PostMargin", nil)
	cdc.RegisterConcrete(&MsgWithdrawMargin{}, "clearing/WithdrawMargin", nil)
	cdc.RegisterConcrete(&MsgFundDefaultFund{}, "clearing/FundDefaultFund", nil)
	cdc.RegisterConcrete(&MsgOpenCycle{}, "clearing/OpenCycle", nil)
	cdc.RegisterConcrete(&MsgSubmitObligation{}, "clearing/SubmitObligation", nil)
	cdc.RegisterConcrete(&MsgCloseCycle{}, "clearing/CloseCycle", nil)
	cdc.RegisterConcrete(&MsgSettleCycle{}, "clearing/SettleCycle", nil)
	cdc.RegisterConcrete(&MsgCancelCycle{}, "clearing/CancelCycle", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "clearing/UpdateParams", nil)
}

func RegisterInterfaces(reg cdctypes.InterfaceRegistry) {
	reg.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterMember{},
		&MsgUpdateMemberStatus{},
		&MsgPostMargin{},
		&MsgWithdrawMargin{},
		&MsgFundDefaultFund{},
		&MsgOpenCycle{},
		&MsgSubmitObligation{},
		&MsgCloseCycle{},
		&MsgSettleCycle{},
		&MsgCancelCycle{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(reg, &_Msg_serviceDesc)
}
