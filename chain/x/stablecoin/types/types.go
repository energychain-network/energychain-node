package types

import (
	"fmt"
	"regexp"
	"strings"
)

// Hard caps. Mirrors the conservative bounds used by x/policy and
// x/sanctions so query walks stay cheap and state never explodes.
const (
	DenomIDMaxLen     = 32
	DenomSymbolMaxLen = 16
	DenomNameMaxLen   = 64

	IssuerIDMaxLen          = 64
	IssuerDisplayNameMaxLen = 128
	DIDMaxLen               = 256

	JurisdictionMaxLen = 8
	PolicyIDMaxLen     = 64
	ReasonMaxLen       = 512
	MemoMaxLen         = 256
	PayoutRefMaxLen    = 256
	OracleTopicMaxLen  = 64
	CredentialMaxLen   = 256

	// Decimals must be reasonable. ERC-20 / SPL stablecoins use 6 / 18.
	DecimalsMax uint32 = 18

	DefaultMaxDenoms                       uint32 = 64
	MaxDenomsUpper                         uint32 = 256
	DefaultMaxIssuers                      uint32 = 256
	MaxIssuersUpper                        uint32 = 4_096
	DefaultMaxMintAuthoritiesPerIssuer     uint32 = 16
	MaxMintAuthoritiesPerIssuerUpper       uint32 = 64
	DefaultMaxRedemptionsPendingPerHolder  uint32 = 32
	MaxRedemptionsPendingPerHolderUpper    uint32 = 256
)

const MaxQueryResults = 1000

// Stable identifier shapes — same alphanumeric / dot / dash /
// underscore set used elsewhere.
var (
	denomIDRe       = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,31}$`)
	denomSymbolRe   = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,16}$`)
	denomNameRe     = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,64}$`)
	issuerIDRe      = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	issuerNameRe    = regexp.MustCompile(`^[A-Za-z0-9 _.\-:/()]{1,128}$`)
	jurisdictionRe  = regexp.MustCompile(`^[A-Z]{0,8}$`)
	policyIDRe      = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
	oracleTopicRe   = regexp.MustCompile(`^[a-z][a-z0-9._\-]{0,63}$`)
)

func ValidateDenomID(id string) error {
	if !denomIDRe.MatchString(id) {
		return fmt.Errorf("denom id %q must match [a-z][a-z0-9._-]{0,31}", id)
	}
	return nil
}

func ValidateDenomSymbol(sym string) error {
	if !denomSymbolRe.MatchString(sym) {
		return fmt.Errorf("denom symbol %q out of allowed character set", sym)
	}
	return nil
}

func ValidateDenomName(n string) error {
	if !denomNameRe.MatchString(n) {
		return fmt.Errorf("denom name %q out of allowed character set", n)
	}
	return nil
}

func ValidateDecimals(d uint32) error {
	if d > DecimalsMax {
		return fmt.Errorf("decimals %d exceeds max %d", d, DecimalsMax)
	}
	return nil
}

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
	if !strings.HasPrefix(did, "did:") {
		return fmt.Errorf("did %q must start with 'did:'", did)
	}
	return nil
}

func ValidateJurisdiction(code string) error {
	if !jurisdictionRe.MatchString(code) {
		return fmt.Errorf("jurisdiction %q must be 0-8 uppercase letters", code)
	}
	return nil
}

// ValidatePolicyID accepts the empty string (no binding) or a
// well-formed x/policy id.
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

func ValidatePayoutRef(p string) error {
	if len(p) > PayoutRefMaxLen {
		return fmt.Errorf("payout_ref too long (max %d)", PayoutRefMaxLen)
	}
	return nil
}

func ValidateCredentialType(c string) error {
	if len(c) > CredentialMaxLen {
		return fmt.Errorf("credential type too long (max %d)", CredentialMaxLen)
	}
	return nil
}

// DenomStatusValid filters the zero (UNSPECIFIED) variant.
func DenomStatusValid(s DenomStatus) bool {
	switch s {
	case DenomStatus_DENOM_STATUS_ACTIVE,
		DenomStatus_DENOM_STATUS_PAUSED,
		DenomStatus_DENOM_STATUS_RETIRED:
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

func RedemptionStatusValid(s RedemptionStatus) bool {
	switch s {
	case RedemptionStatus_REDEMPTION_STATUS_PENDING,
		RedemptionStatus_REDEMPTION_STATUS_FULFILLED,
		RedemptionStatus_REDEMPTION_STATUS_CANCELLED:
		return true
	default:
		return false
	}
}

// SafeAdd returns x + y, refusing the operation on overflow. Stablecoin
// supply / quotas / balances are uint64 and we MUST never silently
// wrap because that would let an attacker mint past `MaxUint64` to a
// small balance. The keeper aborts with an explicit error instead.
func SafeAdd(x, y uint64) (uint64, error) {
	if y > 0 && x > ^uint64(0)-y {
		return 0, fmt.Errorf("uint64 overflow: %d + %d", x, y)
	}
	return x + y, nil
}

// SafeSub returns x - y, refusing the operation on underflow.
func SafeSub(x, y uint64) (uint64, error) {
	if y > x {
		return 0, fmt.Errorf("uint64 underflow: %d - %d", x, y)
	}
	return x - y, nil
}

// U128 is a small high:low pair used by SafeMulU128 to compare two
// 128-bit products without pulling math/big into the hot path. We
// only need addition-free comparisons (no division), so this is a
// minimal carrier type.
type U128 struct {
	Hi, Lo uint64
}

// SafeMulU128 returns the full 128-bit product of two uint64s. Used
// by the reserve-coverage check where outstanding * 10_000 may
// already exceed 2^64 for hyperinflated denominations. Output is
// always exact; the caller compares two U128s.
func SafeMulU128(x, y uint64) (U128, error) {
	const mask32 = uint64(0xFFFFFFFF)
	xLo, xHi := x&mask32, x>>32
	yLo, yHi := y&mask32, y>>32

	t00 := xLo * yLo
	t01 := xLo * yHi
	t10 := xHi * yLo
	t11 := xHi * yHi

	lo := t00 + (t01 << 32)
	carry := uint64(0)
	if lo < t00 {
		carry = 1
	}
	lo2 := lo + (t10 << 32)
	if lo2 < lo {
		carry++
	}
	hi := t11 + (t01 >> 32) + (t10 >> 32) + carry
	return U128{Hi: hi, Lo: lo2}, nil
}
