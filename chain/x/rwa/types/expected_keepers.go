package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// PolicyKeeper is the slim transfer-policy gate (DSL evaluation).
// asset_class is "rwa" so a single x/policy registry can multiplex
// rules across different RWA token IDs.
type PolicyKeeper interface {
	EvaluateTransfer(ctx sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error
}

// SanctionsKeeper is the address-only blacklist gate.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// StablecoinKeeper is the cross-module hop the rwa keeper takes for
// dividend payouts and redemption settlements. Implementations MUST
// move per-account balances atomically and reject moves whose `from`
// account is sanctioned / frozen at the stablecoin layer.
type StablecoinKeeper interface {
	HasDenom(ctx sdk.Context, denomID string) bool
	// Move debits `amount` from `from` and credits the same amount to
	// `to` on the named denom. Pre-conditions (balance, freeze,
	// sanctions on either side) are the implementation's
	// responsibility.
	Move(ctx sdk.Context, denomID, from, to string, amount uint64) error
}

// AuditKeeper is the optional cross-module audit hook the rwa keeper
// uses for force-transfers, freezes, mints, burns, and redemptions.
type AuditKeeper interface {
	RecordRWAAction(ctx sdk.Context, tokenID uint64, issuerID, action, actor, subject, detail string)
}
