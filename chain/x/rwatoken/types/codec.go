package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateToken{}, "rwatoken/MsgCreateToken", nil)
	cdc.RegisterConcrete(&MsgUpdateToken{}, "rwatoken/MsgUpdateToken", nil)
	cdc.RegisterConcrete(&MsgSetTokenStatus{}, "rwatoken/MsgSetTokenStatus", nil)
	cdc.RegisterConcrete(&MsgMint{}, "rwatoken/MsgMint", nil)
	cdc.RegisterConcrete(&MsgBurn{}, "rwatoken/MsgBurn", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "rwatoken/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgForceTransfer{}, "rwatoken/MsgForceTransfer", nil)
	cdc.RegisterConcrete(&MsgFreeze{}, "rwatoken/MsgFreeze", nil)
	cdc.RegisterConcrete(&MsgUnfreeze{}, "rwatoken/MsgUnfreeze", nil)
	cdc.RegisterConcrete(&MsgTakeSnapshot{}, "rwatoken/MsgTakeSnapshot", nil)
	cdc.RegisterConcrete(&MsgCreateDistribution{}, "rwatoken/MsgCreateDistribution", nil)
	cdc.RegisterConcrete(&MsgClaimDistribution{}, "rwatoken/MsgClaimDistribution", nil)
	cdc.RegisterConcrete(&MsgFundPool{}, "rwatoken/MsgFundPool", nil)
	cdc.RegisterConcrete(&MsgRequestRedemption{}, "rwatoken/MsgRequestRedemption", nil)
	cdc.RegisterConcrete(&MsgExecuteRedemption{}, "rwatoken/MsgExecuteRedemption", nil)
	cdc.RegisterConcrete(&MsgCancelRedemption{}, "rwatoken/MsgCancelRedemption", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "rwatoken/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgAddIssuer{}, "rwatoken/MsgAddIssuer", nil)
	cdc.RegisterConcrete(&MsgRemoveIssuer{}, "rwatoken/MsgRemoveIssuer", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateToken{},
		&MsgUpdateToken{},
		&MsgSetTokenStatus{},
		&MsgMint{},
		&MsgBurn{},
		&MsgTransfer{},
		&MsgForceTransfer{},
		&MsgFreeze{},
		&MsgUnfreeze{},
		&MsgTakeSnapshot{},
		&MsgCreateDistribution{},
		&MsgClaimDistribution{},
		&MsgFundPool{},
		&MsgRequestRedemption{},
		&MsgExecuteRedemption{},
		&MsgCancelRedemption{},
		&MsgUpdateParams{},
		&MsgAddIssuer{},
		&MsgRemoveIssuer{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
