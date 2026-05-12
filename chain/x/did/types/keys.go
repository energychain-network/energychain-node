package types

import "cosmossdk.io/collections"

const (
	ModuleName   = "did"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Persistent-store collection prefixes. Each is a 1-byte namespace owned by
// a single Map / KeySet / Item; collections handles all sub-key encoding.
var (
	ParamsCollectionPrefix      = collections.NewPrefix(0x00)
	DIDDocumentCollectionPrefix = collections.NewPrefix(0x01)
	// ControllerIndexPrefix is (controller, subject) — answers "which DIDs
	// does this address control?". The pair encoding lets callers walk a
	// single controller without scanning every DID.
	ControllerIndexPrefix = collections.NewPrefix(0x02)

	CredentialCollectionPrefix         = collections.NewPrefix(0x10)
	CredentialBySubjectIndexPrefix     = collections.NewPrefix(0x11)
	CredentialByIssuerIndexPrefix      = collections.NewPrefix(0x12)
	CredentialByExpiryIndexPrefix      = collections.NewPrefix(0x13) // (expires_at, id) for end-blocker GC

	AnchorCollectionPrefix = collections.NewPrefix(0x20)
)
