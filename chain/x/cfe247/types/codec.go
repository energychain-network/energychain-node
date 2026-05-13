package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterDataProvider{}, "cfe247/MsgRegisterDataProvider", nil)
	cdc.RegisterConcrete(&MsgUpdateDataProvider{}, "cfe247/MsgUpdateDataProvider", nil)
	cdc.RegisterConcrete(&MsgSuspendDataProvider{}, "cfe247/MsgSuspendDataProvider", nil)
	cdc.RegisterConcrete(&MsgRevokeDataProvider{}, "cfe247/MsgRevokeDataProvider", nil)
	cdc.RegisterConcrete(&MsgRegisterGridZone{}, "cfe247/MsgRegisterGridZone", nil)
	cdc.RegisterConcrete(&MsgUpdateGridZone{}, "cfe247/MsgUpdateGridZone", nil)
	cdc.RegisterConcrete(&MsgRemoveGridZone{}, "cfe247/MsgRemoveGridZone", nil)
	cdc.RegisterConcrete(&MsgRegisterSubject{}, "cfe247/MsgRegisterSubject", nil)
	cdc.RegisterConcrete(&MsgUpdateSubject{}, "cfe247/MsgUpdateSubject", nil)
	cdc.RegisterConcrete(&MsgDeactivateSubject{}, "cfe247/MsgDeactivateSubject", nil)
	cdc.RegisterConcrete(&MsgAttestHourlyConsumption{}, "cfe247/MsgAttestHourlyConsumption", nil)
	cdc.RegisterConcrete(&MsgAttestHourlyConsumptionBatch{}, "cfe247/MsgAttestHourlyConsumptionBatch", nil)
	cdc.RegisterConcrete(&MsgAllocateMatch{}, "cfe247/MsgAllocateMatch", nil)
	cdc.RegisterConcrete(&MsgGenerateReport{}, "cfe247/MsgGenerateReport", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "cfe247/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterDataProvider{},
		&MsgUpdateDataProvider{},
		&MsgSuspendDataProvider{},
		&MsgRevokeDataProvider{},
		&MsgRegisterGridZone{},
		&MsgUpdateGridZone{},
		&MsgRemoveGridZone{},
		&MsgRegisterSubject{},
		&MsgUpdateSubject{},
		&MsgDeactivateSubject{},
		&MsgAttestHourlyConsumption{},
		&MsgAttestHourlyConsumptionBatch{},
		&MsgAllocateMatch{},
		&MsgGenerateReport{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
