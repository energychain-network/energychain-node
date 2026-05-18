package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the optional fail-closed sanctions hook.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
}

// StablecoinKeeper is the cross-module surface dataslash uses
// to escrow bonds into the module pool and disburse on
// withdraw / slash. Mirrors the surface used by escrow / market
// / clearing / dispute for consistency.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// AuditKeeper is the optional cross-module audit hook.
type AuditKeeper interface {
	RecordDataslashAction(ctx sdk.Context, providerID uint64, action, actor, subject, detail string)
}
