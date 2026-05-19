package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// PolicyKeeper is the slice of x/policy the stablecoin keeper consults
// on every transfer / mint / burn. The keeper passes the canonical
// binding tuple (asset_class="stablecoin", asset_id=<denom_id>); x/policy
// resolves the bound rule set internally and returns nil when the
// transfer passes (or no binding exists), or an error otherwise.
//
// Implementations MUST be deterministic. Side-effect-free reads are
// safe; the existing x/policy.EvaluateTransfer also writes denial logs
// — that is intentional and audited (see the EvaluateDryRun split).
type PolicyKeeper interface {
	EvaluateTransfer(ctx sdk.Context, assetClass, assetID, sender, receiver string, amount uint64) error
}

// SanctionsKeeper is the defense-in-depth gate. The stablecoin keeper
// checks this BEFORE evaluating x/policy so a sanctioned counterparty
// is rejected even when no policy is bound to the denom.
type SanctionsKeeper interface {
	IsSanctioned(ctx sdk.Context, subject string) bool
}

// DIDKeeper is the optional KYC gate. When Params.RequireKycCredential
// is non-empty, the stablecoin keeper requires both counterparties to
// hold a matching credential (defence-in-depth independent of the
// policy DSL).
type DIDKeeper interface {
	IsActive(ctx sdk.Context, subject string) bool
	HasCredential(ctx sdk.Context, subject, credentialType string) bool
}

// OracleKeeper is the reserve-attestation gate. Called from MsgMint to
// ensure the issuer has freshly attested reserves covering the
// post-mint outstanding at DenomReserve.required_ratio_bps.
//
// GetAggregatedReserve returns:
//   value     : the aggregated reserve attestation as a non-negative
//               integer in the same unit as outstanding. Negative
//               oracle reports are treated as missing.
//   timestamp : block time (unix seconds) of the latest aggregation;
//   ok        : false when the topic is missing OR has no attestation.
//
// Invariant: implementations MUST NOT mutate state.
type OracleKeeper interface {
	GetAggregatedReserve(ctx sdk.Context, topicID string) (value int64, timestamp int64, ok bool)
}

// AuditKeeper records compliance-relevant operations: forced transfers,
// freezes, blacklists, redemption fulfilment. Optional — when wired to
// nil the keeper simply skips the hook.
type AuditKeeper interface {
	RecordStablecoinAction(ctx sdk.Context, denomID, issuerID, action, actor, subject, detail string)
}

// ERC20Keeper is the slice of Cosmos EVM's x/erc20 the stablecoin
// keeper invokes immediately after a new denom is registered. The
// hook reserves a deterministic ERC20 contract address for the
// stablecoin denom so EVM wallets and tooling can list it without
// the operator having to issue a separate governance proposal
// against x/erc20.
//
// Optional: when wired to nil the stablecoin keeper skips the
// registration. Failures from CreateNewTokenPair are NEVER fatal
// to the underlying RegisterDenom message — they are logged via
// the SDK logger and surfaced as an event so the operator can
// re-attempt registration manually. This keeps the stablecoin
// module functional even when the EVM stack is down or has been
// removed from the build (smaller validator binaries).
type ERC20Keeper interface {
	// IsDenomRegistered reports whether the denom already has a
	// TokenPair entry. Used to make CreateNewTokenPair idempotent
	// across re-runs of the same Tx (e.g. ante handler retries).
	IsDenomRegistered(ctx sdk.Context, denom string) bool
	// CreateNewTokenPair allocates an STRv2-style ERC20 address
	// derived from the denom string and stores the TokenPair.
	// Returns ErrTokenPairAlreadyExists when an EVM contract has
	// already deployed code at the derived address.
	CreateNewTokenPair(ctx sdk.Context, denom string) error
}
