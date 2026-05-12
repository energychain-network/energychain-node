package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// DIDKeeper is the slice of x/did's surface area that x/device needs.
// Re-declared here as an interface (rather than imported as the concrete
// keeper) so that:
//
//   1. x/device can be unit-tested with a stub DID keeper.
//   2. We avoid a hard module-import cycle through app.go's wiring.
//
// Methods MUST stay in lock-step with the real keeper. Adding fields here
// is cheap; the wiring just needs to expose more keeper methods.
type DIDKeeper interface {
	// IsActive returns true iff `subject` has a registered DID Document
	// in StatusActive. Used to refuse RegisterDevice / SubmitAttestation
	// from owners that don't have a DID.
	IsActive(ctx sdk.Context, subject string) bool
}
