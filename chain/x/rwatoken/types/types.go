package types

import (
	"fmt"
	"math/big"
	"regexp"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	SymbolMaxLen      = 32
	NameMaxLen        = 128
	AssetClassMaxLen  = 64
	DenomIDMaxLen     = 64
	PolicyIDMaxLen    = 64
	MetadataURIMaxLen = 512
	ReasonMaxLen      = 512
	MaxDecimals       = 18

	// DeviceIDMaxLen bounds a single bound device id; MaxDeviceBindings
	// bounds how many devices one token may declare (keeps the EndBlocker
	// and create/update validation O(small) and the Token row bounded).
	DeviceIDMaxLen    = 128
	MaxDeviceBindings = 256

	// HardMaxRedemptionDelay bounds the T+N window so a token can't be
	// configured with an effectively infinite (or negative) settlement
	// delay. 5 years is well beyond any realistic RWA buy-back cycle.
	HardMaxRedemptionDelay int64 = 5 * 365 * 24 * 3600

	DefaultMaxTokens                      uint32 = 1024
	DefaultMaxPendingRedemptionsPerHolder uint32 = 64
	DefaultMaxHoldersPerSnapshot          uint32 = 10_000

	HardMaxTokens             uint32 = 1_048_576
	HardMaxPendingRedemptions uint32 = 4_096
	HardMaxHoldersPerSnapshot uint32 = 1_000_000
)

var (
	symbolRe     = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	assetClassRe = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,63}$`)
	denomIDRe    = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,63}$`)
	policyIDRe   = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,63}$`)
)

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateSymbol(s string) error {
	if !symbolRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "symbol %q invalid", s)
	}
	return nil
}

func ValidateAssetClass(s string) error {
	if !assetClassRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "asset_class %q must match [a-z][a-z0-9._/-]{0,63}", s)
	}
	return nil
}

func ValidateSettlementDenom(s string) error {
	if !denomIDRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "settlement_denom %q invalid", s)
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

func ValidateMaxLen(field, v string, max int) error {
	if len(v) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s too long (max %d)", field, max)
	}
	return nil
}

func ValidateRedemptionDelay(d int64) error {
	if d < 0 || d > HardMaxRedemptionDelay {
		return errorsmod.Wrapf(ErrInvalidField, "redemption_delay_seconds must be in [0, %d]", HardMaxRedemptionDelay)
	}
	return nil
}

// ValidateMaturityTime accepts 0 (perpetual / no auto-expiry) or any
// positive unix second. Setting a past time is allowed: the next
// EndBlocker simply matures the token immediately.
func ValidateMaturityTime(t int64) error {
	if t < 0 {
		return errorsmod.Wrap(ErrInvalidField, "maturity_time must be >= 0")
	}
	return nil
}

// ValidateDeviceIDs bounds the bound-device set: at most MaxDeviceBindings
// ids, each non-empty and within DeviceIDMaxLen, with no duplicates.
// Existence / operator / active checks happen in the keeper against
// x/assethub state.
func ValidateDeviceIDs(ids []string) error {
	if len(ids) > MaxDeviceBindings {
		return errorsmod.Wrapf(ErrInvalidField, "device_ids count %d > %d", len(ids), MaxDeviceBindings)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return errorsmod.Wrap(ErrInvalidField, "device_ids contains empty id")
		}
		if len(id) > DeviceIDMaxLen {
			return errorsmod.Wrapf(ErrInvalidField, "device id %q too long (max %d)", id, DeviceIDMaxLen)
		}
		if _, dup := seen[id]; dup {
			return errorsmod.Wrapf(ErrInvalidField, "duplicate device id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func TokenStatusValid(s TokenStatus) bool {
	switch s {
	case TokenStatus_TOKEN_STATUS_ACTIVE,
		TokenStatus_TOKEN_STATUS_PAUSED,
		TokenStatus_TOKEN_STATUS_MATURED:
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

// SafeMul multiplies two uint64 values, erroring on overflow. Used for
// redemption payouts (units * redemption_price).
func SafeMul(x, y uint64) (uint64, error) {
	if x == 0 || y == 0 {
		return 0, nil
	}
	if x > ^uint64(0)/y {
		return 0, errorsmod.Wrapf(ErrOverflow, "%d * %d", x, y)
	}
	return x * y, nil
}

// MulDivFloor returns floor(a*b/denom) computed over big.Int so the
// intermediate product cannot overflow. It errors when denom == 0 or the
// quotient exceeds uint64. This is the pro-rata dividend kernel:
//
//	share = floor(total_amount * holder_balance / snapshot_total_supply)
//
// Flooring guarantees sum(shares) <= total_amount (no over-distribution);
// the unclaimed dust remains in the token pool.
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

func DefaultParams() Params {
	return Params{
		MaxTokens:                      DefaultMaxTokens,
		MaxPendingRedemptionsPerHolder: DefaultMaxPendingRedemptionsPerHolder,
		RequireSanctionsClear:          true,
		MaxHoldersPerSnapshot:          DefaultMaxHoldersPerSnapshot,
	}
}

func (p Params) Validate() error {
	if p.MaxTokens == 0 || p.MaxTokens > HardMaxTokens {
		return fmt.Errorf("max_tokens must be in (0, %d]", HardMaxTokens)
	}
	if p.MaxPendingRedemptionsPerHolder == 0 || p.MaxPendingRedemptionsPerHolder > HardMaxPendingRedemptions {
		return fmt.Errorf("max_pending_redemptions_per_holder must be in (0, %d]", HardMaxPendingRedemptions)
	}
	if p.MaxHoldersPerSnapshot == 0 || p.MaxHoldersPerSnapshot > HardMaxHoldersPerSnapshot {
		return fmt.Errorf("max_holders_per_snapshot must be in (0, %d]", HardMaxHoldersPerSnapshot)
	}
	return nil
}
