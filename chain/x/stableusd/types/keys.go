package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "stableusd"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	// RedemptionEscrow is the module-reserved ledger account that custodies
	// tokens for PENDING redemptions. It is intentionally not a bech32
	// address so no real account can collide with it. Tokens are moved here
	// on RequestRedemption (supply unchanged), burned from here on
	// SettleRedemption, and returned to the holder on CancelRedemption. This
	// keeps the invariant supply == sum(all balances incl. escrow) at all
	// times and lets genesis tie escrow to the pending-redemption set.
	RedemptionEscrow = "stableusd/redemption-escrow"
)

const (
	EventTypeDenom      = "stableusd_denom"
	EventTypeSupply     = "stableusd_supply"
	EventTypeTransfer   = "stableusd_transfer"
	EventTypeCompliance = "stableusd_compliance"
	EventTypeRedemption = "stableusd_redemption"

	AttrAction   = "action"
	AttrDenom    = "denom"
	AttrAccount  = "account"
	AttrFrom     = "from"
	AttrTo       = "to"
	AttrAmount   = "amount"
	AttrRedeemID = "redemption_id"
	AttrReason   = "reason"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)
	DenomPrefix            = collections.NewPrefix(0x01) // id -> StableDenom
	BalancePrefix          = collections.NewPrefix(0x02) // (denom, account) -> uint64
	SupplyPrefix           = collections.NewPrefix(0x03) // denom -> uint64
	AllowancePrefix        = collections.NewPrefix(0x04) // (denom, owner, spender) -> uint64
	FlagsPrefix            = collections.NewPrefix(0x05) // (denom, account) -> AccountFlags
	RedemptionPrefix       = collections.NewPrefix(0x06) // id -> Redemption
	RedemptionByHolderPfx  = collections.NewPrefix(0x07) // (holder, id) pending only
	RedemptionIDSeqPrefix  = collections.NewPrefix(0x08)
)
