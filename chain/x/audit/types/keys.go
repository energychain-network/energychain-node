package types

import (
	"encoding/binary"

	"cosmossdk.io/collections"
)

const (
	ModuleName   = "audit"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent-store collection prefixes. Each is a 1-byte namespace owned
// by a single collections.Map / KeySet / Item / Sequence. Prefixes that
// pre-date the M1 evolution keep their original byte values so existing
// state migrates in place; new collections claim contiguous bytes above.
var (
	ParamsCollectionPrefix       = collections.NewPrefix(0x00)
	AuditLogCollectionPrefix     = collections.NewPrefix(0x01)
	AuditByActorCollectionPrefix = collections.NewPrefix(0x02)
	AuditByTypeCollectionPrefix  = collections.NewPrefix(0x03)
	IDSequenceCollectionPrefix   = collections.NewPrefix(0x04)
	AuditByTimeCollectionPrefix  = collections.NewPrefix(0x05)

	// M1 evolution collections.
	AuditBySeverityCollectionPrefix = collections.NewPrefix(0x06) // (severity, log_id)
	AuditBySchemaCollectionPrefix   = collections.NewPrefix(0x07) // (schema_id, log_id)
	SchemaCollectionPrefix          = collections.NewPrefix(0x08) // event_type → SchemaDescriptor
	ArchiveSegmentCollectionPrefix  = collections.NewPrefix(0x09) // segment_id → ArchiveSegment
	ArchiveIDSequencePrefix         = collections.NewPrefix(0x0A)
	ViewKeyGrantCollectionPrefix    = collections.NewPrefix(0x0B) // grant_id → ViewKeyGrant
	ViewKeyByGranteePrefix          = collections.NewPrefix(0x0C) // (grantee, grant_id)
	ViewKeyByExpiryPrefix           = collections.NewPrefix(0x0D) // (expires_at, grant_id) — drives the sweep
	GrantIDSequencePrefix           = collections.NewPrefix(0x0E)

	// AuditCountByCreatorPrefix is keyed in the per-block transient store
	// to count audit submissions by creator within the current block. The
	// transient store is wiped between blocks; collections is intentionally
	// NOT used here because there is no schema to maintain.
	AuditCountByCreatorPrefix = []byte{0x10}
)

// GetAuditCountByCreatorKey returns the transient-store key tracking the
// number of audit logs a single creator has landed in the current block.
func GetAuditCountByCreatorKey(creator string) []byte {
	return append(AuditCountByCreatorPrefix, []byte(creator)...)
}

// uint64ToBytes / bytesToUint64 are kept for any downstream consumers that
// still depend on the legacy raw-KV layout.
func uint64ToBytes(v uint64) []byte {
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, v)
	return bz
}

func bytesToUint64(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}

var (
	_ = uint64ToBytes
	_ = bytesToUint64
)
