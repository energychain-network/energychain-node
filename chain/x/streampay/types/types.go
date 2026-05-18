package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 256
	ReasonMaxLen = 512
	DenomMaxLen  = 64

	DefaultMaxStreams          uint32 = 200_000
	DefaultMaxStreamsPerSender uint32 = 1_000
	DefaultMinRate             uint64 = 1
	DefaultMaxRate             uint64 = 1_000_000_000 // 1e9 / sec
	DefaultMaxDeposit          uint64 = 1_000_000_000_000_000_000 // ~1e18 (ledger cap)
	DefaultMaxHorizonSeconds   int64  = 100 * 365 * 24 * 3600
	DefaultMemoMaxLen          uint32 = MemoMaxLen

	HardMaxStreams          uint32 = 10_000_000
	HardMaxStreamsPerSender uint32 = 100_000
	HardMaxRate             uint64 = 1 << 62
	HardMaxHorizonSeconds   int64  = 1_000 * 365 * 24 * 3600
	HardMemoMaxLen          uint32 = 4096
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

func StatusValid(s Status) bool {
	switch s {
	case Status_STATUS_ACTIVE,
		Status_STATUS_PAUSED,
		Status_STATUS_CANCELLED,
		Status_STATUS_COMPLETED:
		return true
	}
	return false
}

// IsTerminal flags terminal lifecycle states; further mutating
// messages are refused on terminal streams.
func (s Status) IsTerminal() bool {
	return s == Status_STATUS_CANCELLED || s == Status_STATUS_COMPLETED
}

// SafeMul guards rate * elapsed multiplications. Returns
// (^uint64(0), true) when the product would overflow so the
// caller can cap to deposit.
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

// EffectiveElapsed returns the active-streaming seconds between
// start_time and `now`, with all PAUSED intervals excluded and
// capped at end_time. Result is always >= 0.
//
// Critical invariant: this must be a pure function of (s, now)
// so on-chain accrual is replayable from any height.
func EffectiveElapsed(s Stream, now int64) int64 {
	if now <= s.StartTime {
		return 0
	}
	elapsed := now - s.StartTime
	if s.EndTime != 0 {
		horizon := s.EndTime - s.StartTime
		if elapsed > horizon {
			elapsed = horizon
		}
	}
	elapsed -= s.PausedAccumulatedSeconds
	if s.Status == Status_STATUS_PAUSED && s.PausedAt != 0 {
		// In-flight pause: subtract the current pause-so-far so
		// receiver math stops accruing while paused.
		var inflight int64
		// Clamp `now` to end_time when computing in-flight pause
		// duration too — otherwise a stream paused past end_time
		// would over-subtract.
		cur := now
		if s.EndTime != 0 && cur > s.EndTime {
			cur = s.EndTime
		}
		if cur > s.PausedAt {
			inflight = cur - s.PausedAt
		}
		elapsed -= inflight
	}
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// StreamedAmount is the cumulative amount that has flowed to the
// receiver, capped at deposit and guarded against rate*elapsed
// overflow.
func StreamedAmount(s Stream, now int64) uint64 {
	e := EffectiveElapsed(s, now)
	if e <= 0 || s.RatePerSecond == 0 {
		return 0
	}
	prod, overflow := SafeMul(uint64(e), s.RatePerSecond)
	if overflow || prod > s.Deposit {
		return s.Deposit
	}
	return prod
}

// Withdrawable is streamed-not-yet-withdrawn at `now`.
func Withdrawable(s Stream, now int64) uint64 {
	streamed := StreamedAmount(s, now)
	if s.Withdrawn >= streamed {
		return 0
	}
	return streamed - s.Withdrawn
}
