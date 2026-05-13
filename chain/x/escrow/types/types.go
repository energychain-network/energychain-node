package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 256
	ReasonMaxLen = 512

	DenomMaxLen = 64

	DefaultMaxEscrows                uint32 = 50_000
	DefaultMaxSignersPerEscrow       uint32 = 16
	DefaultMaxReleaseHorizonSeconds  int64  = 50 * 365 * 24 * 3600 // 50 years
	DefaultRequireSanctionsClear     bool   = true

	HardMaxEscrows                uint32 = 1_000_000
	HardMaxSignersPerEscrow       uint32 = 64
	HardMaxReleaseHorizonSeconds  int64  = 100 * 365 * 24 * 3600
)

func ValidateMemo(s string) error {
	if len(s) > MemoMaxLen {
		return fmt.Errorf("memo too long (max %d)", MemoMaxLen)
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

func ValidateOptionalAddr(field, s string) error {
	if s == "" {
		return nil
	}
	return ValidateAddr(field, s)
}

func AssetKindValid(k AssetKind) bool {
	switch k {
	case AssetKind_ASSET_KIND_STABLECOIN, AssetKind_ASSET_KIND_RWA:
		return true
	}
	return false
}

func StatusValid(s Status) bool {
	switch s {
	case Status_STATUS_DRAFT,
		Status_STATUS_FUNDED,
		Status_STATUS_DISPUTED,
		Status_STATUS_RELEASED,
		Status_STATUS_REFUNDED,
		Status_STATUS_CANCELLED:
		return true
	}
	return false
}

func IntentValid(i Intent) bool {
	switch i {
	case Intent_INTENT_RELEASE, Intent_INTENT_REFUND:
		return true
	}
	return false
}

// SafeAdd / SafeSub guard counter arithmetic against overflow.
func SafeAddU32(x, y uint32) (uint32, error) {
	if y > 0 && x > ^uint32(0)-y {
		return 0, fmt.Errorf("uint32 overflow: %d + %d", x, y)
	}
	return x + y, nil
}

func SafeSubU32(x, y uint32) (uint32, error) {
	if y > x {
		return 0, fmt.Errorf("uint32 underflow: %d - %d", x, y)
	}
	return x - y, nil
}
