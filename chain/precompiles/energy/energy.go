// Package energy exposes the x/energy module to Solidity contracts via a
// stateful EVM precompile.
//
// The precompile is intentionally READ-ONLY. Solidity contracts can fetch
// existing records by ID, but they cannot submit new records — all writes
// must originate from a Cosmos-native MsgSubmitEnergyData (or BatchSubmit)
// signed by an allow-listed submitter.
//
// Why read-only?
//   - A precompile write would let any deployed contract proxy through to
//     the native module's allow-list using its own EVM caller as the
//     submitter, expanding the attack surface (re-entrancy, delegatecall
//     trickery, indirect rate-limit evasion) to anything reachable on
//     0x...0900.
//   - Native txs are signed by a real account whose key custody is auditable;
//     a precompile call is signed by whatever EOA invoked the contract,
//     potentially many hops away.
//   - Removing the only state-modifying selector also removes its entire
//     gas-metering / reentrancy / event-emission surface from the precompile.
//
// Energy data therefore always enters the chain through the Cosmos message
// path; Solidity dApps that need to read records use this precompile, and
// dApps that need to *display* the rate-limit / allow-list status read it
// via the same path.
package energy

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	energykeeper "energychain/x/energy/keeper"
)

// PrecompileAddress is the EIP-1352-style fixed address at which this
// precompile lives. Picked from the unallocated 0x0900 slot to avoid
// colliding with the cosmos-evm builtins (0x0800-0x0807).
const PrecompileAddress = "0x0000000000000000000000000000000000000900"

// Method names used to dispatch ABI calls and price RequiredGas.
const (
	GetEnergyDataMethod = "getEnergyData"
)

// Gas costs for the read-only methods. Storage reads from the keeper are
// charged separately by the SDK gas meter via KvGasConfig; these are flat
// surcharges for the EVM-side bookkeeping.
const (
	GasGetEnergyData uint64 = 5_000
)

var (
	//go:embed abi.json
	abiJSON []byte

	// ABI is the parsed contract ABI shared by every Run() call.
	ABI abi.ABI
)

func init() {
	parsed, err := abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("energy precompile: parse ABI: %w", err))
	}
	ABI = parsed
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

// Precompile wires the EVM precompile contract to the x/energy keeper.
// It exposes read-only access only — see the package doc for rationale.
type Precompile struct {
	cmn.Precompile

	abi.ABI
	keeper energykeeper.Keeper
}

// NewPrecompile builds a Precompile bound to the given keeper. Use the
// chain-wide x/energy keeper from app.go - reads honour any keeper-side
// access controls applied to queries.
func NewPrecompile(keeper energykeeper.Keeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.KVGasConfig(),
			TransientKVGasConfig: storetypes.TransientGasConfig(),
			ContractAddress:      common.HexToAddress(PrecompileAddress),
		},
		ABI:    ABI,
		keeper: keeper,
	}
}

// RequiredGas returns the flat EVM-side gas surcharge for the called method.
// Per-byte storage costs are charged separately by the SDK gas meter inside
// RunNativeAction.
func (p Precompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}
	method, err := p.MethodById(input[:4])
	if err != nil {
		return 0
	}
	switch method.Name {
	case GetEnergyDataMethod:
		return GasGetEnergyData
	}
	return 0
}

// Run is the EVM entry-point. It delegates to RunNativeAction so the SDK
// context, gas metering, and revert journaling are handled by cmn.Precompile.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.execute(ctx, contract, readonly)
	})
}

func (p Precompile) execute(ctx sdk.Context, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	switch method.Name {
	case GetEnergyDataMethod:
		return p.getEnergyData(ctx, method, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}
}

// IsTransaction is always false: the energy precompile is read-only.
// All writes must go through the native MsgSubmitEnergyData / MsgBatchSubmit
// path. See the package doc for the security rationale.
func (Precompile) IsTransaction(_ *abi.Method) bool {
	return false
}
