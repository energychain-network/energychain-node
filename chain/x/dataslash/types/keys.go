package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "dataslash"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	ProviderCollectionPrefix = collections.NewPrefix(0x10) // id → Provider
	ProviderIDSeqPrefix      = collections.NewPrefix(0x11)
	ProviderByDIDPrefix      = collections.NewPrefix(0x12) // did → id
	ProviderBySignerPrefix   = collections.NewPrefix(0x13) // signer_address → id
	ProviderByStatusPrefix   = collections.NewPrefix(0x14) // (status:uint32, id)
	// ProviderByJailUntilPrefix drives the auto-unjail
	// sweep: rows in the index are ordered by jailed_until,
	// so end-block can stop as soon as it hits a row in the
	// future.
	ProviderByJailUntilPrefix = collections.NewPrefix(0x15) // (jail_until:int64, id)
	// ProviderByWithdrawAtPrefix drives the unbond-cooldown
	// sweep similarly.
	ProviderByWithdrawAtPrefix = collections.NewPrefix(0x16) // (withdraw_at:int64, id)

	InfractionCollectionPrefix = collections.NewPrefix(0x20) // id → Infraction
	InfractionIDSeqPrefix      = collections.NewPrefix(0x21)
	InfractionByProviderPrefix = collections.NewPrefix(0x22) // (provider_id, id)
	InfractionByKindPrefix     = collections.NewPrefix(0x23) // (kind:uint32, id)

	JailRecordCollectionPrefix = collections.NewPrefix(0x30)
	JailRecordIDSeqPrefix      = collections.NewPrefix(0x31)
	JailRecordByProviderPrefix = collections.NewPrefix(0x32) // (provider_id, id)

	BanRecordCollectionPrefix = collections.NewPrefix(0x40)
	BanRecordIDSeqPrefix      = collections.NewPrefix(0x41)
	BanRecordByProviderPrefix = collections.NewPrefix(0x42) // (provider_id, id)
)
