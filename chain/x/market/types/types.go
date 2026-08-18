package types

import (
	"fmt"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// PriceScale fixes the price fixed-point: a price of `PriceScale` means 1 quote
// unit per 1 base unit. quote_notional = price * base_qty / PriceScale.
const PriceScale uint64 = 1_000_000

const (
	DenomMaxLen = 64

	DefaultMaxMarkets             uint32 = 1024
	DefaultMaxOpenOrdersPerMarket uint32 = 10000
	DefaultMaxFillsPerBatch       uint32 = 1000
	DefaultMaxFeeBps              uint32 = 1000 // 10%
	DefaultMinBatchInterval       int64  = 1

	HardMaxMarkets uint32 = 1 << 20
	HardMaxFeeBps  uint32 = 10000
	BpsDenominator uint64 = 10000

	// DefaultListingBondDenom is the native staking denom the market
	// operator escrows to open a newly created market for trading.
	DefaultListingBondDenom = "uecy"
)

// DefaultListingBond is 10,000 ECY in 18-decimal base units. Governance
// tunes it via MsgUpdateParams; 0 disables the bond gate.
func DefaultListingBond() sdkmath.Int { return sdkmath.NewIntWithDecimal(10_000, 18) }

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateDenom(d string) error {
	if l := len(d); l < 1 || l > DenomMaxLen {
		return errorsmod.Wrapf(ErrInvalidField, "denom length %d out of range", l)
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

// MulDivFloor returns floor(a*b/d) computed in 256-bit space (no uint64
// overflow). d must be > 0.
func MulDivFloor(a, b, d uint64) (uint64, error) {
	if d == 0 {
		return 0, errorsmod.Wrap(ErrOverflow, "division by zero")
	}
	prod := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	q := prod.Div(prod, new(big.Int).SetUint64(d))
	if !q.IsUint64() {
		return 0, errorsmod.Wrapf(ErrOverflow, "muldiv overflow %d*%d/%d", a, b, d)
	}
	return q.Uint64(), nil
}

// MulDivCeil returns ceil(a*b/d) in 256-bit space. d must be > 0.
func MulDivCeil(a, b, d uint64) (uint64, error) {
	if d == 0 {
		return 0, errorsmod.Wrap(ErrOverflow, "division by zero")
	}
	prod := new(big.Int).Mul(new(big.Int).SetUint64(a), new(big.Int).SetUint64(b))
	bigD := new(big.Int).SetUint64(d)
	q := new(big.Int).Add(prod, new(big.Int).Sub(bigD, big.NewInt(1)))
	q.Div(q, bigD)
	if !q.IsUint64() {
		return 0, errorsmod.Wrapf(ErrOverflow, "mulceil overflow %d*%d/%d", a, b, d)
	}
	return q.Uint64(), nil
}

// QuoteFloor is the clearing quote owed for `qty` base units at `price`,
// rounded down. Used cumulatively for telescoping settlement.
func QuoteFloor(price, qty uint64) (uint64, error) { return MulDivFloor(price, qty, PriceScale) }

// QuoteCeil is the quote a BUY order must escrow to cover `qty` at `price`,
// rounded up so the order is always fully funded at its limit.
func QuoteCeil(price, qty uint64) (uint64, error) { return MulDivCeil(price, qty, PriceScale) }

func OrderSideValid(s OrderSide) bool {
	return s == OrderSide_ORDER_SIDE_BUY || s == OrderSide_ORDER_SIDE_SELL
}

func OrderStatusValid(s OrderStatus) bool {
	switch s {
	case OrderStatus_ORDER_STATUS_OPEN, OrderStatus_ORDER_STATUS_FILLED, OrderStatus_ORDER_STATUS_CANCELLED:
		return true
	}
	return false
}

// MarketStatusValid reports whether s is a status governance may SET via
// MsgSetMarketStatus. PENDING_BOND is only entered at creation and DELISTED
// only through MsgDelistMarket, so neither is settable here.
func MarketStatusValid(s MarketStatus) bool {
	return s == MarketStatus_MARKET_STATUS_ACTIVE || s == MarketStatus_MARKET_STATUS_PAUSED
}

// MarketStatusValidGenesis reports whether s is a status a market may hold
// in state (genesis import/export).
func MarketStatusValidGenesis(s MarketStatus) bool {
	switch s {
	case MarketStatus_MARKET_STATUS_ACTIVE, MarketStatus_MARKET_STATUS_PAUSED,
		MarketStatus_MARKET_STATUS_PENDING_BOND, MarketStatus_MARKET_STATUS_DELISTED:
		return true
	}
	return false
}

func DefaultParams() Params {
	return Params{
		MaxMarkets:             DefaultMaxMarkets,
		MaxOpenOrdersPerMarket: DefaultMaxOpenOrdersPerMarket,
		MaxFillsPerBatch:       DefaultMaxFillsPerBatch,
		MaxFeeBps:              DefaultMaxFeeBps,
		MinBatchInterval:       DefaultMinBatchInterval,
		Paused:                 false,
		ListingBond:            DefaultListingBond(),
		ListingBondDenom:       DefaultListingBondDenom,
	}
}

func (p Params) Validate() error {
	if p.MaxMarkets == 0 || p.MaxMarkets > HardMaxMarkets {
		return fmt.Errorf("max_markets must be in (0, %d]", HardMaxMarkets)
	}
	if p.MaxOpenOrdersPerMarket == 0 {
		return fmt.Errorf("max_open_orders_per_market must be > 0")
	}
	if p.MaxFillsPerBatch == 0 {
		return fmt.Errorf("max_fills_per_batch must be > 0")
	}
	if p.MaxFeeBps > HardMaxFeeBps {
		return fmt.Errorf("max_fee_bps must be <= %d", HardMaxFeeBps)
	}
	if p.MinBatchInterval < 1 {
		return fmt.Errorf("min_batch_interval must be >= 1")
	}
	if p.ListingBond.IsNil() || p.ListingBond.IsNegative() {
		return fmt.Errorf("listing_bond must be set and >= 0")
	}
	if p.ListingBond.IsPositive() {
		if err := sdk.ValidateDenom(p.ListingBondDenom); err != nil {
			return fmt.Errorf("listing_bond_denom: %w", err)
		}
	}
	return nil
}
