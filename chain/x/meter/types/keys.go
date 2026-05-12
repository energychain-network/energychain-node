package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "meter"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent-store collection prefixes. One byte per logical collection
// keeps the layout migration-friendly: future schema changes add new
// bytes rather than overload an existing one.
var (
	ParamsCollectionPrefix             = collections.NewPrefix(0x00)
	MeteringPointCollectionPrefix      = collections.NewPrefix(0x01) // id -> MeteringPoint
	MeteringPointByOwnerPrefix         = collections.NewPrefix(0x02) // (owner, id)
	MeteringPointByZonePrefix          = collections.NewPrefix(0x03) // (zone, id)
	ReadingCollectionPrefix            = collections.NewPrefix(0x04) // (mp_id, start_time) -> Reading
	BatchCollectionPrefix              = collections.NewPrefix(0x05) // batch_id -> Batch
	BatchByMPPrefix                    = collections.NewPrefix(0x06) // (mp_id, batch_id)
	StreamAuthorizationPrefix          = collections.NewPrefix(0x07) // id -> StreamAuthorization
	StreamAuthByMPPrefix               = collections.NewPrefix(0x08) // (mp_id, id)
	StreamAuthByExpiryPrefix           = collections.NewPrefix(0x09) // (expires_at, id) — sweep queue
	StreamAuthIDSeqPrefix              = collections.NewPrefix(0x0A) // global counter
)
