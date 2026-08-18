package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "mincast"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// InvestEscrow is a reserved, non-bech32 holder slot that custodies units
// locked in an active Invest. Units move here on OpenInvest (supply
// unchanged) and back to the investor on CloseInvest/CancelInvest, so the
// supply == sum(balances) invariant holds across the invest lifecycle.
const InvestEscrow = "mincast/invest-escrow"

const (
	EventTypeMarket   = "mincast_market"
	EventTypeTrade    = "mincast_trade"
	EventTypeTransfer = "mincast_transfer"
	EventTypeTreasury = "mincast_treasury"
	EventTypeInvest   = "mincast_invest"

	AttrAction     = "action"
	AttrMarketID   = "market_id"
	AttrDenom      = "denom"
	AttrAccount    = "account"
	AttrFrom       = "from"
	AttrTo         = "to"
	AttrAmount     = "amount"
	AttrUnits      = "units"
	AttrSettlement = "settlement"
	AttrFee        = "fee"
	AttrFloor      = "floor_price"
	AttrInvestID   = "invest_id"
	AttrYield      = "yield"
	AttrStatus     = "status"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)
	MarketPrefix           = collections.NewPrefix(0x01) // id -> Market
	MarketByDenomPrefix    = collections.NewPrefix(0x02) // denom -> id
	MarketIDSeqPrefix      = collections.NewPrefix(0x03)
	BalancePrefix          = collections.NewPrefix(0x04) // (marketID, holder) -> amount
	InvestPrefix           = collections.NewPrefix(0x05) // id -> Invest
	InvestByInvestorPrefix = collections.NewPrefix(0x06) // (investor, id) -> keyset
	InvestIDSeqPrefix      = collections.NewPrefix(0x07)
)
