package types

import (
	"fmt"
	"regexp"
)

const (
	IssuerIDMaxLen   = 64
	IssuerNameMaxLen = 128
	DIDMaxLen        = 256

	ProjectIDMaxLen      = 128
	DeviceIDMaxLen       = 128
	GridZoneMaxLen       = 64
	SourceRegistryMaxLen = 64
	SourceSerialMaxLen   = 256
	PolicyIDMaxLen       = 64
	OracleTopicMaxLen    = 64

	PurposeMaxLen = 64
	ReasonMaxLen  = 512
	MemoMaxLen    = 256

	DefaultMaxIssuers              uint32 = 256
	MaxIssuersUpper                uint32 = 4_096
	DefaultMaxKindsPerIssuer       uint32 = 8
	MaxKindsPerIssuerUpper         uint32 = 32
	DefaultMaxUnitsPerBatch        uint64 = 1_000_000_000 // 1 TWh in MWh
	DefaultHourWindowSeconds       uint32 = 3_600

	// VintageYearMin/Max bound the production year. 1990 is well
	// before any RE certificate program existed; 2100 is a safe
	// far-future bound (re-tunable via upgrade).
	VintageYearMin uint32 = 1990
	VintageYearMax uint32 = 2100
)

const MaxQueryResults = 1000

var (
	issuerIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	issuerNameRe     = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	projectIDRe      = regexp.MustCompile(`^[a-zA-Z0-9._\-:/]{1,128}$`)
	deviceIDRe       = regexp.MustCompile(`^[a-zA-Z0-9._\-:/]{0,128}$`)
	gridZoneRe       = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,64}$`)
	sourceRegistryRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{0,64}$`)
	sourceSerialRe   = regexp.MustCompile(`^[A-Za-z0-9._\-:/]{0,256}$`)
	policyIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	oracleTopicRe    = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	purposeRe        = regexp.MustCompile(`^[a-z][a-z0-9._\-:]{0,63}$`)
)

func ValidateIssuerID(id string) error {
	if !issuerIDRe.MatchString(id) {
		return fmt.Errorf("issuer id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateIssuerName(n string) error {
	if !issuerNameRe.MatchString(n) {
		return fmt.Errorf("issuer display_name %q out of allowed character set", n)
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

func ValidateProjectID(id string) error {
	if !projectIDRe.MatchString(id) {
		return fmt.Errorf("project_id %q out of allowed character set", id)
	}
	return nil
}

func ValidateDeviceID(id string) error {
	if !deviceIDRe.MatchString(id) {
		return fmt.Errorf("device_id %q out of allowed character set", id)
	}
	return nil
}

func ValidateGridZone(z string) error {
	if !gridZoneRe.MatchString(z) {
		return fmt.Errorf("grid_zone %q must match [A-Za-z0-9._-]{1,64}", z)
	}
	return nil
}

func ValidateSourceRegistry(s string) error {
	if !sourceRegistryRe.MatchString(s) {
		return fmt.Errorf("source_registry %q out of allowed character set", s)
	}
	return nil
}

func ValidateSourceSerial(s string) error {
	if !sourceSerialRe.MatchString(s) {
		return fmt.Errorf("source_serial %q out of allowed character set", s)
	}
	return nil
}

func ValidatePolicyID(id string) error {
	if id == "" {
		return nil
	}
	if !policyIDRe.MatchString(id) {
		return fmt.Errorf("policy_id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateOracleTopic(id string) error {
	if id == "" {
		return fmt.Errorf("oracle_topic_id must be non-empty")
	}
	if !oracleTopicRe.MatchString(id) {
		return fmt.Errorf("oracle_topic_id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidatePurpose(p string) error {
	if p == "" {
		return nil
	}
	if !purposeRe.MatchString(p) {
		return fmt.Errorf("purpose %q must match [a-z][a-z0-9._-:]{0,63}", p)
	}
	return nil
}

func ValidateReason(r string) error {
	if len(r) > ReasonMaxLen {
		return fmt.Errorf("reason too long (max %d)", ReasonMaxLen)
	}
	return nil
}

func ValidateMemo(m string) error {
	if len(m) > MemoMaxLen {
		return fmt.Errorf("memo too long (max %d)", MemoMaxLen)
	}
	return nil
}

func ValidateVintageYear(y uint32) error {
	if y < VintageYearMin || y > VintageYearMax {
		return fmt.Errorf("vintage_year %d out of range [%d, %d]", y, VintageYearMin, VintageYearMax)
	}
	return nil
}

// CertificateKindValid filters the zero (UNSPECIFIED) variant.
func CertificateKindValid(k CertificateKind) bool {
	switch k {
	case CertificateKind_CERTIFICATE_KIND_IREC,
		CertificateKind_CERTIFICATE_KIND_EU_GO,
		CertificateKind_CERTIFICATE_KIND_US_REC,
		CertificateKind_CERTIFICATE_KIND_CN_GEC,
		CertificateKind_CERTIFICATE_KIND_AU_LGC,
		CertificateKind_CERTIFICATE_KIND_JP_J_CREDIT,
		CertificateKind_CERTIFICATE_KIND_NATIVE:
		return true
	default:
		return false
	}
}

func TechnologyValid(t Technology) bool {
	switch t {
	case Technology_TECHNOLOGY_SOLAR,
		Technology_TECHNOLOGY_WIND,
		Technology_TECHNOLOGY_HYDRO,
		Technology_TECHNOLOGY_NUCLEAR,
		Technology_TECHNOLOGY_BIOMASS,
		Technology_TECHNOLOGY_GEOTHERMAL,
		Technology_TECHNOLOGY_STORAGE,
		Technology_TECHNOLOGY_OTHER:
		return true
	default:
		return false
	}
}

func IssuerStatusValid(s IssuerStatus) bool {
	switch s {
	case IssuerStatus_ISSUER_STATUS_ACTIVE,
		IssuerStatus_ISSUER_STATUS_SUSPENDED,
		IssuerStatus_ISSUER_STATUS_REVOKED:
		return true
	default:
		return false
	}
}

func CertificateStatusValid(s CertificateStatus) bool {
	switch s {
	case CertificateStatus_CERTIFICATE_STATUS_ACTIVE,
		CertificateStatus_CERTIFICATE_STATUS_SEALED,
		CertificateStatus_CERTIFICATE_STATUS_FULLY_RETIRED:
		return true
	default:
		return false
	}
}

// SafeAdd / SafeSub guard the certificate ledger against silent
// uint64 wrap-around. Same pattern used in x/stablecoin.
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

// IssuerKindAllowed returns true iff `kind` is in the issuer's
// whitelist or the whitelist is empty (= any kind).
func IssuerKindAllowed(allowed []int32, kind CertificateKind) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, k := range allowed {
		if CertificateKind(k) == kind {
			return true
		}
	}
	return false
}
