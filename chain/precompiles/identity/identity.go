// Package identity exposes read-only access to the x/identity module via an
// EVM precompile. Solidity contracts can look up registered participants and
// check role membership without round-tripping through a Cosmos query.
//
// Writes are deliberately NOT exposed: the native module restricts identity
// management to a configured admin authority, and replicating that
// authentication boundary inside the EVM would require either signing-key
// passthrough or a separate access-control surface. Read-only is the
// safer floor.
package identity

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

	identitykeeper "energychain/x/identity/keeper"
)

// PrecompileAddress is the EIP-1352-style fixed address at which this
// precompile lives. Sits in the unallocated 0x0900-range alongside the
// Energy precompile.
const PrecompileAddress = "0x0000000000000000000000000000000000000901"

// Method names used to dispatch ABI calls and price RequiredGas.
const (
	GetIdentityMethod             = "getIdentity"
	GetIdentityByEvmAddressMethod = "getIdentityByEvmAddress"
	HasRoleMethod                 = "hasRole"
	IsRegisteredMethod            = "isRegistered"
)

// Per-method gas surcharges for the EVM-side bookkeeping. Storage reads
// performed inside the keeper are charged separately by the SDK gas meter
// when KvGasConfig is set.
const (
	GasGetIdentity   uint64 = 5_000
	GasHasRole       uint64 = 3_000
	GasIsRegistered  uint64 = 2_000
)

var (
	//go:embed abi.json
	abiJSON []byte

	ABI abi.ABI
)

func init() {
	parsed, err := abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("identity precompile: parse ABI: %w", err))
	}
	ABI = parsed
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

// Precompile binds the precompiled contract to the x/identity keeper.
type Precompile struct {
	cmn.Precompile

	abi.ABI
	keeper identitykeeper.Keeper
}

// NewPrecompile builds a Precompile instance bound to the given keeper.
func NewPrecompile(keeper identitykeeper.Keeper) *Precompile {
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

// RequiredGas returns the flat EVM-side gas surcharge per method.
func (p Precompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}
	method, err := p.MethodById(input[:4])
	if err != nil {
		return 0
	}
	switch method.Name {
	case GetIdentityMethod, GetIdentityByEvmAddressMethod:
		return GasGetIdentity
	case HasRoleMethod:
		return GasHasRole
	case IsRegisteredMethod:
		return GasIsRegistered
	}
	return 0
}

// Run delegates to RunNativeAction so the SDK context, gas metering, and
// revert journaling are handled by cmn.Precompile.
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
	case GetIdentityMethod:
		return p.getIdentity(ctx, method, args)
	case GetIdentityByEvmAddressMethod:
		return p.getIdentityByEvmAddress(ctx, method, args)
	case HasRoleMethod:
		return p.hasRole(ctx, method, args)
	case IsRegisteredMethod:
		return p.isRegistered(ctx, method, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}
}

// IsTransaction is always false: the identity precompile is read-only.
func (Precompile) IsTransaction(_ *abi.Method) bool {
	return false
}
