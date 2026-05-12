package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "oracle"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	// PortID is reserved for future IBC-based oracle channels (e.g.
	// cross-chain price relays). Unused today but allocated to keep the
	// store layout stable.
	PortID = "oracle-port"
)

// Persistent-store collection prefixes. Each is a single byte owned by
// exactly one collections object so the store layout stays
// upgrade-safe; a future migration adds new bytes rather than mutating
// an existing one.
var (
	ParamsCollectionPrefix             = collections.NewPrefix(0x00)
	TopicCollectionPrefix              = collections.NewPrefix(0x01)
	ProviderCollectionPrefix           = collections.NewPrefix(0x02)
	SubmissionCollectionPrefix         = collections.NewPrefix(0x03) // (topic_id, provider) → Submission
	AggregatedCollectionPrefix         = collections.NewPrefix(0x04) // topic_id → AggregatedValue
	ReserveAttestationCollectionPrefix = collections.NewPrefix(0x05) // id → ReserveAttestation
	ReserveAttByAssetPrefix            = collections.NewPrefix(0x06) // (asset, id)
	BondReleaseQueuePrefix             = collections.NewPrefix(0x07) // (release_height, provider)
)
