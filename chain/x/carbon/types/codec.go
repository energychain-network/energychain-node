package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterIssuer{}, "carbon/MsgRegisterIssuer", nil)
	cdc.RegisterConcrete(&MsgUpdateIssuer{}, "carbon/MsgUpdateIssuer", nil)
	cdc.RegisterConcrete(&MsgSuspendIssuer{}, "carbon/MsgSuspendIssuer", nil)
	cdc.RegisterConcrete(&MsgRevokeIssuer{}, "carbon/MsgRevokeIssuer", nil)
	cdc.RegisterConcrete(&MsgIssueAllowance{}, "carbon/MsgIssueAllowance", nil)
	cdc.RegisterConcrete(&MsgIssueOffset{}, "carbon/MsgIssueOffset", nil)
	cdc.RegisterConcrete(&MsgSealAsset{}, "carbon/MsgSealAsset", nil)
	cdc.RegisterConcrete(&MsgUnsealAsset{}, "carbon/MsgUnsealAsset", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "carbon/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgRetire{}, "carbon/MsgRetire", nil)
	cdc.RegisterConcrete(&MsgSetArticle6Status{}, "carbon/MsgSetArticle6Status", nil)
	cdc.RegisterConcrete(&MsgSetBridgeAttestation{}, "carbon/MsgSetBridgeAttestation", nil)
	cdc.RegisterConcrete(&MsgBridgeMint{}, "carbon/MsgBridgeMint", nil)
	cdc.RegisterConcrete(&MsgBridgeBurn{}, "carbon/MsgBridgeBurn", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "carbon/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterIssuer{},
		&MsgUpdateIssuer{},
		&MsgSuspendIssuer{},
		&MsgRevokeIssuer{},
		&MsgIssueAllowance{},
		&MsgIssueOffset{},
		&MsgSealAsset{},
		&MsgUnsealAsset{},
		&MsgTransfer{},
		&MsgRetire{},
		&MsgSetArticle6Status{},
		&MsgSetBridgeAttestation{},
		&MsgBridgeMint{},
		&MsgBridgeBurn{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
