package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "sanctions"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// One-byte collection prefixes; new schema bytes append rather than
// overload existing ones so future migrations stay surgical.
var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	ListCollectionPrefix    = collections.NewPrefix(0x01) // id -> SanctionList
	EntryCollectionPrefix   = collections.NewPrefix(0x02) // (list_id, subject) -> SanctionEntry
	EntryBySubjectPrefix    = collections.NewPrefix(0x03) // (subject, list_id) -> bool, ACTIVE only
	EntryHistoryPrefix      = collections.NewPrefix(0x04) // (subject, list_id) -> bool, all entries (audit)
	ProposalCollectionPrefix = collections.NewPrefix(0x05) // id -> Proposal
	ProposalByListPrefix     = collections.NewPrefix(0x06) // (list_id, id) -> bool
	ProposalIDSeqPrefix      = collections.NewPrefix(0x07)

	HitCollectionPrefix = collections.NewPrefix(0x08) // id -> SanctionHit
	HitBySubjectPrefix  = collections.NewPrefix(0x09) // (subject, id) -> bool
	HitByListPrefix     = collections.NewPrefix(0x0a) // (list_id, id) -> bool
	HitIDSeqPrefix      = collections.NewPrefix(0x0b)
	// HitOldestPrefix is the head cursor for the hits ring buffer; same
	// O(1) eviction trick as x/policy.
	HitOldestPrefix = collections.NewPrefix(0x0c)
)
