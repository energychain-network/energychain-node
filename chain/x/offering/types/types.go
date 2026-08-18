package types

import (
	"fmt"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	ReasonMaxLen = 512

	DefaultMaxOfferings uint32 = 4096
	DefaultMaxTranches  uint32 = 60

	HardMaxOfferings uint32 = 1_048_576
	HardMaxTranches  uint32 = 1024
)

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

func SafeMul(x, y uint64) (uint64, error) {
	if x == 0 || y == 0 {
		return 0, nil
	}
	if x > ^uint64(0)/y {
		return 0, errorsmod.Wrapf(ErrOverflow, "%d * %d", x, y)
	}
	return x * y, nil
}

// MulDivFloor returns floor(a*b/denom) over big.Int (no intermediate
// overflow), erroring on denom==0 or a uint64-overflowing quotient. It is
// the pro-rata kernel for yield distribution and default refunds; flooring
// guarantees the sum of shares never exceeds the funded pool.
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

// TrancheAmount returns the settlement to release for the 1-indexed tranche
// k of total tranches, splitting raised as evenly as possible with the
// final tranche absorbing the rounding remainder. The sum over k=1..total
// equals raised exactly.
func TrancheAmount(raised uint64, total, k uint32) uint64 {
	if total == 0 || k == 0 || k > total {
		return 0
	}
	per := raised / uint64(total)
	if k == total {
		return raised - per*uint64(total-1)
	}
	return per
}

// DueInjections is the number of APY injections the issuer should have
// completed by `now` after a successful close: one per elapsed
// injection_interval, capped at total_tranches. interval<=0 means no
// recurring obligation (never delinquent).
func DueInjections(now, succeededAt, interval int64, total uint32) uint32 {
	if interval <= 0 || now <= succeededAt {
		return 0
	}
	elapsed := now - succeededAt
	due := elapsed / interval
	if due < 0 {
		return 0
	}
	if due > int64(total) {
		return total
	}
	return uint32(due)
}

func OfferingStatusValid(s OfferingStatus) bool {
	switch s {
	case OfferingStatus_OFFERING_STATUS_OPEN,
		OfferingStatus_OFFERING_STATUS_SUCCEEDED,
		OfferingStatus_OFFERING_STATUS_FAILED,
		OfferingStatus_OFFERING_STATUS_CANCELLED,
		OfferingStatus_OFFERING_STATUS_DEFAULTED:
		return true
	}
	return false
}

func DefaultParams() Params {
	return Params{
		MaxOfferings: DefaultMaxOfferings,
		MaxTranches:  DefaultMaxTranches,
	}
}

func (p Params) Validate() error {
	if p.MaxOfferings == 0 || p.MaxOfferings > HardMaxOfferings {
		return fmt.Errorf("max_offerings must be in (0, %d]", HardMaxOfferings)
	}
	if p.MaxTranches == 0 || p.MaxTranches > HardMaxTranches {
		return fmt.Errorf("max_tranches must be in (0, %d]", HardMaxTranches)
	}
	return nil
}
