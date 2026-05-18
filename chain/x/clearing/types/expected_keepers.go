package types

import sdk "github.com/cosmos/cosmos-sdk/types"

type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the same defense-in-depth surface used
// by escrow / market / contract / auction. The clearing
// module funnels every denom move (margin, default fund,
// netting payouts) through Move() so per-denom freezes and
// pauses apply uniformly.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

type AuditKeeper interface {
	RecordClearingAction(ctx sdk.Context, cycleID, memberID uint64, action, actor, subject, detail string)
}
