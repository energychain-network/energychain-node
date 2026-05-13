package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "stablecoin"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// One-byte collection prefixes; new schema bytes append rather than
// overload existing ones so future migrations stay surgical.
var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	DenomCollectionPrefix     = collections.NewPrefix(0x01) // id -> Denom
	IssuerCollectionPrefix    = collections.NewPrefix(0x02) // id -> Issuer
	QuotaCollectionPrefix     = collections.NewPrefix(0x03) // (issuer_id, denom_id) -> MintQuota
	QuotaByDenomPrefix        = collections.NewPrefix(0x04) // (denom_id, issuer_id) -> bool — secondary index
	ReserveCollectionPrefix   = collections.NewPrefix(0x05) // denom_id -> DenomReserve

	BalanceCollectionPrefix   = collections.NewPrefix(0x06) // (denom_id, account) -> uint64 — explicit zero-balance rows are pruned by the keeper
	AllowanceCollectionPrefix = collections.NewPrefix(0x07) // (denom_id, owner, spender) -> uint64
	SupplyCollectionPrefix    = collections.NewPrefix(0x08) // denom_id -> uint64
	FlagsCollectionPrefix     = collections.NewPrefix(0x09) // (denom_id, account) -> AccountFlags

	RedemptionCollectionPrefix = collections.NewPrefix(0x0a) // id -> Redemption
	RedemptionByHolderPrefix   = collections.NewPrefix(0x0b) // (holder, id) -> bool
	RedemptionByDenomPrefix    = collections.NewPrefix(0x0c) // (denom_id, id) -> bool — pending only
	RedemptionIDSeqPrefix      = collections.NewPrefix(0x0d)
)
