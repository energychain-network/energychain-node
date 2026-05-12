package types

// AttestationValidityUpper bounds the configurable expiry; one calendar
// year is the longest interval any vendor recommends between fresh
// attestations (intel TDX, ARM CCA both stop accepting older quotes).
const (
	AttestationValidityLower = int64(60)            // 1 minute (sane for tests)
	AttestationValidityUpper = int64(365 * 24 * 3600)
	DefaultAttestationValidity = int64(30 * 24 * 3600) // 30 days

	MaxDevicesPerOwnerUpper   = uint32(1_000_000)
	DefaultMaxDevicesPerOwner = uint32(10_000)
)

// Verdicts attached to AttestationEvidence.
const (
	VerdictPending  uint32 = 0
	VerdictAccepted uint32 = 1
	VerdictRejected uint32 = 2
)
