// Package granularid encodes the canonical "hour bucket" identifier shared
// by the energy-chain modules that operate on hourly-granular data —
// notably x/eac (granular EnergyTag certs), x/cfe247 (24/7 matching), and
// x/meter (default reading period).
//
// The identifier is a simple uint32 = hours-since-Unix-epoch, packaged with
// the originating timezone so that downstream reports can render the bucket
// in its production-time-of-day. Two design rules motivate the encoding:
//
//  1. Deterministic: every node MUST agree on which hour bucket a
//     timestamp falls into. We therefore truncate to UTC seconds and divide
//     by 3600. Local-time arithmetic is forbidden inside the chain; the
//     timezone field is metadata for off-chain reports only.
//
//  2. Composable: the bucket is comparable as a plain uint32, so secondary
//     indexes in keepers can use it as a numeric key without bespoke
//     serialization.
package granularid

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// SecondsPerHour is the divisor for converting a Unix timestamp into a
// granular bucket index. Hard-coded rather than derived from time.Hour so a
// future refactor of the time package cannot silently change the on-chain
// numbering scheme.
const SecondsPerHour int64 = 3600

// MaxBucket caps the bucket index to roughly the year 2400 — large enough
// to never bind real-world inputs, small enough to catch caller bugs that
// pass milliseconds where seconds are expected.
const MaxBucket uint32 = 4_000_000_000

// Bucket is the canonical hour-bucket identifier. The zero value (bucket 0)
// is treated as "unset" by Validate so accidentally-zeroed structs surface
// as malformed rather than as "January 1 1970, hour 0".
type Bucket uint32

// FromUnix returns the bucket containing the given Unix timestamp. Negative
// timestamps clamp to 0; timestamps far in the future clamp to MaxBucket.
// Both clamps are deliberate: callers that pass garbage should fail loudly
// in Validate, and never silently round-trip through a wraparound.
func FromUnix(unix int64) Bucket {
	if unix <= 0 {
		return 0
	}
	b := unix / SecondsPerHour
	if b > int64(MaxBucket) {
		return Bucket(MaxBucket)
	}
	return Bucket(b)
}

// FromTime is a convenience for callers that already hold a time.Time.
// Always uses .UTC() to match the chain's deterministic behaviour.
func FromTime(t time.Time) Bucket {
	return FromUnix(t.UTC().Unix())
}

// StartUnix returns the Unix timestamp of the first second contained in the
// bucket. EndUnix returns the first second of the next bucket (exclusive).
// The pair is intentionally [start, end) so range queries over time spans
// can be expressed without off-by-one math.
func (b Bucket) StartUnix() int64 { return int64(b) * SecondsPerHour }
func (b Bucket) EndUnix() int64   { return (int64(b) + 1) * SecondsPerHour }

// Validate ensures the bucket is in the supported range. The "0 is unset"
// convention mirrors how `commitment.SchemeUnspecified` is handled.
func (b Bucket) Validate() error {
	if b == 0 {
		return errors.New("granularid: bucket 0 is reserved for unset")
	}
	if uint32(b) > MaxBucket {
		return fmt.Errorf("granularid: bucket %d exceeds MaxBucket %d", b, MaxBucket)
	}
	return nil
}

// String renders the bucket as ISO-8601 hour for log readability:
// "2026-04-19T15Z".
func (b Bucket) String() string {
	if b == 0 {
		return "<unset>"
	}
	t := time.Unix(b.StartUnix(), 0).UTC()
	return fmt.Sprintf("%04d-%02d-%02dT%02dZ", t.Year(), t.Month(), t.Day(), t.Hour())
}

// Range describes a half-open interval of hour buckets, [Start, End).
// Used by 24/7 CFE matching and EAC retire-coverage queries.
type Range struct {
	Start Bucket
	End   Bucket
}

// Validate ensures the range is non-empty and both endpoints are themselves
// valid. End == Start is rejected to match the half-open semantics: a zero
// width range carries no meaningful information.
func (r Range) Validate() error {
	if err := r.Start.Validate(); err != nil {
		return fmt.Errorf("range start: %w", err)
	}
	if err := r.End.Validate(); err != nil {
		return fmt.Errorf("range end: %w", err)
	}
	if r.End <= r.Start {
		return fmt.Errorf("granularid: range end %d must exceed start %d", r.End, r.Start)
	}
	return nil
}

// Hours returns the number of distinct hour buckets covered by the range.
// Returns 0 for invalid ranges to keep downstream math safe.
func (r Range) Hours() uint32 {
	if err := r.Validate(); err != nil {
		return 0
	}
	return uint32(r.End - r.Start)
}

// Contains is a half-open membership test. Mirrors the semantics promised
// by Range.Validate.
func (r Range) Contains(b Bucket) bool {
	return b >= r.Start && b < r.End
}

// ParseTimezone validates an IANA timezone string the way x/eac and x/meter
// store it on Asset / MeteringPoint records. The name is bounded so a
// malicious caller cannot inflate state by submitting megabyte-long
// "timezones".
//
// We do NOT load the zoneinfo database here: that is a per-OS concern and
// the chain may run on minimal images that lack /usr/share/zoneinfo. The
// parser only enforces the format constraint.
func ParseTimezone(tz string) error {
	if tz == "" {
		return nil // unset is allowed; default UTC is implied.
	}
	if len(tz) > 64 {
		return fmt.Errorf("granularid: timezone %q exceeds 64 chars", tz)
	}
	if strings.ContainsAny(tz, " \t\n\r") {
		return fmt.Errorf("granularid: timezone %q contains whitespace", tz)
	}
	return nil
}
