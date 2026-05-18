package types

import (
	"fmt"
	"math"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Default / hard parameter caps. DEFAULT values are what the
// chain ships with; HARD ceilings are enforced by Params.Validate
// so that even a misconfigured governance proposal can't
// trip O(N) walks or unreasonable bond requirements.
const (
	DefaultMaxArbitrators                uint32 = 256
	HardMaxArbitrators                   uint32 = 16384
	DefaultMaxOpenDisputes               uint32 = 1024
	HardMaxOpenDisputes                  uint32 = 65536
	DefaultMaxEvidencePerDispute         uint32 = 256
	HardMaxEvidencePerDispute            uint32 = 4096
	DefaultPanelSize                     uint32 = 3
	HardMaxPanelSize                     uint32 = 25
	DefaultQuorumBps                     uint32 = 6700 // 67%
	HardMaxQuorumBps                     uint32 = 10_000
	DefaultRespondPeriodSeconds          int64  = 7 * 24 * 3600
	HardMaxRespondPeriodSeconds          int64  = 90 * 24 * 3600
	DefaultDeliberationPeriodSeconds     int64  = 14 * 24 * 3600
	HardMaxDeliberationPeriodSeconds     int64  = 180 * 24 * 3600
	DefaultMinPlaintiffBond              uint64 = 1
	DefaultMaxPlaintiffBond              uint64 = 1_000_000_000
	HardMaxPlaintiffBond                 uint64 = 1_000_000_000_000_000
	DefaultSlashBps                      uint32 = 10_000 // 100% (loser forfeits full bond)
	HardMaxSlashBps                      uint32 = 10_000
	DefaultMemoMaxLen                    uint32 = 256
	HardMemoMaxLen                       uint32 = 1024
	DefaultReasonMaxLen                  uint32 = 512
	HardReasonMaxLen                     uint32 = 2048
	DefaultURIMaxLen                     uint32 = 512
	HardURIMaxLen                        uint32 = 4096
	DefaultMaxJurisdictionsPerArbitrator uint32 = 64
	HardMaxJurisdictionsPerArbitrator    uint32 = 256
	DefaultMaxStandardsPerArbitrator     uint32 = 64
	HardMaxStandardsPerArbitrator        uint32 = 256
	DefaultMaxFinalizationsPerBlock      uint32 = 64
	HardMaxFinalizationsPerBlock         uint32 = 1024

	DenomMaxLen    = 64
	NameMaxLen     = 128
	DIDMaxLen      = 256
	HashHexLen     = 64
	SubjectRefMaxLen = 256
	JurisCodeLen   = 2
)

// ---- Numerical safety --------------------------------------------------

func SafeAdd(a, b uint64) (uint64, error) {
	if a > math.MaxUint64-b {
		return 0, fmt.Errorf("uint64 overflow: %d + %d", a, b)
	}
	return a + b, nil
}

func SafeSub(a, b uint64) (uint64, error) {
	if a < b {
		return 0, fmt.Errorf("uint64 underflow: %d - %d", a, b)
	}
	return a - b, nil
}

// SlashAmount applies basis-points to a bond using
// floor-division. The remainder is the refundable portion.
// Both values are guaranteed to sum to `bond`.
func SlashAmount(bond uint64, bps uint32) (slashed, refunded uint64) {
	if bps == 0 || bond == 0 {
		return 0, bond
	}
	if bps >= 10_000 {
		return bond, 0
	}
	slashed = bond * uint64(bps) / 10_000
	refunded = bond - slashed
	return
}

// ---- Validators --------------------------------------------------------

func ValidateAddr(field, addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("%s: invalid bech32: %w", field, err)
	}
	return nil
}

func ValidateDID(field, did string) error {
	if did == "" {
		return fmt.Errorf("%s: empty", field)
	}
	if len(did) > DIDMaxLen {
		return fmt.Errorf("%s: too long (>%d)", field, DIDMaxLen)
	}
	if !strings.HasPrefix(did, "did:") {
		return fmt.Errorf("%s: must start with 'did:'", field)
	}
	return nil
}

func ValidateJurisdiction(field, j string) error {
	if j == "GLOBAL" {
		return nil
	}
	if len(j) != JurisCodeLen {
		return fmt.Errorf("%s: ISO-3166-1 alpha-2 or 'GLOBAL', got %q", field, j)
	}
	for _, r := range j {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%s: ISO code must be uppercase A-Z, got %q", field, j)
		}
	}
	return nil
}

func ValidateHash(field, h string) error {
	if h == "" {
		return nil
	}
	if len(h) != HashHexLen {
		return fmt.Errorf("%s: must be sha256 hex (%d chars), got %d", field, HashHexLen, len(h))
	}
	for _, r := range h {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return fmt.Errorf("%s: non-hex char %q", field, r)
		}
	}
	return nil
}

func ValidateDenom(d string) error {
	if d == "" {
		return fmt.Errorf("denom: empty")
	}
	if len(d) > DenomMaxLen {
		return fmt.Errorf("denom: too long (>%d)", DenomMaxLen)
	}
	return nil
}

func ValidateURI(field, u string, max uint32) error {
	if u == "" {
		return nil
	}
	if uint32(len(u)) > max {
		return fmt.Errorf("%s: too long (%d > %d)", field, len(u), max)
	}
	return nil
}

func ValidateNonEmpty(field, s string, max int) error {
	if s == "" {
		return fmt.Errorf("%s: empty", field)
	}
	if len(s) > max {
		return fmt.Errorf("%s: too long (%d > %d)", field, len(s), max)
	}
	return nil
}

func ValidateSubjectRef(s string) error {
	if s == "" {
		return fmt.Errorf("subject_ref: empty")
	}
	if len(s) > SubjectRefMaxLen {
		return fmt.Errorf("subject_ref: too long (%d > %d)", len(s), SubjectRefMaxLen)
	}
	return nil
}

func ValidateMemo(s string, max uint32) error {
	if uint32(len(s)) > max {
		return fmt.Errorf("memo: too long (%d > %d)", len(s), max)
	}
	return nil
}

func ValidateReason(s string, max uint32) error {
	if uint32(len(s)) > max {
		return fmt.Errorf("reason: too long (%d > %d)", len(s), max)
	}
	return nil
}

// ---- Enum guards -------------------------------------------------------

func ArbitratorStatusValid(s ArbitratorStatus) bool {
	switch s {
	case ArbitratorStatus_ARBITRATOR_STATUS_ACCREDITED,
		ArbitratorStatus_ARBITRATOR_STATUS_SUSPENDED,
		ArbitratorStatus_ARBITRATOR_STATUS_REVOKED:
		return true
	}
	return false
}

func SubjectKindValid(s SubjectKind) bool {
	switch s {
	case SubjectKind_SUBJECT_KIND_ORACLE_TOPIC,
		SubjectKind_SUBJECT_KIND_METER_READING,
		SubjectKind_SUBJECT_KIND_CONTRACT_SETTLEMENT,
		SubjectKind_SUBJECT_KIND_MARKET_FILL,
		SubjectKind_SUBJECT_KIND_EAC_ISSUANCE,
		SubjectKind_SUBJECT_KIND_CARBON_RETIRE,
		SubjectKind_SUBJECT_KIND_OTHER:
		return true
	}
	return false
}

func DisputeStatusValid(s DisputeStatus) bool {
	switch s {
	case DisputeStatus_DISPUTE_STATUS_OPEN,
		DisputeStatus_DISPUTE_STATUS_RESPONDED,
		DisputeStatus_DISPUTE_STATUS_DELIBERATING,
		DisputeStatus_DISPUTE_STATUS_RESOLVED,
		DisputeStatus_DISPUTE_STATUS_CANCELLED:
		return true
	}
	return false
}

func RulingOutcomeValid(r RulingOutcome) bool {
	switch r {
	case RulingOutcome_RULING_OUTCOME_PLAINTIFF,
		RulingOutcome_RULING_OUTCOME_RESPONDENT,
		RulingOutcome_RULING_OUTCOME_SPLIT,
		RulingOutcome_RULING_OUTCOME_NO_FAULT:
		return true
	}
	return false
}

func VoteChoiceValid(c VoteChoice) bool {
	switch c {
	case VoteChoice_VOTE_CHOICE_PLAINTIFF,
		VoteChoice_VOTE_CHOICE_RESPONDENT,
		VoteChoice_VOTE_CHOICE_SPLIT,
		VoteChoice_VOTE_CHOICE_NO_FAULT,
		VoteChoice_VOTE_CHOICE_ABSTAIN:
		return true
	}
	return false
}

func DisputeIsTerminal(s DisputeStatus) bool {
	return s == DisputeStatus_DISPUTE_STATUS_RESOLVED ||
		s == DisputeStatus_DISPUTE_STATUS_CANCELLED
}

func DisputeIsActive(s DisputeStatus) bool {
	return s == DisputeStatus_DISPUTE_STATUS_OPEN ||
		s == DisputeStatus_DISPUTE_STATUS_RESPONDED ||
		s == DisputeStatus_DISPUTE_STATUS_DELIBERATING
}
