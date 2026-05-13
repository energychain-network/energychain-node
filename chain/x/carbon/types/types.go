package types

import (
	"fmt"
	"regexp"
)

const (
	IssuerIDMaxLen   = 64
	IssuerNameMaxLen = 128
	DIDMaxLen        = 256

	RegistryMaxLen     = 64
	ProgramMaxLen      = 32
	ProjectIDMaxLen    = 128
	MethodologyMaxLen  = 128
	JurisdictionMaxLen = 32
	SourceSerialMaxLen = 256
	PolicyIDMaxLen     = 64
	OracleTopicMaxLen  = 64

	CCPLabelMaxLen   = 64
	PurposeMaxLen    = 64
	ClaimMaxLen      = 256
	ReasonMaxLen     = 512
	MemoMaxLen       = 256
	DocumentURIMax   = 512
	DocumentHashSize = 64 // hex-encoded sha256

	DefaultMaxIssuers              uint32 = 256
	MaxIssuersUpper                uint32 = 4_096
	DefaultMaxCategoriesPerIssuer  uint32 = 2
	MaxCategoriesPerIssuerUpper    uint32 = 8
	DefaultMaxUnitsPerBatch        uint32 = 1_000_000_000 // 1B tCO2e
	DefaultMaxCCPLabels            uint32 = 16

	VintageYearMin uint32 = 1990
	VintageYearMax uint32 = 2100
)

var (
	issuerIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	issuerNameRe     = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	registryRe       = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,64}$`)
	programRe        = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	projectIDRe      = regexp.MustCompile(`^[a-zA-Z0-9._\-:/]{0,128}$`)
	methodologyRe    = regexp.MustCompile(`^[a-zA-Z0-9._\-:/]{0,128}$`)
	jurisdictionRe   = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	sourceSerialRe   = regexp.MustCompile(`^[A-Za-z0-9._\-:/]{0,256}$`)
	policyIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	oracleTopicRe    = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	purposeRe        = regexp.MustCompile(`^[a-z][a-z0-9._\-:]{0,63}$`)
	ccpLabelRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-:]{0,63}$`)
	documentHashRe   = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
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

func ValidateRegistry(s string) error {
	if !registryRe.MatchString(s) {
		return fmt.Errorf("registry %q must match [A-Za-z0-9._-]{1,64}", s)
	}
	return nil
}

func ValidateProgram(s string) error {
	if !programRe.MatchString(s) {
		return fmt.Errorf("program %q must match [A-Za-z0-9._-]{1,32}", s)
	}
	return nil
}

func ValidateProjectID(s string) error {
	if !projectIDRe.MatchString(s) {
		return fmt.Errorf("project_id %q out of allowed character set", s)
	}
	return nil
}

func ValidateMethodology(s string) error {
	if !methodologyRe.MatchString(s) {
		return fmt.Errorf("methodology %q out of allowed character set", s)
	}
	return nil
}

func ValidateJurisdiction(s string) error {
	if !jurisdictionRe.MatchString(s) {
		return fmt.Errorf("jurisdiction %q must match [A-Za-z0-9._-]{1,32}", s)
	}
	return nil
}

func ValidateOptionalJurisdiction(s string) error {
	if s == "" {
		return nil
	}
	return ValidateJurisdiction(s)
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

func ValidateClaim(c string) error {
	if len(c) > ClaimMaxLen {
		return fmt.Errorf("claim too long (max %d)", ClaimMaxLen)
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

func ValidateCCPLabels(labels []string, max uint32) error {
	if uint32(len(labels)) > max {
		return fmt.Errorf("too many ccp_labels (got %d, max %d)", len(labels), max)
	}
	seen := map[string]bool{}
	for _, l := range labels {
		if seen[l] {
			return fmt.Errorf("duplicate ccp_label %q", l)
		}
		seen[l] = true
		if !ccpLabelRe.MatchString(l) {
			return fmt.Errorf("ccp_label %q must match [a-z][a-z0-9._-:]{0,63}", l)
		}
	}
	return nil
}

func ValidateDocumentURI(s string) error {
	if len(s) > DocumentURIMax {
		return fmt.Errorf("document_uri too long (max %d)", DocumentURIMax)
	}
	return nil
}

func ValidateDocumentHash(s string) error {
	if s == "" {
		return nil
	}
	if !documentHashRe.MatchString(s) {
		return fmt.Errorf("document_hash %q must be 64-char hex (sha256)", s)
	}
	return nil
}

func ValidateVintageYear(y uint32) error {
	if y < VintageYearMin || y > VintageYearMax {
		return fmt.Errorf("vintage_year %d out of range [%d, %d]", y, VintageYearMin, VintageYearMax)
	}
	return nil
}

func AssetCategoryValid(c AssetCategory) bool {
	return c == AssetCategory_ASSET_CATEGORY_ALLOWANCE || c == AssetCategory_ASSET_CATEGORY_OFFSET
}

func AssetStatusValid(s AssetStatus) bool {
	switch s {
	case AssetStatus_ASSET_STATUS_ACTIVE,
		AssetStatus_ASSET_STATUS_SEALED,
		AssetStatus_ASSET_STATUS_FULLY_RETIRED:
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

func Article6StatusValid(s Article6Status) bool {
	switch s {
	case Article6Status_ARTICLE6_STATUS_NOT_APPLICABLE,
		Article6Status_ARTICLE6_STATUS_PENDING_AUTHORIZATION,
		Article6Status_ARTICLE6_STATUS_AUTHORIZED,
		Article6Status_ARTICLE6_STATUS_CA_APPLIED:
		return true
	default:
		return false
	}
}

// IssuerCategoryAllowed checks whether `category` is in the issuer's
// whitelist (empty = both).
func IssuerCategoryAllowed(allowed []int32, category AssetCategory) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, c := range allowed {
		if AssetCategory(c) == category {
			return true
		}
	}
	return false
}

// SafeAdd / SafeSub guard ledger arithmetic against silent overflow.
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
