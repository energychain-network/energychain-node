package types

import (
	"fmt"
	"math/big"
	"regexp"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	DenomMaxLen = 64
	NameMaxLen  = 128
	Bps         = 10_000

	DefaultMaxMarkets        uint32 = 1024
	DefaultMaxFeeBps         uint32 = 1000   // 10%
	DefaultMaxApyBps         uint32 = 50_000 // 500%
	DefaultMinInitialPrice   uint64 = 1
	DefaultYearSeconds       int64  = 31_536_000 // 365d
	DefaultMaxInvestTermSecs int64  = 5 * 31_536_000

	HardMaxMarkets uint32 = 1_048_576
	HardMaxFeeBps  uint32 = Bps        // a fee may never exceed 100%
	HardMaxApyBps  uint32 = 10_000_000 // sanity ceiling
)

var denomRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9/:._-]{1,63}$`)

func ValidateDenom(d string) error {
	if !denomRe.MatchString(d) {
		return errorsmod.Wrapf(ErrInvalidField, "denom %q invalid", d)
	}
	return nil
}

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateMaxLen(field, v string, max int) error {
	if len(v) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s too long (max %d)", field, max)
	}
	return nil
}

func SafeAdd(x, y uint64) (uint64, error) {
	if y > 0 && x > ^uint64(0)-y {
		return 0, errorsmod.Wrapf(ErrOverflow, "%d + %d", x, y)
	}
	return x + y, nil
}

func SafeSub(x, y uint64) (uint64, error) {
	if y > x {
		return 0, errorsmod.Wrapf(ErrOverflow, "underflow %d - %d", x, y)
	}
	return x - y, nil
}

// MulDivFloor returns floor(a*b/denom) computed over big.Int (no
// intermediate overflow), erroring on denom==0 or a uint64-overflowing
// quotient. This is the curve and pro-rata kernel; flooring guarantees
// outputs never exceed the exact rational value, which is what keeps the
// floor price monotonic non-decreasing.
func MulDivFloor(a, b, denom uint64) (uint64, error) {
	if denom == 0 {
		return 0, errorsmod.Wrap(ErrOverflow, "division by zero")
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	prod := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	q := prod.Quo(prod, new(big.Int).SetUint64(denom))
	if !q.IsUint64() {
		return 0, errorsmod.Wrapf(ErrOverflow, "floor(%d*%d/%d) overflows uint64", a, b, denom)
	}
	return q.Uint64(), nil
}

// MulBps returns floor(x*bps/10000).
func MulBps(x uint64, bps uint32) (uint64, error) {
	return MulDivFloor(x, uint64(bps), Bps)
}

// FloorPrice is the redeemable settlement backing per unit: treasury/supply
// (floored), or 0 when there is no supply.
func FloorPrice(treasury, supply uint64) uint64 {
	if supply == 0 {
		return 0
	}
	return treasury / supply
}

// MintUnits returns the mincast units a post-fee principal buys. On a
// bootstrap mint (supply==0) it prices at initialPrice; otherwise it prices
// at the current floor via floor(principal*supply/treasury). Because the
// units are floored and the FULL pay (principal+fee) is added to treasury,
// the post-mint floor (treasury+pay)/(supply+units) is always >= the prior
// floor.
func MintUnits(treasury, supply, initialPrice, principal uint64) (uint64, error) {
	if principal == 0 {
		return 0, nil
	}
	if supply == 0 {
		if initialPrice == 0 {
			return 0, errorsmod.Wrap(ErrInvalidField, "initial_price must be > 0 for bootstrap mint")
		}
		return principal / initialPrice, nil
	}
	if treasury == 0 {
		return 0, errorsmod.Wrap(ErrOverflow, "non-empty supply with empty treasury")
	}
	return MulDivFloor(principal, supply, treasury)
}

// MeltGross returns the gross settlement owed for burning `units` at the
// current floor: floor(units*treasury/supply). Flooring keeps the post-melt
// floor >= the prior floor.
func MeltGross(treasury, supply, units uint64) (uint64, error) {
	if units == 0 {
		return 0, nil
	}
	if units > supply {
		return 0, errorsmod.Wrapf(ErrInsufficient, "melt %d > supply %d", units, supply)
	}
	if supply == 0 {
		return 0, errorsmod.Wrap(ErrMarketState, "empty supply")
	}
	return MulDivFloor(units, treasury, supply)
}

// ProrateYield computes the settlement yield for an invest: the floor-valued
// principal times the annual rate, prorated by term/year. Two floored
// steps bias the result down (favouring the reward pool).
func ProrateYield(principalValue uint64, apyBps uint32, termSeconds, yearSeconds int64) (uint64, error) {
	if termSeconds <= 0 || yearSeconds <= 0 {
		return 0, errorsmod.Wrap(ErrInvalidField, "term and year must be > 0")
	}
	annual, err := MulBps(principalValue, apyBps)
	if err != nil {
		return 0, err
	}
	return MulDivFloor(annual, uint64(termSeconds), uint64(yearSeconds))
}

func MarketStatusValid(s MarketStatus) bool {
	switch s {
	case MarketStatus_MARKET_STATUS_ACTIVE, MarketStatus_MARKET_STATUS_PAUSED, MarketStatus_MARKET_STATUS_CLOSED:
		return true
	}
	return false
}

func DefaultParams() Params {
	return Params{
		MaxMarkets:            DefaultMaxMarkets,
		MaxFeeBps:             DefaultMaxFeeBps,
		MaxApyBps:             DefaultMaxApyBps,
		MaxInvestTermSeconds:  DefaultMaxInvestTermSecs,
		MinInitialPrice:       DefaultMinInitialPrice,
		YearSeconds:           DefaultYearSeconds,
		RequireSanctionsClear: true,
	}
}

func (p Params) Validate() error {
	if p.MaxMarkets == 0 || p.MaxMarkets > HardMaxMarkets {
		return fmt.Errorf("max_markets must be in (0, %d]", HardMaxMarkets)
	}
	if p.MaxFeeBps > HardMaxFeeBps {
		return fmt.Errorf("max_fee_bps must be <= %d", HardMaxFeeBps)
	}
	if p.MaxApyBps == 0 || p.MaxApyBps > HardMaxApyBps {
		return fmt.Errorf("max_apy_bps must be in (0, %d]", HardMaxApyBps)
	}
	if p.MaxInvestTermSeconds <= 0 {
		return fmt.Errorf("max_invest_term_seconds must be > 0")
	}
	if p.MinInitialPrice == 0 {
		return fmt.Errorf("min_initial_price must be > 0")
	}
	if p.YearSeconds <= 0 {
		return fmt.Errorf("year_seconds must be > 0")
	}
	return nil
}
