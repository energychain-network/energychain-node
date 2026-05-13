package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterIssuer{}, "rwa/MsgRegisterIssuer", nil)
	cdc.RegisterConcrete(&MsgUpdateIssuer{}, "rwa/MsgUpdateIssuer", nil)
	cdc.RegisterConcrete(&MsgSuspendIssuer{}, "rwa/MsgSuspendIssuer", nil)
	cdc.RegisterConcrete(&MsgRevokeIssuer{}, "rwa/MsgRevokeIssuer", nil)

	cdc.RegisterConcrete(&MsgCreateToken{}, "rwa/MsgCreateToken", nil)
	cdc.RegisterConcrete(&MsgUpdateToken{}, "rwa/MsgUpdateToken", nil)
	cdc.RegisterConcrete(&MsgPauseToken{}, "rwa/MsgPauseToken", nil)
	cdc.RegisterConcrete(&MsgUnpauseToken{}, "rwa/MsgUnpauseToken", nil)
	cdc.RegisterConcrete(&MsgTerminateToken{}, "rwa/MsgTerminateToken", nil)

	cdc.RegisterConcrete(&MsgSetAccountFlags{}, "rwa/MsgSetAccountFlags", nil)

	cdc.RegisterConcrete(&MsgMint{}, "rwa/MsgMint", nil)
	cdc.RegisterConcrete(&MsgBurn{}, "rwa/MsgBurn", nil)

	cdc.RegisterConcrete(&MsgTransfer{}, "rwa/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgForceTransfer{}, "rwa/MsgForceTransfer", nil)

	cdc.RegisterConcrete(&MsgSetFrozenBalance{}, "rwa/MsgSetFrozenBalance", nil)
	cdc.RegisterConcrete(&MsgAddLockup{}, "rwa/MsgAddLockup", nil)

	cdc.RegisterConcrete(&MsgTakeSnapshot{}, "rwa/MsgTakeSnapshot", nil)
	cdc.RegisterConcrete(&MsgCreateDistribution{}, "rwa/MsgCreateDistribution", nil)
	cdc.RegisterConcrete(&MsgFundDistribution{}, "rwa/MsgFundDistribution", nil)
	cdc.RegisterConcrete(&MsgClaimDistribution{}, "rwa/MsgClaimDistribution", nil)
	cdc.RegisterConcrete(&MsgFinalizeDistribution{}, "rwa/MsgFinalizeDistribution", nil)

	cdc.RegisterConcrete(&MsgRequestRedemption{}, "rwa/MsgRequestRedemption", nil)
	cdc.RegisterConcrete(&MsgSettleRedemption{}, "rwa/MsgSettleRedemption", nil)
	cdc.RegisterConcrete(&MsgCancelRedemption{}, "rwa/MsgCancelRedemption", nil)

	cdc.RegisterConcrete(&MsgUpdateParams{}, "rwa/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterIssuer{},
		&MsgUpdateIssuer{},
		&MsgSuspendIssuer{},
		&MsgRevokeIssuer{},
		&MsgCreateToken{},
		&MsgUpdateToken{},
		&MsgPauseToken{},
		&MsgUnpauseToken{},
		&MsgTerminateToken{},
		&MsgSetAccountFlags{},
		&MsgMint{},
		&MsgBurn{},
		&MsgTransfer{},
		&MsgForceTransfer{},
		&MsgSetFrozenBalance{},
		&MsgAddLockup{},
		&MsgTakeSnapshot{},
		&MsgCreateDistribution{},
		&MsgFundDistribution{},
		&MsgClaimDistribution{},
		&MsgFinalizeDistribution{},
		&MsgRequestRedemption{},
		&MsgSettleRedemption{},
		&MsgCancelRedemption{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
