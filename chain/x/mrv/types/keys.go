package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "mrv"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent collection prefixes. Each is a 1-byte namespace
// owned by a single collections.Map / KeySet / Item / Sequence.
// Prefixes are not reused across collections so on-disk
// reflectors and migrations have a stable byte layout.
var (
	ParamsCollectionPrefix = collections.NewPrefix(0x00)

	SchemaCollectionPrefix         = collections.NewPrefix(0x10) // id → Schema
	SchemaIDSeqPrefix              = collections.NewPrefix(0x11)
	SchemaByAssetPrefix            = collections.NewPrefix(0x12) // (asset_class, id) index

	VerifierCollectionPrefix       = collections.NewPrefix(0x20) // id → Verifier
	VerifierIDSeqPrefix            = collections.NewPrefix(0x21)
	VerifierByDIDPrefix            = collections.NewPrefix(0x22) // did → id
	// VerifierBySignerPrefix maps a verifier's chain
	// signer_address → verifier_id. Used both to enforce
	// signer-address uniqueness (one address may not back
	// two different verifiers) and to look up an attestor's
	// verifier without scanning the table.
	VerifierBySignerPrefix         = collections.NewPrefix(0x23) // bech32 → id

	ReportCollectionPrefix         = collections.NewPrefix(0x30) // id → Report
	ReportIDSeqPrefix              = collections.NewPrefix(0x31)
	ReportBySubjectPrefix          = collections.NewPrefix(0x32) // (subject, id) index
	ReportBySchemaPrefix           = collections.NewPrefix(0x33) // (schema_id, id) index
	ReportCountBySubjectPrefix     = collections.NewPrefix(0x34) // subject → uint64 count

	GrantCollectionPrefix          = collections.NewPrefix(0x40) // id → ViewKeyGrant
	GrantIDSeqPrefix               = collections.NewPrefix(0x41)
	GrantByGranterPrefix           = collections.NewPrefix(0x42) // (granter, id)
	GrantByGranteePrefix           = collections.NewPrefix(0x43) // (grantee_did, id)
	// GrantByExpiryPrefix drives the end-block sweep that
	// flips expired grants to revoked. Keying on the unix
	// expires_at lets the sweep stop early once it walks past
	// `now`.
	GrantByExpiryPrefix            = collections.NewPrefix(0x44) // (expires_at, id)
)
