package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// SettlementKeeper is the x/stableusd surface offering uses to escrow
// subscriptions, release tranches, pay yields, and refund. Funds live in
// per-offering module accounts inside the stableusd ledger.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error
}

// RWAKeeper is the x/rwatoken surface offering uses to authorize the issuer
// against the underlying token, learn its KYC requirement, and allocate
// units to investors on a successful raise.
type RWAKeeper interface {
	TokenAdmin(ctx context.Context, tokenID uint64) (string, bool)
	TokenSettlementDenom(ctx context.Context, tokenID uint64) (string, bool)
	TokenRequiresKYC(ctx context.Context, tokenID uint64) (bool, bool)
	AllocateUnits(ctx context.Context, tokenID uint64, caller, recipient string, amount uint64) error
}

// ComplianceKeeper mirrors x/identity: a primary-market subscription moves
// real capital and later mints a (potentially KYC-gated) security, so the
// investor must clear sanctions — and KYC when the token requires it —
// before subscribing, and capital must never be paid back out to a
// sanctioned party. It is nil-safe at the keeper boundary.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
	RequireKYC(ctx sdk.Context, addr string) error
	RecordAction(ctx sdk.Context, module, action, actor, subject, detail string)
}
