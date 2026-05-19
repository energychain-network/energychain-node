// Package eac implements the EVM precompile for the x/eac module.
// The precompile exposes a uint64-cert-indexed surface mirroring
// x/eac's native Msgs: holder transfers + holder retirements (with
// optional proxy beneficiary) plus two queries (balanceOf,
// certificateIssuedUnits). Issuance / bridge-mint paths are NOT
// exposed; those require governance authority and stay native-Msg
// only.
package eac

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

	eackeeper "energychain/x/eac/keeper"
	eactypes "energychain/x/eac/types"
)

// PrecompileAddressHex is the EVM-side address of the EAC precompile.
// Reserved as 0x901 in the EnergyChain native range.
const PrecompileAddressHex = "0x0000000000000000000000000000000000000901"

// PrecompileAddress is the parsed common.Address form.
var PrecompileAddress = common.HexToAddress(PrecompileAddressHex)

// Method names.
const (
	BalanceOfMethod              = "balanceOf"
	CertificateIssuedUnitsMethod = "certificateIssuedUnits"
	TransferMethod               = "transfer"
	RetireMethod                 = "retire"

	TransferEventName = "Transfer"
	RetireEventName   = "Retire"
)

// Gas costs (tuned conservatively to cover keeper KV ops; SDK gas
// meter additionally charges per-KV op).
const (
	GasBalanceOf              uint64 = 3_000
	GasCertificateIssuedUnits uint64 = 3_000
	GasTransfer               uint64 = 35_000
	GasRetire                 uint64 = 45_000
)

//go:embed abi.json
var abiJSON []byte

var ABI abi.ABI

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("eac precompile: ABI parse failed: %w", err))
	}
}

// KeeperAPI is the precompile-side view of x/eac.Keeper.
type KeeperAPI interface {
	GetBalance(ctx context.Context, certID uint64, holder string) (uint64, error)
	GetCertificateIssuedUnits(ctx sdk.Context, certID uint64) (uint64, bool)
}

// MsgServerAPI is the precompile-side view of the keeper's msgServer.
// Used so transfer + retire reuse the full compliance pipeline.
type MsgServerAPI interface {
	Transfer(ctx sdk.Context, msg *eactypes.MsgTransfer) error
	Retire(ctx sdk.Context, msg *eactypes.MsgRetire) (uint64, error)
}

// Precompile is the EAC precompile contract.
type Precompile struct {
	cmn.Precompile
	abi.ABI

	keeper KeeperAPI
	msgs   MsgServerAPI
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

// NewPrecompile binds the precompile to a live x/eac keeper.
func NewPrecompile(k eackeeper.Keeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      PrecompileAddress,
		},
		ABI:    ABI,
		keeper: keeperAdapter{k: k},
		msgs:   msgServerAdapter{srv: eackeeper.NewMsgServerImpl(k)},
	}
}

// NewPrecompileWithAPI is the test-only constructor.
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
	case CertificateIssuedUnitsMethod:
		return GasCertificateIssuedUnits
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
		return nil, fmt.Errorf("eac precompile not bound to keeper")
	}
	switch method.Name {
	case BalanceOfMethod:
		return p.BalanceOf(ctx, method, args)
	case CertificateIssuedUnitsMethod:
		return p.CertificateIssuedUnits(ctx, method, args)
	case TransferMethod:
		return p.Transfer(ctx, evm, contract, method, args)
	case RetireMethod:
		return p.Retire(ctx, evm, contract, method, args)
	}
	return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
}
