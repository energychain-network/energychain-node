package types

import (
	"fmt"
	"regexp"
	"strings"

	sdkmath "cosmossdk.io/math"
)

// Hard caps and defaults for Params validation.
const (
	DefaultMinSubmissions    uint32 = 3
	DefaultDataMaxAge        int64  = 600 // 10 minutes
	DefaultMaxMetadataSize   uint32 = 4_096
	MaxMetadataSizeUpper     uint32 = 1_048_576 // 1 MiB
	DefaultBondDenom                = "uec"
	DefaultBondReleaseCool   int64  = 100_000 // ≈ 1.5 days at 1.5s blocks
	BondReleaseCooldownUpper int64  = 100_000_000

	DefaultMaxTopicsPerBlock     uint32 = 64
	MaxTopicsPerBlockUpper       uint32 = 4_096
	DefaultMaxSubmissionsPerTopic uint32 = 256
	MaxSubmissionsPerTopicUpper   uint32 = 4_096

	OutlierBandUpperBps uint32 = 50_000 // 500% — anything wider is meaningless
	ValueDecimalsUpper  uint32 = 36

	TopicIDMaxLen      = 128
	ProviderNameMaxLen = 128
	ContactURIMaxLen   = 512
	QuoteMaxLen        = 32
	ReasonMaxLen       = 256

	// Reserve attestation bounds. The signers list is held verbatim; cap
	// it so a single attestation can't blow a block.
	ReserveAssetMaxLen        = 32
	ReserveURIMaxLen          = 512
	ReserveHashHexLen         = 64
	ReserveMaxSigners         = 32
	ReserveAmountStringMaxLen = 96
)

// MaxQueryResults caps the size of a single list-walk response.
const MaxQueryResults = 1000

var (
	// topicIDRe matches the dotted "namespace.subtype.market.symbol" format
	// the docs propose (e.g. "power.spot.PJM.WHUB"). Allowed characters
	// are alphanumerics plus dot, dash, underscore. We deliberately
	// disallow anything that could be ambiguous in URLs / file paths.
	topicIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,127}$`)
)

// ValidateTopicID enforces the canonical id shape.
func ValidateTopicID(id string) error {
	if !topicIDRe.MatchString(id) {
		return fmt.Errorf("topic id %q must match [A-Za-z0-9][A-Za-z0-9._-]{0,127}", id)
	}
	return nil
}

// AggregationFnValid returns true for known enum values.
func AggregationFnValid(v AggregationFn) bool {
	switch v {
	case AggregationFn_AGG_MEDIAN,
		AggregationFn_AGG_MEAN,
		AggregationFn_AGG_TRIMMED_MEAN,
		AggregationFn_AGG_MEDIAN_OF_MEDIANS:
		return true
	default:
		return false
	}
}

// TopicKindValid returns true for known enum values.
func TopicKindValid(v TopicKind) bool {
	switch v {
	case TopicKind_TOPIC_KIND_GENERIC,
		TopicKind_TOPIC_KIND_PRICE,
		TopicKind_TOPIC_KIND_FX,
		TopicKind_TOPIC_KIND_CARBON,
		TopicKind_TOPIC_KIND_RESERVE,
		TopicKind_TOPIC_KIND_WEATHER,
		TopicKind_TOPIC_KIND_INDEX:
		return true
	default:
		return false
	}
}

// ProviderStatusValid returns true for non-default enum values; the
// UNSPECIFIED zero value is reserved for "fresh" structs and never
// stored.
func ProviderStatusValid(v ProviderStatus) bool {
	switch v {
	case ProviderStatus_PROVIDER_STATUS_ACTIVE,
		ProviderStatus_PROVIDER_STATUS_SUSPENDED,
		ProviderStatus_PROVIDER_STATUS_WITHDRAWING,
		ProviderStatus_PROVIDER_STATUS_RETIRED:
		return true
	default:
		return false
	}
}

// ParseAmountString validates a decimal big-int string and returns the
// parsed sdkmath.Int. Used by both bond and reserve amount fields.
func ParseAmountString(s string) (sdkmath.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return sdkmath.ZeroInt(), fmt.Errorf("amount empty")
	}
	v, ok := sdkmath.NewIntFromString(s)
	if !ok {
		return sdkmath.ZeroInt(), fmt.Errorf("amount %q is not a base-10 integer", s)
	}
	if v.IsNegative() {
		return sdkmath.ZeroInt(), fmt.Errorf("amount %s must be non-negative", s)
	}
	return v, nil
}
