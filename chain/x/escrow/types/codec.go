package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateEscrow{}, "escrow/MsgCreateEscrow", nil)
	cdc.RegisterConcrete(&MsgFund{}, "escrow/MsgFund", nil)
	cdc.RegisterConcrete(&MsgCancel{}, "escrow/MsgCancel", nil)
	cdc.RegisterConcrete(&MsgApprove{}, "escrow/MsgApprove", nil)
	cdc.RegisterConcrete(&MsgRevokeApproval{}, "escrow/MsgRevokeApproval", nil)
	cdc.RegisterConcrete(&MsgRelease{}, "escrow/MsgRelease", nil)
	cdc.RegisterConcrete(&MsgRefund{}, "escrow/MsgRefund", nil)
	cdc.RegisterConcrete(&MsgArbiterRelease{}, "escrow/MsgArbiterRelease", nil)
	cdc.RegisterConcrete(&MsgArbiterRefund{}, "escrow/MsgArbiterRefund", nil)
	cdc.RegisterConcrete(&MsgMarkDisputed{}, "escrow/MsgMarkDisputed", nil)
	cdc.RegisterConcrete(&MsgResolveDispute{}, "escrow/MsgResolveDispute", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "escrow/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateEscrow{},
		&MsgFund{},
		&MsgCancel{},
		&MsgApprove{},
		&MsgRevokeApproval{},
		&MsgRelease{},
		&MsgRefund{},
		&MsgArbiterRelease{},
		&MsgArbiterRefund{},
		&MsgMarkDisputed{},
		&MsgResolveDispute{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
