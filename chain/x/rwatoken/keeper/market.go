package keeper

import (
	"context"

	"energychain/x/rwatoken/types"
)

// This file exposes the compliant RWA-unit movement primitives that x/market
// needs to run a secondary order book over a security token (e.g. MYSOLAR).
//
// The market escrow account is a module-owned bech32 slot. Crediting it during
// a SELL placement is exempt from KYC and the per-holder cap (it is not a real
// investor and never accumulates beyond in-flight matching). Delivery to a
// BUYER, by contrast, runs the full receiver compliance gate (token status,
// freeze, sanctions, KYC, transfer policy) and the per-holder cap — so a
// secondary trade can never seat units in a non-eligible holder.

// MarketHasToken reports whether tokenID exists (CreateMarket validation).
func (k Keeper) MarketHasToken(ctx context.Context, tokenID uint64) bool {
	return k.HasToken(ctx, tokenID)
}

// MarketTokenTradable reports whether the token is ACTIVE and thus eligible
// for secondary trading. PAUSED/MATURED tokens are not tradable.
func (k Keeper) MarketTokenTradable(ctx context.Context, tokenID uint64) bool {
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil || !ok {
		return false
	}
	return t.Status == types.TokenStatus_TOKEN_STATUS_ACTIVE
}

// MarketTokenAdmin returns the token's admin (issuer). x/market requires the
// listing operator of an rwa/<id> market to be exactly this account, so only
// the issuer can post the listing bond and receive its refund.
func (k Keeper) MarketTokenAdmin(ctx context.Context, tokenID uint64) (string, bool) {
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil || !ok {
		return "", false
	}
	return t.Admin, true
}

// MarketSettlementDenom returns the token's settlement denom so the market can
// require the quote leg to match (price discovery happens in the token's own
// stablecoin, the same one dividends/redemptions settle in).
func (k Keeper) MarketSettlementDenom(ctx context.Context, tokenID uint64) (string, bool) {
	return k.TokenSettlementDenom(ctx, tokenID)
}

// MarketUnitBalance returns a holder's tradable unit balance (used for UI/API
// available-to-sell hints and is the authority the indexer re-queries).
func (k Keeper) MarketUnitBalance(ctx context.Context, tokenID uint64, holder string) uint64 {
	bal, _ := k.GetBalance(ctx, tokenID, holder)
	return bal
}

// MarketReceiverOK prechecks whether `to` can receive `amount` units right now
// (full receiver compliance + per-holder cap headroom). x/market calls this at
// order placement and again before batch settlement so a non-deliverable BUY
// order is rejected/voided rather than reverting the whole uniform-price batch.
func (k Keeper) MarketReceiverOK(ctx context.Context, tokenID uint64, to string, amount uint64) error {
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrNotFound.Wrapf("token %d", tokenID)
	}
	if err := k.complianceCheck(ctx, t, "", to, amount, false); err != nil {
		return err
	}
	return k.requirePerHolderCap(ctx, t, to, amount)
}

// MarketEscrowUnits moves a seller's units into the market escrow slot at
// placement. The seller divests, so it is gated as the `from` party (token
// status, freeze, sanctions, transfer policy); the escrow credit is exempt
// from KYC/cap because the slot is module-owned.
func (k Keeper) MarketEscrowUnits(ctx context.Context, tokenID uint64, from, escrow string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrNotFound.Wrapf("token %d", tokenID)
	}
	if err := k.complianceCheck(ctx, t, from, "", amount, false); err != nil {
		return err
	}
	return k.moveUnits(ctx, tokenID, from, escrow, amount)
}

// MarketReleaseUnits delivers escrowed units to a filled buyer with the full
// receiver compliance gate and per-holder cap. `from` is empty so only the
// receiving buyer is gated (the escrow slot is module-owned), mirroring how a
// dividend receipt is gated on the recipient only.
func (k Keeper) MarketReleaseUnits(ctx context.Context, tokenID uint64, escrow, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	t, ok, err := k.GetToken(ctx, tokenID)
	if err != nil {
		return err
	}
	if !ok {
		return types.ErrNotFound.Wrapf("token %d", tokenID)
	}
	if err := k.complianceCheck(ctx, t, "", to, amount, false); err != nil {
		return err
	}
	if err := k.requirePerHolderCap(ctx, t, to, amount); err != nil {
		return err
	}
	return k.moveUnits(ctx, tokenID, escrow, to, amount)
}

// MarketRefundUnits returns escrowed units to the original seller on cancel or
// on a market clear failure. The seller already held these units, so the path
// only moves them back without a per-holder-cap check (returning their own
// units must never strand escrow).
func (k Keeper) MarketRefundUnits(ctx context.Context, tokenID uint64, escrow, to string, amount uint64) error {
	if amount == 0 {
		return nil
	}
	return k.moveUnits(ctx, tokenID, escrow, to, amount)
}
