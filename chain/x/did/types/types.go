package types

// Document statuses. Strings (not enum) because they need to be readable
// in JSON dumps and the set is small enough that a typo guard isn't worth
// the extra plumbing.
const (
	StatusActive      = "active"
	StatusDeactivated = "deactivated"
)

// Credential statuses follow W3C Status List 2021. Stored as strings so
// future spec values land without bumping the proto enum.
const (
	CredentialValid     = "valid"
	CredentialRevoked   = "revoked"
	CredentialSuspended = "suspended"
	CredentialExpired   = "expired"
)

// Anchor tiers.
const (
	AnchorTierRoot         = "root"
	AnchorTierIntermediate = "intermediate"
)

// Hard upper/lower bounds for Params validation. These are policy choices,
// not capacity limits — we want them small enough that pathological
// documents (a million keys) cannot DoS query paths, large enough that
// realistic enterprise issuers (a few dozen keys) fit comfortably.
const (
	MaxControllersUpperBound          = 32
	MaxVerificationMethodsUpperBound  = 64
	MaxServiceEndpointsUpperBound     = 32
	MaxCredentialHashLenUpperBound    = 256

	DefaultMaxControllers         = 8
	DefaultMaxVerificationMethods = 16
	DefaultMaxServiceEndpoints    = 8
	DefaultMaxCredentialHashLen   = 128
	DefaultRevocationGracePeriod  = int64(7 * 24 * 3600) // 7 days
)

// VerificationMethod purposes recognized by the chain. Matches the W3C
// DID-Core verificationRelationship vocabulary. Exposed as constants so
// keepers can validate against the set without re-typing the URIs.
const (
	PurposeAuthentication       = "authentication"
	PurposeAssertionMethod      = "assertionMethod"
	PurposeKeyAgreement         = "keyAgreement"
	PurposeCapabilityInvocation = "capabilityInvocation"
	PurposeCapabilityDelegation = "capabilityDelegation"
)

// VerificationMethodPurposesValid checks that every entry in `purposes`
// is one of the supported W3C relationship URIs. Empty list is allowed
// (key has no use until purposes are added in a follow-up update).
func VerificationMethodPurposesValid(purposes []string) bool {
	for _, p := range purposes {
		switch p {
		case PurposeAuthentication, PurposeAssertionMethod,
			PurposeKeyAgreement, PurposeCapabilityInvocation,
			PurposeCapabilityDelegation:
			// ok
		default:
			return false
		}
	}
	return true
}
