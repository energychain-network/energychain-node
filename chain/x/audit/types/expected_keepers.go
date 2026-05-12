package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// DIDKeeper is the slice of x/did the audit module needs. We declare it
// here (rather than importing did/types directly) so the modules can be
// wired in any order without an import cycle, and so unit tests can stub
// the dependency.
//
// IsActive must return true iff the supplied bech32 address has an active
// DID document. Audit uses it to refuse view-key grants whose grantee
// has no on-chain identity — otherwise a regulator-side credential would
// have nothing to bind to.
type DIDKeeper interface {
	IsActive(ctx sdk.Context, did string) bool
}
