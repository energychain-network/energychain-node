package types

import (
	"fmt"
	"math"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Limits. The DEFAULT constants are the params the chain ships
// with; the HARD constants are the absolute ceilings beyond
// which `Params.Validate` will reject any governance update.
// HARD ceilings exist so a misconfigured governance proposal
// can never trip O(N) state walks or unbounded URI/string
// memory growth.
const (
	DefaultMaxSchemas                  uint32 = 1024
	HardMaxSchemas                     uint32 = 65536
	DefaultMaxVerifiers                uint32 = 256
	HardMaxVerifiers                   uint32 = 16384
	DefaultMaxReportsPerSubject        uint32 = 65536
	HardMaxReportsPerSubject           uint32 = 1_048_576
	DefaultMaxViewKeyGrants            uint32 = 4096
	HardMaxViewKeyGrants               uint32 = 1_048_576
	DefaultMemoMaxLen                  uint32 = 256
	HardMemoMaxLen                     uint32 = 1024
	DefaultURIMaxLen                   uint32 = 512
	HardURIMaxLen                      uint32 = 4096
	DefaultMaxJurisdictionsPerVerifier uint32 = 64
	HardMaxJurisdictionsPerVerifier    uint32 = 256
	DefaultMaxStandardsPerVerifier     uint32 = 64
	HardMaxStandardsPerVerifier        uint32 = 256
	DefaultReasonMaxLen                uint32 = 256
	HardReasonMaxLen                   uint32 = 1024
	DefaultMaxGrantTTLSeconds          int64  = 365 * 24 * 3600
	HardMaxGrantTTLSeconds             int64  = 5 * 365 * 24 * 3600
	DefaultMaxGrantsPerBlockSweep      uint32 = 256
	HardMaxGrantsPerBlockSweep         uint32 = 4096

	DenomMaxLen    = 64
	NameMaxLen     = 128
	VersionMaxLen  = 32
	HashHexLen     = 64 // sha256 hex
	DIDMaxLen      = 256
	JurisCodeLen   = 2 // ISO-3166-1 alpha-2; "GLOBAL" is exempt
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

// ---- Validators --------------------------------------------------------

// ValidateAddr enforces bech32 well-formedness for chain
// addresses used as subjects, granters, or verifier signers.
func ValidateAddr(field, addr string) error {
	if _, err := sdk.AccAddressFromBech32(addr); err != nil {
		return fmt.Errorf("%s: invalid bech32: %w", field, err)
	}
	return nil
}

// ValidateDID checks the DID is well-formed enough on the
// surface (non-empty, "did:" prefix, bounded length). The
// chain does NOT resolve DIDs at this layer; resolution
// happens off-chain or via the optional DIDKeeper hook.
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

// ValidateJurisdiction accepts ISO-3166-1 alpha-2 (2 upper
// letters) OR the literal "GLOBAL". Any other value is
// rejected so off-chain consumers can rely on the constraint.
func ValidateJurisdiction(field, j string) error {
	if j == "GLOBAL" {
		return nil
	}
	if len(j) != JurisCodeLen {
		return fmt.Errorf("%s: must be ISO-3166-1 alpha-2 or 'GLOBAL', got %q", field, j)
	}
	for _, r := range j {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%s: ISO code must be uppercase A-Z, got %q", field, j)
		}
	}
	return nil
}

// ValidateHash enforces hex sha256 (64 lower/upper hex chars).
// Empty hash is permitted to support payload-less placeholder
// records during draft flows; callers that require a hash
// must check non-empty separately.
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

func SchemaStatusValid(s SchemaStatus) bool {
	return s == SchemaStatus_SCHEMA_STATUS_ACTIVE ||
		s == SchemaStatus_SCHEMA_STATUS_DEPRECATED
}

func AssetClassValid(a AssetClass) bool {
	switch a {
	case AssetClass_ASSET_CLASS_METER,
		AssetClass_ASSET_CLASS_EAC,
		AssetClass_ASSET_CLASS_CARBON,
		AssetClass_ASSET_CLASS_CFE_247,
		AssetClass_ASSET_CLASS_MIXED:
		return true
	}
	return false
}

func TimeWindowValid(t TimeWindow) bool {
	switch t {
	case TimeWindow_TIME_WINDOW_HOURLY,
		TimeWindow_TIME_WINDOW_DAILY,
		TimeWindow_TIME_WINDOW_MONTHLY,
		TimeWindow_TIME_WINDOW_QUARTERLY,
		TimeWindow_TIME_WINDOW_YEARLY,
		TimeWindow_TIME_WINDOW_AD_HOC:
		return true
	}
	return false
}

func ReportFormatValid(f ReportFormat) bool {
	switch f {
	case ReportFormat_REPORT_FORMAT_JSON_LD,
		ReportFormat_REPORT_FORMAT_XBRL,
		ReportFormat_REPORT_FORMAT_ENERGYTAG,
		ReportFormat_REPORT_FORMAT_CSRD_ESRS,
		ReportFormat_REPORT_FORMAT_CDP,
		ReportFormat_REPORT_FORMAT_TCFD,
		ReportFormat_REPORT_FORMAT_GHG_PROTO,
		ReportFormat_REPORT_FORMAT_RE100:
		return true
	}
	return false
}

func ReportStatusValid(s ReportStatus) bool {
	switch s {
	case ReportStatus_REPORT_STATUS_DRAFT,
		ReportStatus_REPORT_STATUS_ATTESTED,
		ReportStatus_REPORT_STATUS_REJECTED,
		ReportStatus_REPORT_STATUS_RETRACTED:
		return true
	}
	return false
}

func VerifierStatusValid(s VerifierStatus) bool {
	switch s {
	case VerifierStatus_VERIFIER_STATUS_ACCREDITED,
		VerifierStatus_VERIFIER_STATUS_SUSPENDED,
		VerifierStatus_VERIFIER_STATUS_REVOKED:
		return true
	}
	return false
}

// ReportIsTerminal returns true when the report has reached a
// state from which no further transition is allowed.
func ReportIsTerminal(s ReportStatus) bool {
	return s == ReportStatus_REPORT_STATUS_REJECTED ||
		s == ReportStatus_REPORT_STATUS_RETRACTED
}
