package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// DIDKeeper is the slice of x/did the meter module needs. We declare it
// here (rather than importing did/keeper directly) so the modules can be
// wired in any order without an import cycle, and so unit tests can stub
// the dependency.
//
// IsActive returns true iff the supplied bech32 address has an active
// DID. Meter uses it to refuse MeteringPoint registrations whose owner
// has no on-chain identity — otherwise downstream auditors / regulators
// have nothing to bind a reading back to.
type DIDKeeper interface {
	IsActive(ctx sdk.Context, subject string) bool
}

// DeviceKeeper is the slice of x/device the meter module needs. The
// hot-path is IsAttested: every reading and batch flowing in must come
// from a device whose attestation is currently fresh, otherwise the
// data quality guarantees collapse.
//
// nil-safe semantics — see Keeper.requireAttested for how the strict
// path is gated by Params.RequireAttestedDevice + a non-nil keeper.
type DeviceKeeper interface {
	IsAttested(ctx sdk.Context, did string) bool
}
