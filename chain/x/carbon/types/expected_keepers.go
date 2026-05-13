package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// PolicyKeeper is the slice of x/policy the carbon keeper consults
// on transfers and retirements. asset_class is "carbon".
type PolicyKeeper interface {
	EvaluateTransfer(ctx sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error
}

// SanctionsKeeper is the defense-in-depth address gate.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// OracleKeeper is the bridge attestation gate. Same shape as
// x/eac.OracleKeeper so the chain-level adapter can satisfy both.
type OracleKeeper interface {
	GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool)
}

// EACKeeper is the cross-module hook used for OFFSET / EAC mutual
// exclusion. The carbon keeper only needs to know how many MWh a
// given x/eac certificate originally certified so it can ensure
// the cumulative OFFSET claims against that certificate never
// exceed the certificate's IssuedUnits.
//
// Implementations MUST be read-only.
type EACKeeper interface {
	// GetCertificateIssuedUnits returns the certificate's total
	// issued MWh count and a boolean indicating whether the
	// certificate exists. A non-existent reference is treated as
	// fail-closed in the carbon keeper.
	GetCertificateIssuedUnits(ctx sdk.Context, certificateID uint64) (units uint64, ok bool)
}

// AuditKeeper is the optional cross-module hook.
type AuditKeeper interface {
	RecordCarbonAction(ctx sdk.Context, assetID uint64, issuerID, action, actor, beneficiary, detail string)
}
