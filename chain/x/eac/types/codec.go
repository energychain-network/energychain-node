package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterIssuer{}, "eac/MsgRegisterIssuer", nil)
	cdc.RegisterConcrete(&MsgUpdateIssuer{}, "eac/MsgUpdateIssuer", nil)
	cdc.RegisterConcrete(&MsgSuspendIssuer{}, "eac/MsgSuspendIssuer", nil)
	cdc.RegisterConcrete(&MsgRevokeIssuer{}, "eac/MsgRevokeIssuer", nil)
	cdc.RegisterConcrete(&MsgIssueBatch{}, "eac/MsgIssueBatch", nil)
	cdc.RegisterConcrete(&MsgSealCertificate{}, "eac/MsgSealCertificate", nil)
	cdc.RegisterConcrete(&MsgUnsealCertificate{}, "eac/MsgUnsealCertificate", nil)
	cdc.RegisterConcrete(&MsgTransfer{}, "eac/MsgTransfer", nil)
	cdc.RegisterConcrete(&MsgRetire{}, "eac/MsgRetire", nil)
	cdc.RegisterConcrete(&MsgSetBridgeAttestation{}, "eac/MsgSetBridgeAttestation", nil)
	cdc.RegisterConcrete(&MsgBridgeMint{}, "eac/MsgBridgeMint", nil)
	cdc.RegisterConcrete(&MsgBridgeBurn{}, "eac/MsgBridgeBurn", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "eac/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterIssuer{},
		&MsgUpdateIssuer{},
		&MsgSuspendIssuer{},
		&MsgRevokeIssuer{},
		&MsgIssueBatch{},
		&MsgSealCertificate{},
		&MsgUnsealCertificate{},
		&MsgTransfer{},
		&MsgRetire{},
		&MsgSetBridgeAttestation{},
		&MsgBridgeMint{},
		&MsgBridgeBurn{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
