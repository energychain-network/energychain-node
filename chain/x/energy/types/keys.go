package types

import "cosmossdk.io/collections"

// Persistent-store collection prefixes. Each is a 1-byte namespace owned by
// a single collections.Map / KeySet / Item / Sequence; collections handles
// the rest of the key encoding (length-prefix + sub-key) internally.
//
// We deliberately use NewPrefix(byte) rather than the older raw [...]byte
// literals: collections needs an *opaque* prefix value, and exposing the
// internal byte to consumers would tempt them to hand-craft keys and bypass
// the schema's invariants.
var (
	ParamsCollectionPrefix      = collections.NewPrefix(0x00)
	EnergyDataCollectionPrefix  = collections.NewPrefix(0x01)
	ByCategoryCollectionPrefix  = collections.NewPrefix(0x02)
	BySubmitterCollectionPrefix = collections.NewPrefix(0x03)
	BatchCollectionPrefix       = collections.NewPrefix(0x04)
	IDSequenceCollectionPrefix  = collections.NewPrefix(0x05)

	// SubmitCountByAddrPrefix is used in the per-block transient store to
	// count how many submissions a single address has landed in the current
	// block. The transient store is wiped between blocks so no migration
	// is needed when this prefix changes; collections is intentionally NOT
	// used here because there is no schema to maintain.
	SubmitCountByAddrPrefix = []byte{0x10}
)

// GetSubmitCountByAddrKey returns the transient-store key tracking the
// number of submissions a single address has landed in the current block.
func GetSubmitCountByAddrKey(addr string) []byte {
	return append(SubmitCountByAddrPrefix, []byte(addr)...)
}
