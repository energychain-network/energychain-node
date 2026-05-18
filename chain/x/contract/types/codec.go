package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateContract{}, "contract/MsgCreateContract", nil)
	cdc.RegisterConcrete(&MsgDepositMargin{}, "contract/MsgDepositMargin", nil)
	cdc.RegisterConcrete(&MsgWithdrawMargin{}, "contract/MsgWithdrawMargin", nil)
	cdc.RegisterConcrete(&MsgSign{}, "contract/MsgSign", nil)
	cdc.RegisterConcrete(&MsgRevoke{}, "contract/MsgRevoke", nil)
	cdc.RegisterConcrete(&MsgSettle{}, "contract/MsgSettle", nil)
	cdc.RegisterConcrete(&MsgDefault{}, "contract/MsgDefault", nil)
	cdc.RegisterConcrete(&MsgTerminate{}, "contract/MsgTerminate", nil)
	cdc.RegisterConcrete(&MsgMarkDisputed{}, "contract/MsgMarkDisputed", nil)
	cdc.RegisterConcrete(&MsgResolveDispute{}, "contract/MsgResolveDispute", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "contract/MsgUpdateParams", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateContract{},
		&MsgDepositMargin{},
		&MsgWithdrawMargin{},
		&MsgSign{},
		&MsgRevoke{},
		&MsgSettle{},
		&MsgDefault{},
		&MsgTerminate{},
		&MsgMarkDisputed{},
		&MsgResolveDispute{},
		&MsgUpdateParams{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
