package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "eac"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// One-byte collection prefixes — append-only across schema migrations.
var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	IssuerCollectionPrefix      = collections.NewPrefix(0x01) // id -> Issuer
	CertificateCollectionPrefix = collections.NewPrefix(0x02) // id -> Certificate
	CertByIssuerPrefix          = collections.NewPrefix(0x03) // (issuer_id, id)
	CertIDSeqPrefix             = collections.NewPrefix(0x04)

	BalanceCollectionPrefix = collections.NewPrefix(0x05) // (cert_id, holder) -> uint64
	BalanceByOwnerPrefix    = collections.NewPrefix(0x06) // (holder, cert_id) — nonzero rows only

	RetirementCollectionPrefix    = collections.NewPrefix(0x07) // id -> Retirement
	RetirementByBeneficiaryPrefix = collections.NewPrefix(0x08) // (beneficiary, id)
	RetirementByCertificatePrefix = collections.NewPrefix(0x09) // (cert_id, id)
	RetirementIDSeqPrefix         = collections.NewPrefix(0x0a)

	BridgeCollectionPrefix = collections.NewPrefix(0x0b) // cert_id -> BridgeAttestation

	// SourceSerialIndexPrefix maps the canonical
	// (kind, source_registry, source_serial) triple to its certificate
	// id, preventing two certificates from claiming the same upstream
	// serial. The keeper only writes this row when source_serial is
	// non-empty (NATIVE kind is allowed to omit it).
	SourceSerialIndexPrefix = collections.NewPrefix(0x0c)
)
