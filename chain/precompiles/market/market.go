// Package market implements the EVM precompile for the x/market
// order-book module. The precompile is the trader-facing surface:
// place / cancel limit orders + read order/position state. Admin
// surfaces (CreatePair, UpdatePairRisk, PauseUnpausePair,
// ClearBatch, UpdateParams) remain native-Msg and governance-gated.
package market

import (
	"bytes"
	"context"
	"fmt"

	_ "embed"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	marketkeeper "energychain/x/market/keeper"
	markettypes "energychain/x/market/types"
)

// PrecompileAddressHex / PrecompileAddress are the market precompile
// slot in the reserved 0x900 EnergyChain native range.
const PrecompileAddressHex = "0x0000000000000000000000000000000000000903"

var PrecompileAddress = common.HexToAddress(PrecompileAddressHex)

const (
	PlaceLimitOrderMethod = "placeLimitOrder"
	CancelOrderMethod     = "cancelOrder"
	GetOrderMethod        = "getOrder"
	GetPositionMethod     = "getPosition"

	LimitOrderPlacedEventName = "LimitOrderPlaced"
	OrderCancelledEventName   = "OrderCancelled"
)

// Gas costs. Place/Cancel involve matching + balance moves so
// they're heavier than the simple stablecoin transfer; getOrder /
// getPosition are single keeper reads.
const (
	GasPlaceLimitOrder uint64 = 80_000
	GasCancelOrder     uint64 = 40_000
	GasGetOrder        uint64 = 4_000
	GasGetPosition     uint64 = 4_000
)

//go:embed abi.json
var abiJSON []byte

var ABI abi.ABI

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("market precompile: ABI parse failed: %w", err))
	}
}

type KeeperAPI interface {
	GetOrder(ctx context.Context, id uint64) (markettypes.Order, bool, error)
	GetPosition(ctx context.Context, pairID uint64, owner string) (markettypes.Position, error)
}

type MsgServerAPI interface {
	PlaceLimitOrder(ctx sdk.Context, msg *markettypes.MsgPlaceLimitOrder) (orderID, filledQty, remainingQty uint64, err error)
	CancelOrder(ctx sdk.Context, msg *markettypes.MsgCancelOrder) (refundedAmount uint64, err error)
}

type Precompile struct {
	cmn.Precompile
	abi.ABI

	keeper KeeperAPI
	msgs   MsgServerAPI
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

func NewPrecompile(k marketkeeper.Keeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      PrecompileAddress,
		},
		ABI:    ABI,
		keeper: keeperAdapter{k: k},
		msgs:   msgServerAdapter{srv: marketkeeper.NewMsgServerImpl(k)},
	}
}

func NewPrecompileWithAPI(k KeeperAPI, s MsgServerAPI) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      PrecompileAddress,
		},
		ABI:    ABI,
		keeper: k,
		msgs:   s,
	}
}

func (p Precompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}
	method, err := p.MethodById(input[:4])
	if err != nil {
		return 0
	}
	switch method.Name {
	case PlaceLimitOrderMethod:
		return GasPlaceLimitOrder
	case CancelOrderMethod:
		return GasCancelOrder
	case GetOrderMethod:
		return GasGetOrder
	case GetPositionMethod:
		return GasGetPosition
	}
	return 0
}

func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case PlaceLimitOrderMethod, CancelOrderMethod:
		return true
	}
	return false
}

func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm, contract, readonly)
	})
}

func (p Precompile) Execute(ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}
	if p.keeper == nil {
		return nil, fmt.Errorf("market precompile not bound to keeper")
	}
	switch method.Name {
	case PlaceLimitOrderMethod:
		return p.PlaceLimitOrder(ctx, evm, contract, method, args)
	case CancelOrderMethod:
		return p.CancelOrder(ctx, evm, contract, method, args)
	case GetOrderMethod:
		return p.GetOrder(ctx, method, args)
	case GetPositionMethod:
		return p.GetPosition(ctx, method, args)
	}
	return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
}
