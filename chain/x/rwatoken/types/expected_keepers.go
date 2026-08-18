package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper is the x/identity surface rwatoken consults on every
// transfer-like operation. All methods are read-only except RecordAction
// (append-only audit). It is nil-safe at the keeper boundary: when not
// wired, rwatoken falls back to its own per-token freeze + KYC flag checks.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
	RequireKYC(ctx sdk.Context, addr string) error
	EvaluateTransfer(ctx sdk.Context, module, assetRef, policyID, from, to string, amount uint64) error
	RecordAction(ctx sdk.Context, module, action, actor, subject, detail string)
}

// AssetHubKeeper is the x/assethub surface rwatoken consults to validate
// device bindings: a token may only declare device_ids it can prove it
// operates. It is nil-safe at the keeper boundary — when not wired (e.g.
// in unit tests) device validation is skipped and the ids are stored
// as-is. In production it is always wired so the on-chain asset is
// provably anchored to real, attested hardware.
type AssetHubKeeper interface {
	// DeviceOperator reports a device's operator (bech32), whether it is
	// ACTIVE, and whether it exists at all.
	DeviceOperator(ctx context.Context, deviceID string) (operator string, active bool, found bool)
}

// SettlementKeeper is the x/stableusd surface used to pay dividends and
// redemptions in a stable denom. The token pool is a module-derived
// account that holds the settlement denom inside the stableusd ledger;
// dividends and buy-backs draw from it. It is nil-safe: when not wired,
// settlement operations (CreateDistribution, FundPool, redemption
// execution) are refused with ErrSettlement.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error
}
