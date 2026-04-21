package types

import "cosmossdk.io/collections"

// Persistent-store collection prefixes. Each is a 1-byte namespace owned by
// a single collections.Map / Item; collections handles all sub-key encoding.
var (
	ParamsCollectionPrefix     = collections.NewPrefix(0x00)
	OracleDataCollectionPrefix = collections.NewPrefix(0x01)
	OracleInfoCollectionPrefix = collections.NewPrefix(0x02)
	LatestDataCollectionPrefix = collections.NewPrefix(0x03)
)
