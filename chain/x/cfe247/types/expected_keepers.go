package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// EACRetirementView is the slim read-only window cfe247 needs into
// an x/eac retirement + its underlying certificate. Implementations
// MUST NOT mutate state.
type EACRetirementView struct {
	RetirementID  uint64
	CertificateID uint64
	Retirer       string
	Beneficiary   string
	Amount        uint64 // certificate-side units (Wh)

	// Certificate snapshot fields needed for matching:
	CertHourStart int64
	CertHourEnd   int64
	GridZone      string
	// Technology is the protobuf int32 of x/eac.Technology so we don't
	// import x/eac into this package.
	Technology int32
	IsStorage  bool
}

// EACKeeper exposes the read-only retirement+certificate join.
type EACKeeper interface {
	// LookupRetirement returns the joined view; ok=false when either
	// the retirement or its certificate is missing. Fail-closed.
	LookupRetirement(ctx sdk.Context, retirementID uint64) (EACRetirementView, bool)
}

// SubjectAuthKeeper is the optional sanctions hook applied to subject
// admins on every consumption attestation / report generation. Kept
// narrow so a sanctions-less deployment can pass nil.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// AuditKeeper is the optional cross-module audit hook.
type AuditKeeper interface {
	RecordCFEAction(ctx sdk.Context, subjectID, action, actor, detail string)
}
