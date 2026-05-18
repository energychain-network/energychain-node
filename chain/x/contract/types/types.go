package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 512
	ReasonMaxLen = 512
	DenomMaxLen  = 64
	TopicMaxLen  = 128
	URIMaxLen    = 512
	HashMaxLen   = 64

	// PriceScaleDecimals is the fixed-point scale used for
	// strike_price and oracle index readings. 1.0 unit of
	// asset_denom per unit-of-quantity == PriceScale.
	PriceScaleDecimals = 6
	PriceScale         = uint64(1_000_000)

	DefaultMaxContracts             uint32 = 50_000
	DefaultMaxContractsPerParty     uint32 = 1_000
	DefaultMinSettlementPeriod      int64  = 60                  // 1 minute
	DefaultMaxSettlementPeriod      int64  = 366 * 24 * 3600     // ~1 year
	DefaultGracePeriodSeconds       int64  = 24 * 3600           // 1 day
	DefaultMaxOracleStalenessCap    int64  = 7 * 24 * 3600       // 7 days
	DefaultMaxCatchupPerSettle      uint32 = 24
	DefaultMemoMaxLen               uint32 = MemoMaxLen

	HardMaxContracts             uint32 = 1_000_000
	HardMaxContractsPerParty     uint32 = 100_000
	HardMinSettlementPeriod      int64  = 1
	HardMaxSettlementPeriod      int64  = 50 * 365 * 24 * 3600
	HardMaxOracleStalenessCap    int64  = 30 * 24 * 3600
	HardMaxCatchupPerSettle      uint32 = 10_000
	HardMemoMaxLen               uint32 = 4096

	// Terminate signal bit-mask values (kept in
	// Contract.terminate_signal_mask). Both bits set == both
	// parties signed termination → TERMINATED.
	TerminateSigBuyer  int64 = 1
	TerminateSigSeller int64 = 2
	TerminateSigBoth   int64 = TerminateSigBuyer | TerminateSigSeller
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

func ValidateTopic(s string) error {
	if len(s) > TopicMaxLen {
		return fmt.Errorf("topic too long (max %d)", TopicMaxLen)
	}
	return nil
}

func ValidateURI(s string) error {
	if len(s) > URIMaxLen {
		return fmt.Errorf("uri too long (max %d)", URIMaxLen)
	}
	return nil
}

func ValidateHash(b []byte) error {
	if len(b) > HashMaxLen {
		return fmt.Errorf("hash too long (max %d)", HashMaxLen)
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

func KindValid(k Kind) bool {
	switch k {
	case Kind_KIND_PPA, Kind_KIND_VPPA, Kind_KIND_CFD:
		return true
	}
	return false
}

func StatusValid(s Status) bool {
	switch s {
	case Status_STATUS_DRAFT,
		Status_STATUS_ACTIVE,
		Status_STATUS_PAUSED,
		Status_STATUS_DISPUTED,
		Status_STATUS_TERMINATED,
		Status_STATUS_EXPIRED,
		Status_STATUS_DEFAULTED:
		return true
	}
	return false
}

func (s Status) IsTerminal() bool {
	return s == Status_STATUS_TERMINATED ||
		s == Status_STATUS_EXPIRED ||
		s == Status_STATUS_DEFAULTED
}

// SafeMul / SafeAdd / SafeSub guard ledger arithmetic.
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
