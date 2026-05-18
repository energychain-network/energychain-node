package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 256
	ReasonMaxLen = 256
	DenomMaxLen  = 64

	// PriceScale is the fixed-point unit for `price` in micro-
	// units of quote_denom per unit-of-base_denom.
	PriceScaleDecimals = 6
	PriceScale         = uint64(1_000_000)

	// MaxPrice is the sentinel used to invert keys for the buy
	// book so that highest-price-first iteration is achieved by
	// natural ascending iteration on (MaxPrice - price). Sized
	// so any realistic price comfortably fits inside, while
	// leaving the top of the uint64 range free.
	MaxPrice = uint64(1 << 60)

	DefaultMaxPairs                       uint32 = 1_000
	DefaultMaxOpenOrdersPerPair           uint32 = 50_000
	DefaultMaxOpenOrdersPerUserPerPair    uint32 = 500
	DefaultMaxFillsPerMatch               uint32 = 100
	DefaultMaxMatchesPerClear             uint32 = 1_000
	DefaultMemoMaxLen                     uint32 = MemoMaxLen

	HardMaxPairs                       uint32 = 100_000
	HardMaxOpenOrdersPerPair           uint32 = 10_000_000
	HardMaxOpenOrdersPerUserPerPair    uint32 = 100_000
	HardMaxFillsPerMatch               uint32 = 100_000
	HardMaxMatchesPerClear             uint32 = 1_000_000
	HardMemoMaxLen                     uint32 = 4096
)

func ValidateMemo(s string, maxLen uint32) error {
	if maxLen == 0 {
		maxLen = MemoMaxLen
	}
	if uint32(len(s)) > maxLen {
		return fmt.Errorf("memo too long (max %d)", maxLen)
	}
	return nil
}
func ValidateReason(s string) error {
	if len(s) > ReasonMaxLen {
		return fmt.Errorf("reason too long (max %d)", ReasonMaxLen)
	}
	return nil
}
func ValidateDenom(s string) error {
	if s == "" {
		return fmt.Errorf("denom must be non-empty")
	}
	if len(s) > DenomMaxLen {
		return fmt.Errorf("denom too long (max %d)", DenomMaxLen)
	}
	return nil
}
func ValidateAddr(field, s string) error {
	if s == "" {
		return fmt.Errorf("%s must be non-empty", field)
	}
	if _, err := sdk.AccAddressFromBech32(s); err != nil {
		return fmt.Errorf("%s %q invalid bech32: %w", field, s, err)
	}
	return nil
}

func SideValid(s Side) bool { return s == Side_SIDE_BUY || s == Side_SIDE_SELL }
func ModeValid(m MatchMode) bool {
	return m == MatchMode_MATCH_MODE_CONTINUOUS || m == MatchMode_MATCH_MODE_FBA
}
func PairStatusValid(s PairStatus) bool {
	return s == PairStatus_PAIR_STATUS_ACTIVE || s == PairStatus_PAIR_STATUS_PAUSED
}
func OrderStatusValid(s OrderStatus) bool {
	switch s {
	case OrderStatus_ORDER_STATUS_OPEN,
		OrderStatus_ORDER_STATUS_PARTIALLY_FILLED,
		OrderStatus_ORDER_STATUS_FILLED,
		OrderStatus_ORDER_STATUS_CANCELLED:
		return true
	}
	return false
}
func (s OrderStatus) IsTerminal() bool {
	return s == OrderStatus_ORDER_STATUS_FILLED || s == OrderStatus_ORDER_STATUS_CANCELLED
}

// SafeMulUint64 guards `price * quantity` from overflow. The
// chain refuses orders whose implied notional would exceed
// uint64 because the BUY-leg escrow accounting cannot otherwise
// represent it; SELL-leg escrow is in base units so this only
// affects BUY-side calculations.
func SafeMul(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	if a > ^uint64(0)/b {
		return 0, true
	}
	return a * b, false
}

func SafeAdd(x, y uint64) (uint64, error) {
	if y > 0 && x > ^uint64(0)-y {
		return 0, fmt.Errorf("uint64 overflow: %d + %d", x, y)
	}
	return x + y, nil
}
func SafeSub(x, y uint64) (uint64, error) {
	if y > x {
		return 0, fmt.Errorf("uint64 underflow: %d - %d", x, y)
	}
	return x - y, nil
}

// QuoteForFill returns the quote_denom amount transferred at
// `price` for `qty` of base_denom. Uses fixed-point PriceScale
// division — i.e. price=1.5 (= 1_500_000 micro) on qty=10
// produces 15 quote units.
func QuoteForFill(price, qty uint64) (uint64, error) {
	raw, overflow := SafeMul(price, qty)
	if overflow {
		return 0, fmt.Errorf("price*qty overflows uint64: %d * %d", price, qty)
	}
	return raw / PriceScale, nil
}
