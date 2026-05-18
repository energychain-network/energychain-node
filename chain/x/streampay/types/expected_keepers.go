package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the chain-level sanctions read. Used at
// every create / withdraw / cancel / transfer leg so a freshly-
// sanctioned counterparty cannot continue receiving / draining a
// previously-opened stream.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the narrow funding hop x/streampay uses for
// pool funding, withdrawal, and settlement transfers. The
// defensive reads (IsAccountBlocked / IsDenomPaused) mirror the
// surface x/escrow and x/scheduler use; without them, a frozen
// account or paused denom could be silently laundered through
// the stream-pay pool.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// AuditKeeper is the optional cross-module audit hook used for
// create / cancel / pause / resume / transfer / rate-change /
// param-update. Withdraws are intentionally NOT audited (they're
// high-frequency receiver actions; the running total is on the
// stream row itself).
type AuditKeeper interface {
	RecordStreamPayAction(ctx sdk.Context, streamID uint64, action, actor, subject, detail string)
}
