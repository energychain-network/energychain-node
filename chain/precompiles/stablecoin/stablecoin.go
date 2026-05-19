// Package stablecoin implements the EVM precompile for the
// x/stablecoin module. The precompile exposes a multi-denom ERC-20-
// shaped surface: each method takes a string `denomId` plus the
// usual ERC-20 args, dispatches to the x/stablecoin keeper, and
// runs the full ComplianceCheck pipeline on transfers (policy DSL +
// sanctions + per-denom flags + per-account freeze). Module-controlled
// MoveBalance / Mint / Burn are NOT exposed — those require explicit
// issuer authority and remain governance-gated through native Msgs.
//
// Wire-up summary (see chain/app.go):
//
//	app.EVMKeeper.RegisterStaticPrecompile(
//	    stablecoinprecompile.PrecompileAddress,
//	    stablecoinprecompile.NewPrecompile(app.StablecoinKeeper),
//	)
//
// and add PrecompileAddress to chain/genesis.go::NativePrecompileAddresses().
package stablecoin

import (
	"bytes"
	"fmt"

	_ "embed"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	cmn "github.com/cosmos/evm/precompiles/common"

	stablecoinkeeper "energychain/x/stablecoin/keeper"
	stablecointypes "energychain/x/stablecoin/types"
)

// PrecompileAddressHex is the EVM-side address of the stablecoin
// precompile. Reserved in the 0x900 range for EnergyChain native
// precompiles (eac = 0x901, carbon = 0x902, market = 0x903 follow).
const PrecompileAddressHex = "0x0000000000000000000000000000000000000900"

// PrecompileAddress is the parsed common.Address form of
// PrecompileAddressHex. Exported so app.go and genesis.go reference
// the same canonical value without re-parsing the hex string.
var PrecompileAddress = common.HexToAddress(PrecompileAddressHex)

// Method names. Centralized constants so RequiredGas / Execute /
// IsTransaction all dispatch off the same identifiers and a typo in
// one file is a compile error in the other.
const (
	BalanceOfMethod            = "balanceOf"
	TotalSupplyMethod          = "totalSupply"
	IsDenomPausedMethod        = "isDenomPaused"
	IsAccountBlockedMethod     = "isAccountBlocked"
	AllowanceMethod            = "allowance"
	TransferMethod             = "transfer"
	TransferFromMethod         = "transferFrom"
	ApproveMethod              = "approve"

	TransferEventName = "Transfer"
	ApprovalEventName = "Approval"
)

// Gas costs. Queries are flat (one keeper read + minor in-memory
// processing); state-changing methods carry the SDK's standard write
// flat cost plus a per-input byte fee handled by cmn.Precompile's
// RequiredGas helper. Tuned conservatively so a misuse cannot
// out-charge the underlying KV ops they trigger.
const (
	GasBalanceOf        uint64 = 2_851  // matches ERC-20 balanceOf
	GasTotalSupply      uint64 = 2_477
	GasIsDenomPaused    uint64 = 2_000
	GasIsAccountBlocked uint64 = 2_400
	GasAllowance        uint64 = 2_500
	GasTransfer         uint64 = 30_000
	GasTransferFrom     uint64 = 35_000
	GasApprove          uint64 = 25_000
)

//go:embed abi.json
var abiJSON []byte

// ABI is parsed once at package-init. Failure to parse panics
// (the JSON is part of the binary, so a parse failure is a build-time
// programming error, not a runtime input).
var ABI abi.ABI

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		panic(fmt.Errorf("stablecoin precompile: ABI parse failed: %w", err))
	}
}

// StablecoinKeeperAPI is the precompile-side view of x/stablecoin.
// Defined here (rather than imported from x/stablecoin/types) so the
// precompile's surface stays minimal and tests can wire a fake.
type StablecoinKeeperAPI interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	GetBalance(ctx sdk.Context, denomID, account string) uint64
	GetSupply(ctx sdk.Context, denomID string) uint64
	IsDenomPaused(ctx sdk.Context, denomID string) bool
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	GetAllowance(ctx sdk.Context, denomID, owner, spender string) uint64
}

// MsgServerAPI is the precompile-side view of the keeper's msgServer.
// State-changing methods bridge through here so they reuse the
// keeper's full ComplianceCheck + emit-event pipeline rather than
// re-implementing it in the precompile (any drift would be a
// compliance bypass).
type MsgServerAPI interface {
	Transfer(ctx sdk.Context, msg *stablecointypes.MsgTransfer) error
	TransferFrom(ctx sdk.Context, msg *stablecointypes.MsgTransferFrom) error
	Approve(ctx sdk.Context, msg *stablecointypes.MsgApprove) error
}

// Precompile is the stablecoin precompile contract.
type Precompile struct {
	cmn.Precompile
	abi.ABI

	keeper StablecoinKeeperAPI
	msgs   MsgServerAPI
}

var _ vm.PrecompiledContract = (*Precompile)(nil)

// NewPrecompile binds the precompile to a live x/stablecoin keeper.
// The MsgServer is constructed once and reused for the lifetime of
// the app; both keeper and msgs are nil-checked on dispatch so that
// a partially-wired binary fails closed rather than silently
// accepting bypass calls.
func NewPrecompile(k stablecoinkeeper.Keeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      PrecompileAddress,
		},
		ABI:    ABI,
		keeper: stablecoinKeeperAdapter{k: k},
		msgs:   msgServerAdapter{srv: stablecoinkeeper.NewMsgServerImpl(k)},
	}
}

// NewPrecompileWithAPI is a test-only alternate constructor that
// accepts hand-rolled interface stubs in place of a real keeper.
// app.go MUST use NewPrecompile, not this; the chain compile keeps
// NewPrecompile as the only path that produces a precompile bound
// to live state.
func NewPrecompileWithAPI(k StablecoinKeeperAPI, s MsgServerAPI) *Precompile {
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

// RequiredGas reports the base gas cost. Method-specific costs above
// already cover the dominant keeper read/write; the cmn.Precompile
// runtime additionally charges per-KV op the SDK gas meter records.
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
	case TotalSupplyMethod:
		return GasTotalSupply
	case IsDenomPausedMethod:
		return GasIsDenomPaused
	case IsAccountBlockedMethod:
		return GasIsAccountBlocked
	case AllowanceMethod:
		return GasAllowance
	case TransferMethod:
		return GasTransfer
	case TransferFromMethod:
		return GasTransferFrom
	case ApproveMethod:
		return GasApprove
	}
	return 0
}

// IsTransaction reports whether the method mutates state. Used by
// cmn.SetupABI to refuse a state-mutating call in a static (eth_call)
// EVM context.
func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case TransferMethod, TransferFromMethod, ApproveMethod:
		return true
	}
	return false
}

// Run is the EVM entry point. We always wrap execution in
// RunNativeAction so the multi-store snapshot + journal entry let the
// EVM revert keeper writes on a downstream require()/revert.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm, contract, readonly)
	})
}

// Execute dispatches to per-method handlers in query.go / tx.go.
func (p Precompile) Execute(ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	if p.keeper == nil {
		return nil, fmt.Errorf("stablecoin precompile not bound to keeper")
	}

	switch method.Name {
	// queries
	case BalanceOfMethod:
		return p.BalanceOf(ctx, method, args)
	case TotalSupplyMethod:
		return p.TotalSupply(ctx, method, args)
	case IsDenomPausedMethod:
		return p.IsDenomPaused(ctx, method, args)
	case IsAccountBlockedMethod:
		return p.IsAccountBlocked(ctx, method, args)
	case AllowanceMethod:
		return p.Allowance(ctx, method, args)

	// transactions
	case TransferMethod:
		return p.Transfer(ctx, evm, contract, method, args)
	case TransferFromMethod:
		return p.TransferFrom(ctx, evm, contract, method, args)
	case ApproveMethod:
		return p.Approve(ctx, evm, contract, method, args)
	}

	return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
}
