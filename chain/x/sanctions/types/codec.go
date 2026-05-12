package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgRegisterList{}, "sanctions/MsgRegisterList", nil)
	cdc.RegisterConcrete(&MsgUpdateList{}, "sanctions/MsgUpdateList", nil)
	cdc.RegisterConcrete(&MsgDeprecateList{}, "sanctions/MsgDeprecateList", nil)
	cdc.RegisterConcrete(&MsgArchiveList{}, "sanctions/MsgArchiveList", nil)
	cdc.RegisterConcrete(&MsgAddEntry{}, "sanctions/MsgAddEntry", nil)
	cdc.RegisterConcrete(&MsgRemoveEntry{}, "sanctions/MsgRemoveEntry", nil)
	cdc.RegisterConcrete(&MsgProposeDelta{}, "sanctions/MsgProposeDelta", nil)
	cdc.RegisterConcrete(&MsgConfirmProposal{}, "sanctions/MsgConfirmProposal", nil)
	cdc.RegisterConcrete(&MsgRejectProposal{}, "sanctions/MsgRejectProposal", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "sanctions/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgRegisterList{},
		&MsgUpdateList{},
		&MsgDeprecateList{},
		&MsgArchiveList{},
		&MsgAddEntry{},
		&MsgRemoveEntry{},
		&MsgProposeDelta{},
		&MsgConfirmProposal{},
		&MsgRejectProposal{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
