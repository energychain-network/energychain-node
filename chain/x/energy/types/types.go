package types

// Module-level constants.
//
// EnergyData and BatchSubmission are now generated from
// proto/energychain/energy/v1/types.proto. Validation rules and helpers live
// in genesis.go (Params, GenesisState) and msgs.go (Msg* messages).
//
// The chain does NOT define what categories or metadata fields are valid -
// users are free to submit any category string and any JSON metadata.
// Only the hash of the off-chain payload is stored; the full data lives off-chain.
const (
	ModuleName = "energy"
	StoreKey   = ModuleName
	RouterKey  = ModuleName

	// TStoreKey is the transient store key for per-block, non-persistent
	// state such as submission rate-limit counters. Anything written here
	// is wiped at the start of every block by CometBFT.
	TStoreKey = "transient_" + ModuleName
)
