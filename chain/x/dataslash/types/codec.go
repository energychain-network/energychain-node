package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterProvider{}, "dataslash/RegisterProvider", nil)
	cdc.RegisterConcrete(&MsgUpdateProviderInfo{}, "dataslash/UpdateProviderInfo", nil)
	cdc.RegisterConcrete(&MsgPostBond{}, "dataslash/PostBond", nil)
	cdc.RegisterConcrete(&MsgIncreaseBond{}, "dataslash/IncreaseBond", nil)
	cdc.RegisterConcrete(&MsgRequestUnbond{}, "dataslash/RequestUnbond", nil)
	cdc.RegisterConcrete(&MsgWithdrawUnbonded{}, "dataslash/WithdrawUnbonded", nil)
	cdc.RegisterConcrete(&MsgReportInfraction{}, "dataslash/ReportInfraction", nil)
	cdc.RegisterConcrete(&MsgJailProvider{}, "dataslash/JailProvider", nil)
	cdc.RegisterConcrete(&MsgUnjailProvider{}, "dataslash/UnjailProvider", nil)
	cdc.RegisterConcrete(&MsgBanProvider{}, "dataslash/BanProvider", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "dataslash/UpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterProvider{}, &MsgUpdateProviderInfo{},
		&MsgPostBond{}, &MsgIncreaseBond{},
		&MsgRequestUnbond{}, &MsgWithdrawUnbonded{},
		&MsgReportInfraction{},
		&MsgJailProvider{}, &MsgUnjailProvider{}, &MsgBanProvider{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
