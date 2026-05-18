package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the optional fail-closed sanctions hook.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
}

// StablecoinKeeper is the cross-module surface dispute uses
// to lock bonds into a module pool and disburse on
// resolution. Mirrors the surface used by escrow / market /
// clearing / contract for consistency.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// AuditKeeper is the optional cross-module audit hook.
type AuditKeeper interface {
	RecordDisputeAction(ctx sdk.Context, disputeID uint64, action, actor, subject, detail string)
}

// DIDKeeper resolves a DID to its controllers so an
// arbitrator can rotate signing keys through their DID
// document without re-registering. Optional.
type DIDKeeper interface {
	IsControllerOf(ctx sdk.Context, did, addr string) bool
}
