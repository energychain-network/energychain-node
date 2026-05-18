package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "market"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	PairCollectionPrefix = collections.NewPrefix(0x01)
	PairIDSeqPrefix      = collections.NewPrefix(0x02)

	OrderCollectionPrefix = collections.NewPrefix(0x10)
	OrderIDSeqPrefix      = collections.NewPrefix(0x11)
	OrderByOwnerPrefix    = collections.NewPrefix(0x12)

	// Buy / Sell book indices. Keys encode the order's
	// price-time priority directly into the sort, so iteration
	// returns orders in the correct match sequence.
	//
	// BuyBook key: (pair_id, MaxPrice - price, order_id) so
	//   iteration yields the HIGHEST price first; same-price
	//   resolved by oldest order_id first (price-time).
	// SellBook key: (pair_id, price, order_id) so iteration
	//   yields the LOWEST price first; same-price by oldest
	//   order_id (price-time).
	BuyBookPrefix  = collections.NewPrefix(0x20)
	SellBookPrefix = collections.NewPrefix(0x21)

	PositionCollectionPrefix = collections.NewPrefix(0x30)
)
