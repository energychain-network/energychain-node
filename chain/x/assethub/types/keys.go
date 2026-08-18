package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "assethub"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	// BondPoolName is the module sub-account that custodies provider bonds.
	BondPoolName = "assethub_bond_pool"
)

// Event types and attribute keys.
const (
	EventTypeProvider = "assethub_provider"
	EventTypeDevice   = "assethub_device"
	EventTypeReading  = "assethub_reading"
	EventTypeTopic    = "assethub_topic"

	AttrAction   = "action"
	AttrSubject  = "subject"
	AttrVerified = "verified"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	ProviderPrefix = collections.NewPrefix(0x01) // address -> Provider

	DevicePrefix           = collections.NewPrefix(0x02) // id -> Device
	DeviceByOperatorPrefix = collections.NewPrefix(0x03) // (operator, id)

	ReadingPrefix         = collections.NewPrefix(0x04) // id -> MeteringReading
	ReadingByDevicePrefix = collections.NewPrefix(0x05) // (device_id, id)
	ReadingIDSeqPrefix    = collections.NewPrefix(0x06)

	TopicPrefix      = collections.NewPrefix(0x07) // id -> OracleTopic
	SubmissionPrefix = collections.NewPrefix(0x08) // (topic_id, provider) -> OracleSubmission
)
