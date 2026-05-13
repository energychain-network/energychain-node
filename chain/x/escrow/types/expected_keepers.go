package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the address-only blacklist gate. Used for
// every state transition that pays out funds (Release / Refund /
// Cancel / Arbiter*) and at Fund time on the depositor.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the cross-module hop x/escrow takes for
// stablecoin-backed escrows. Implementations move per-account
// balances atomically; the escrow module enforces sanctions and
// status checks before invoking Move.
//
// HasDenom is consulted at CreateEscrow so an escrow cannot be
// opened against a non-existent denom.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
	// IsAccountBlocked returns true when the per-denom flags on the
	// account would refuse a transfer (frozen OR blacklisted). It is
	// the escrow module's defensive bridge against the fact that
	// Move bypasses x/stablecoin's own ComplianceCheck pipeline —
	// without re-checking here, escrows could be used as a laundromat
	// to unblock frozen / blacklisted accounts.
	IsAccountBlocked(ctx sdk.Context, denomID, account string) bool
	// IsDenomPaused returns true when the denom is paused / retired
	// at the stablecoin layer. Pay-in (Fund) is refused for paused
	// denoms; pay-out (release / refund / arbiter overrides) is
	// allowed so in-flight escrows can wind down even after a pause.
	IsDenomPaused(ctx sdk.Context, denomID string) bool
}

// RWAKeeper is the cross-module hop x/escrow takes for RWA-token
// escrows. Lock and Release are the two narrow primitives the
// escrow module needs:
//
//   - EscrowLock debits the depositor's transferable balance into
//     the escrow module pool. The implementation MUST honor
//     transferable (frozen + lockup) and refuse paused / terminated
//     tokens.
//   - EscrowRelease debits the pool and credits the recipient with
//     full holder-side compliance (sanctions / KYC / per-holder
//     cap). The implementation MAY allow a TERMINATED token's
//     residual escrow to be released so winding down is possible,
//     but MUST refuse PAUSED tokens.
//
// HasToken is consulted at CreateEscrow to fail-fast on bad IDs.
type RWAKeeper interface {
	HasToken(ctx sdk.Context, tokenID uint64) bool
	EscrowLock(ctx sdk.Context, tokenID uint64, depositor string, amount uint64) error
	EscrowRelease(ctx sdk.Context, tokenID uint64, recipient string, amount uint64) error
}

// OracleKeeper is the optional cross-module oracle gate. When an
// escrow's Triggers.OracleTopicId is non-empty, Release demands the
// latest aggregated value be >= OracleMinValue and (when a max
// staleness is set) the attestation timestamp be within the
// freshness window. Returning ok=false is treated as
// "value missing" and gates Release.
type OracleKeeper interface {
	GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool)
}

// AuditKeeper is the optional cross-module audit hook the escrow
// keeper uses for every state-changing transition (create / fund /
// approve / release / refund / cancel / dispute mark / dispute
// resolve / arbiter override). Action strings are stable and
// machine-readable; subject identifies the affected counterparty
// (beneficiary on release, fallback on refund, etc.).
type AuditKeeper interface {
	RecordEscrowAction(ctx sdk.Context, escrowID uint64, action, actor, subject, detail string)
}
