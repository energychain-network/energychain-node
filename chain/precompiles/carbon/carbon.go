// Package carbon implements the EVM precompile for the x/carbon
// module. Each asset (compliance quota or voluntary offset) is
// identified by a uint64 assetId. The precompile mirrors x/carbon's
// holder-side surface only: transfer + retire + two views.
// Issuance / bridge mint / Article 6 admin are NOT exposed.
package carbon

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

	carbonkeeper "energychain/x/carbon/keeper"
	carbontypes "energychain/x/carbon/types"
)

// PrecompileAddressHex / PrecompileAddress are the carbon precompile
// slot in the reserved 0x900 EnergyChain native range.
const PrecompileAddressHex = "0x0000000000000000000000000000000000000902"

var PrecompileAddress = common.HexToAddress(PrecompileAddressHex)

const (
	BalanceOfMethod  = "balanceOf"
	EACClaimedMethod = "eacClaimed"
	TransferMethod   = "transfer"
	RetireMethod     = "retire"

	TransferEventName = "Transfer"
	RetireEventName   = "Retire"
)

const (
	GasBalanceOf  uint64 = 3_000
	GasEACClaimed uint64 = 3_000
	GasTransfer   uint64 = 35_000
	GasRetire     uint64 = 50_000
)

//go:embed abi.json
var abiJSON []byte

var ABI abi.ABI

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("carbon precompile: ABI parse failed: %w", err))
	}
}

type KeeperAPI interface {
	GetBalance(ctx context.Context, assetID uint64, holder string) (uint64, error)
	GetEACClaimed(ctx context.Context, eacCertID uint64) (uint64, error)
}

type MsgServerAPI interface {
	Transfer(ctx sdk.Context, msg *carbontypes.MsgTransfer) error
	Retire(ctx sdk.Context, msg *carbontypes.MsgRetire) (uint64, error)
}

type Precompile struct {
	cmn.Precompile
	abi.ABI

	keeper KeeperAPI
	msgs   MsgServerAPI
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

func NewPrecompile(k carbonkeeper.Keeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      PrecompileAddress,
		},
		ABI:    ABI,
		keeper: keeperAdapter{k: k},
		msgs:   msgServerAdapter{srv: carbonkeeper.NewMsgServerImpl(k)},
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
	case BalanceOfMethod:
		return GasBalanceOf
	case EACClaimedMethod:
		return GasEACClaimed
	case TransferMethod:
		return GasTransfer
	case RetireMethod:
		return GasRetire
	}
	return 0
}

func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case TransferMethod, RetireMethod:
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
		return nil, fmt.Errorf("carbon precompile not bound to keeper")
	}
	switch method.Name {
	case BalanceOfMethod:
		return p.BalanceOf(ctx, method, args)
	case EACClaimedMethod:
		return p.EACClaimed(ctx, method, args)
	case TransferMethod:
		return p.Transfer(ctx, evm, contract, method, args)
	case RetireMethod:
		return p.Retire(ctx, evm, contract, method, args)
	}
	return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
}
