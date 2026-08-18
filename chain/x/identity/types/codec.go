package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgAddRegistrar{}, "identity/MsgAddRegistrar", nil)
	cdc.RegisterConcrete(&MsgRemoveRegistrar{}, "identity/MsgRemoveRegistrar", nil)
	cdc.RegisterConcrete(&MsgSetAccount{}, "identity/MsgSetAccount", nil)
	cdc.RegisterConcrete(&MsgFreezeAccount{}, "identity/MsgFreezeAccount", nil)
	cdc.RegisterConcrete(&MsgUnfreezeAccount{}, "identity/MsgUnfreezeAccount", nil)
	cdc.RegisterConcrete(&MsgAddSanction{}, "identity/MsgAddSanction", nil)
	cdc.RegisterConcrete(&MsgRemoveSanction{}, "identity/MsgRemoveSanction", nil)
	cdc.RegisterConcrete(&MsgCreatePolicy{}, "identity/MsgCreatePolicy", nil)
	cdc.RegisterConcrete(&MsgUpdatePolicy{}, "identity/MsgUpdatePolicy", nil)
	cdc.RegisterConcrete(&MsgPausePolicy{}, "identity/MsgPausePolicy", nil)
	cdc.RegisterConcrete(&MsgDeletePolicy{}, "identity/MsgDeletePolicy", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "identity/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgAddRegistrar{},
		&MsgRemoveRegistrar{},
		&MsgSetAccount{},
		&MsgFreezeAccount{},
		&MsgUnfreezeAccount{},
		&MsgAddSanction{},
		&MsgRemoveSanction{},
		&MsgCreatePolicy{},
		&MsgUpdatePolicy{},
		&MsgPausePolicy{},
		&MsgDeletePolicy{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
