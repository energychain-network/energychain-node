package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "carbon"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	IssuerCollectionPrefix = collections.NewPrefix(0x01) // id -> Issuer

	AssetCollectionPrefix = collections.NewPrefix(0x02) // id -> Asset
	AssetByIssuerPrefix   = collections.NewPrefix(0x03) // (issuer_id, id)
	AssetIDSeqPrefix      = collections.NewPrefix(0x04)

	BalanceCollectionPrefix = collections.NewPrefix(0x05) // (asset_id, holder) -> uint64
	BalanceByOwnerPrefix    = collections.NewPrefix(0x06) // (holder, asset_id)

	RetirementCollectionPrefix    = collections.NewPrefix(0x07) // id -> Retirement
	RetirementByBeneficiaryPrefix = collections.NewPrefix(0x08) // (beneficiary, id)
	RetirementByAssetPrefix       = collections.NewPrefix(0x09) // (asset_id, id)
	RetirementIDSeqPrefix         = collections.NewPrefix(0x0a)

	Article6AuthorizationPrefix = collections.NewPrefix(0x0b) // asset_id -> Article6Authorization

	BridgeCollectionPrefix = collections.NewPrefix(0x0c) // asset_id -> BridgeAttestation

	// SourceSerialIndexPrefix maps (category, registry, serial) to
	// the asset id that holds it; refuses double-issuance.
	SourceSerialIndexPrefix = collections.NewPrefix(0x0d)

	// EACOffsetClaimPrefix maps an x/eac certificate id to the
	// running tally of OFFSET units that have referenced it. Used
	// to enforce the "1 MWh cannot be both RE and offset" rule.
	EACOffsetClaimPrefix = collections.NewPrefix(0x0e)
)
