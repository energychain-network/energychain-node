package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

// RegisterCodec registers the legacy Amino concrete types for txs.
func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgSubmitEnergyData{}, "energy/MsgSubmitEnergyData", nil)
	cdc.RegisterConcrete(&MsgBatchSubmit{}, "energy/MsgBatchSubmit", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "energy/MsgUpdateParams", nil)
}

// RegisterInterfaces registers the energy module's proto Msg implementations
// into the SDK's interface registry so the Tx decoder can route them.
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgSubmitEnergyData{},
		&MsgBatchSubmit{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
