package types

import (
	"fmt"
	"regexp"

	"energychain/internal/commitment"
)

// Hard caps and defaults. Values are picked to be lenient enough for
// dev / testnet but hold up under adversarial production load. Anything
// here that becomes a real bottleneck should move to Params and reach
// governance, not be silently raised in code.
const (
	MeteringPointIDMaxLen = 128
	GridZoneMaxLen        = 64
	GridOperatorMaxLen    = 64
	TariffMaxLen          = 64
	UnitMaxLen            = 16
	TimezoneMaxLen        = 64
	MetadataMaxLen        = 4_096

	// Reading bounds.
	DefaultMaxCommitmentSize uint32 = 256
	MaxCommitmentSizeUpper   uint32 = 4_096
	DefaultMaxSignatureSize  uint32 = 256
	MaxSignatureSizeUpper    uint32 = 4_096

	// Batch bounds.
	DefaultMaxBatchCount uint32 = 4_096
	MaxBatchCountUpper   uint32 = 1_048_576
	MerkleHashHexLen            = 64
	MerkleAlgoSHA256            = "sha256"
	BatchURIMaxLen              = 512

	// StreamAuthorization bounds.
	StreamMetadataMaxLen      = 1_024
	DefaultStreamMaxValidity  int64  = 31_536_000 // 1 year (seconds)
	StreamMaxValidityUpper    int64  = 31_536_000 * 5
	DefaultFutureSkewSeconds  int64  = 60
	FutureSkewSecondsUpper    int64  = 3_600
	DefaultMaxMetadataURISize uint32 = 512
	MaxMetadataURISizeUpper   uint32 = 4_096

	// Sweep bound: how many StreamAuthorization rows the EndBlocker will
	// visit per block when expiring authorisations.
	StreamSweepMaxPerBlock = 256
)

// MaxQueryResults caps any non-paginated walk so the gRPC layer cannot
// be made to return arbitrarily large result sets.
const MaxQueryResults = 1000

// meteringPointIDRe matches the canonical id format used in the docs:
// `mp.<jurisdiction>.<grid>.<seq>` style identifiers, plus arbitrary
// alphanumeric / dot / dash / underscore for legacy systems. The first
// character must be alphanumeric so a stray "." can never act as a key
// separator inside the collections range engine.
var meteringPointIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]{0,127}$`)

// ValidateMeteringPointID enforces the canonical id shape.
func ValidateMeteringPointID(id string) error {
	if !meteringPointIDRe.MatchString(id) {
		return fmt.Errorf("metering point id %q must match [A-Za-z0-9][A-Za-z0-9._-]{0,127}", id)
	}
	return nil
}

// MeasurementTypeValid returns true for known enum values; the
// UNSPECIFIED zero is rejected so unconfigured / forgotten registrations
// surface as malformed.
func MeasurementTypeValid(v MeasurementType) bool {
	switch v {
	case MeasurementType_MEASUREMENT_TYPE_ACTIVE_POWER,
		MeasurementType_MEASUREMENT_TYPE_REACTIVE_POWER,
		MeasurementType_MEASUREMENT_TYPE_VOLTAGE,
		MeasurementType_MEASUREMENT_TYPE_FREQUENCY,
		MeasurementType_MEASUREMENT_TYPE_GAS_FLOW,
		MeasurementType_MEASUREMENT_TYPE_HEAT,
		MeasurementType_MEASUREMENT_TYPE_OTHER:
		return true
	default:
		return false
	}
}

// ReadingPeriodSeconds returns the canonical period length in seconds.
// Used to enforce Reading.start_time + period == Reading.end_time so two
// providers cannot disagree about "what does '15 min' mean".
func ReadingPeriodSeconds(p ReadingPeriod) (int64, bool) {
	switch p {
	case ReadingPeriod_READING_PERIOD_15MIN:
		return 15 * 60, true
	case ReadingPeriod_READING_PERIOD_1HOUR:
		return 3600, true
	case ReadingPeriod_READING_PERIOD_1DAY:
		return 86_400, true
	default:
		return 0, false
	}
}

// DataQualityValid returns true for known enum values.
func DataQualityValid(v DataQualityFlag) bool {
	switch v {
	case DataQualityFlag_DATA_QUALITY_SETTLED,
		DataQualityFlag_DATA_QUALITY_ESTIMATED,
		DataQualityFlag_DATA_QUALITY_INTERPOLATED,
		DataQualityFlag_DATA_QUALITY_SUSPECT,
		DataQualityFlag_DATA_QUALITY_INVALID:
		return true
	default:
		return false
	}
}

// CommitmentSchemeValid returns true for known enum values.
func CommitmentSchemeValid(v CommitmentScheme) bool {
	switch v {
	case CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256,
		CommitmentScheme_COMMITMENT_SCHEME_PEDERSEN_SECP256K1:
		return true
	default:
		return false
	}
}

// CommitmentSchemeToInternal maps the proto enum to the internal/commitment
// scheme tag. Keep in sync with the constant table in
// internal/commitment/commitment.go.
func CommitmentSchemeToInternal(v CommitmentScheme) commitment.Scheme {
	switch v {
	case CommitmentScheme_COMMITMENT_SCHEME_PLAINTEXT_SHA256:
		return commitment.SchemePlaintextSHA256
	case CommitmentScheme_COMMITMENT_SCHEME_PEDERSEN_SECP256K1:
		return commitment.SchemePedersenSecp256k1
	default:
		return commitment.SchemeUnspecified
	}
}
