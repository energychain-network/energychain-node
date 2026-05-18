package types

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 512
	ReasonMaxLen = 512
	DenomMaxLen  = 64
	AssetRefMaxLen = 256
	SaltMaxLen   = 64
	HashLen      = 32

	DefaultMaxAuctions               uint32 = 50_000
	DefaultMaxAuctionsPerSeller      uint32 = 1_000
	DefaultMaxBidsPerAuction         uint32 = 10_000
	DefaultMinAuctionDurationSeconds int64  = 60
	DefaultMaxAuctionDurationSeconds int64  = 365 * 24 * 3600
	DefaultMemoMaxLen                uint32 = MemoMaxLen

	HardMaxAuctions               uint32 = 1_000_000
	HardMaxAuctionsPerSeller      uint32 = 100_000
	HardMaxBidsPerAuction         uint32 = 1_000_000
	HardMinAuctionDurationSeconds int64  = 1
	HardMaxAuctionDurationSeconds int64  = 10 * 365 * 24 * 3600
	HardMemoMaxLen                uint32 = 4096
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
func ValidateAssetRef(s string) error {
	if s == "" {
		return fmt.Errorf("asset_ref must be non-empty")
	}
	if len(s) > AssetRefMaxLen {
		return fmt.Errorf("asset_ref too long (max %d)", AssetRefMaxLen)
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
	case Kind_KIND_ENGLISH, Kind_KIND_DUTCH, Kind_KIND_SEALED_FIRST, Kind_KIND_SEALED_SECOND:
		return true
	}
	return false
}

func StatusValid(s Status) bool {
	switch s {
	case Status_STATUS_DRAFT, Status_STATUS_OPEN, Status_STATUS_COMMIT,
		Status_STATUS_REVEAL, Status_STATUS_CLOSED, Status_STATUS_SETTLED, Status_STATUS_CANCELLED:
		return true
	}
	return false
}

func IsSealed(k Kind) bool {
	return k == Kind_KIND_SEALED_FIRST || k == Kind_KIND_SEALED_SECOND
}

// ComputeCommitHash is the canonical commit binding used by
// SEALED_* modes. price is decimal-ASCII; salt is raw bytes
// the bidder picks. Off-chain wallets MUST use this exact
// encoding to ensure their reveal verifies on-chain.
func ComputeCommitHash(price uint64, salt []byte) []byte {
	h := sha256.New()
	h.Write([]byte(strconv.FormatUint(price, 10)))
	h.Write([]byte(":"))
	h.Write(salt)
	return h.Sum(nil)
}

// DutchPriceAt returns the current Dutch-auction price at `now`.
//
// Linear decay from dutch_start_price at start_time to
// dutch_floor_price at start_time+dutch_decay_seconds. Floors
// thereafter. now < start_time returns dutch_start_price so a
// just-created Dutch auction has a sane initial price.
func DutchPriceAt(a Auction, now int64) uint64 {
	start := a.DutchStartPrice
	floor := a.DutchFloorPrice
	if floor >= start {
		return start // misconfigured but defensive
	}
	if a.DutchDecaySeconds <= 0 {
		return floor
	}
	elapsed := now - a.StartTime
	if elapsed <= 0 {
		return start
	}
	if elapsed >= a.DutchDecaySeconds {
		return floor
	}
	gap := start - floor
	// gap * elapsed / decay — fits because elapsed <= decay and
	// gap <= uint64. Use uint128-style trick by going through
	// big-ish uint64 division: gap * (elapsed / decay) approx
	// would lose precision, so do gap/decay scaled then * elapsed.
	// Simplest safe path: compute via int128-equivalent two-step.
	// gap and elapsed fit in int64 in realistic params so:
	delta := (gap / uint64(a.DutchDecaySeconds)) * uint64(elapsed)
	// add residual for precision
	rem := (gap % uint64(a.DutchDecaySeconds)) * uint64(elapsed) / uint64(a.DutchDecaySeconds)
	return start - delta - rem
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
