package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "identity"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent-store collection prefixes. Each is a 1-byte namespace owned by
// a single collections.Map / KeySet / Item; collections handles all sub-key
// encoding.
var (
	ParamsCollectionPrefix         = collections.NewPrefix(0x00)
	IdentityCollectionPrefix       = collections.NewPrefix(0x01)
	IdentityByRoleCollectionPrefix = collections.NewPrefix(0x02)
)
