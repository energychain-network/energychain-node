package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper mirrors x/identity: sanctions, KYC, and the policy DSL
// gate the parties of mints/melts/transfers and invest open/redeem.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
	RequireKYC(ctx sdk.Context, addr string) error
	EvaluateTransfer(ctx sdk.Context, module, assetRef, policyID, from, to string, amount uint64) error
	RecordAction(ctx sdk.Context, module, action, actor, subject, detail string)
}

// SettlementKeeper mirrors x/stableusd: the reserve stablecoin ledger that
// mincast escrows into per-market treasury and reward module accounts.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error
}
