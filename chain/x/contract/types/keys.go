package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "contract"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	ContractCollectionPrefix = collections.NewPrefix(0x01)
	ContractIDSeqPrefix      = collections.NewPrefix(0x02)

	// ContractByPartyPrefix indexes (party, id) where party is
	// either buyer or seller. Two index entries per contract; we
	// keep them in a single key-set so lookups by either role
	// scan the same prefix.
	ContractByPartyPrefix = collections.NewPrefix(0x03)
)
