package evmd

import (
	"encoding/json"
	"sort"

	erc20types "github.com/cosmos/evm/x/erc20/types"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"

	carbonprecompile "energychain/precompiles/carbon"
	eacprecompile "energychain/precompiles/eac"
	marketprecompile "energychain/precompiles/market"
	stablecoinprecompile "energychain/precompiles/stablecoin"
)

const (
	NativeDenom         = "uecy"
	NativeDisplayDenom  = "ecy"
	NativeERC20Contract = "0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"
	EVMChainID          = uint64(9001)
)

// GenesisState of the blockchain is represented here as a map of raw json
// messages key'd by an identifier string.
type GenesisState map[string]json.RawMessage

// NewEVMGenesisState returns the default genesis state for the EVM module.
//
// The list of active precompiles starts with the upstream defaults and
// is extended by NativePrecompileAddresses() — populated as M2/M3 land
// the EAC, Carbon, Stablecoin and Market precompiles. The EVM module
// enforces lexicographic ordering (see evmtypes.ValidatePrecompiles), so
// we sort after concatenation.
func NewEVMGenesisState() *evmtypes.GenesisState {
	evmGenState := evmtypes.DefaultGenesisState()

	custom := NativePrecompileAddresses()
	active := make([]string, 0, len(evmtypes.AvailableStaticPrecompiles)+len(custom))
	active = append(active, evmtypes.AvailableStaticPrecompiles...)
	active = append(active, custom...)
	sort.Strings(active)
	evmGenState.Params.ActiveStaticPrecompiles = active
	evmGenState.Preinstalls = evmtypes.DefaultPreinstalls

	return evmGenState
}

// NativePrecompileAddresses returns the EVM-side addresses of every custom
// EnergyChain precompile registered in NewEnergyChainApp. New precompiles
// (eac, carbon, stablecoin, market) MUST be appended here so genesis
// surfaces them in the active set; the registration call inside app.go
// MUST stay in sync with this list.
//
// Returning a fresh slice on every call avoids accidental mutation of the
// shared state by callers that sort/append the result.
func NativePrecompileAddresses() []string {
	return []string{
		stablecoinprecompile.PrecompileAddressHex,
		eacprecompile.PrecompileAddressHex,
		carbonprecompile.PrecompileAddressHex,
		marketprecompile.PrecompileAddressHex,
		// New native precompiles MUST be appended here and registered
		// via EVMKeeper.RegisterStaticPrecompile in NewEnergyChainApp;
		// the two lists are kept in lockstep.
	}
}

// NewErc20GenesisState returns the default genesis state for the ERC20 module.
func NewErc20GenesisState() *erc20types.GenesisState {
	erc20GenState := erc20types.DefaultGenesisState()
	erc20GenState.TokenPairs = []erc20types.TokenPair{
		{
			Erc20Address:  NativeERC20Contract,
			Denom:         NativeDenom,
			Enabled:       true,
			ContractOwner: erc20types.OWNER_MODULE,
		},
	}
	erc20GenState.NativePrecompiles = []string{NativeERC20Contract}

	return erc20GenState
}

// NewMintGenesisState returns the default genesis state for the mint module.
func NewMintGenesisState() *minttypes.GenesisState {
	mintGenState := minttypes.DefaultGenesisState()
	mintGenState.Params.MintDenom = NativeDenom
	return mintGenState
}

// NewFeeMarketGenesisState returns the default genesis state for the feemarket module.
func NewFeeMarketGenesisState() *feemarkettypes.GenesisState {
	feeMarketGenState := feemarkettypes.DefaultGenesisState()
	feeMarketGenState.Params.NoBaseFee = false

	return feeMarketGenState
}
