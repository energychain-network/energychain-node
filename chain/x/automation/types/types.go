package types

import (
	"fmt"
	"math/big"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	DenomMaxLen = 64

	DefaultMaxSchedules        uint32 = 4096
	DefaultMaxStreams          uint32 = 65536
	DefaultMaxRunsPerBlock     uint32 = 64
	DefaultMaxFinalizePerBlock uint32 = 64
	DefaultMaxClosePerRun      uint32 = 64
	DefaultMinIntervalSeconds  int64  = 60

	HardMaxSchedules uint32 = 1_048_576
	HardMaxStreams   uint32 = 16_777_216
	HardMaxPerBlock  uint32 = 4096
)

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

// CeilDiv returns ceil(a/b) without intermediate overflow.
func CeilDiv(a, b uint64) (uint64, error) {
	if b == 0 {
		return 0, errorsmod.Wrap(ErrOverflow, "division by zero")
	}
	if a == 0 {
		return 0, nil
	}
	q := a / b
	if a%b != 0 {
		q++
	}
	return q, nil
}

// StopTime is the second at which a stream is fully vested:
// start + ceil(deposit/rate).
func StopTime(start int64, deposit, rate uint64) (int64, error) {
	secs, err := CeilDiv(deposit, rate)
	if err != nil {
		return 0, err
	}
	if secs > uint64(1<<62) {
		return 0, errorsmod.Wrap(ErrOverflow, "stream duration too long")
	}
	return start + int64(secs), nil
}

// VestedAmount is the linearly vested portion of a stream at `now`:
// min(deposit, rate * (min(now, stop) - start)), clamped at [0, deposit].
// Computed over big.Int so rate*elapsed cannot overflow.
func VestedAmount(deposit, rate uint64, start, stop, now int64) uint64 {
	if now <= start || deposit == 0 {
		return 0
	}
	t := now
	if t > stop {
		t = stop
	}
	elapsed := t - start
	if elapsed <= 0 {
		return 0
	}
	v := new(big.Int).Mul(new(big.Int).SetUint64(rate), big.NewInt(elapsed))
	dep := new(big.Int).SetUint64(deposit)
	if v.Cmp(dep) >= 0 {
		return deposit
	}
	return v.Uint64()
}

func ActionKindValid(a ActionKind) bool {
	switch a {
	case ActionKind_ACTION_KIND_MINCAST_INJECT,
		ActionKind_ACTION_KIND_RWA_SNAPSHOT,
		ActionKind_ACTION_KIND_RWA_DIVIDEND,
		ActionKind_ACTION_KIND_MINCAST_CLOSE_MATURED:
		return true
	}
	return false
}

// ActionNeedsAmount reports whether the action funds settlement per run
// (and therefore requires amount_per_run > 0).
func ActionNeedsAmount(a ActionKind) bool {
	return a == ActionKind_ACTION_KIND_MINCAST_INJECT || a == ActionKind_ACTION_KIND_RWA_DIVIDEND
}

func ScheduleStatusValid(s ScheduleStatus) bool {
	switch s {
	case ScheduleStatus_SCHEDULE_STATUS_ACTIVE, ScheduleStatus_SCHEDULE_STATUS_PAUSED, ScheduleStatus_SCHEDULE_STATUS_COMPLETED:
		return true
	}
	return false
}

func StreamStatusValid(s StreamStatus) bool {
	switch s {
	case StreamStatus_STREAM_STATUS_ACTIVE, StreamStatus_STREAM_STATUS_COMPLETED, StreamStatus_STREAM_STATUS_CANCELLED:
		return true
	}
	return false
}

// AdvanceNextRun rolls a schedule's next_run forward past every missed slot
// in O(1) so a chain halt does not cause a burst of catch-up runs.
func AdvanceNextRun(nextRun, interval, now int64) int64 {
	if interval <= 0 {
		return nextRun
	}
	if nextRun > now {
		return nextRun
	}
	missed := (now - nextRun) / interval
	return nextRun + (missed+1)*interval
}

func DefaultParams() Params {
	return Params{
		MaxSchedules:        DefaultMaxSchedules,
		MaxStreams:          DefaultMaxStreams,
		MaxRunsPerBlock:     DefaultMaxRunsPerBlock,
		MaxFinalizePerBlock: DefaultMaxFinalizePerBlock,
		MaxClosePerRun:      DefaultMaxClosePerRun,
		MinIntervalSeconds:  DefaultMinIntervalSeconds,
	}
}

func (p Params) Validate() error {
	if p.MaxSchedules == 0 || p.MaxSchedules > HardMaxSchedules {
		return fmt.Errorf("max_schedules must be in (0, %d]", HardMaxSchedules)
	}
	if p.MaxStreams == 0 || p.MaxStreams > HardMaxStreams {
		return fmt.Errorf("max_streams must be in (0, %d]", HardMaxStreams)
	}
	if p.MaxRunsPerBlock == 0 || p.MaxRunsPerBlock > HardMaxPerBlock {
		return fmt.Errorf("max_runs_per_block must be in (0, %d]", HardMaxPerBlock)
	}
	if p.MaxFinalizePerBlock == 0 || p.MaxFinalizePerBlock > HardMaxPerBlock {
		return fmt.Errorf("max_finalize_per_block must be in (0, %d]", HardMaxPerBlock)
	}
	if p.MaxClosePerRun == 0 || p.MaxClosePerRun > HardMaxPerBlock {
		return fmt.Errorf("max_close_per_run must be in (0, %d]", HardMaxPerBlock)
	}
	if p.MinIntervalSeconds <= 0 {
		return fmt.Errorf("min_interval_seconds must be > 0")
	}
	return nil
}
