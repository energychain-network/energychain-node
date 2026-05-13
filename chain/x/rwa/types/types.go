package types

import (
	"fmt"
	"math/big"
	"regexp"
)

const (
	IssuerIDMaxLen   = 64
	IssuerNameMaxLen = 128
	DIDMaxLen        = 256

	TokenSymbolMaxLen   = 32
	TokenDisplayMaxLen  = 128
	JurisdictionMaxLen  = 32
	PolicyIDMaxLen      = 64
	StablecoinDenomMax  = 64
	ReasonMaxLen        = 512
	MemoMaxLen          = 256

	MaxDecimals          uint32 = 18
	MaxRedemptionDelay   int64  = 365 * 24 * 3600 // 1 year
	MaxLockupHorizon     int64  = 50 * 365 * 24 * 3600
	MaxFutureRedemption  int64  = 365 * 24 * 3600

	DefaultMaxIssuers                     uint32 = 256
	DefaultMaxTokens                      uint32 = 4_096
	DefaultMaxHoldersPerSnapshot          uint32 = 50_000
	DefaultMaxPendingRedemptionsPerHolder uint32 = 64
	DefaultMaxLockupsPerHolder            uint32 = 64

	HardMaxIssuers              uint32 = 16_384
	HardMaxTokens               uint32 = 65_535
	HardMaxHoldersPerSnapshot   uint32 = 500_000
	HardMaxPendingRedemptions   uint32 = 4_096
	HardMaxLockupsPerHolder     uint32 = 4_096
)

var (
	issuerIDRe        = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	displayNameRe     = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	tokenSymbolRe     = regexp.MustCompile(`^[A-Z][A-Z0-9._\-]{0,31}$`)
	jurisdictionRe    = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,32}$`)
	policyIDRe        = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	stablecoinDenomRe = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
)

func ValidateIssuerID(id string) error {
	if !issuerIDRe.MatchString(id) {
		return fmt.Errorf("issuer id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateIssuerName(n string) error {
	if !displayNameRe.MatchString(n) {
		return fmt.Errorf("display_name %q out of allowed character set", n)
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

func ValidateTokenSymbol(s string) error {
	if !tokenSymbolRe.MatchString(s) {
		return fmt.Errorf("token symbol %q must match [A-Z][A-Z0-9._-]{0,31}", s)
	}
	return nil
}

func ValidateDisplayName(n string) error {
	if !displayNameRe.MatchString(n) {
		return fmt.Errorf("display_name %q out of allowed character set", n)
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

func ValidatePolicyID(id string) error {
	if id == "" {
		return nil
	}
	if !policyIDRe.MatchString(id) {
		return fmt.Errorf("policy_id %q must match [a-z][a-z0-9._-]{0,63}", id)
	}
	return nil
}

func ValidateStablecoinDenom(d string) error {
	if d == "" {
		return fmt.Errorf("settlement_denom must be non-empty")
	}
	if !stablecoinDenomRe.MatchString(d) {
		return fmt.Errorf("settlement_denom %q must match [a-z][a-z0-9._-]{0,63}", d)
	}
	return nil
}

func ValidateOptionalStablecoinDenom(d string) error {
	if d == "" {
		return nil
	}
	return ValidateStablecoinDenom(d)
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

func IssuerStatusValid(s IssuerStatus) bool {
	switch s {
	case IssuerStatus_ISSUER_STATUS_ACTIVE,
		IssuerStatus_ISSUER_STATUS_SUSPENDED,
		IssuerStatus_ISSUER_STATUS_REVOKED:
		return true
	}
	return false
}

func TokenStatusValid(s TokenStatus) bool {
	switch s {
	case TokenStatus_TOKEN_STATUS_ACTIVE,
		TokenStatus_TOKEN_STATUS_PAUSED,
		TokenStatus_TOKEN_STATUS_TERMINATED:
		return true
	}
	return false
}

func AssetClassValid(c AssetClass) bool {
	switch c {
	case AssetClass_ASSET_CLASS_PPA,
		AssetClass_ASSET_CLASS_VPPA,
		AssetClass_ASSET_CLASS_GREEN_BOND,
		AssetClass_ASSET_CLASS_PROJECT_EQUITY,
		AssetClass_ASSET_CLASS_STORAGE_SHARE,
		AssetClass_ASSET_CLASS_RECEIVABLE,
		AssetClass_ASSET_CLASS_FUND_UNIT,
		AssetClass_ASSET_CLASS_OTHER:
		return true
	}
	return false
}

func DistributionStatusValid(s DistributionStatus) bool {
	switch s {
	case DistributionStatus_DISTRIBUTION_STATUS_CREATED,
		DistributionStatus_DISTRIBUTION_STATUS_FUNDED,
		DistributionStatus_DISTRIBUTION_STATUS_FINALIZED:
		return true
	}
	return false
}

func RedemptionStatusValid(s RedemptionStatus) bool {
	switch s {
	case RedemptionStatus_REDEMPTION_STATUS_PENDING,
		RedemptionStatus_REDEMPTION_STATUS_SETTLED,
		RedemptionStatus_REDEMPTION_STATUS_CANCELLED:
		return true
	}
	return false
}

// SafeAdd / SafeSub guard ledger arithmetic.
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

// MulDivFloor performs floor((x * y) / div) using big.Int so the
// dividend / per-unit math cannot silently overflow uint64.
// Returns an error when the result does not fit in a uint64.
func MulDivFloor(x, y, div uint64) (uint64, error) {
	if div == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	if x == 0 || y == 0 {
		return 0, nil
	}
	bx := new(big.Int).SetUint64(x)
	by := new(big.Int).SetUint64(y)
	bd := new(big.Int).SetUint64(div)
	prod := new(big.Int).Mul(bx, by)
	q := new(big.Int).Quo(prod, bd)
	if !q.IsUint64() {
		return 0, fmt.Errorf("result %s exceeds uint64", q.String())
	}
	return q.Uint64(), nil
}
