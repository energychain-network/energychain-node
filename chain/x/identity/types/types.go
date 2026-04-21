package types

// Conventional status values used by the module's lifecycle logic.
// These are plain strings - no enum, no range check.
const (
	StatusPending = "pending"
	StatusActive  = "active"
	StatusRevoked = "revoked"
)

// Hard upper/lower bounds for Params validation.
const (
	IdentityMaxMetadataLowerBound = 128
	IdentityMaxMetadataUpperBound = 65_536
	DefaultIdentityMaxMetadata    = 4_096
)
