package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterProvider{}, "assethub/MsgRegisterProvider", nil)
	cdc.RegisterConcrete(&MsgIncreaseBond{}, "assethub/MsgIncreaseBond", nil)
	cdc.RegisterConcrete(&MsgWithdrawBond{}, "assethub/MsgWithdrawBond", nil)
	cdc.RegisterConcrete(&MsgDeregisterProvider{}, "assethub/MsgDeregisterProvider", nil)
	cdc.RegisterConcrete(&MsgSlashProvider{}, "assethub/MsgSlashProvider", nil)
	cdc.RegisterConcrete(&MsgUnjailProvider{}, "assethub/MsgUnjailProvider", nil)
	cdc.RegisterConcrete(&MsgRegisterDevice{}, "assethub/MsgRegisterDevice", nil)
	cdc.RegisterConcrete(&MsgUpdateDevice{}, "assethub/MsgUpdateDevice", nil)
	cdc.RegisterConcrete(&MsgAttestDevice{}, "assethub/MsgAttestDevice", nil)
	cdc.RegisterConcrete(&MsgRevokeDevice{}, "assethub/MsgRevokeDevice", nil)
	cdc.RegisterConcrete(&MsgSubmitReading{}, "assethub/MsgSubmitReading", nil)
	cdc.RegisterConcrete(&MsgCreateTopic{}, "assethub/MsgCreateTopic", nil)
	cdc.RegisterConcrete(&MsgSubmitValue{}, "assethub/MsgSubmitValue", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "assethub/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterProvider{},
		&MsgIncreaseBond{},
		&MsgWithdrawBond{},
		&MsgDeregisterProvider{},
		&MsgSlashProvider{},
		&MsgUnjailProvider{},
		&MsgRegisterDevice{},
		&MsgUpdateDevice{},
		&MsgAttestDevice{},
		&MsgRevokeDevice{},
		&MsgSubmitReading{},
		&MsgCreateTopic{},
		&MsgSubmitValue{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
