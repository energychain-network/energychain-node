package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "device"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix       = collections.NewPrefix(0x00)
	DeviceCollectionPrefix       = collections.NewPrefix(0x01)
	DeviceByOwnerIndexPrefix     = collections.NewPrefix(0x02)
	DeviceByGridIndexPrefix      = collections.NewPrefix(0x03)
	DeviceByExpiryIndexPrefix    = collections.NewPrefix(0x04) // (expires_at, did) for end-blocker
	AttestationCollectionPrefix  = collections.NewPrefix(0x10)
	AttestationByDeviceIndexPrefix = collections.NewPrefix(0x11)
	AttestationIDSequencePrefix  = collections.NewPrefix(0x12)
	AttestationPendingIndexPrefix = collections.NewPrefix(0x13) // verdict==0 entries for pagination
)
