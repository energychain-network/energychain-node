package types

import (
	"fmt"
	"regexp"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	DenomIDMaxLen  = 64
	SymbolMaxLen   = 32
	PolicyIDMaxLen = 64
	TopicIDMaxLen  = 128
	MemoMaxLen     = 256
	ReasonMaxLen   = 512
	MaxDecimals    = 18
	PegCurrencyLen = 16
	BpsDenominator = 10_000

	DefaultMaxDenoms                      uint32 = 256
	DefaultMaxMintersPerDenom             uint32 = 32
	DefaultMaxPendingRedemptionsPerHolder uint32 = 64

	HardMaxDenoms             uint32 = 65_536
	HardMaxMintersPerDenom    uint32 = 256
	HardMaxPendingRedemptions uint32 = 4_096
)

var (
	denomIDRe  = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,63}$`)
	symbolRe   = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	policyIDRe = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,63}$`)
	topicIDRe  = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,127}$`)
	pegCurRe   = regexp.MustCompile(`^[A-Z]{1,16}$`)
)

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateDenomID(id string) error {
	if !denomIDRe.MatchString(id) {
		return errorsmod.Wrapf(ErrInvalidField, "denom id %q must match [a-z][a-z0-9._/-]{0,63}", id)
	}
	return nil
}

func ValidateSymbol(s string) error {
	if !symbolRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "symbol %q invalid", s)
	}
	return nil
}

func ValidateOptionalPolicyID(s string) error {
	if s == "" {
		return nil
	}
	if !policyIDRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "policy_id %q invalid", s)
	}
	return nil
}

func ValidateOptionalTopicID(s string) error {
	if s == "" {
		return nil
	}
	if !topicIDRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "reserve_topic %q invalid", s)
	}
	return nil
}

func ValidateOptionalPegCurrency(s string) error {
	if s == "" {
		return nil
	}
	if !pegCurRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "peg_currency %q must be 1-16 uppercase letters", s)
	}
	return nil
}

func ValidateMaxLen(field, v string, max int) error {
	if len(v) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s too long (max %d)", field, max)
	}
	return nil
}

func DenomStatusValid(s DenomStatus) bool {
	switch s {
	case DenomStatus_DENOM_STATUS_ACTIVE, DenomStatus_DENOM_STATUS_PAUSED, DenomStatus_DENOM_STATUS_RETIRED:
		return true
	}
	return false
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

// CoverageOK reports whether reserve covers outstanding at the required
// ratio. required_ratio_bps is the fraction of outstanding that must be
// backed: 10000 = 100% (reserve >= outstanding), 15000 = 150%
// over-collateralized. The check is reserve*10000 >= outstanding*ratioBps,
// i.e. outstanding*ratioBps <= reserve*10000, computed with 128-bit
// intermediates so neither product can overflow.
func CoverageOK(outstanding, reserve uint64, ratioBps uint32) bool {
	lhs := mul128(outstanding, uint64(ratioBps))
	rhs := mul128(reserve, BpsDenominator)
	return lhs.lte(rhs)
}

func DefaultParams() Params {
	return Params{
		MaxDenoms:                      DefaultMaxDenoms,
		MaxMintersPerDenom:             DefaultMaxMintersPerDenom,
		MintRequiresReserve:            false,
		RequireKyc:                     false,
		MaxPendingRedemptionsPerHolder: DefaultMaxPendingRedemptionsPerHolder,
	}
}

func (p Params) Validate() error {
	if p.MaxDenoms == 0 || p.MaxDenoms > HardMaxDenoms {
		return fmt.Errorf("max_denoms must be in (0, %d]", HardMaxDenoms)
	}
	if p.MaxMintersPerDenom == 0 || p.MaxMintersPerDenom > HardMaxMintersPerDenom {
		return fmt.Errorf("max_minters_per_denom must be in (0, %d]", HardMaxMintersPerDenom)
	}
	if p.MaxPendingRedemptionsPerHolder == 0 || p.MaxPendingRedemptionsPerHolder > HardMaxPendingRedemptions {
		return fmt.Errorf("max_pending_redemptions_per_holder must be in (0, %d]", HardMaxPendingRedemptions)
	}
	return nil
}

// ---- 128-bit helper -------------------------------------------------------

type u128 struct{ hi, lo uint64 }

func mul128(a, b uint64) u128 {
	const mask = 0xffffffff
	a0, a1 := a&mask, a>>32
	b0, b1 := b&mask, b>>32
	lo := a0 * b0
	mid1 := a1 * b0
	mid2 := a0 * b1
	hi := a1 * b1
	carry := (lo >> 32) + (mid1 & mask) + (mid2 & mask)
	loFinal := (lo & mask) | (carry&mask)<<32
	hiFinal := hi + (mid1 >> 32) + (mid2 >> 32) + (carry >> 32)
	return u128{hi: hiFinal, lo: loFinal}
}

func (x u128) lte(y u128) bool {
	if x.hi != y.hi {
		return x.hi < y.hi
	}
	return x.lo <= y.lo
}
