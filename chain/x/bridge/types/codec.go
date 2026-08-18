package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterChain{}, "bridge/MsgRegisterChain", nil)
	cdc.RegisterConcrete(&MsgUpdateChain{}, "bridge/MsgUpdateChain", nil)
	cdc.RegisterConcrete(&MsgSetChainStatus{}, "bridge/MsgSetChainStatus", nil)
	cdc.RegisterConcrete(&MsgRegisterAsset{}, "bridge/MsgRegisterAsset", nil)
	cdc.RegisterConcrete(&MsgSetAssetStatus{}, "bridge/MsgSetAssetStatus", nil)
	cdc.RegisterConcrete(&MsgLock{}, "bridge/MsgLock", nil)
	cdc.RegisterConcrete(&MsgAttest{}, "bridge/MsgAttest", nil)
	cdc.RegisterConcrete(&MsgRelease{}, "bridge/MsgRelease", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "bridge/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgRegisterChain{},
		&MsgUpdateChain{},
		&MsgSetChainStatus{},
		&MsgRegisterAsset{},
		&MsgSetAssetStatus{},
		&MsgLock{},
		&MsgAttest{},
		&MsgRelease{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
