package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper mirrors x/identity for stream-leg sanctions gating.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
}

// SettlementKeeper mirrors x/stableusd: the reserve ledger streams escrow
// into and out of.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error
}

// MincastKeeper is the cron's view of x/mincast. InjectTreasuryFrom funds a
// market treasury from `funder` (lifting the floor); CloseMaturedInvests
// settles up to `limit` matured invests of a market.
type MincastKeeper interface {
	HasMarket(ctx context.Context, marketID uint64) bool
	InjectTreasuryFrom(ctx context.Context, funder string, marketID, amount uint64) error
	CloseMaturedInvests(ctx context.Context, caller string, marketID uint64, limit uint32) (uint32, error)
}

// RWAKeeper is the cron's view of x/rwatoken. The acting `admin` must be the
// token admin (enforced inside x/rwatoken).
type RWAKeeper interface {
	HasToken(ctx context.Context, tokenID uint64) bool
	SnapshotHolders(ctx context.Context, admin string, tokenID uint64) (uint64, error)
	DistributeDividend(ctx context.Context, admin string, tokenID, snapshotID, amount uint64) (uint64, error)
}
