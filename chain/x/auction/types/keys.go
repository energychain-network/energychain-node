package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "auction"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	AuctionCollectionPrefix = collections.NewPrefix(0x01)
	AuctionIDSeqPrefix      = collections.NewPrefix(0x02)
	AuctionBySellerPrefix   = collections.NewPrefix(0x03)

	BidCollectionPrefix    = collections.NewPrefix(0x10)
	BidIDSeqPrefix         = collections.NewPrefix(0x11)
	BidByAuctionPrefix     = collections.NewPrefix(0x12) // (auction_id, bid_id)
)
