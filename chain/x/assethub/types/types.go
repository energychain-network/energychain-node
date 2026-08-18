package types

import (
	"fmt"
	"math/bits"
	"regexp"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	IDMaxLen           = 128
	DisplayNameMaxLen  = 128
	DeviceTypeMaxLen   = 64
	UnitMaxLen         = 16
	PubkeyMaxLen       = 256
	HashMaxLen         = 128
	FirmwareMaxLen     = 64
	ReasonMaxLen       = 512
	DescriptionMaxLen  = 256
	JurisdictionMaxLen = 32

	DefaultMinBond              uint64 = 1_000_000
	DefaultMaxProviders         uint32 = 100_000
	DefaultMaxDevices           uint32 = 20_000_000 // AntChain-Inside scale (>15M devices)
	DefaultMaxTopics            uint32 = 4_096
	DefaultMaxSourcesPerTopic   uint32 = 64
	DefaultReadingToleranceBps  uint32 = 200 // 2%
	DefaultSlashFractionBps     uint32 = 500 // 5%
	DefaultJailAfterInfractions uint32 = 3
	DefaultJailDurationSeconds  int64  = 7 * 24 * 3600

	BpsDenominator uint32 = 10_000

	HardMaxProviders       uint32 = 5_000_000
	HardMaxDevices         uint32 = 100_000_000
	HardMaxTopics          uint32 = 1_000_000
	HardMaxSourcesPerTopic uint32 = 1_024
)

var (
	idRe           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-:]{0,127}$`)
	topicIDRe      = regexp.MustCompile(`^[a-z][a-z0-9._\-/]{0,127}$`)
	deviceTypeRe   = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	denomRe        = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9/:._\-]{1,127}$`)
	jurisdictionRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
)

func MustBech32(addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return errorsmod.Wrapf(ErrInvalidAddress, "%q: %v", addr, err)
	}
	return nil
}

func ValidateID(id string) error {
	if !idRe.MatchString(id) {
		return errorsmod.Wrapf(ErrInvalidField, "id %q invalid", id)
	}
	return nil
}

func ValidateTopicID(id string) error {
	if !topicIDRe.MatchString(id) {
		return errorsmod.Wrapf(ErrInvalidField, "topic id %q must match [a-z][a-z0-9._/-]{0,127}", id)
	}
	return nil
}

func ValidateDeviceType(s string) error {
	if !deviceTypeRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "device_type %q must match [a-z][a-z0-9._-]{0,63}", s)
	}
	return nil
}

func ValidateDenom(d string) error {
	if !denomRe.MatchString(d) {
		return errorsmod.Wrapf(ErrInvalidField, "bond_denom %q invalid", d)
	}
	return nil
}

func ValidateOptionalJurisdiction(s string) error {
	if s == "" {
		return nil
	}
	if !jurisdictionRe.MatchString(s) {
		return errorsmod.Wrapf(ErrInvalidField, "jurisdiction %q invalid", s)
	}
	return nil
}

func ValidateMaxLen(field, v string, max int) error {
	if len(v) > max {
		return errorsmod.Wrapf(ErrInvalidField, "%s too long (max %d)", field, max)
	}
	return nil
}

func ProviderRoleValid(r ProviderRole) bool {
	switch r {
	case ProviderRole_PROVIDER_ROLE_ORACLE, ProviderRole_PROVIDER_ROLE_METER,
		ProviderRole_PROVIDER_ROLE_DEVICE, ProviderRole_PROVIDER_ROLE_BRIDGE:
		return true
	}
	return false
}

func ProviderStatusValid(s ProviderStatus) bool {
	switch s {
	case ProviderStatus_PROVIDER_STATUS_ACTIVE, ProviderStatus_PROVIDER_STATUS_JAILED,
		ProviderStatus_PROVIDER_STATUS_BANNED:
		return true
	}
	return false
}

func DeviceStatusValid(s DeviceStatus) bool {
	switch s {
	case DeviceStatus_DEVICE_STATUS_ACTIVE, DeviceStatus_DEVICE_STATUS_REVOKED:
		return true
	}
	return false
}

// SafeAdd / SafeSub guard bond and counter arithmetic.
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

// MulBps returns floor(value * bps / 10000) using 128-bit intermediates so a
// large bond cannot overflow before the division.
func MulBps(value uint64, bps uint32) uint64 {
	prod := mul128(value, uint64(bps))
	return prod.divU64(uint64(BpsDenominator))
}

// CrossVerified reports whether the IoT-reported value and the
// operational value reconcile within toleranceBps of the larger of the
// two. Both zero is treated as verified (no production, consistent).
func CrossVerified(iot, operational uint64, toleranceBps uint32) bool {
	hi, lo := iot, operational
	if operational > iot {
		hi, lo = operational, iot
	}
	if hi == 0 {
		return true
	}
	diff := hi - lo
	// diff/hi <= bps/10000  <=>  diff*10000 <= hi*bps  (use big to avoid overflow)
	left := mul128(diff, uint64(BpsDenominator))
	right := mul128(hi, uint64(toleranceBps))
	return left.lte(right)
}

func DefaultParams() Params {
	return Params{
		// EnergyChain native staking/gas denom (see chain/genesis.go
		// NativeDenom). Provider bonds are escrowed in this denom.
		BondDenom:            "uecy",
		MinBond:              DefaultMinBond,
		MaxProviders:         DefaultMaxProviders,
		MaxDevices:           DefaultMaxDevices,
		MaxTopics:            DefaultMaxTopics,
		MaxSourcesPerTopic:   DefaultMaxSourcesPerTopic,
		ReadingToleranceBps:  DefaultReadingToleranceBps,
		SlashFractionBps:     DefaultSlashFractionBps,
		JailAfterInfractions: DefaultJailAfterInfractions,
		JailDurationSeconds:  DefaultJailDurationSeconds,
	}
}

func (p Params) Validate() error {
	if err := ValidateDenom(p.BondDenom); err != nil {
		return err
	}
	if p.MaxProviders == 0 || p.MaxProviders > HardMaxProviders {
		return fmt.Errorf("max_providers must be in (0, %d]", HardMaxProviders)
	}
	if p.MaxDevices == 0 || p.MaxDevices > HardMaxDevices {
		return fmt.Errorf("max_devices must be in (0, %d]", HardMaxDevices)
	}
	if p.MaxTopics == 0 || p.MaxTopics > HardMaxTopics {
		return fmt.Errorf("max_topics must be in (0, %d]", HardMaxTopics)
	}
	if p.MaxSourcesPerTopic == 0 || p.MaxSourcesPerTopic > HardMaxSourcesPerTopic {
		return fmt.Errorf("max_sources_per_topic must be in (0, %d]", HardMaxSourcesPerTopic)
	}
	if p.ReadingToleranceBps > BpsDenominator {
		return fmt.Errorf("reading_tolerance_bps must be <= %d", BpsDenominator)
	}
	if p.SlashFractionBps > BpsDenominator {
		return fmt.Errorf("slash_fraction_bps must be <= %d", BpsDenominator)
	}
	if p.JailDurationSeconds < 0 {
		return fmt.Errorf("jail_duration_seconds must be >= 0")
	}
	return nil
}

// ---- tiny 128-bit helper for overflow-safe comparisons -------------------

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

// divU64 returns floor(x / d) assuming the quotient fits in uint64. When the
// quotient would overflow (x.hi >= d) it clamps to the max uint64; callers
// (MulBps with bps <= 10000) never hit that branch.
func (x u128) divU64(d uint64) uint64 {
	if d == 0 {
		return 0
	}
	if x.hi == 0 {
		return x.lo / d
	}
	if x.hi >= d {
		return ^uint64(0)
	}
	q, _ := bits.Div64(x.hi, x.lo, d)
	return q
}
