package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterArbitrator{}, "dispute/RegisterArbitrator", nil)
	cdc.RegisterConcrete(&MsgUpdateArbitratorStatus{}, "dispute/UpdateArbitratorStatus", nil)
	cdc.RegisterConcrete(&MsgOpenDispute{}, "dispute/OpenDispute", nil)
	cdc.RegisterConcrete(&MsgRespond{}, "dispute/Respond", nil)
	cdc.RegisterConcrete(&MsgSubmitEvidence{}, "dispute/SubmitEvidence", nil)
	cdc.RegisterConcrete(&MsgAssignTribunal{}, "dispute/AssignTribunal", nil)
	cdc.RegisterConcrete(&MsgCastVote{}, "dispute/CastVote", nil)
	cdc.RegisterConcrete(&MsgFinalizeRuling{}, "dispute/FinalizeRuling", nil)
	cdc.RegisterConcrete(&MsgCancelDispute{}, "dispute/CancelDispute", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "dispute/UpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterArbitrator{}, &MsgUpdateArbitratorStatus{},
		&MsgOpenDispute{}, &MsgRespond{}, &MsgSubmitEvidence{},
		&MsgAssignTribunal{}, &MsgCastVote{}, &MsgFinalizeRuling{},
		&MsgCancelDispute{}, &MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
