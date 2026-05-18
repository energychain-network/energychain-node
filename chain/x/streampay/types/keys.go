package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "streampay"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	StreamCollectionPrefix = collections.NewPrefix(0x01)
	StreamIDSeqPrefix      = collections.NewPrefix(0x02)

	StreamBySenderPrefix   = collections.NewPrefix(0x03)
	StreamByReceiverPrefix = collections.NewPrefix(0x04)
)
