package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// SanctionsKeeper is the optional fail-closed sanctions hook
// applied at submit/grant time. A sanctions-less deployment
// can pass nil to NewKeeper.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, addr string) bool
}

// DIDKeeper resolves a DID to its current controllers so the
// mrv module can verify that a verifier's chain signer is
// authorized by the verifier DID. Optional — when nil, the
// chain-address ↔ DID binding is set at verifier-registration
// time and the DID is not re-resolved on every attestation.
type DIDKeeper interface {
	// IsControllerOf returns true if the supplied bech32 chain
	// address is currently listed as a controller of the DID.
	IsControllerOf(ctx sdk.Context, did, addr string) bool
}

// AuditKeeper is the optional cross-module audit hook. mrv
// records governance / verifier actions to provide the
// monitoring trail required by ESG regulators.
type AuditKeeper interface {
	RecordMRVAction(ctx sdk.Context, reportID uint64, action, actor, subject, detail string)
}

// MeterKeeper / EACKeeper / CarbonKeeper / CFEKeeper are
// kept as named optional interfaces for the v2 auto-aggregation
// pipeline. v1 stores subject-supplied aggregates verbatim
// and lets the verifier attest correctness off-chain — these
// hooks remain as forward-looking surface so a future
// MsgGenerateReport can populate aggregates directly from
// chain state.
type MeterKeeper interface {
	TotalReadingsWh(ctx sdk.Context, subject string, periodStart, periodEnd int64) uint64
}

type EACKeeper interface {
	TotalIssuedWh(ctx sdk.Context, subject string, periodStart, periodEnd int64) uint64
	TotalRetiredWh(ctx sdk.Context, subject string, periodStart, periodEnd int64) uint64
}

type CarbonKeeper interface {
	TotalRetiredMilliTCO2e(ctx sdk.Context, subject string, periodStart, periodEnd int64) uint64
}

type CFEKeeper interface {
	HourlyScoreBps(ctx sdk.Context, subject string, periodStart, periodEnd int64) uint32
	AnnualScoreBps(ctx sdk.Context, subject string, year int64) uint32
}
