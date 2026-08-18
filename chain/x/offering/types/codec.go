package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateOffering{}, "offering/MsgCreateOffering", nil)
	cdc.RegisterConcrete(&MsgSubscribe{}, "offering/MsgSubscribe", nil)
	cdc.RegisterConcrete(&MsgCancelOffering{}, "offering/MsgCancelOffering", nil)
	cdc.RegisterConcrete(&MsgCloseOffering{}, "offering/MsgCloseOffering", nil)
	cdc.RegisterConcrete(&MsgClaimAllocation{}, "offering/MsgClaimAllocation", nil)
	cdc.RegisterConcrete(&MsgInjectReturn{}, "offering/MsgInjectReturn", nil)
	cdc.RegisterConcrete(&MsgReleaseTranche{}, "offering/MsgReleaseTranche", nil)
	cdc.RegisterConcrete(&MsgClaimReturns{}, "offering/MsgClaimReturns", nil)
	cdc.RegisterConcrete(&MsgFlagDefault{}, "offering/MsgFlagDefault", nil)
	cdc.RegisterConcrete(&MsgClaimRefund{}, "offering/MsgClaimRefund", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "offering/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateOffering{},
		&MsgSubscribe{},
		&MsgCancelOffering{},
		&MsgCloseOffering{},
		&MsgClaimAllocation{},
		&MsgInjectReturn{},
		&MsgReleaseTranche{},
		&MsgClaimReturns{},
		&MsgFlagDefault{},
		&MsgClaimRefund{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
