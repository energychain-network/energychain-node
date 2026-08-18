package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateDenom{}, "stableusd/MsgCreateDenom", nil)
	cdc.RegisterConcrete(&MsgAddMinter{}, "stableusd/MsgAddMinter", nil)
	cdc.RegisterConcrete(&MsgRemoveMinter{}, "stableusd/MsgRemoveMinter", nil)
	cdc.RegisterConcrete(&MsgSetDenomStatus{}, "stableusd/MsgSetDenomStatus", nil)
	cdc.RegisterConcrete(&MsgBindReserve{}, "stableusd/MsgBindReserve", nil)
	cdc.RegisterConcrete(&MsgMint{}, "stableusd/MsgMint", nil)
	cdc.RegisterConcrete(&MsgBurn{}, "stableusd/MsgBurn", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "stableusd/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgApprove{}, "stableusd/MsgApprove", nil)
	cdc.RegisterConcrete(&MsgTransferFrom{}, "stableusd/MsgTransferFrom", nil)
	cdc.RegisterConcrete(&MsgFreeze{}, "stableusd/MsgFreeze", nil)
	cdc.RegisterConcrete(&MsgUnfreeze{}, "stableusd/MsgUnfreeze", nil)
	cdc.RegisterConcrete(&MsgBlacklist{}, "stableusd/MsgBlacklist", nil)
	cdc.RegisterConcrete(&MsgUnblacklist{}, "stableusd/MsgUnblacklist", nil)
	cdc.RegisterConcrete(&MsgForceTransfer{}, "stableusd/MsgForceTransfer", nil)
	cdc.RegisterConcrete(&MsgRequestRedemption{}, "stableusd/MsgRequestRedemption", nil)
	cdc.RegisterConcrete(&MsgSettleRedemption{}, "stableusd/MsgSettleRedemption", nil)
	cdc.RegisterConcrete(&MsgCancelRedemption{}, "stableusd/MsgCancelRedemption", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "stableusd/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateDenom{},
		&MsgAddMinter{},
		&MsgRemoveMinter{},
		&MsgSetDenomStatus{},
		&MsgBindReserve{},
		&MsgMint{},
		&MsgBurn{},
		&MsgTransfer{},
		&MsgApprove{},
		&MsgTransferFrom{},
		&MsgFreeze{},
		&MsgUnfreeze{},
		&MsgBlacklist{},
		&MsgUnblacklist{},
		&MsgForceTransfer{},
		&MsgRequestRedemption{},
		&MsgSettleRedemption{},
		&MsgCancelRedemption{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
