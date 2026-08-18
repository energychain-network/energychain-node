package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ComplianceKeeper mirrors x/identity: order owners are gated on sanctions
// always, plus KYC and a bound transfer policy when the market opts in. It
// is nil-safe at the keeper boundary.
type ComplianceKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
	RequireKYC(ctx sdk.Context, addr string) error
	EvaluateTransfer(ctx sdk.Context, module, assetRef, policyID, from, to string, amount uint64) error
}

// SettlementKeeper mirrors x/stableusd: orders escrow and settle through the
// reserve ledger. The quote leg is always a settlement denom; the base leg is
// a settlement denom for stable/stable markets.
type SettlementKeeper interface {
	HasDenom(ctx context.Context, denomID string) bool
	DenomActive(ctx context.Context, denomID string) bool
	GetBalance(ctx context.Context, denomID, account string) uint64
	MoveBalance(ctx context.Context, denomID, from, to string, amount uint64) error
}

// RWAAssetKeeper mirrors x/rwatoken: it lets a security token trade as a
// market's base asset against a stablecoin quote. Base units escrow/settle
// through the rwatoken ledger with the full receiver compliance gate and
// per-holder cap enforced on delivery to a buyer; the module escrow slot is
// exempt. It is nil-safe at the keeper boundary — stable/stable markets never
// reach it. A market opts in by setting base_denom to "rwa/<tokenID>".
// BankKeeper is the minimal x/bank surface for the listing-bond escrow: the
// operator's native-denom bond moves into the module account on MsgPostBond
// and back out on MsgDelistMarket. GetBalance backs the genesis
// bond-vs-module-account reconciliation.
type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

type RWAAssetKeeper interface {
	MarketHasToken(ctx context.Context, tokenID uint64) bool
	MarketTokenTradable(ctx context.Context, tokenID uint64) bool
	// MarketTokenAdmin returns the token's admin (issuer); RWA-base markets
	// require the listing operator to be exactly this account.
	MarketTokenAdmin(ctx context.Context, tokenID uint64) (string, bool)
	MarketSettlementDenom(ctx context.Context, tokenID uint64) (string, bool)
	MarketUnitBalance(ctx context.Context, tokenID uint64, holder string) uint64
	MarketReceiverOK(ctx context.Context, tokenID uint64, to string, amount uint64) error
	MarketEscrowUnits(ctx context.Context, tokenID uint64, from, escrow string, amount uint64) error
	MarketReleaseUnits(ctx context.Context, tokenID uint64, escrow, to string, amount uint64) error
	MarketRefundUnits(ctx context.Context, tokenID uint64, escrow, to string, amount uint64) error
}
