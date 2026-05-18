package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	MemoMaxLen      = 256
	ReasonMaxLen    = 256
	DenomMaxLen     = 64
	SourceRefMaxLen = 256

	DefaultMaxOpenCycles           uint32 = 16
	DefaultMaxObligationsPerCycle  uint32 = 10_000
	DefaultMaxMembers              uint32 = 10_000
	DefaultMemoMaxLen              uint32 = MemoMaxLen
	DefaultSourceRefMaxLen         uint32 = SourceRefMaxLen

	HardMaxOpenCycles          uint32 = 1_024
	HardMaxObligationsPerCycle uint32 = 1_000_000
	HardMaxMembers             uint32 = 1_000_000
	HardMemoMaxLen             uint32 = 4096
	HardSourceRefMaxLen        uint32 = 4096
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

func ValidateSourceRef(s string, maxLen uint32) error {
	if maxLen == 0 {
		maxLen = SourceRefMaxLen
	}
	if uint32(len(s)) > maxLen {
		return fmt.Errorf("source_ref too long (max %d)", maxLen)
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

func MemberStatusValid(s MemberStatus) bool {
	switch s {
	case MemberStatus_MEMBER_STATUS_ACTIVE,
		MemberStatus_MEMBER_STATUS_SUSPENDED,
		MemberStatus_MEMBER_STATUS_EXPELLED:
		return true
	}
	return false
}

func CycleStatusValid(s CycleStatus) bool {
	switch s {
	case CycleStatus_CYCLE_STATUS_OPEN,
		CycleStatus_CYCLE_STATUS_CLOSED,
		CycleStatus_CYCLE_STATUS_SETTLED,
		CycleStatus_CYCLE_STATUS_DEFAULTED,
		CycleStatus_CYCLE_STATUS_CANCELLED:
		return true
	}
	return false
}

func (s CycleStatus) IsTerminal() bool {
	return s == CycleStatus_CYCLE_STATUS_SETTLED ||
		s == CycleStatus_CYCLE_STATUS_DEFAULTED ||
		s == CycleStatus_CYCLE_STATUS_CANCELLED
}

// Safe arithmetic — see the same primitives in x/escrow /
// x/market for invariants.
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
