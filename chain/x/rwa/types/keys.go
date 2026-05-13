package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "rwa"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName

	// RedemptionRateScale is the fixed-point scale of Token.redemption_rate.
	// settlement_amount = floor(amount * redemption_rate / RedemptionRateScale).
	RedemptionRateScale uint64 = 1_000_000

	// DistributionRateScale is the fixed-point scale of
	// Distribution.per_unit_rate. Larger than RedemptionRateScale
	// so per-unit dividends on large supplies retain meaningful
	// precision. amount = floor(snapshot_balance * per_unit_rate / DistributionRateScale).
	DistributionRateScale uint64 = 1_000_000_000_000
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	IssuerCollectionPrefix = collections.NewPrefix(0x01) // id -> Issuer

	TokenCollectionPrefix    = collections.NewPrefix(0x02) // id -> Token
	TokenBySymbolPrefix      = collections.NewPrefix(0x03) // symbol -> id
	TokenByIssuerPrefix      = collections.NewPrefix(0x04) // (issuer_id, id)
	TokenIDSeqPrefix         = collections.NewPrefix(0x05)

	BalanceCollectionPrefix = collections.NewPrefix(0x06) // (token_id, account) -> uint64
	BalanceByOwnerPrefix    = collections.NewPrefix(0x07) // (account, token_id)

	AccountFlagsPrefix = collections.NewPrefix(0x08) // (token_id, account) -> AccountFlags

	FrozenBalancePrefix = collections.NewPrefix(0x09) // (token_id, account) -> FrozenBalance

	LockupCollectionPrefix = collections.NewPrefix(0x0a) // (token_id, account, id) -> Lockup
	LockupIDSeqPrefix      = collections.NewPrefix(0x0b)

	SnapshotCollectionPrefix = collections.NewPrefix(0x0c) // id -> SnapshotMeta
	SnapshotByTokenPrefix    = collections.NewPrefix(0x0d) // (token_id, id)
	SnapshotBalancePrefix    = collections.NewPrefix(0x0e) // (snapshot_id, account) -> uint64
	SnapshotIDSeqPrefix      = collections.NewPrefix(0x0f)

	DistributionCollectionPrefix = collections.NewPrefix(0x10) // id -> Distribution
	DistributionByTokenPrefix    = collections.NewPrefix(0x11) // (token_id, id)
	DistributionClaimPrefix      = collections.NewPrefix(0x12) // (distribution_id, account) -> DistributionClaim
	DistributionIDSeqPrefix      = collections.NewPrefix(0x13)

	RedemptionCollectionPrefix = collections.NewPrefix(0x14) // id -> RedemptionRequest
	RedemptionByHolderPrefix   = collections.NewPrefix(0x15) // (holder, id)
	RedemptionPendingByTokenPrefix = collections.NewPrefix(0x16) // (token_id, id) where status==PENDING
	RedemptionIDSeqPrefix      = collections.NewPrefix(0x17)

	// RedemptionPendingByHolderPrefix indexes (holder, id) for rows
	// whose status==PENDING. Maintained alongside the by-holder index
	// so per-holder pending counts are O(pending_for_holder) instead
	// of O(all_redemptions_for_holder).
	RedemptionPendingByHolderPrefix = collections.NewPrefix(0x19)

	// PendingRedemptionUnits: token_id -> total units sitting in
	// pending redemption requests (debited from holders, not yet
	// burned). Tracked so total_supply == sum(balances) +
	// pending_redemption_units always holds.
	PendingRedemptionUnitsPrefix = collections.NewPrefix(0x18)
)
