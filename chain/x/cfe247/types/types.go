package types

import (
	"fmt"
	"regexp"
	"time"
)

const (
	ProviderIDMaxLen   = 64
	GridZoneIDMaxLen   = 64
	SubjectIDMaxLen    = 64
	DisplayNameMaxLen  = 128
	DIDMaxLen          = 256
	CountryCodeMaxLen  = 8
	DescriptionMaxLen  = 256
	BatchRefMaxLen     = 128
	ReportURIMaxLen    = 512
	ReportHashSize     = 64 // hex sha256
	ReasonMaxLen       = 512

	DefaultMaxDataProviders        uint32 = 256
	DefaultMaxGridZones            uint32 = 1024
	DefaultMaxSubjects             uint32 = 8_192
	DefaultMaxAttestationsPerBatch uint32 = 168 // a week of hours
	DefaultMaxMatchesPerCall       uint32 = 1
	DefaultMaxReportPeriodDays     uint32 = 366

	MaxAttestationsPerBatchUpper uint32 = 24 * 31
	MaxReportPeriodDaysUpper     uint32 = 366 * 5
)

var (
	idRe          = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	displayNameRe = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	countryRe     = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,8}$`)
	descRe        = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()\[\],;\n]{0,256}$`)
	batchRefRe    = regexp.MustCompile(`^[A-Za-z0-9._\-:/]{0,128}$`)
	hashRe        = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
)

func ValidateID(id, label string) error {
	if !idRe.MatchString(id) {
		return fmt.Errorf("%s id %q must match [a-z][a-z0-9._-]{0,63}", label, id)
	}
	return nil
}

func ValidateDisplayName(s string) error {
	if !displayNameRe.MatchString(s) {
		return fmt.Errorf("display_name %q out of allowed character set", s)
	}
	return nil
}

func ValidateDID(did string) error {
	if did == "" {
		return fmt.Errorf("did must be non-empty")
	}
	if len(did) > DIDMaxLen {
		return fmt.Errorf("did too long (max %d)", DIDMaxLen)
	}
	if len(did) < 4 || did[:4] != "did:" {
		return fmt.Errorf("did %q must start with 'did:'", did)
	}
	return nil
}

func ValidateCountry(s string) error {
	if !countryRe.MatchString(s) {
		return fmt.Errorf("country %q must match [A-Za-z0-9._-]{1,8}", s)
	}
	return nil
}

func ValidateDescription(s string) error {
	if !descRe.MatchString(s) {
		return fmt.Errorf("description out of allowed character set")
	}
	return nil
}

func ValidateBatchRef(s string) error {
	if !batchRefRe.MatchString(s) {
		return fmt.Errorf("meter_batch_id %q out of allowed character set", s)
	}
	return nil
}

func ValidateReportURI(s string) error {
	if len(s) > ReportURIMaxLen {
		return fmt.Errorf("report_uri too long (max %d)", ReportURIMaxLen)
	}
	return nil
}

func ValidateReportHash(s string) error {
	if !hashRe.MatchString(s) {
		return fmt.Errorf("report_hash %q must be 64-char hex (sha256)", s)
	}
	return nil
}

func ValidateReason(r string) error {
	if len(r) > ReasonMaxLen {
		return fmt.Errorf("reason too long (max %d)", ReasonMaxLen)
	}
	return nil
}

// HourAligned returns true iff t is a non-negative unix timestamp at
// the start of an hour.
func HourAligned(t int64) bool { return t >= 0 && t%HourSeconds == 0 }

func ValidateHourStart(t int64) error {
	if !HourAligned(t) {
		return fmt.Errorf("hour_start %d must be aligned to %d-second boundary", t, HourSeconds)
	}
	return nil
}

// HourYear returns the calendar year (UTC) the given hour_start
// belongs to.
func HourYear(t int64) uint32 {
	return uint32(time.Unix(t, 0).UTC().Year())
}

func DataProviderStatusValid(s DataProviderStatus) bool {
	switch s {
	case DataProviderStatus_DATA_PROVIDER_STATUS_ACTIVE,
		DataProviderStatus_DATA_PROVIDER_STATUS_SUSPENDED,
		DataProviderStatus_DATA_PROVIDER_STATUS_REVOKED:
		return true
	default:
		return false
	}
}

func SubjectStatusValid(s SubjectStatus) bool {
	switch s {
	case SubjectStatus_SUBJECT_STATUS_ACTIVE,
		SubjectStatus_SUBJECT_STATUS_INACTIVE:
		return true
	default:
		return false
	}
}

func ReportFormatValid(f ReportFormat) bool {
	switch f {
	case ReportFormat_REPORT_FORMAT_CFE_COMPACT,
		ReportFormat_REPORT_FORMAT_ENERGYTAG,
		ReportFormat_REPORT_FORMAT_GHG_PROTOCOL,
		ReportFormat_REPORT_FORMAT_RE100:
		return true
	default:
		return false
	}
}

// SafeAdd / SafeSub bound the aggregate / score arithmetic.
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

// Min64 returns the smaller of two uint64 values.
func Min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// BasisPoints scales numerator/denominator into 0..10000.
// Returns 0 when denominator is 0. Caps at 10000 so a buggy upstream
// that over-allocates doesn't surface > 100% scores.
func BasisPoints(num, den uint64) uint32 {
	if den == 0 {
		return 0
	}
	bps := (num * 10000) / den
	if bps > 10000 {
		bps = 10000
	}
	return uint32(bps)
}
