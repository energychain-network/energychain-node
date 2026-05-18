package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the chain-level sanctions read.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the cross-module funding hop for the
// contract margin pool. The IsAccountBlocked / IsDenomPaused
// reads are the bridge against the MoveBalance ComplianceCheck
// bypass (the same gate we re-use across x/escrow, x/scheduler,
// x/streampay).
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// OracleKeeper is the price / quantity index read used at
// settlement time. Returns the aggregated value plus the
// timestamp it was computed at (for staleness checks). Fail-
// closed: ok=false MUST fail the calling settlement.
type OracleKeeper interface {
	GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool)
}

// AuditKeeper is the optional cross-module audit hook used for
// create / sign / settle / default / terminate / dispute hooks.
// Margin deposit / withdraw are NOT audited (high-frequency,
// tracked on the row).
type AuditKeeper interface {
	RecordContractAction(ctx sdk.Context, contractID uint64, action, actor, subject, detail string)
}
