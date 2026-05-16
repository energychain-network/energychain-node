package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen   = 256
	ReasonMaxLen = 512

	DenomMaxLen = 64

	DefaultMaxJobs            uint32 = 50_000
	DefaultMaxJobsPerOwner    uint32 = 256
	DefaultMinIntervalSeconds int64  = 60       // 1 minute
	DefaultMaxIntervalSeconds int64  = 365 * 24 * 3600
	DefaultMaxPayloadBytes    uint32 = 16 * 1024 // 16 KiB
	DefaultMaxJobsPerBlock    uint32 = 200
	DefaultErrorMaxLen        uint32 = 512

	HardMaxJobs            uint32 = 1_000_000
	HardMaxJobsPerOwner    uint32 = 10_000
	HardMinIntervalSeconds int64  = 1
	HardMaxIntervalSeconds int64  = 50 * 365 * 24 * 3600
	HardMaxPayloadBytes    uint32 = 1 * 1024 * 1024 // 1 MiB
	HardMaxJobsPerBlock    uint32 = 10_000
	HardErrorMaxLen        uint32 = 4096
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

func StatusValid(s Status) bool {
	switch s {
	case Status_STATUS_ACTIVE,
		Status_STATUS_PAUSED,
		Status_STATUS_EXHAUSTED,
		Status_STATUS_CANCELLED:
		return true
	}
	return false
}

// IsTerminal flags terminal statuses. Terminal jobs are not
// scheduled and cannot be resumed.
func (s Status) IsTerminal() bool {
	return s == Status_STATUS_CANCELLED
}

// SafeAdd / SafeSub guard ledger arithmetic on fee budgets.
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

// AdvanceNextRun returns the next-run time after `current` given a
// starting anchor, an interval, and a clock `now`. The rule is:
//
//   - If next_run_time <= now, fast-forward to the smallest
//     start_time + k*interval that is > now. This avoids the
//     "burst catch-up" anti-pattern where a long downtime would
//     produce N stacked executions.
//   - Otherwise the natural progression is current + interval.
//
// Both `current` and `now` are unix seconds.
func AdvanceNextRun(start, current, interval, now int64) int64 {
	if interval <= 0 {
		return current
	}
	next := current + interval
	if next > now {
		return next
	}
	// Skip past missed slots in O(1).
	delta := now - start
	if delta < 0 {
		return start
	}
	k := delta/interval + 1
	return start + k*interval
}

// TruncateError caps the length of the last_error column so a
// verbose handler error cannot bloat module state.
func TruncateError(s string, maxLen uint32) string {
	if maxLen == 0 || uint32(len(s)) <= maxLen {
		return s
	}
	return s[:maxLen]
}
