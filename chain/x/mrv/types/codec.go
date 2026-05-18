package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterSchema{}, "mrv/RegisterSchema", nil)
	cdc.RegisterConcrete(&MsgUpdateSchemaStatus{}, "mrv/UpdateSchemaStatus", nil)
	cdc.RegisterConcrete(&MsgRegisterVerifier{}, "mrv/RegisterVerifier", nil)
	cdc.RegisterConcrete(&MsgUpdateVerifierStatus{}, "mrv/UpdateVerifierStatus", nil)
	cdc.RegisterConcrete(&MsgSubmitReport{}, "mrv/SubmitReport", nil)
	cdc.RegisterConcrete(&MsgAttestReport{}, "mrv/AttestReport", nil)
	cdc.RegisterConcrete(&MsgRejectReport{}, "mrv/RejectReport", nil)
	cdc.RegisterConcrete(&MsgRetractReport{}, "mrv/RetractReport", nil)
	cdc.RegisterConcrete(&MsgGrantViewKey{}, "mrv/GrantViewKey", nil)
	cdc.RegisterConcrete(&MsgRevokeViewKey{}, "mrv/RevokeViewKey", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "mrv/UpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterSchema{}, &MsgUpdateSchemaStatus{},
		&MsgRegisterVerifier{}, &MsgUpdateVerifierStatus{},
		&MsgSubmitReport{}, &MsgAttestReport{},
		&MsgRejectReport{}, &MsgRetractReport{},
		&MsgGrantViewKey{}, &MsgRevokeViewKey{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
