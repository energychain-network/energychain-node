package types

import sdk "github.com/cosmos/cosmos-sdk/types"

type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the cross-module funding hop. Same shape
// as x/escrow / x/scheduler / x/streampay / x/contract:
// IsAccountBlocked / IsDenomPaused are the defence-in-depth
// gates against the MoveBalance ComplianceCheck bypass.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

type AuditKeeper interface {
	RecordAuctionAction(ctx sdk.Context, auctionID uint64, action, actor, subject, detail string)
}
