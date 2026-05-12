package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// DIDKeeper is the slice of x/did the policy engine consumes when
// building the dsl.Context for a transfer evaluation. We intentionally
// expose narrow per-question methods rather than a "give me the whole
// DID Document" call so the policy module never accidentally pulls in
// data it does not need (privacy minimisation).
//
// All methods MUST be deterministic and read-only.
type DIDKeeper interface {
	// IsActive returns true iff the supplied bech32 address has an
	// active DID Document.
	IsActive(ctx sdk.Context, subject string) bool
	// Jurisdiction returns the ISO-3166 alpha-2 code stored in the
	// subject's DID Document, or "" if unknown / not yet wired.
	Jurisdiction(ctx sdk.Context, subject string) string
	// HasCredential returns true iff the subject currently holds a
	// non-revoked verifiable credential matching the supplied type
	// (e.g. "urn:vc:kyc:passed").
	HasCredential(ctx sdk.Context, subject, credentialType string) bool
}

// SanctionsKeeper is the slice of x/sanctions the policy engine
// consults to populate dsl.Context.Sanctioned. The expected x/sanctions
// module lands later in M2; until then, the dependency is wired as nil
// and the policy keeper falls through with "not sanctioned".
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// AuditKeeper is the slice of x/audit the policy engine uses to record
// denial events for forensic analysis. Optional — wired as nil when
// the operator does not want cross-module dependencies in the policy
// hot path.
type AuditKeeper interface {
	RecordPolicyDenial(ctx sdk.Context, policyID, assetClass, assetID, sender, receiver string, code uint32, detail string)
}
