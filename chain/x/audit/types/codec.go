package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRecordAudit{}, "audit/MsgRecordAudit", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "audit/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgRegisterSchema{}, "audit/MsgRegisterSchema", nil)
	cdc.RegisterConcrete(&MsgDeprecateSchema{}, "audit/MsgDeprecateSchema", nil)
	cdc.RegisterConcrete(&MsgArchive{}, "audit/MsgArchive", nil)
	cdc.RegisterConcrete(&MsgGrantViewKey{}, "audit/MsgGrantViewKey", nil)
	cdc.RegisterConcrete(&MsgRevokeViewKey{}, "audit/MsgRevokeViewKey", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRecordAudit{},
		&MsgUpdateParams{},
		&MsgRegisterSchema{},
		&MsgDeprecateSchema{},
		&MsgArchive{},
		&MsgGrantViewKey{},
		&MsgRevokeViewKey{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
