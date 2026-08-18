package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "rwatoken"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	// RedemptionEscrow is the module-reserved holder slot that custodies
	// RWA units for PENDING redemptions. It is intentionally not a bech32
	// address so no real account can collide with it. Units are moved here
	// on RequestRedemption (total_supply unchanged), burned from here on
	// ExecuteRedemption (supply decreases), and returned to the holder on
	// CancelRedemption. This keeps the invariant
	//   total_supply == sum(all balances incl. escrow)
	// at all times and lets genesis tie escrow to the pending set.
	RedemptionEscrow = "rwatoken/redemption-escrow"
)

const (
	EventTypeToken        = "rwatoken_token"
	EventTypeSupply       = "rwatoken_supply"
	EventTypeTransfer     = "rwatoken_transfer"
	EventTypeCompliance   = "rwatoken_compliance"
	EventTypeSnapshot     = "rwatoken_snapshot"
	EventTypeDistribution = "rwatoken_distribution"
	EventTypeRedemption   = "rwatoken_redemption"
	EventTypePool         = "rwatoken_pool"
	EventTypeIssuer       = "rwatoken_issuer"

	AttrAction   = "action"
	AttrTokenID  = "token_id"
	AttrSymbol   = "symbol"
	AttrAccount  = "account"
	AttrFrom     = "from"
	AttrTo       = "to"
	AttrAmount   = "amount"
	AttrSnapshot = "snapshot_id"
	AttrDistrib  = "distribution_id"
	AttrRedeemID = "redemption_id"
	AttrPayout   = "payout"
	AttrReason   = "reason"
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	TokenPrefix         = collections.NewPrefix(0x01) // id -> Token
	TokenBySymbolPrefix = collections.NewPrefix(0x02) // symbol -> id
	TokenIDSeqPrefix    = collections.NewPrefix(0x03)

	BalancePrefix = collections.NewPrefix(0x04) // (tokenID, holder) -> uint64
	FlagsPrefix   = collections.NewPrefix(0x05) // (tokenID, holder) -> AccountFlags

	SnapshotPrefix        = collections.NewPrefix(0x06) // id -> Snapshot
	SnapshotIDSeqPrefix   = collections.NewPrefix(0x07)
	SnapshotBalancePrefix = collections.NewPrefix(0x08) // (snapshotID, holder) -> uint64

	DistributionPrefix      = collections.NewPrefix(0x09) // id -> Distribution
	DistributionIDSeqPrefix = collections.NewPrefix(0x0A)
	DistributionClaimPrefix = collections.NewPrefix(0x0B) // (distID, holder) -> DistributionClaimRow

	RedemptionPrefix             = collections.NewPrefix(0x0C) // id -> Redemption
	RedemptionIDSeqPrefix        = collections.NewPrefix(0x0D)
	RedemptionByHolderPfx        = collections.NewPrefix(0x0E) // (holder, id) all
	RedemptionPendingByHolderPfx = collections.NewPrefix(0x0F) // (holder, id) pending only

	IssuerPrefix = collections.NewPrefix(0x10) // bech32 -> whitelisted issuer
)
