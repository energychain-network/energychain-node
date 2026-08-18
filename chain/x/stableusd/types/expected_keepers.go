package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper is the x/identity surface stableusd consults on every
// transfer-like operation. All methods are read-only except RecordAction
// (append-only audit). It is nil-safe at the keeper boundary: when not
// wired, stableusd falls back to its own per-denom flag checks only.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
	RequireKYC(ctx sdk.Context, addr string) error
	EvaluateTransfer(ctx sdk.Context, module, assetRef, policyID, from, to string, amount uint64) error
	RecordAction(ctx sdk.Context, module, action, actor, subject, detail string)
}

// ReserveKeeper is the x/assethub oracle surface used to gate mints
// against an attested reserve. ok is false when the topic is unknown or
// below its min_sources threshold, so callers fail closed.
type ReserveKeeper interface {
	GetTopicValue(ctx context.Context, topicID string) (value uint64, updatedAt int64, ok bool)
}
