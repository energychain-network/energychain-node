package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "market"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// EscrowName is the module-derived account that custodies every order's
// escrowed base/quote settlement (per-denom in the x/stableusd ledger).
const EscrowName = "market/escrow"

// FeeName is the module-derived account that accrues protocol fees.
const FeeName = "market/fees"

// BondPoolName is the x/bank module account that custodies listing bonds
// (native denom, escrowed on MsgPostBond, refunded on MsgDelistMarket).
// It is the module's own name so it is registered in maccPerms.
const BondPoolName = ModuleName

const (
	EventTypeMarket = "market_market"
	EventTypeOrder  = "market_order"
	EventTypeClear  = "market_clear"
	EventTypeBond   = "market_bond"
	EventTypeDelist = "market_delist"

	AttrAction   = "action"
	AttrOperator = "operator"
	AttrAmount   = "amount"
	AttrDenom    = "denom"
	AttrMarketID = "market_id"
	AttrOrderID  = "order_id"
	AttrOwner    = "owner"
	AttrSide     = "side"
	AttrPrice    = "price"
	AttrQty      = "qty"
	AttrStatus   = "status"
	AttrFilled   = "filled"
	AttrRefunded = "refunded"
)

var (
	ParamsPrefix     = collections.NewPrefix(0x00)
	MarketPrefix     = collections.NewPrefix(0x01) // id -> Market
	MarketSeqPrefix  = collections.NewPrefix(0x02)
	OrderPrefix      = collections.NewPrefix(0x03) // id -> Order
	OrderSeqPrefix   = collections.NewPrefix(0x04) // order id sequence
	PlaceSeqPrefix   = collections.NewPrefix(0x05) // global placement sequence (price-time priority)
	OpenByMarketPref = collections.NewPrefix(0x06) // (market_id, order_id) -> {} index of OPEN orders
)
