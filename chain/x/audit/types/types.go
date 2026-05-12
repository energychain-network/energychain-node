package types

// Chain-level constants and helpers backing the proto-generated types in
// this package. The audit module deliberately stays event-type-agnostic:
// upstream modules pick their own event_type strings (e.g.
// "param_update.x/eac"), the chain only enforces structural and rate
// constraints.

// TStoreKey is the transient store key used by per-block rate-limit
// counters. Wiped at the start of each block by CometBFT.
const TStoreKey = "transient_" + ModuleName

// Hard upper bounds for Params validation.
const (
	AuditMaxDataLowerBound      = 128
	AuditMaxDataUpperBound      = 1_048_576 // 1 MiB
	AuditRetentionUpperBound    = 100_000_000
	AuditMaxAuditsPerBlockUpper = 10_000

	DefaultAuditMaxData     = 65_536
	DefaultAuditMaxPerBlock = 100

	// M1 evolution defaults / bounds.

	// DefaultArchiveMinAgeBlocks is the minimum number of blocks a log
	// must live in the live store before it can be archived. ~1 day at
	// a 1.5s block time; tunable via governance.
	DefaultArchiveMinAgeBlocks int64 = 50_000
	ArchiveMinAgeUpperBound    int64 = 100_000_000

	// DefaultViewKeyMaxValidity bounds how far in the future a grant may
	// expire (one year by default; 0 disables the cap).
	DefaultViewKeyMaxValidity int64 = 365 * 24 * 3600
	ViewKeyMaxValidityUpper   int64 = 100 * 365 * 24 * 3600

	// DefaultArchiveMaxBatch caps how many logs a single MsgArchive may
	// evict, keeping block gas predictable. Operators can lower it via
	// governance, but never set it above ArchiveMaxBatchUpperBound.
	DefaultArchiveMaxBatch uint32 = 1_000
	ArchiveMaxBatchUpper   uint32 = 10_000

	// SchemaURIMaxLen / SchemaHashLen / GrantEncryptedKeyMaxLen bound
	// the bytes a single record can hold so a malicious tx cannot
	// commit blocks of arbitrary size.
	SchemaURIMaxLen         = 512
	SchemaHashHexLen        = 64 // sha256 hex
	GrantEncryptedKeyMaxLen = 4_096
	GrantScopePrefixMaxLen  = 256
)

// MerkleAlgorithmSHA256 is the canonical algorithm tag stored in archive
// segments. New algorithms can be introduced without touching old segments.
const MerkleAlgorithmSHA256 = "sha256"

// SeverityValid returns true when v is one of the known Severity enum
// values. Used by msg validation; we deliberately accept the proto
// default (SEVERITY_INFO) so legacy callers that omit the field continue
// to work.
func SeverityValid(v Severity) bool {
	switch v {
	case Severity_SEVERITY_INFO,
		Severity_SEVERITY_NOTICE,
		Severity_SEVERITY_WARN,
		Severity_SEVERITY_CRITICAL,
		Severity_SEVERITY_FATAL:
		return true
	default:
		return false
	}
}
