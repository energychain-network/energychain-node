package types

// AuditLog and Params are now generated from
// proto/energychain/audit/v1/types.proto.
//
// The chain does NOT restrict what event types are valid - users define
// their own (e.g. "contract_deploy", "large_transfer", "custom",
// "governance_action", etc.).

// TStoreKey is the transient store key used by per-block rate-limit
// counters. Wiped at the start of each block by CometBFT.
const TStoreKey = "transient_" + ModuleName

// Hard upper bounds for Params validation.
const (
	AuditMaxDataLowerBound      = 128
	AuditMaxDataUpperBound      = 1_048_576 // 1 MiB
	AuditRetentionUpperBound    = 100_000_000
	AuditMaxAuditsPerBlockUpper = 10_000
	DefaultAuditMaxData         = 65_536
	DefaultAuditMaxPerBlock     = 100
)
