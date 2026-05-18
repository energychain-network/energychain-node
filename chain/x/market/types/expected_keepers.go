package types

import sdk "github.com/cosmos/cosmos-sdk/types"

type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the cross-module funding hop, same shape
// shared with escrow / scheduler / streampay / contract /
// auction. Defense-in-depth on the per-account freeze and
// paused-denom dimensions that MoveBalance does not enforce
// directly.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

type AuditKeeper interface {
	RecordMarketAction(ctx sdk.Context, pairID, orderID uint64, action, actor, subject, detail string)
}
