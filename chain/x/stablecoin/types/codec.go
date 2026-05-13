package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterDenom{}, "stablecoin/MsgRegisterDenom", nil)
	cdc.RegisterConcrete(&MsgUpdateDenom{}, "stablecoin/MsgUpdateDenom", nil)
	cdc.RegisterConcrete(&MsgPauseDenom{}, "stablecoin/MsgPauseDenom", nil)
	cdc.RegisterConcrete(&MsgResumeDenom{}, "stablecoin/MsgResumeDenom", nil)
	cdc.RegisterConcrete(&MsgRetireDenom{}, "stablecoin/MsgRetireDenom", nil)

	cdc.RegisterConcrete(&MsgRegisterIssuer{}, "stablecoin/MsgRegisterIssuer", nil)
	cdc.RegisterConcrete(&MsgUpdateIssuer{}, "stablecoin/MsgUpdateIssuer", nil)
	cdc.RegisterConcrete(&MsgSuspendIssuer{}, "stablecoin/MsgSuspendIssuer", nil)
	cdc.RegisterConcrete(&MsgRevokeIssuer{}, "stablecoin/MsgRevokeIssuer", nil)

	cdc.RegisterConcrete(&MsgSetMintQuota{}, "stablecoin/MsgSetMintQuota", nil)
	cdc.RegisterConcrete(&MsgSetMintPaused{}, "stablecoin/MsgSetMintPaused", nil)
	cdc.RegisterConcrete(&MsgSetReserveRequirement{}, "stablecoin/MsgSetReserveRequirement", nil)

	cdc.RegisterConcrete(&MsgMint{}, "stablecoin/MsgMint", nil)
	cdc.RegisterConcrete(&MsgBurn{}, "stablecoin/MsgBurn", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "stablecoin/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgApprove{}, "stablecoin/MsgApprove", nil)
	cdc.RegisterConcrete(&MsgTransferFrom{}, "stablecoin/MsgTransferFrom", nil)

	cdc.RegisterConcrete(&MsgFreeze{}, "stablecoin/MsgFreeze", nil)
	cdc.RegisterConcrete(&MsgUnfreeze{}, "stablecoin/MsgUnfreeze", nil)
	cdc.RegisterConcrete(&MsgBlacklist{}, "stablecoin/MsgBlacklist", nil)
	cdc.RegisterConcrete(&MsgUnblacklist{}, "stablecoin/MsgUnblacklist", nil)
	cdc.RegisterConcrete(&MsgForceTransfer{}, "stablecoin/MsgForceTransfer", nil)

	cdc.RegisterConcrete(&MsgRequestRedemption{}, "stablecoin/MsgRequestRedemption", nil)
	cdc.RegisterConcrete(&MsgFulfillRedemption{}, "stablecoin/MsgFulfillRedemption", nil)
	cdc.RegisterConcrete(&MsgCancelRedemption{}, "stablecoin/MsgCancelRedemption", nil)

	cdc.RegisterConcrete(&MsgUpdateParams{}, "stablecoin/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterDenom{},
		&MsgUpdateDenom{},
		&MsgPauseDenom{},
		&MsgResumeDenom{},
		&MsgRetireDenom{},
		&MsgRegisterIssuer{},
		&MsgUpdateIssuer{},
		&MsgSuspendIssuer{},
		&MsgRevokeIssuer{},
		&MsgSetMintQuota{},
		&MsgSetMintPaused{},
		&MsgSetReserveRequirement{},
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
		&MsgFulfillRedemption{},
		&MsgCancelRedemption{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
