package keeper

import (
	"context"
	"strconv"
	"strings"

	"energychain/x/market/types"
)

// RWADenomPrefix marks a market's base asset as an x/rwatoken security token
// (rather than an x/stableusd settlement denom). The convention is
// "rwa/<tokenID>", e.g. "rwa/2". The quote leg is always a stablecoin.
const RWADenomPrefix = "rwa/"

// parseRWADenom returns the token id when denom is an "rwa/<id>" base asset.
func parseRWADenom(denom string) (uint64, bool) {
	if !strings.HasPrefix(denom, RWADenomPrefix) {
		return 0, false
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(denom, RWADenomPrefix), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// IsRWAMarket reports whether a market's base asset is an RWA security token.
func IsRWAMarket(m types.Market) bool {
	_, ok := parseRWADenom(m.BaseDenom)
	return ok
}

// baseEscrow locks a SELL order's base units into the market escrow account.
// For an RWA base it routes through the compliant rwatoken ledger; otherwise it
// uses the stableusd reserve ledger exactly as before.
func (k Keeper) baseEscrow(ctx context.Context, m types.Market, owner string, qty uint64) error {
	if id, ok := parseRWADenom(m.BaseDenom); ok {
		if k.asset == nil {
			return types.ErrSettlement.Wrap("rwa asset keeper not wired")
		}
		return k.asset.MarketEscrowUnits(ctx, id, owner, EscrowAccount(), qty)
	}
	return k.moveSettlement(ctx, m.BaseDenom, owner, EscrowAccount(), qty)
}

// baseDeliver releases matched base units from escrow to a filled buyer. For an
// RWA base this enforces the full receiver compliance gate and per-holder cap.
func (k Keeper) baseDeliver(ctx context.Context, m types.Market, to string, qty uint64) error {
	if id, ok := parseRWADenom(m.BaseDenom); ok {
		if k.asset == nil {
			return types.ErrSettlement.Wrap("rwa asset keeper not wired")
		}
		return k.asset.MarketReleaseUnits(ctx, id, EscrowAccount(), to, qty)
	}
	return k.moveSettlement(ctx, m.BaseDenom, EscrowAccount(), to, qty)
}

// baseRefund returns unmatched base escrow to the seller on cancel.
func (k Keeper) baseRefund(ctx context.Context, m types.Market, to string, qty uint64) error {
	if id, ok := parseRWADenom(m.BaseDenom); ok {
		if k.asset == nil {
			return types.ErrSettlement.Wrap("rwa asset keeper not wired")
		}
		return k.asset.MarketRefundUnits(ctx, id, EscrowAccount(), to, qty)
	}
	return k.moveSettlement(ctx, m.BaseDenom, EscrowAccount(), to, qty)
}

// baseBalance reports the spendable base balance of an account for the market's
// base asset, regardless of whether it is an RWA token or a settlement denom.
func (k Keeper) baseBalance(ctx context.Context, m types.Market, account string) uint64 {
	if id, ok := parseRWADenom(m.BaseDenom); ok {
		if k.asset == nil {
			return 0
		}
		return k.asset.MarketUnitBalance(ctx, id, account)
	}
	if k.settlement == nil {
		return 0
	}
	return k.settlement.GetBalance(ctx, m.BaseDenom, account)
}
