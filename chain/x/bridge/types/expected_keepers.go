package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper mirrors x/identity for lock/release sanctions gating.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
}

// SettlementKeeper mirrors x/stableusd: the mint/burn bridge issues the native
// stablecoin 1:1 against attested off-chain collateral on inbound release and
// destroys it on outbound lock.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	// DenomActive reports the denom is registered and ACTIVE (not PAUSED or
	// RETIRED) so the bridge cannot move settlement the ledger has halted.
	DenomActive(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	// BridgeMint issues `amount` to `recipient` on inbound release. ALWAYS
	// reserve-gated by x/stableusd: issuance can never exceed the attested
	// off-chain collateral, bounding loss even on a full attestor compromise.
	BridgeMint(ctx context.Context, denomID, recipient string, amount uint64) error
	// BridgeBurn destroys `amount` of `holder`'s balance on outbound lock.
	BridgeBurn(ctx context.Context, denomID, holder string, amount uint64) error
}
